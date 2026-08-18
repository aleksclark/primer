package webhook

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AlertLease is a separately fenced lease for collision alert delivery. It is
// intentionally not interchangeable with the main webhook-event lease.
type AlertLease struct {
	ID           uuid.UUID
	Token        uuid.UUID
	Version      int64
	AttemptCount int
}

type AlertWorker struct {
	Pool     *pgxpool.Pool
	Owner    string
	LeaseTTL time.Duration
	Now      func() time.Time
}

func (w *AlertWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
func (w *AlertWorker) ttl() time.Duration {
	if w.LeaseTTL > 0 {
		return w.LeaseTTL
	}
	return time.Minute
}

func (w *AlertWorker) Claim(ctx context.Context) (AlertLease, error) {
	if w == nil || w.Pool == nil || w.Owner == "" {
		return AlertLease{}, errors.New("webhook alert worker: pool and owner are required")
	}
	now, token := w.now(), uuid.New()
	var lease AlertLease
	err := w.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT id FROM webhook_security_alerts
 WHERE (status IN ('pending','failed_retryable') AND next_attempt_at <= $1)
    OR (status='claimed' AND lease_expires_at <= $1)
 ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE webhook_security_alerts a SET status='claimed',lease_owner=$2,lease_token=$3,lease_expires_at=$4,claimed_at=COALESCE(a.claimed_at,$1),version=a.version+1
FROM candidate WHERE a.id=candidate.id
RETURNING a.id,a.lease_token,a.version,a.attempt_count`, now, w.Owner, token, now.Add(w.ttl())).Scan(&lease.ID, &lease.Token, &lease.Version, &lease.AttemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return AlertLease{}, pgx.ErrNoRows
	}
	return lease, err
}

func (w *AlertWorker) Alerted(ctx context.Context, lease AlertLease) error {
	if w == nil || w.Pool == nil {
		return errors.New("webhook alert worker: pool is required")
	}
	result, err := w.Pool.Exec(ctx, `UPDATE webhook_security_alerts SET status='alerted',alerted_at=$1,terminal_at=$1,claimed_at=NULL,lease_owner=NULL,lease_token=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$2 AND status='claimed' AND lease_owner=$3 AND lease_token=$4 AND version=$5`, w.now(), lease.ID, w.Owner, lease.Token, lease.Version)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("webhook alert worker: stale lease")
	}
	return nil
}

func (w *AlertWorker) Fail(ctx context.Context, lease AlertLease, code string) error {
	if w == nil || w.Pool == nil {
		return errors.New("webhook alert worker: pool is required")
	}
	now := w.now()
	result, err := w.Pool.Exec(ctx, `UPDATE webhook_security_alerts SET status=CASE WHEN attempt_count+1 >= max_attempts THEN 'dead_letter' ELSE 'failed_retryable' END,attempt_count=attempt_count+1,next_attempt_at=$1,last_error_code=$2,claimed_at=NULL,terminal_at=CASE WHEN attempt_count+1 >= max_attempts THEN $3 ELSE NULL END,lease_owner=NULL,lease_token=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$4 AND status='claimed' AND lease_owner=$5 AND lease_token=$6 AND version=$7`, now.Add(delayForAttempt(lease.AttemptCount)), code, now, lease.ID, w.Owner, lease.Token, lease.Version)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("webhook alert worker: stale lease")
	}
	return nil
}
