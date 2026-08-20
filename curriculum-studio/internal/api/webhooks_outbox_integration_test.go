package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/outbox"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func TestAPICreatedWebhookDeliversWithSharedSecretStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := outbox.NewMemorySecrets()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	validator, err := authn.NewValidator(authn.Options{
		Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL,
		Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{
		Validator: validator,
		Now:       func() time.Time { return now },
		Secrets:   secrets,
	})

	var gotBody []byte
	var gotSignature, gotTimestamp string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSignature = r.Header.Get(outbox.SignatureHeader)
		gotTimestamp = r.Header.Get(outbox.TimestampHeader)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleOwner)
	token := mintHuman(t, key, now, subject)
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/webhooks"
	created := doJSON(t, handler, http.MethodPost, path, map[string]any{
		"url":        receiver.URL,
		"eventTypes": []string{domain.EventCurriculumCreated},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.NotContains(t, created.Body.String(), "secret")
	var hook struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &hook))

	_, err = repo.NewOutboxRepo(pool).Enqueue(ctx, &domain.OutboxEvent{
		WorkspaceID: &workspace.ID, EventType: domain.EventCurriculumCreated,
		AggregateKind: "curriculum", AggregateID: uuid.New(),
		Payload: json.RawMessage(`{"curriculum_id":"` + uuid.NewString() + `"}`),
	})
	require.NoError(t, err)

	worker, err := outbox.NewWorker(pool, outbox.Config{
		Owner: "api-webhook-integration-" + uuid.NewString(), Secrets: secrets,
		DeliveryTimeout: time.Second, LeaseTTL: 30 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, worker.DrainUntilIdle(ctx))

	endpointID := decodePrefixed(t, hook.ID, "wh_")
	endpoint, err := repo.NewWebhookEndpointRepo(pool).Get(ctx, endpointID)
	require.NoError(t, err)
	secret, err := secrets.Resolve(ctx, endpoint.SecretRef)
	require.NoError(t, err)
	require.NotEmpty(t, gotBody)
	require.True(t, outbox.VerifyV1(secret, gotSignature, gotTimestamp, gotBody))

	deliveries := doJSON(t, handler, http.MethodGet, "/studio/v1/webhooks/"+hook.ID+"/deliveries", nil, token)
	require.Equal(t, http.StatusOK, deliveries.Code, deliveries.Body.String())
	var page struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(deliveries.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	require.NotContains(t, deliveries.Body.String(), "httpStatus")
	require.Eventually(t, func() bool {
		var deliveryStatus string
		return pool.QueryRow(ctx, `SELECT status FROM curriculum_studio.webhook_deliveries WHERE endpoint_id=$1`, endpointID).Scan(&deliveryStatus) == nil && deliveryStatus == "delivered"
	}, 3*time.Second, 20*time.Millisecond)
}
