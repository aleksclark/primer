package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RetentionPolicy controls cleanup of ephemeral operational rows. Protected
// plans, runs, and items are intentionally not part of this policy.
type RetentionPolicy struct {
	WebhookDeliveries time.Duration
	IdempotencyKeys   time.Duration
	AuditEvents       time.Duration
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{WebhookDeliveries: 90 * 24 * time.Hour, IdempotencyKeys: 90 * 24 * time.Hour, AuditEvents: 400 * 24 * time.Hour}
}

type RetentionCounts struct {
	WebhookDeliveries int64
	IdempotencyKeys   int64
	AuditEvents       int64
}

type RetentionRepo struct{ Q Querier }

func NewRetentionRepo(q Querier) *RetentionRepo { return &RetentionRepo{Q: q} }

// Retain reports eligible rows and, unless dryRun, deletes only operational
// records whose foreign keys cannot orphan durable curriculum data.
func (r *RetentionRepo) Retain(ctx context.Context, workspaceID uuid.UUID, now time.Time, policy RetentionPolicy, dryRun bool) (RetentionCounts, error) {
	if r == nil || r.Q == nil {
		return RetentionCounts{}, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return RetentionCounts{}, fmt.Errorf("workspace is required")
	}
	if policy.WebhookDeliveries <= 0 || policy.IdempotencyKeys <= 0 || policy.AuditEvents <= 0 {
		return RetentionCounts{}, fmt.Errorf("retention windows must be positive")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var out RetentionCounts
	query := func(sql string, age time.Duration) (int64, error) {
		var n int64
		e := r.Q.QueryRow(ctx, sql, workspaceID, now.Add(-age)).Scan(&n)
		return n, e
	}
	var e error
	if out.WebhookDeliveries, e = query(`SELECT count(*) FROM curriculum_studio.webhook_deliveries d JOIN curriculum_studio.webhook_endpoints ep ON ep.id=d.endpoint_id WHERE ep.workspace_id=$1 AND d.created_at < $2 AND d.status IN ('delivered','failed')`, policy.WebhookDeliveries); e != nil {
		return out, MapError(e)
	}
	if out.IdempotencyKeys, e = query(`SELECT count(*) FROM curriculum_studio.idempotency_keys WHERE workspace_id=$1 AND created_at < $2`, policy.IdempotencyKeys); e != nil {
		return out, MapError(e)
	}
	if out.AuditEvents, e = query(`SELECT count(*) FROM curriculum_studio.audit_events WHERE workspace_id=$1 AND created_at < $2`, policy.AuditEvents); e != nil {
		return out, MapError(e)
	}
	if dryRun {
		return out, nil
	}
	err := WithTx(ctx, r.Q, func(q Querier) error {
		if _, e := q.Exec(ctx, `DELETE FROM curriculum_studio.webhook_deliveries d USING curriculum_studio.webhook_endpoints ep WHERE d.endpoint_id=ep.id AND ep.workspace_id=$1 AND d.created_at<$2 AND d.status IN ('delivered','failed')`, workspaceID, now.Add(-policy.WebhookDeliveries)); e != nil {
			return e
		}
		if _, e := q.Exec(ctx, `DELETE FROM curriculum_studio.idempotency_keys WHERE workspace_id=$1 AND created_at<$2`, workspaceID, now.Add(-policy.IdempotencyKeys)); e != nil {
			return e
		}
		_, e := q.Exec(ctx, `DELETE FROM curriculum_studio.audit_events WHERE workspace_id=$1 AND created_at<$2`, workspaceID, now.Add(-policy.AuditEvents))
		return e
	})
	return out, MapError(err)
}
