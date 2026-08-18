package webhook

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/stytchcache"
)

var retryDelays = [...]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 4 * time.Hour, 8 * time.Hour, 12 * time.Hour}

type Worker struct {
	Pool     *pgxpool.Pool
	Cache    *stytchcache.Cache
	Owner    string
	LeaseTTL time.Duration
	Now      func() time.Time
}

type Lease struct {
	ID           uuid.UUID
	Token        uuid.UUID
	Version      int64
	AttemptCount int
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
func (w *Worker) ttl() time.Duration {
	if w.LeaseTTL > 0 {
		return w.LeaseTTL
	}
	return time.Minute
}

// Claim claims one due event using owner, random token, expiry, and version.
// Expired claims are reclaimed with a new fence token.
func (w *Worker) Claim(ctx context.Context) (Lease, error) {
	if w == nil || w.Pool == nil || w.Owner == "" {
		return Lease{}, errors.New("webhook worker: pool and owner are required")
	}
	token := uuid.New()
	now := w.now()
	var l Lease
	err := w.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT id FROM webhook_events
 WHERE (status IN ('received','failed_retryable') AND next_attempt_at <= $1)
    OR (status='claimed' AND lease_expires_at <= $1)
 ORDER BY next_attempt_at,received_at FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE webhook_events e SET status='claimed',lease_owner=$2,lease_token=$3,lease_expires_at=$4,claimed_at=COALESCE(e.claimed_at,$1),version=e.version+1
FROM candidate WHERE e.id=candidate.id
RETURNING e.id,e.lease_token,e.version,e.attempt_count`, now, w.Owner, token, now.Add(w.ttl())).Scan(&l.ID, &l.Token, &l.Version, &l.AttemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, pgx.ErrNoRows
	}
	return l, err
}

// Apply processes a claimed event. Updates are receipt-only unless a
// configured authoritative terminal mapping exists; DELETE is unambiguously
// terminal and revokes only provider-associated Primer authority.
func (w *Worker) Apply(ctx context.Context, l Lease) error {
	if w == nil || w.Pool == nil {
		return errors.New("webhook worker: pool is required")
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var objectType, action, project, entity string
	err = tx.QueryRow(ctx, `SELECT object_type,action,provider_project_id,entity_id FROM webhook_events WHERE id=$1 AND status='claimed' AND lease_owner=$2 AND lease_token=$3 AND version=$4 FOR UPDATE`, l.ID, w.Owner, l.Token, l.Version).Scan(&objectType, &action, &project, &entity)
	if err != nil {
		return err
	}
	relevant := (objectType == "member" || objectType == "organization") && (action == "update" || action == "delete")
	if relevant && w.Cache != nil {
		w.Cache.InvalidateAll()
	}
	if relevant && action == "delete" {
		if err := revokeDelete(ctx, tx, l.ID, objectType, project, entity, w.now()); err != nil {
			return err
		}
	}
	tag := "applied"
	if !relevant {
		tag = "ignored"
	}
	result, err := tx.Exec(ctx, `UPDATE webhook_events SET status=$1,lease_owner=NULL,lease_token=NULL,lease_expires_at=NULL,claimed_at=NULL,applied_at=$2,terminal_at=$2,version=version+1 WHERE id=$3 AND status='claimed' AND lease_owner=$4 AND lease_token=$5 AND version=$6`, tag, w.now(), l.ID, w.Owner, l.Token, l.Version)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("webhook worker: stale lease")
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func revokeDelete(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, objectType, project, entity string, now time.Time) error {
	selector := "provider_project_id=$2 AND provider_member_id=$3"
	if objectType == "organization" {
		selector = "provider_project_id=$2 AND provider_organization_id=$3"
	}
	if objectType != "member" && objectType != "organization" {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id FROM provider_session_associations WHERE `+selector+` AND status='active' FOR UPDATE`, project, entity)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var association uuid.UUID
		if err := rows.Scan(&association); err != nil {
			return err
		}
		const reason = "provider_webhook_delete"
		if _, err := tx.Exec(ctx, `UPDATE provider_session_associations SET status='revoked',revoked_at=$1,revoke_reason_code=$2,updated_at=$1 WHERE id=$3 AND status='active'`, now, reason, association); err != nil {
			return err
		}
		var grants []uuid.UUID
		gr, err := tx.Query(ctx, `SELECT id FROM oauth_grants WHERE provider_session_association_id=$1 AND status='active' FOR UPDATE`, association)
		if err != nil {
			return err
		}
		for gr.Next() {
			var id uuid.UUID
			if err := gr.Scan(&id); err != nil {
				gr.Close()
				return err
			}
			grants = append(grants, id)
		}
		if err := gr.Err(); err != nil {
			gr.Close()
			return err
		}
		gr.Close()
		for _, grant := range grants {
			if _, err := tx.Exec(ctx, `UPDATE oauth_refresh_tokens t SET revoked_at=$1 FROM oauth_refresh_families f WHERE t.family_id=f.id AND f.grant_id=$2 AND t.consumed_at IS NULL AND t.revoked_at IS NULL`, now, grant); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE oauth_refresh_families SET status='revoked',revoked_at=$1,revoke_reason_code=$2,version=version+1 WHERE grant_id=$3 AND status='active'`, now, reason, grant); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE oauth_grants SET status='revoked',revoked_at=$1,revoke_reason_code=$2,version=version+1 WHERE id=$3 AND status='active'`, now, reason, grant); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO oauth_revocations(grant_id,source,reason_code,webhook_event_id) VALUES($1,'provider_webhook',$2,$3) ON CONFLICT DO NOTHING`, grant, reason, eventID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO oauth_revocations(provider_session_association_id,source,reason_code,webhook_event_id) VALUES($1,'provider_webhook',$2,$3) ON CONFLICT DO NOTHING`, association, reason, eventID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity_audit_events(actor_class,action,target_type,target_id,outcome,reason_code,webhook_event_id) VALUES('provider_webhook','revoke','provider_session_association',$1,'success',$2,$3)`, association, reason, eventID); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (w *Worker) Fail(ctx context.Context, l Lease, code string) error {
	if w == nil || w.Pool == nil {
		return errors.New("webhook worker: pool is required")
	}
	now := w.now()
	_, err := w.Pool.Exec(ctx, `UPDATE webhook_events SET status=CASE WHEN attempt_count+1 >= max_attempts THEN 'dead_letter' ELSE 'failed_retryable' END,attempt_count=attempt_count+1,next_attempt_at=$1,last_error_code=$2,claimed_at=NULL,terminal_at=CASE WHEN attempt_count+1 >= max_attempts THEN $3 ELSE NULL END,lease_owner=NULL,lease_token=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$4 AND status='claimed' AND lease_owner=$5 AND lease_token=$6 AND version=$7`, now.Add(delayForAttempt(l.AttemptCount)), code, now, l.ID, w.Owner, l.Token, l.Version)
	return err
}
func delayForAttempt(attempt int) time.Duration {
	if attempt < 1 {
		return retryDelays[0]
	}
	i := attempt - 1
	if i >= len(retryDelays) {
		i = len(retryDelays) - 1
	}
	return retryDelays[i]
}
