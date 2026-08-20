package jobs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

// CallbackProcessor authenticates and binds a callback before handing only a
// typed accepted/rejected result to the generic decision committer.
type CallbackProcessor struct {
	Outbox    *repo.ExternalRepository
	Catalog   *repo.VerifierCatalogRepository
	Secrets   SecretResolver
	Committer verification.DecisionCommitter
	MaxSkew   time.Duration
	Now       func() time.Time
}

func (p *CallbackProcessor) Process(ctx context.Context, method, path, keyID, timestamp, signature string, body []byte) (bool, error) {
	if p == nil || p.Outbox == nil || p.Catalog == nil || p.Secrets == nil {
		return false, errors.New("callback processor is not configured")
	}
	c, err := verification.DecodeCallback(body)
	if err != nil {
		return false, err
	}
	verifierID, err := uuid.Parse(c.VerifierID)
	if err != nil {
		return false, verification.ErrExternalBinding
	}
	catalog, err := p.Catalog.Get(ctx, verifierID)
	if err != nil {
		return false, verification.ErrExternalBinding
	}
	binding, err := p.Outbox.Binding(ctx, c.RequestID, c.VerifierID, c.AttemptRef)
	if err != nil {
		return false, verification.ErrExternalBinding
	}
	if strings.TrimSpace(keyID) == "" || keyID != binding.SecretVersion {
		return false, verification.ErrExternalInvalidSignature
	}
	secretRef := catalog.SecretRef
	if keyID != catalog.SecretVersion {
		secretRef, err = p.Catalog.SecretRefForVersion(ctx, verifierID, keyID)
		if err != nil {
			return false, verification.ErrExternalInvalidSignature
		}
	}
	secret, err := p.Secrets.Resolve(ctx, secretRef, keyID)
	if err != nil {
		return false, verification.ErrExternalInvalidSignature
	}
	stamp, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return false, verification.ErrExternalInvalidSignature
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	skew := p.MaxSkew
	if skew <= 0 {
		skew = 5 * time.Minute
	}
	if err = verification.Verify(method, path, stamp, c.CallbackID, signature, body, secret, now, skew); err != nil {
		return false, err
	}
	if binding.CallbackPath != path || c.RequestDigest != binding.PayloadDigest || c.SchemaVersion != binding.SchemaVersion {
		return false, verification.ErrExternalBinding
	}
	_, err = p.Outbox.RecordCallback(ctx, binding, c, body)
	if err != nil {
		return false, err
	}
	return verification.HandleExternalCallback(ctx, p.Committer, binding, c)
}

// HTTPHandler exposes only the callback protocol; it never accepts a tenant,
// endpoint, secret, or arbitrary forwarding target from the caller.
func (p *CallbackProcessor) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid callback", 400)
			return
		}
		if callback, decodeErr := verification.DecodeCallback(body); decodeErr == nil && r.Header.Get("X-Primer-Request-ID") != callback.CallbackID {
			http.Error(w, "callback rejected", http.StatusUnauthorized)
			return
		}
		accepted, err := p.Process(r.Context(), r.Method, r.URL.EscapedPath(), r.Header.Get("X-Primer-Key-ID"), r.Header.Get("X-Primer-Timestamp"), r.Header.Get("X-Primer-Signature"), body)
		if err != nil {
			http.Error(w, "callback rejected", 401)
			return
		}
		if accepted {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
