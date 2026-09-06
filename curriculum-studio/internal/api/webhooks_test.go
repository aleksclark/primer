package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/outbox"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestWebhookCRUDListDeliveriesAndIsolation(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/webhooks"

	created := doJSON(t, handler, http.MethodPost, path, map[string]any{
		"url":        "https://hooks.example.test/studio",
		"eventTypes": []string{domain.EventCurriculumCreated, domain.EventPlanRevisionPublished},
		"enabled":    true,
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var hook struct {
		ID, WorkspaceID, URL string
		EventTypes           []string
		Enabled              bool
		SecretRef            string
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &hook))
	assert.True(t, strings.HasPrefix(hook.ID, "wh_"))
	assert.True(t, strings.HasPrefix(hook.WorkspaceID, "ws_"))
	assert.Equal(t, "https://hooks.example.test/studio", hook.URL)
	assert.Equal(t, []string{domain.EventCurriculumCreated, domain.EventPlanRevisionPublished}, hook.EventTypes)
	assert.True(t, hook.Enabled)
	assert.Empty(t, hook.SecretRef)
	assert.NotContains(t, created.Body.String(), "secret")

	listed := doJSON(t, handler, http.MethodGet, path, nil, token)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var page struct {
		Items      []map[string]any `json:"items"`
		TotalCount int
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, 1, page.TotalCount)
	assert.NotContains(t, listed.Body.String(), "secretRef")

	got := doJSON(t, handler, http.MethodGet, "/studio/v1/webhooks/"+hook.ID, nil, token)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())

	updated := doJSON(t, handler, http.MethodPatch, "/studio/v1/webhooks/"+hook.ID, map[string]any{
		"url":        "https://hooks.example.test/studio-v2",
		"eventTypes": []string{domain.EventCurriculumCreated},
		"enabled":    false,
	}, token)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Contains(t, updated.Body.String(), "studio-v2")
	assert.Contains(t, updated.Body.String(), `"enabled":false`)

	event, err := repo.NewOutboxRepo(pool).Enqueue(t.Context(), &domain.OutboxEvent{
		WorkspaceID: &workspace.ID, EventType: domain.EventCurriculumCreated,
		AggregateKind: "curriculum", AggregateID: uuid.New(),
		Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	endpointID := decodePrefixed(t, hook.ID, "wh_")
	_, err = repo.NewWebhookDeliveryRepo(pool).Schedule(t.Context(), endpointID, event.ID, outbox.IdempotencyKey(endpointID, event.ID))
	require.NoError(t, err)

	deliveries := doJSON(t, handler, http.MethodGet, "/studio/v1/webhooks/"+hook.ID+"/deliveries", nil, token)
	require.Equal(t, http.StatusOK, deliveries.Code, deliveries.Body.String())
	var dpage struct {
		Items      []map[string]any `json:"items"`
		TotalCount int
	}
	require.NoError(t, json.Unmarshal(deliveries.Body.Bytes(), &dpage))
	require.Equal(t, 1, dpage.TotalCount)
	require.Len(t, dpage.Items, 1)
	assert.Equal(t, event.ID.String(), dpage.Items[0]["eventId"])

	foreign := factory.Workspace(t, pool)
	denied := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+foreign.ID.String()+"/webhooks", nil, token)
	assert.Equal(t, http.StatusNotFound, denied.Code)

	foreignSubject := uuid.New()
	factory.SeedMembership(t, pool, foreign.ID, domain.HumanSubjectRef(foreignSubject), domain.MembershipRoleOwner)
	foreignToken := mintHuman(t, key, now, foreignSubject)
	leak := doJSON(t, handler, http.MethodGet, "/studio/v1/webhooks/"+hook.ID, nil, foreignToken)
	assert.Equal(t, http.StatusNotFound, leak.Code)
	leakDeliveries := doJSON(t, handler, http.MethodGet, "/studio/v1/webhooks/"+hook.ID+"/deliveries", nil, foreignToken)
	assert.Equal(t, http.StatusNotFound, leakDeliveries.Code)

	viewer := uuid.New()
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(viewer), domain.MembershipRoleViewer)
	viewerTok := mintHuman(t, key, now, viewer)
	forbidden := doJSON(t, handler, http.MethodPost, path, map[string]any{"url": "https://hooks.example.test/nope"}, viewerTok)
	assert.Equal(t, http.StatusForbidden, forbidden.Code)

	deleted := doJSON(t, handler, http.MethodDelete, "/studio/v1/webhooks/"+hook.ID, nil, token)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
	gone := doJSON(t, handler, http.MethodGet, "/studio/v1/webhooks/"+hook.ID, nil, token)
	assert.Equal(t, http.StatusNotFound, gone.Code)
}

func TestWebhookCreateRejectsBadURLAndUnknownType(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleOwner)
	token := mintHuman(t, key, now, subject)
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/webhooks"

	badURL := doJSON(t, handler, http.MethodPost, path, map[string]any{"url": "ftp://example.test/hook"}, token)
	assert.Equal(t, http.StatusBadRequest, badURL.Code)
	badType := doJSON(t, handler, http.MethodPost, path, map[string]any{
		"url": "https://hooks.example.test/ok", "eventTypes": []string{"not.an.event"},
	}, token)
	assert.Equal(t, http.StatusBadRequest, badType.Code)
}
