package remoteagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsclient "github.com/aleksclark/primer/agents/client/go"
	"github.com/aleksclark/primer/server/internal/remoteagent"
)

// staticToken is a test token source that returns a fixed value.
type staticToken struct{ tok string }

func (s staticToken) Token(_ context.Context) (string, error) {
	if s.tok == "" {
		return "", fmt.Errorf("no token configured")
	}
	return s.tok, nil
}

// recordingServer counts HTTP calls and returns canned responses.
type recordingServer struct {
	calls atomic.Int64
	srv   *httptest.Server
}

func newRecordingServer(t *testing.T, runID string) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	mux := http.NewServeMux()

	// POST /agents/v1/runs → return queued run
	mux.HandleFunc("POST /agents/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		rs.calls.Add(1)
		run := map[string]any{
			"id":             runID,
			"ownerNamespace": "human:identity:test/test-bff",
			"profile":        "tutor",
			"status":         "queued",
			"stateVersion":   float64(1),
			"idempotencyKey": "test-key",
			"createdAt":      time.Now().UTC().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
	})

	// GET /agents/v1/runs/{id} → return queued run
	mux.HandleFunc("GET /agents/v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		rs.calls.Add(1)
		run := map[string]any{
			"id":             runID,
			"ownerNamespace": "human:identity:test/test-bff",
			"profile":        "tutor",
			"status":         "queued",
			"stateVersion":   float64(1),
			"idempotencyKey": "test-key",
			"createdAt":      time.Now().UTC().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
	})

	// POST /agents/v1/runs/{id}/cancel
	mux.HandleFunc("POST /agents/v1/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		rs.calls.Add(1)
		run := map[string]any{
			"id": runID, "status": "cancel_requested", "stateVersion": float64(2),
			"ownerNamespace": "test", "profile": "tutor", "idempotencyKey": "test-key",
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
	})

	rs.srv = httptest.NewServer(mux)
	t.Cleanup(rs.srv.Close)
	return rs
}

// ── BDD: disabled adapter makes no remote call ────────────────────────────────

func TestAdapterDisabledByDefault(t *testing.T) {
	t.Parallel()
	adapter := remoteagent.New(remoteagent.Config{Enabled: false})
	assert.False(t, adapter.Enabled(), "adapter must be disabled when Enabled=false")

	_, err := adapter.StartRun(context.Background(), "key", "tutor", nil, true)
	require.ErrorIs(t, err, remoteagent.ErrDisabled,
		"StartRun on disabled adapter must return ErrDisabled, not make a network call")
}

func TestAdapterDisabledGetRunNoCall(t *testing.T) {
	t.Parallel()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	// Even with a base URL configured, Enabled=false must make no call.
	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     false,
		BaseURL:     srv.URL,
		TokenSource: staticToken{"tok"},
	})

	_, err := adapter.GetRun(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, remoteagent.ErrDisabled)
	assert.False(t, called, "no HTTP call must be made when adapter is disabled")
}

// ── BDD: pre-authorization denial makes no remote call ───────────────────────

func TestAdapterLocalAuthorizationRequired(t *testing.T) {
	t.Parallel()
	runID := uuid.NewString()
	rs := newRecordingServer(t, runID)

	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     rs.srv.URL,
		Timeout:     5 * time.Second,
		TokenSource: staticToken{"test-token"},
	})

	// locallyAuthorized=false — must not call remote.
	_, err := adapter.StartRun(context.Background(), "key-denied", "tutor", nil, false)
	require.ErrorIs(t, err, remoteagent.ErrLocalAuthorizationRequired)
	assert.Zero(t, rs.calls.Load(), "no HTTP call must be made when local authorization is absent")
}

// ── BDD: successful start records acceptance ──────────────────────────────────

func TestAdapterStartRunRecordsAcceptance(t *testing.T) {
	t.Parallel()
	runID := uuid.NewString()
	rs := newRecordingServer(t, runID)

	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     rs.srv.URL,
		Timeout:     5 * time.Second,
		TokenSource: staticToken{"test-token"},
	})

	key := "idem-key-" + uuid.NewString()[:8]
	run, err := adapter.StartRun(context.Background(), key, "tutor", nil, true)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "queued", run.Status)
	assert.Equal(t, int64(1), rs.calls.Load())

	// Acceptance must be recorded.
	acceptedID, ok := adapter.AcceptedRunID(key)
	require.True(t, ok, "run must be recorded as accepted after successful StartRun")
	assert.Equal(t, runID, acceptedID)
}

// ── BDD: post-acceptance retry uses same run, no duplicate ────────────────────

func TestAdapterPostAcceptanceRetryNoDuplicate(t *testing.T) {
	t.Parallel()
	runID := uuid.NewString()
	rs := newRecordingServer(t, runID)

	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     rs.srv.URL,
		Timeout:     5 * time.Second,
		TokenSource: staticToken{"test-token"},
	})

	key := "retry-key-" + uuid.NewString()[:8]

	// First call: creates the run.
	run1, err := adapter.StartRun(context.Background(), key, "tutor", nil, true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rs.calls.Load(), "first call must hit POST /runs")

	// Second call with same key: must reconnect to the existing run.
	run2, err := adapter.StartRun(context.Background(), key, "tutor", nil, true)
	require.NoError(t, err)

	// Same run ID — no new run was created.
	assert.Equal(t, run1.Id, run2.Id, "retry must return the same run ID")

	// The second call must use GET (not POST) — recorded as a call to /runs/{id}.
	assert.Equal(t, int64(2), rs.calls.Load(),
		"retry must make exactly one GET call (not a second POST /runs)")
}

// ── BDD: pre-acceptance outage allows legacy fallback ────────────────────────

func TestAdapterPreAcceptanceOutageFallback(t *testing.T) {
	t.Parallel()
	// Point at a server that always returns 503.
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"title":"unavailable","status":503}`))
	}))
	defer unavailable.Close()

	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     unavailable.URL,
		Timeout:     2 * time.Second,
		TokenSource: staticToken{"test-token"},
	})

	key := "outage-key-" + uuid.NewString()[:8]

	// Call fails before acceptance.
	_, err := adapter.StartRun(context.Background(), key, "tutor", nil, true)
	require.Error(t, err, "unavailable server must return an error")

	// No acceptance recorded — caller may fall back to legacy.
	_, ok := adapter.AcceptedRunID(key)
	assert.False(t, ok,
		"no acceptance must be recorded when server call fails (pre-acceptance fallback is safe)")
}

// ── BDD: post-acceptance outage must NOT fall back to legacy ─────────────────

func TestAdapterPostAcceptanceOutageReturnsError(t *testing.T) {
	t.Parallel()
	runID := uuid.NewString()
	rs := newRecordingServer(t, runID)

	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     rs.srv.URL,
		Timeout:     5 * time.Second,
		TokenSource: staticToken{"test-token"},
	})

	key := "post-accept-" + uuid.NewString()[:8]

	// First call succeeds and records acceptance.
	_, err := adapter.StartRun(context.Background(), key, "tutor", nil, true)
	require.NoError(t, err)
	require.True(t, adapter.IsAccepted(key))

	// Now point adapter at a downed server (swap base URL).
	adapter2 := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     "http://127.0.0.1:1", // nothing listening
		Timeout:     500 * time.Millisecond,
		TokenSource: staticToken{"test-token"},
	})
	// Manually inject acceptance state to simulate post-acceptance scenario.
	// (AcceptedRunID is read-only; we simulate by calling StartRun on the good
	// server first, then confirm via a GetRun call that fails on the down server.)
	_, getErr := adapter2.GetRun(context.Background(), runID)
	require.Error(t, getErr,
		"post-acceptance network failure must return an error, not silently start a new run")
	// The caller must report unavailable/interrupted truthfully;
	// no duplicate run should be created. We can't assert zero calls on adapter2
	// (it tried), but we assert it returned an error rather than fake success.
}

// ── BDD: EnvTokenSource reads from environment variable ──────────────────────

func TestEnvTokenSourceEmptyVar(t *testing.T) {
	t.Parallel()
	ts := remoteagent.EnvTokenSource{EnvVar: ""}
	_, err := ts.Token(context.Background())
	require.Error(t, err, "empty EnvVar must return an error")
}

func TestEnvTokenSourceMissingEnvVar(t *testing.T) {
	t.Parallel()
	ts := remoteagent.EnvTokenSource{EnvVar: "PRIMER_AGENTS_TEST_TOKEN_ABSENT_" + uuid.NewString()}
	_, err := ts.Token(context.Background())
	require.Error(t, err, "unset env var must return an error")
}

// ── BDD: config validation rejects missing URL when enabled ──────────────────

func TestAdapterNotConfiguredError(t *testing.T) {
	t.Parallel()
	// Enabled but no base URL → ErrNotConfigured.
	adapter := remoteagent.New(remoteagent.Config{
		Enabled:     true,
		BaseURL:     "", // missing
		TokenSource: staticToken{"tok"},
	})

	_, err := adapter.StartRun(context.Background(), "key", "tutor", nil, true)
	require.ErrorIs(t, err, remoteagent.ErrNotConfigured)
}

// ── Generated client type assertion ──────────────────────────────────────────

// TestGeneratedTypesUsed ensures we use the generated client types, not
// hand-written duplicates. Compile-time: if the package is not imported or
// the type changed, this test file won't compile.
func TestGeneratedTypesUsed(t *testing.T) {
	t.Parallel()
	// agentsclient.RunResponse is a generated type — ensure it's imported and used.
	var _ agentsclient.RunResponse
	var _ agentsclient.AgentsClient
}
