package repo_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestP12E1AuditMutationRow(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, _ := planFixture(t, tx)
	r := repo.NewAuditRepo(tx)
	actor := "identity:" + uuid.NewString()
	id := uuid.New()
	got, err := r.Insert(ctx, &repo.AuditEvent{WorkspaceID: &ws.ID, ActorSubjectRef: actor, Action: "item.locked", EntityKind: "materialized_item", EntityID: &id, Before: json.RawMessage(`{}`), After: json.RawMessage(`{"locked":true}`)})
	require.NoError(t, err)
	require.Equal(t, actor, got.ActorSubjectRef)
	rows, err := r.ListByWorkspace(ctx, ws.ID, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.JSONEq(t, `{"locked":true}`, string(rows[0].After))
}

func TestP12E2RetentionDryRunAndApplyProtectsDurableRows(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowFixture(t, tx)
	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{WorkspaceID: &ws.ID, EventType: domain.EventCurriculumCreated, AggregateKind: "run", AggregateID: run.ID, Payload: json.RawMessage(`{}`)})
	require.NoError(t, err)
	endpoint, err := repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{WorkspaceID: ws.ID, URL: "https://example.test/hook"})
	require.NoError(t, err)
	delivery, err := repo.NewWebhookDeliveryRepo(tx).Schedule(ctx, endpoint.ID, event.ID, "retention:"+uuid.NewString())
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE curriculum_studio.webhook_deliveries SET status='delivered',created_at=now()-interval '100 days' WHERE id=$1`, delivery.ID)
	require.NoError(t, err)
	_, err = repo.NewIdempotencyRepo(tx).Begin(ctx, ws.ID, "test", "old", "hash")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE curriculum_studio.idempotency_keys SET created_at=now()-interval '100 days' WHERE workspace_id=$1 AND key='old'`, ws.ID)
	require.NoError(t, err)
	before, err := repo.NewRetentionRepo(tx).Retain(ctx, ws.ID, time.Now(), repo.RetentionPolicy{WebhookDeliveries: time.Hour, IdempotencyKeys: time.Hour, AuditEvents: time.Hour}, true)
	require.NoError(t, err)
	require.Equal(t, int64(1), before.WebhookDeliveries)
	require.Equal(t, int64(1), before.IdempotencyKeys)
	after, err := repo.NewRetentionRepo(tx).Retain(ctx, ws.ID, time.Now(), repo.RetentionPolicy{WebhookDeliveries: time.Hour, IdempotencyKeys: time.Hour, AuditEvents: time.Hour}, false)
	require.NoError(t, err)
	require.Equal(t, before, after)
	var runCount int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.materialization_runs WHERE id=$1`, run.ID).Scan(&runCount))
	require.Equal(t, 1, runCount)
}

func TestP12E4MetricsQueries(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{WorkspaceID: &ws.ID, EventType: domain.EventPlanRevisionPublished, AggregateKind: "plan_revision", AggregateID: rev.ID, Payload: json.RawMessage(`{}`)})
	require.NoError(t, err)
	metrics := repo.NewMetricsRepo(tx)
	lag, err := metrics.OutboxLag(ctx, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Positive(t, lag)
	_, err = tx.Exec(ctx, `UPDATE curriculum_studio.outbox_events SET published_at=now() WHERE id=$1`, event.ID)
	require.NoError(t, err)
	lag, err = metrics.OutboxLag(ctx, time.Now())
	require.NoError(t, err)
	require.Zero(t, lag)
	pending, err := metrics.PendingWebhookDeliveries(ctx)
	require.NoError(t, err)
	require.Zero(t, pending)
}
