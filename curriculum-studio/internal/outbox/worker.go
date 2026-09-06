package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

const (
	defaultPollInterval = 200 * time.Millisecond
	defaultDeliveryTO   = 5 * time.Second
	defaultLeaseTTL     = 30 * time.Second
	defaultMaxAttempts  = 5
	maxErrorLen         = 512
)

// Config controls the durable outbox dispatcher.
type Config struct {
	Owner           string
	PollInterval    time.Duration
	DeliveryTimeout time.Duration
	LeaseTTL        time.Duration
	MaxAttempts     int
	Secrets         SecretResolver
	HTTPClient      *http.Client
	Now             func() time.Time
	Logger          *slog.Logger
}

// Worker fans unpublished outbox rows to workspace-scoped webhook endpoints
// and POSTs signed DomainEvent envelopes using DB leases.
type Worker struct {
	q           repo.Querier
	owner       string
	poll        time.Duration
	timeout     time.Duration
	lease       time.Duration
	maxAttempts int
	secrets     SecretResolver
	client      *http.Client
	now         func() time.Time
	logger      *slog.Logger
}

// NewWorker builds a stoppable dispatcher bound to the Studio database.
func NewWorker(q repo.Querier, cfg Config) (*Worker, error) {
	if q == nil {
		return nil, fmt.Errorf("outbox worker: querier is required")
	}
	owner := strings.TrimSpace(cfg.Owner)
	if owner == "" {
		owner = "studio-outbox-" + uuid.NewString()
	}
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	timeout := cfg.DeliveryTimeout
	if timeout <= 0 {
		timeout = defaultDeliveryTO
	}
	lease := cfg.LeaseTTL
	if lease <= 0 {
		lease = defaultLeaseTTL
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	secrets := cfg.Secrets
	if secrets == nil {
		secrets = NewMemorySecrets()
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		q: q, owner: owner, poll: poll, timeout: timeout, lease: lease,
		maxAttempts: maxAttempts, secrets: secrets, client: client, now: now, logger: logger,
	}, nil
}

// Run drains the outbox until ctx is cancelled. A new process continues
// pending, failed, and lease-expired rows; unpublished events are never dropped.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("outbox worker is nil")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		worked, err := w.Tick(ctx)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			w.logger.Error("outbox tick failed", "error", err, "owner", w.owner)
		}
		if worked {
			continue
		}
		timer := time.NewTimer(w.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Tick fans out at most one unpublished event and delivers at most one claimed
// webhook. It returns whether any durable work happened.
func (w *Worker) Tick(ctx context.Context) (bool, error) {
	if w == nil || w.q == nil {
		return false, fmt.Errorf("outbox worker is closed")
	}
	fanned, err := w.fanoutOne(ctx)
	if err != nil {
		return false, err
	}
	delivered, err := w.deliverOne(ctx)
	if err != nil {
		return fanned, err
	}
	return fanned || delivered, nil
}

// DrainUntilIdle repeatedly ticks until both unpublished events and retryable
// deliveries are empty or ctx is cancelled.
func (w *Worker) DrainUntilIdle(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		worked, err := w.Tick(ctx)
		if err != nil {
			return err
		}
		if !worked {
			return nil
		}
	}
}

func (w *Worker) fanoutOne(ctx context.Context) (bool, error) {
	err := repo.NewOutboxRepo(w.q).FanoutUnpublished(ctx, func(q repo.Querier, event *domain.OutboxEvent) error {
		if event.WorkspaceID != nil && *event.WorkspaceID != uuid.Nil {
			endpoints, e := repo.NewWebhookEndpointRepo(q).ListActiveForEvent(ctx, *event.WorkspaceID, event.EventType)
			if e != nil {
				return e
			}
			for i := range endpoints {
				if _, e := repo.NewWebhookDeliveryRepo(q).Schedule(ctx, endpoints[i].ID, event.ID, IdempotencyKey(endpoints[i].ID, event.ID)); e != nil {
					return e
				}
			}
		}
		_, e := repo.NewOutboxRepo(q).MarkPublished(ctx, event.ID)
		return e
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (w *Worker) deliverOne(ctx context.Context) (bool, error) {
	claimed, err := repo.NewWebhookDeliveryRepo(w.q).ClaimRetryable(ctx, w.owner, w.lease, w.maxAttempts)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	err = w.postClaimed(ctx, claimed)
	if err != nil {
		w.logger.Info("webhook delivery attempt failed",
			"delivery_id", claimed.ID.String(),
			"endpoint_id", claimed.EndpointID.String(),
			"event_id", claimed.EventID.String(),
			"attempt", claimed.AttemptCount,
			"error", err,
		)
		if claimed.AttemptCount >= w.maxAttempts {
			if _, markErr := repo.NewWebhookDeliveryRepo(w.q).MarkFailed(ctx, claimed.ID, w.owner, clipErr(err)); markErr != nil {
				if deliveryNoLongerOwned(markErr) {
					return true, nil
				}
				return true, markErr
			}
			return true, nil
		}
		if _, markErr := repo.NewWebhookDeliveryRepo(w.q).ReleaseForRetry(ctx, claimed.ID, w.owner, clipErr(err), backoffFor(claimed.AttemptCount)); markErr != nil {
			if deliveryNoLongerOwned(markErr) {
				return true, nil
			}
			return true, markErr
		}
		return true, nil
	}
	if _, markErr := repo.NewWebhookDeliveryRepo(w.q).MarkDelivered(ctx, claimed.ID, w.owner); markErr != nil {
		if deliveryNoLongerOwned(markErr) {
			return true, nil
		}
		return true, markErr
	}
	return true, nil
}

// deliveryNoLongerOwned means a claimed row was removed or fenced by another
// worker before this worker could persist its result. The other owner now owns
// the row, so it is not a drain failure.
func deliveryNoLongerOwned(err error) bool {
	return errors.Is(err, repo.ErrLeaseLost) || errors.Is(err, repo.ErrNotFound)
}

func (w *Worker) postClaimed(ctx context.Context, claimed *domain.WebhookDelivery) error {
	event, err := repo.NewOutboxRepo(w.q).Get(ctx, claimed.EventID)
	if err != nil {
		return err
	}
	endpoint, err := repo.NewWebhookEndpointRepo(w.q).Get(ctx, claimed.EndpointID)
	if err != nil {
		return err
	}
	if endpoint.Status != "active" {
		return fmt.Errorf("endpoint is not active")
	}
	if event.WorkspaceID == nil || *event.WorkspaceID != endpoint.WorkspaceID {
		return fmt.Errorf("cross-workspace delivery refused")
	}
	secret, err := w.secrets.Resolve(ctx, endpoint.SecretRef)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelopeFor(event))
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	sig, ts := SignV1(secret, w.now(), body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(EventIDHeader, event.ID.String())
	req.Header.Set(EventTypeHeader, event.EventType)
	req.Header.Set(TimestampHeader, ts)
	req.Header.Set(SignatureHeader, sig)
	req.Header.Set(CompatSignatureHeader, sig)
	req.Header.Set("Idempotency-Key", claimed.IdempotencyKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("receiver status %d", resp.StatusCode)
	}
	return nil
}

// OutboxLag returns the age of the oldest unpublished event.
func (w *Worker) OutboxLag(ctx context.Context) (time.Duration, error) {
	return repo.NewMetricsRepo(w.q).OutboxLag(ctx, w.now())
}

func backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(1<<min(attempt-1, 6)) * 50 * time.Millisecond
	if d > 5*time.Second {
		return 5 * time.Second
	}
	return d
}

func clipErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > maxErrorLen {
		return msg[:maxErrorLen]
	}
	return msg
}
