package outbox_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/outbox"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestP14S1ImplementedOutboxTypes(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	subject := domain.HumanSubjectRef(uuid.New())

	cur, err := repo.NewCurriculumRepo(tx).Create(ctx, &domain.Curriculum{
		WorkspaceID: ws.ID, Slug: "math-" + uuid.NewString()[:8], Title: "Math",
	})
	require.NoError(t, err)
	_, err = repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &ws.ID, EventType: domain.EventCurriculumCreated,
		AggregateKind: "curriculum", AggregateID: cur.ID,
		Payload: json.RawMessage(`{"curriculum_id":"` + cur.ID.String() + `"}`),
	})
	require.NoError(t, err)

	rev, err := repo.NewPlanRevisionRepo(tx).Create(ctx, ws.ID, &domain.PlanRevision{
		CurriculumID: cur.ID, Revision: 1, Title: "Draft",
	})
	require.NoError(t, err)
	require.NoError(t, repo.NewPlanRevisionRepo(tx).Publish(ctx, ws.ID, rev.ID, subject))

	_, err = repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &ws.ID, EventType: domain.EventMaterializationRequested,
		AggregateKind: "materialization_run", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{"version":1}`),
	})
	require.NoError(t, err)

	events, err := repo.NewOutboxRepo(tx).ListByWorkspace(ctx, ws.ID)
	require.NoError(t, err)
	got := map[string]int{}
	for _, event := range events {
		got[event.EventType]++
	}
	require.Equal(t, 1, got[domain.EventCurriculumCreated])
	require.Equal(t, 1, got[domain.EventPlanRevisionPublished])
	require.Equal(t, 1, got[domain.EventMaterializationRequested])
	require.Zero(t, got[domain.EventMaterializationReady])
	require.Zero(t, got[domain.EventMaterializationFailed])
	require.Zero(t, got[domain.EventMaterializedItemSuperseded])
	require.Zero(t, got[domain.EventPlanChangeProposed])
}

func TestDeliverSignedJSONToReceiver(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	secret := []byte("receiver-secret")
	secrets := outbox.NewMemorySecrets()
	secrets.Put("secret-ref:deliver", secret)

	var (
		mu      sync.Mutex
		gotBody []byte
		gotReq  *http.Request
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		mu.Lock()
		gotBody = append([]byte(nil), body...)
		gotReq = r.Clone(ctx)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &ws.ID, EventType: domain.EventCurriculumCreated,
		AggregateKind: "curriculum", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{"curriculum_id":"` + uuid.NewString() + `"}`),
	})
	require.NoError(t, err)
	_, err = repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{
		WorkspaceID: ws.ID, URL: srv.URL, SecretRef: "secret-ref:deliver",
		EventTypes: []string{domain.EventCurriculumCreated}, Status: "active",
	})
	require.NoError(t, err)

	worker := mustWorker(t, tx, secrets)
	require.NoError(t, worker.DrainUntilIdle(ctx))

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, gotReq)
	require.Equal(t, "application/json", gotReq.Header.Get("Content-Type"))
	require.Equal(t, event.ID.String(), gotReq.Header.Get(outbox.EventIDHeader))
	require.Equal(t, domain.EventCurriculumCreated, gotReq.Header.Get(outbox.EventTypeHeader))
	require.True(t, outbox.VerifyV1(secret, gotReq.Header.Get(outbox.SignatureHeader), gotReq.Header.Get(outbox.TimestampHeader), gotBody))
	require.Equal(t, gotReq.Header.Get(outbox.SignatureHeader), gotReq.Header.Get(outbox.CompatSignatureHeader))

	var env outbox.DomainEvent
	require.NoError(t, json.Unmarshal(gotBody, &env))
	require.Equal(t, event.ID.String(), env.ID)
	require.Equal(t, domain.EventCurriculumCreated, env.Type)
	require.Equal(t, ws.ID.String(), env.WorkspaceID)

	delivery := mustDelivery(t, tx, event.ID)
	require.Equal(t, "delivered", delivery.Status)
	require.Equal(t, outbox.IdempotencyKey(delivery.EndpointID, event.ID), delivery.IdempotencyKey)
	published, err := repo.NewOutboxRepo(tx).Get(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, published.PublishedAt)
}

func TestDeliverIdempotentRetryStableDedupeKey(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	secrets := outbox.NewMemorySecrets()
	secrets.Put("secret-ref:retry", []byte("retry-secret"))

	var hits atomic.Int32
	var keys []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &ws.ID, EventType: domain.EventPlanRevisionPublished,
		AggregateKind: "plan_revision", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{"revision_id":"` + uuid.NewString() + `"}`),
	})
	require.NoError(t, err)
	endpoint, err := repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{
		WorkspaceID: ws.ID, URL: srv.URL, SecretRef: "secret-ref:retry", Status: "active",
	})
	require.NoError(t, err)

	first, err := repo.NewWebhookDeliveryRepo(tx).Schedule(ctx, endpoint.ID, event.ID, outbox.IdempotencyKey(endpoint.ID, event.ID))
	require.NoError(t, err)
	again, err := repo.NewWebhookDeliveryRepo(tx).Schedule(ctx, endpoint.ID, event.ID, outbox.IdempotencyKey(endpoint.ID, event.ID))
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, first.IdempotencyKey, again.IdempotencyKey)

	worker := mustWorker(t, tx, secrets)
	require.NoError(t, worker.DrainUntilIdle(ctx))
	require.NoError(t, expireDeliveryLease(ctx, tx, first.ID))
	require.NoError(t, worker.DrainUntilIdle(ctx))

	require.GreaterOrEqual(t, hits.Load(), int32(2))
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, keys)
	for _, key := range keys {
		require.Equal(t, first.IdempotencyKey, key)
	}
	got := mustDelivery(t, tx, event.ID)
	require.Equal(t, first.ID, got.ID)
	require.Equal(t, "delivered", got.Status)
}

func TestTenantIsolationDoesNotInvokeForeignEndpoint(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	w1 := factory.Workspace(t, tx)
	w2 := factory.Workspace(t, tx)
	secrets := outbox.NewMemorySecrets()
	secrets.Put("secret-ref:w1", []byte("w1-secret"))
	secrets.Put("secret-ref:w2", []byte("w2-secret"))

	var w1Hits, w2Hits atomic.Int32
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w1Hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s1.Close)
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w2Hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s2.Close)

	_, err := repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{
		WorkspaceID: w1.ID, URL: s1.URL, SecretRef: "secret-ref:w1", Status: "active",
	})
	require.NoError(t, err)
	_, err = repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{
		WorkspaceID: w2.ID, URL: s2.URL, SecretRef: "secret-ref:w2", Status: "active",
	})
	require.NoError(t, err)

	_, err = repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &w2.ID, EventType: domain.EventCurriculumCreated,
		AggregateKind: "curriculum", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	worker := mustWorker(t, tx, secrets)
	require.NoError(t, worker.DrainUntilIdle(ctx))
	require.Equal(t, int32(0), w1Hits.Load(), "W1 endpoint must not receive W2 events")
	require.Equal(t, int32(1), w2Hits.Load())
}

func TestRestartContinuesDrainAfterReceiverDown(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	secrets := outbox.NewMemorySecrets()
	secrets.Put("secret-ref:restart", []byte("restart-secret"))

	var fail atomic.Bool
	fail.Store(true)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &ws.ID, EventType: domain.EventMaterializationRequested,
		AggregateKind: "materialization_run", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{"version":1}`),
	})
	require.NoError(t, err)
	_, err = repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{
		WorkspaceID: ws.ID, URL: srv.URL, SecretRef: "secret-ref:restart", Status: "active",
	})
	require.NoError(t, err)

	first, err := outbox.NewWorker(tx, outbox.Config{
		Owner: "worker-a", Secrets: secrets, PollInterval: 10 * time.Millisecond,
		DeliveryTimeout: time.Second, LeaseTTL: time.Second, MaxAttempts: 5,
	})
	require.NoError(t, err)
	runCtx, stop := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- first.Run(runCtx) }()
	require.Eventually(t, func() bool { return hits.Load() >= 1 }, 3*time.Second, 20*time.Millisecond)

	stop()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first worker did not stop")
	}

	still, err := repo.NewOutboxRepo(tx).Get(ctx, event.ID)
	require.NoError(t, err)
	require.NotNil(t, still)
	delivery := mustDelivery(t, tx, event.ID)
	require.NotEqual(t, "delivered", delivery.Status)
	require.NoError(t, expireDeliveryLease(ctx, tx, delivery.ID))

	fail.Store(false)
	second, err := outbox.NewWorker(tx, outbox.Config{
		Owner: "worker-b", Secrets: secrets, PollInterval: 10 * time.Millisecond,
		DeliveryTimeout: time.Second, LeaseTTL: time.Second, MaxAttempts: 5,
	})
	require.NoError(t, err)
	require.NoError(t, second.DrainUntilIdle(ctx))

	got := mustDelivery(t, tx, event.ID)
	require.Equal(t, "delivered", got.Status)
	require.GreaterOrEqual(t, hits.Load(), int32(2))
	unchanged, err := repo.NewOutboxRepo(tx).Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, event.ID, unchanged.ID)
}

func TestDeliverDoesNotMarkSuccessWithoutHTTP2xx(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	secrets := outbox.NewMemorySecrets()
	secrets.Put("secret-ref:fail", []byte("fail-secret"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	event, err := repo.NewOutboxRepo(tx).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &ws.ID, EventType: domain.EventCurriculumCreated,
		AggregateKind: "curriculum", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	_, err = repo.NewWebhookEndpointRepo(tx).Create(ctx, &domain.WebhookEndpoint{
		WorkspaceID: ws.ID, URL: srv.URL, SecretRef: "secret-ref:fail", Status: "active",
	})
	require.NoError(t, err)

	worker, err := outbox.NewWorker(tx, outbox.Config{
		Owner: "fail-worker", Secrets: secrets, MaxAttempts: 1,
		DeliveryTimeout: time.Second, LeaseTTL: time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, worker.DrainUntilIdle(ctx))
	got := mustDelivery(t, tx, event.ID)
	require.Equal(t, "failed", got.Status)
	require.Nil(t, got.DeliveredAt)
}

func TestSignV1RoundTrip(t *testing.T) {
	secret := []byte("doc-secret")
	body := []byte(`{"id":"evt"}`)
	sig, ts := outbox.SignV1(secret, time.Unix(1_700_000_000, 0).UTC(), body)
	require.True(t, outbox.VerifyV1(secret, sig, ts, body))
	require.False(t, outbox.VerifyV1(secret, sig, ts, []byte(`{"id":"other"}`)))
	require.False(t, outbox.VerifyV1([]byte("other"), sig, ts, body))
}

func mustWorker(t *testing.T, q repo.Querier, secrets outbox.SecretResolver) *outbox.Worker {
	t.Helper()
	w, err := outbox.NewWorker(q, outbox.Config{
		Owner: "test-worker-" + uuid.NewString(), Secrets: secrets,
		PollInterval: 10 * time.Millisecond, DeliveryTimeout: time.Second,
		LeaseTTL: time.Second, MaxAttempts: 5,
	})
	require.NoError(t, err)
	return w
}

func mustDelivery(t *testing.T, q repo.Querier, eventID uuid.UUID) *domain.WebhookDelivery {
	t.Helper()
	var id uuid.UUID
	err := q.QueryRow(context.Background(), `SELECT id FROM curriculum_studio.webhook_deliveries WHERE event_id=$1`, eventID).Scan(&id)
	require.NoError(t, err)
	got, err := repo.NewWebhookDeliveryRepo(q).Get(context.Background(), id)
	require.NoError(t, err)
	return got
}

func expireDeliveryLease(ctx context.Context, q repo.Querier, id uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE curriculum_studio.webhook_deliveries SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, id)
	return err
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	defer r.Body.Close()
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}
