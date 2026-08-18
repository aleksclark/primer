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

func TestP11E1PublishEnqueuesOutboxInSameTransaction(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, cur, rev := planFixture(t, tx)
	require.NoError(t, repo.NewPlanRevisionRepo(tx).Publish(ctx, ws.ID, rev.ID, "identity:"+uuid.NewString()))
	var event domain.OutboxEvent
	require.NoError(t, tx.QueryRow(ctx, `SELECT id,workspace_id,event_type,aggregate_kind,aggregate_id,payload,created_at,published_at FROM curriculum_studio.outbox_events WHERE aggregate_id=$1`, rev.ID).Scan(&event.ID, &event.WorkspaceID, &event.EventType, &event.AggregateKind, &event.AggregateID, &event.Payload, &event.CreatedAt, &event.PublishedAt))
	require.Equal(t, domain.EventPlanRevisionPublished, event.EventType)
	require.Equal(t, cur.WorkspaceID, *event.WorkspaceID)
	require.JSONEq(t, `{"revision_id":"`+rev.ID.String()+`"}`, string(event.Payload))
}

func TestP11E2WebhookDeliveryLeaseAndAtLeastOnce(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{WorkspaceID: &ws.ID, EventType: domain.EventCurriculumCreated, AggregateKind: "curriculum", AggregateID: rev.CurriculumID, Payload: json.RawMessage(`{"version":1}`)})
	require.NoError(t, err)
	endpoint, err := repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{WorkspaceID: ws.ID, URL: "https://example.test/hook", EventTypes: []string{domain.EventCurriculumCreated}, SecretRef: "secret-ref:one"})
	require.NoError(t, err)
	delivery, err := repo.NewWebhookDeliveryRepo(tx).Schedule(ctx, endpoint.ID, event.ID, "delivery:"+event.ID.String())
	require.NoError(t, err)
	claimed, err := repo.NewWebhookDeliveryRepo(tx).Claim(ctx, "worker-a", time.Minute)
	require.NoError(t, err)
	require.Equal(t, delivery.ID, claimed.ID)
	require.Equal(t, 1, claimed.AttemptCount)
	finished, err := repo.NewWebhookDeliveryRepo(tx).MarkDelivered(ctx, claimed.ID, "worker-a")
	require.NoError(t, err)
	require.Equal(t, "delivered", finished.Status)
	again, err := repo.NewWebhookDeliveryRepo(tx).Schedule(ctx, endpoint.ID, event.ID, "delivery:"+event.ID.String())
	require.NoError(t, err)
	require.Equal(t, delivery.ID, again.ID)
}

func TestP11E3InboundIdempotencyScopeAndHash(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, _ := planFixture(t, tx)
	r := repo.NewIdempotencyRepo(tx)
	first, err := r.Begin(ctx, ws.ID, "materialize", "K", "hash-a")
	require.NoError(t, err)
	replay, err := r.Begin(ctx, ws.ID, "materialize", "K", "hash-a")
	require.NoError(t, err)
	require.Equal(t, first.ID, replay.ID)
	_, err = r.SetResponse(ctx, ws.ID, "materialize", "K", "response:1")
	require.NoError(t, err)
	_, err = r.Begin(ctx, ws.ID, "materialize", "K", "hash-b")
	require.ErrorIs(t, err, repo.ErrConflict)
	other, err := r.Begin(ctx, ws.ID, "other-scope", "K", "hash-b")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, other.ID)
}
