package agentsclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsclient "github.com/aleksclark/primer/agents/client/go"
)

// stubServer is a minimal httptest server that drives the generated client
// through every route. It exercises the generated types (not just text
// snapshots) and proves the contract is wired end-to-end.
//
// This is a contract compile/round-trip test: it is not a full integration
// test (no real PostgreSQL). Real integration lives in internal/appservice.
func stubServer(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now().UTC()
	runID := uuid.NewString()
	sessID := uuid.NewString()

	mux := newStubMux(t, runID, sessID, now)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGeneratedClientCreateRun(t *testing.T) {
	t.Parallel()
	srv := stubServer(t)
	c, err := agentsclient.NewAgentsClient(srv.URL, "test-token")
	require.NoError(t, err)

	run, err := c.CreateRun(context.Background(), "key-1", agentsclient.CreateRunJSONRequestBody{
		Profile: "tutor",
	})
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.NotEmpty(t, run.Id)
	assert.Equal(t, "queued", run.Status)
	assert.Equal(t, "tutor", run.Profile)
	assert.Contains(t, run.OwnerNamespace, "test-ns")
}

func TestGeneratedClientGetRun(t *testing.T) {
	t.Parallel()
	srv := stubServer(t)
	c, err := agentsclient.NewAgentsClient(srv.URL, "test-token")
	require.NoError(t, err)

	// Create first, then Get by ID.
	created, err := c.CreateRun(context.Background(), "key-get", agentsclient.CreateRunJSONRequestBody{Profile: "tutor"})
	require.NoError(t, err)

	got, err := c.GetRun(context.Background(), created.Id.String())
	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
}

func TestGeneratedClientListRuns(t *testing.T) {
	t.Parallel()
	srv := stubServer(t)
	c, err := agentsclient.NewAgentsClient(srv.URL, "test-token")
	require.NoError(t, err)

	limit := int64(10)
	runs, err := c.ListRuns(context.Background(), &limit)
	require.NoError(t, err)
	assert.NotNil(t, runs)
}

func TestGeneratedClientCancelRun(t *testing.T) {
	t.Parallel()
	srv := stubServer(t)
	c, err := agentsclient.NewAgentsClient(srv.URL, "test-token")
	require.NoError(t, err)

	created, err := c.CreateRun(context.Background(), "key-cancel", agentsclient.CreateRunJSONRequestBody{Profile: "tutor"})
	require.NoError(t, err)

	canceled, err := c.CancelRun(context.Background(), created.Id.String(), agentsclient.CancelRunJSONRequestBody{})
	require.NoError(t, err)
	assert.Equal(t, "cancel_requested", canceled.Status)
}

func TestGeneratedClientListEvents(t *testing.T) {
	t.Parallel()
	srv := stubServer(t)
	c, err := agentsclient.NewAgentsClient(srv.URL, "test-token")
	require.NoError(t, err)

	created, err := c.CreateRun(context.Background(), "key-events", agentsclient.CreateRunJSONRequestBody{Profile: "tutor"})
	require.NoError(t, err)

	zero := int64(0)
	evs, err := c.ListEvents(context.Background(), created.Id.String(), &zero, nil)
	require.NoError(t, err)
	assert.NotNil(t, evs)
}

func TestGeneratedClientCreateGetSession(t *testing.T) {
	t.Parallel()
	srv := stubServer(t)
	c, err := agentsclient.NewAgentsClient(srv.URL, "test-token")
	require.NoError(t, err)

	sess, err := c.CreateSession(context.Background(), agentsclient.CreateSessionJSONRequestBody{Profile: "admin"})
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, "open", sess.Status)

	got, err := c.GetSession(context.Background(), sess.Id.String())
	require.NoError(t, err)
	assert.Equal(t, sess.Id, got.Id)
}

// ── Stub mux helpers ──────────────────────────────────────────────────────────

func newStubMux(t *testing.T, runID, sessID string, now time.Time) *stubMux {
	return &stubMux{t: t, runID: runID, sessID: sessID, now: now}
}

type stubMux struct {
	t     *testing.T
	runID string
	sessID string
	now   time.Time
}

func (m *stubMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "POST" && r.URL.Path == "/agents/v1/runs":
		writeJSON(w, agentsclient.RunResponse{
			Id:             mustUUID(m.runID),
			OwnerNamespace: "test-ns/bff-client",
			Profile:        "tutor",
			Status:         "queued",
			StateVersion:   1,
			IdempotencyKey: "key",
			CreatedAt:      m.now,
		})
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/runs/"+m.runID):
		writeJSON(w, agentsclient.RunResponse{
			Id: mustUUID(m.runID), Profile: "tutor", Status: "queued",
			OwnerNamespace: "test-ns", StateVersion: 1,
			IdempotencyKey: "key", CreatedAt: m.now,
		})
	case r.Method == "GET" && r.URL.Path == "/agents/v1/runs":
		writeJSON(w, map[string]any{"runs": []agentsclient.RunResponse{}})
	case r.Method == "POST" && strings.Contains(r.URL.Path, "/cancel"):
		writeJSON(w, agentsclient.RunResponse{
			Id: mustUUID(m.runID), Profile: "tutor", Status: "cancel_requested",
			OwnerNamespace: "test-ns", StateVersion: 2,
			IdempotencyKey: "key", CreatedAt: m.now,
		})
	case r.Method == "GET" && strings.Contains(r.URL.Path, "/events"):
		writeJSON(w, map[string]any{"events": []agentsclient.EventResponse{}})
	case r.Method == "POST" && r.URL.Path == "/agents/v1/sessions":
		writeJSON(w, agentsclient.SessionResponse{
			Id: mustUUID(m.sessID), Profile: "admin", Status: "open",
			OwnerNamespace: "test-ns", Revision: 1,
			CreatedAt: m.now, UpdatedAt: m.now,
		})
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/sessions/"+m.sessID):
		writeJSON(w, agentsclient.SessionResponse{
			Id: mustUUID(m.sessID), Profile: "admin", Status: "open",
			OwnerNamespace: "test-ns", Revision: 1,
			CreatedAt: m.now, UpdatedAt: m.now,
		})
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func mustUUID(s string) openapi_types.UUID {
	u, _ := uuid.Parse(s)
	return u
}
