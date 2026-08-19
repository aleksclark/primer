package repo

import (
	"context"
	"fmt"
	"time"
)

type MetricsRepo struct{ Q Querier }

func NewMetricsRepo(q Querier) *MetricsRepo { return &MetricsRepo{Q: q} }

// OutboxLag returns age of the oldest unpublished event, or zero when caught up.
func (r *MetricsRepo) OutboxLag(ctx context.Context, now time.Time) (time.Duration, error) {
	if r == nil || r.Q == nil {
		return 0, fmt.Errorf("%w", ErrClosed)
	}
	var seconds float64
	if e := r.Q.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM ($1 - min(created_at))),0) FROM curriculum_studio.outbox_events WHERE published_at IS NULL`, now).Scan(&seconds); e != nil {
		return 0, MapError(e)
	}
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
func (r *MetricsRepo) PendingWebhookDeliveries(ctx context.Context) (int64, error) {
	if r == nil || r.Q == nil {
		return 0, fmt.Errorf("%w", ErrClosed)
	}
	var n int64
	if e := r.Q.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.webhook_deliveries WHERE status IN ('pending','failed')`).Scan(&n); e != nil {
		return 0, MapError(e)
	}
	return n, nil
}
func (r *MetricsRepo) ExpiredWorkflowLeases(ctx context.Context, now time.Time) (int64, error) {
	if r == nil || r.Q == nil {
		return 0, fmt.Errorf("%w", ErrClosed)
	}
	var n int64
	if e := r.Q.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.workflow_stages WHERE status='running' AND lease_expires_at<= $1`, now).Scan(&n); e != nil {
		return 0, MapError(e)
	}
	return n, nil
}
