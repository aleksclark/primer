package jobs

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/securityreview"
	"primer-tasks/internal/verification"
)

// SecretResolver resolves an administrator-managed reference at delivery time.
// Plaintext secrets never enter a task revision, catalog row, envelope, or log.
type SecretResolver interface {
	Resolve(context.Context, string, string) ([]byte, error)
}
type StaticSecretResolver map[string][]byte

func (s StaticSecretResolver) Resolve(_ context.Context, ref, version string) ([]byte, error) {
	v, ok := s[ref+":"+version]
	if !ok {
		return nil, errors.New("verifier secret unavailable")
	}
	return v, nil
}

type ExternalOutbox interface {
	RequeueExpired(context.Context, time.Time) error
	Claim(context.Context, string, time.Duration) (repo.ExternalDelivery, bool, error)
	Finish(context.Context, repo.ExternalDelivery, string, string, string, time.Time) error
}
type ExternalCatalog interface {
	Get(context.Context, uuid.UUID) (repo.VerifierCatalog, error)
	SecretRefForVersion(context.Context, uuid.UUID, string) (string, error)
}

type ExternalWorker struct {
	Outbox      ExternalOutbox
	Catalog     ExternalCatalog
	Secrets     SecretResolver
	Client      *http.Client
	Owner       string
	Lease, Poll time.Duration
	Now         func() time.Time
}

func NewExternalWorker(outbox ExternalOutbox, catalog ExternalCatalog, secrets SecretResolver) *ExternalWorker {
	return &ExternalWorker{Outbox: outbox, Catalog: catalog, Secrets: secrets, Client: nil, Owner: uuid.NewString(), Lease: 30 * time.Second, Poll: 250 * time.Millisecond, Now: time.Now}
}
func (w *ExternalWorker) Run(ctx context.Context) {
	if w.Poll <= 0 {
		w.Poll = 250 * time.Millisecond
	}
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.Step(ctx)
		}
	}
}
func (w *ExternalWorker) Step(ctx context.Context) error {
	if w.Outbox == nil || w.Catalog == nil {
		return errors.New("external worker is not configured")
	}
	now := w.Now()
	if err := w.Outbox.RequeueExpired(ctx, now); err != nil {
		return err
	}
	d, ok, err := w.Outbox.Claim(ctx, w.Owner, w.Lease)
	if err != nil || !ok {
		return err
	}
	return w.deliver(ctx, d)
}
func (w *ExternalWorker) deliver(ctx context.Context, d repo.ExternalDelivery) error {
	catalog, err := w.Catalog.Get(ctx, uuid.MustParse(d.VerifierID))
	if err != nil {
		return w.fail(ctx, d, "verifier_unavailable", false, err)
	}
	if !catalog.Active {
		return w.fail(ctx, d, "verifier_disabled", true, nil)
	}
	if err = repo.ValidateCatalogEndpoint(ctx, catalog.EndpointURL, catalog.EgressPolicy); err != nil {
		return w.fail(ctx, d, "verifier_endpoint_invalid", true, err)
	}
	if w.Secrets == nil {
		return w.fail(ctx, d, "verifier_secret_unavailable", false, errors.New("secret resolver is not configured"))
	}
	secretVersion := d.SecretVersion
	if secretVersion == "" {
		secretVersion = catalog.SecretVersion
	}
	secretRef := catalog.SecretRef
	if secretVersion != catalog.SecretVersion {
		secretRef, err = w.Catalog.SecretRefForVersion(ctx, uuid.MustParse(d.VerifierID), secretVersion)
		if err != nil {
			return w.fail(ctx, d, "verifier_secret_unavailable", false, err)
		}
	}
	secret, err := w.Secrets.Resolve(ctx, secretRef, secretVersion)
	if err != nil {
		return w.fail(ctx, d, "verifier_secret_unavailable", false, err)
	}
	timeout := catalog.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, catalog.EndpointURL, strings.NewReader(string(d.Envelope)))
	if err != nil {
		return w.fail(ctx, d, "request_build_failed", true, err)
	}
	stamp := w.Now().UTC()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Primer-Request-ID", d.RequestID)
	req.Header.Set("X-Primer-Key-ID", secretVersion)
	req.Header.Set("X-Primer-Timestamp", stamp.Format(time.RFC3339Nano))
	req.Header.Set("X-Primer-Signature", verification.Sign(req.Method, req.URL.EscapedPath(), stamp, d.RequestID, d.Envelope, secret))
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("verifier redirects are not followed")
		}}
		if strings.HasPrefix(strings.ToLower(catalog.EndpointURL), "https://") {
			target, targetErr := repo.VerifierEgressTarget(ctx, catalog.EndpointURL, catalog.EgressPolicy)
			if targetErr != nil {
				return w.fail(ctx, d, "verifier_endpoint_invalid", true, targetErr)
			}
			client.Transport = securityreview.PinnedTransport(ctx, target, nil)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return w.fail(ctx, d, "delivery_transport_error", false, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return w.fail(ctx, d, fmt.Sprintf("delivery_http_%d", resp.StatusCode), resp.StatusCode < 500 && resp.StatusCode != 429, nil)
	}
	// A request acknowledgement is not a decision. The verifier must call the
	// scoped callback; delivery remains waiting until that callback arrives.
	return w.Outbox.Finish(ctx, d, w.Owner, "waiting", "", w.Now().Add(backoff(d.Attempts)))
}
func (w *ExternalWorker) fail(ctx context.Context, d repo.ExternalDelivery, code string, terminal bool, cause error) error {
	if cause != nil {
		slog.Warn("external verifier delivery failed", "code", code, "verifier_id", d.VerifierID, "attempt", d.Attempts)
	}
	status := "retryable_error"
	retry := w.Now().Add(backoff(d.Attempts))
	if terminal || d.Attempts >= d.MaxAttempts || retry.After(d.ExpiresAt) {
		status = "dead"
		retry = time.Time{}
	}
	return w.Outbox.Finish(ctx, d, w.Owner, status, code, retry)
}
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	var jitter [2]byte
	if _, err := crand.Read(jitter[:]); err != nil {
		return time.Duration(1<<attempt) * time.Second
	}
	return time.Duration(1<<attempt)*time.Second + time.Duration((int(jitter[0])<<8|int(jitter[1]))%250)*time.Millisecond
}
