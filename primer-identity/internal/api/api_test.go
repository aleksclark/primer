package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/logging"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestHealthzReturnsOK(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

func TestReadyzReturnsOKWhenDBUp(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/readyz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ready", body["status"])
}

func TestReadyzFailsWhenDBDown(t *testing.T) {
	t.Parallel()
	// Closed pool cannot ping.
	cfg, err := pgxpool.ParseConfig("postgres://primer:primer@127.0.0.1:1/primer_identity_test?sslmode=disable")
	require.NoError(t, err)
	cfg.MaxConns = 1
	dead, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	dead.Close()

	_, handler := api.New(dead, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/readyz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)

	// Liveness may still be OK without a live pool dependency.
	hresp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer hresp.Body.Close()
	assert.Equal(t, http.StatusOK, hresp.StatusCode)
}

func TestRequestIDMiddleware(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", "client-rid-99")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "client-rid-99", resp.Header.Get("X-Request-ID"))

	// Without client id, server assigns one.
	resp2, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.NotEmpty(t, resp2.Header.Get("X-Request-ID"))
}

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Hit health first so the counter increments.
	_, _ = http.Get(srv.URL + "/healthz")

	resp, err := http.Get(srv.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "identity_http_requests_total")
}

func TestRequestIDFromContext(t *testing.T) {
	t.Parallel()
	assert.Empty(t, api.RequestIDFromContext(context.Background()))

	// Exercise middleware path that stores request id in context via a custom handler.
	// RegisterRoutes does not expose ctx value on response; unit-check empty default.
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{Now: func() time.Time { return time.Unix(0, 0) }})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotEmpty(t, rr.Header().Get("X-Request-ID"))
}

func TestReadyzNilPool(t *testing.T) {
	t.Parallel()
	_, handler := api.NewWithPinger(nil, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/readyz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// secretPingError is a synthetic Ping failure that embeds a DSN password.
// It must never appear in the public /readyz body.
type secretPingError struct{}

func (secretPingError) Error() string {
	return "failed to connect to `host=localhost user=identity database=primer_identity`: " +
		"password authentication failed for user \"identity\" " +
		"(dsn=postgres://identity:readyz-leak-secret-ZZ9@localhost:5432/primer_identity)"
}

type secretPinger struct{}

func (secretPinger) Ping(context.Context) error { return secretPingError{} }

// /readyz must return only stable generic public problem details on DB failure.
// The underlying ping error is logged server-side (redacted) with request_id
// and must never appear in the HTTP body.
// Not parallel: temporarily replaces slog.Default for capture.
func TestReadyzDoesNotLeakUnderlyingDBError(t *testing.T) {
	const (
		sentinelSecret = "readyz-leak-secret-ZZ9"
		clientRID      = "rid-readyz-leak-1"
	)

	var logBuf bytes.Buffer
	logger := logging.NewJSONLogger(&logBuf, "info")
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	_, handler := api.NewWithPinger(secretPinger{}, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/readyz", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", clientRID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := string(bodyBytes)
	assert.NotContains(t, body, sentinelSecret)
	assert.NotContains(t, body, "password authentication failed")
	assert.NotContains(t, body, "postgres://")
	assert.NotContains(t, body, "readyz-leak-secret")

	var problem map[string]any
	require.NoError(t, json.Unmarshal(bodyBytes, &problem))
	// Stable generic public detail only — no nested underlying errors.
	detail, _ := problem["detail"].(string)
	assert.NotEmpty(t, detail)
	assert.NotContains(t, strings.ToLower(detail), "password")
	assert.NotContains(t, strings.ToLower(detail), "secret")
	if errs, ok := problem["errors"]; ok {
		raw, _ := json.Marshal(errs)
		assert.NotContains(t, string(raw), sentinelSecret)
		assert.NotContains(t, string(raw), "password authentication")
	}

	logs := logBuf.String()
	require.NotEmpty(t, logs, "expected server-side diagnostic log for readiness failure")
	assert.NotContains(t, logs, sentinelSecret)
	assert.NotContains(t, logs, "readyz-leak-secret-ZZ9")

	found := false
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		msg, _ := entry["msg"].(string)
		if !strings.Contains(strings.ToLower(msg), "ready") &&
			!strings.Contains(strings.ToLower(msg), "database") &&
			!strings.Contains(strings.ToLower(msg), "ping") {
			continue
		}
		if rid, _ := entry["request_id"].(string); rid == clientRID {
			found = true
			// Diagnostic present without secret.
			assert.Contains(t, line, "REDACTED")
			break
		}
	}
	require.True(t, found, "expected readiness failure log with request_id=%s; logs=%s", clientRID, logs)
}

// P1-S6: real router must emit a structured access log for /healthz with
// level/msg plus method, path, status, duration, request_id — and must not leak
// DSN passwords or bearer tokens via the redacting slog path.
// Not parallel: temporarily replaces slog.Default for capture.
func TestRequestAccessLogOnHealthz(t *testing.T) {
	pool := testutil.DB(t)

	var logBuf bytes.Buffer
	logger := logging.NewJSONLogger(&logBuf, "info")
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	const (
		sentinelDSN   = "postgres://identity:access-log-dsn-secret-ZZ9@localhost:5432/primer_identity?sslmode=disable"
		sentinelToken = "Bearer access-log-token-secret-AA7"
		clientRID     = "rid-access-log-1"
	)

	_, handler := api.New(pool, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", clientRID)
	// Adversarial: client Authorization must not be echoed into logs.
	req.Header.Set("Authorization", sentinelToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, clientRID, resp.Header.Get("X-Request-ID"))

	// Explicit redaction probe through the same logger path.
	logger.Info("probe-redaction", "database_url", sentinelDSN, "authorization", sentinelToken)

	logs := logBuf.String()
	require.NotEmpty(t, logs, "expected structured access log output")

	var access map[string]any
	found := false
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		msg, _ := entry["msg"].(string)
		if msg != "request" && msg != "access" && msg != "http_request" {
			continue
		}
		if rid, _ := entry["request_id"].(string); rid == clientRID {
			access = entry
			found = true
			break
		}
	}
	require.True(t, found, "expected access log entry for request_id=%s; logs=%s", clientRID, logs)

	assert.Equal(t, "INFO", access["level"])
	assert.Equal(t, "GET", access["method"])
	// Path only — no query string cardinality.
	path, _ := access["path"].(string)
	assert.Equal(t, "/healthz", path)
	assert.NotContains(t, path, "?")
	// Status may be float64 from JSON numbers.
	switch s := access["status"].(type) {
	case float64:
		assert.Equal(t, float64(200), s)
	case int:
		assert.Equal(t, 200, s)
	case json.Number:
		n, _ := s.Int64()
		assert.Equal(t, int64(200), n)
	default:
		t.Fatalf("status type %T = %v", access["status"], access["status"])
	}
	assert.Equal(t, clientRID, access["request_id"])
	_, hasDur := access["duration_ms"]
	_, hasDurAlt := access["duration"]
	assert.True(t, hasDur || hasDurAlt, "expected duration_ms or duration field")

	// No secret leakage in the entire buffer.
	assert.NotContains(t, logs, "access-log-dsn-secret-ZZ9")
	assert.NotContains(t, logs, "access-log-token-secret-AA7")
	assert.NotContains(t, logs, sentinelToken)
}
