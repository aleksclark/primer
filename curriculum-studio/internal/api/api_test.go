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

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/logging"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestHealthReturnsOK(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/studio/v1/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

func TestReadyReturnsOKWhenDBUp(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/studio/v1/ready")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ready", body["status"])
}

func TestReadyFailsWhenDBDown(t *testing.T) {
	t.Parallel()
	// Closed pool cannot ping.
	cfg, err := pgxpool.ParseConfig("postgres://studio:x@127.0.0.1:1/curriculum_studio_test?sslmode=disable")
	require.NoError(t, err)
	cfg.MaxConns = 1
	dead, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	dead.Close()

	_, handler := api.New(dead, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/studio/v1/ready")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)

	// Liveness may still be OK without a live pool dependency.
	hresp, err := http.Get(srv.URL + "/studio/v1/health")
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

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/health", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", "client-rid-99")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "client-rid-99", resp.Header.Get("X-Request-ID"))

	// Without client id, server assigns one.
	resp2, err := http.Get(srv.URL + "/studio/v1/health")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.NotEmpty(t, resp2.Header.Get("X-Request-ID"))
}

func TestRequestIDRejectsOverlongAndUnsafe(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Overlong client id must be replaced.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/health", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", strings.Repeat("a", 200))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	got := resp.Header.Get("X-Request-ID")
	assert.NotEmpty(t, got)
	assert.NotEqual(t, strings.Repeat("a", 200), got)
	assert.LessOrEqual(t, len(got), 128)

	// Unsafe characters rejected (use recorder so net/http client header
	// validation does not mask middleware behavior).
	req2 := httptest.NewRequest(http.MethodGet, "/studio/v1/health", nil)
	req2.Header.Set("X-Request-ID", "bad rid with spaces")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code)
	assert.NotEqual(t, "bad rid with spaces", rr2.Header().Get("X-Request-ID"))
	assert.NotEmpty(t, rr2.Header().Get("X-Request-ID"))
}

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	_, _ = http.Get(srv.URL + "/studio/v1/health")

	for _, path := range []string{"/metrics", "/studio/v1/metrics"} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "studio_http_requests_total")
	}
}

func TestRequestIDFromContext(t *testing.T) {
	t.Parallel()
	assert.Empty(t, api.RequestIDFromContext(context.Background()))

	pool := testutil.DB(t)
	_, handler := api.New(pool, api.Options{Now: func() time.Time { return time.Unix(0, 0) }})
	req := httptest.NewRequest(http.MethodGet, "/studio/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotEmpty(t, rr.Header().Get("X-Request-ID"))
}

func TestReadyNilPool(t *testing.T) {
	t.Parallel()
	_, handler := api.NewWithPinger(nil, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/studio/v1/ready")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// secretPingError is a synthetic Ping failure that embeds a DSN password.
// It must never appear in the public /studio/v1/ready body.
type secretPingError struct{}

func (secretPingError) Error() string {
	return "failed to connect to `host=localhost user=studio database=curriculum_studio`: " +
		"password authentication failed for user \"studio\" " +
		"(dsn=postgres://studio:readyz-leak-secret-ZZ9@localhost:5432/curriculum_studio)"
}

type secretPinger struct{}

func (secretPinger) Ping(context.Context) error { return secretPingError{} }

// /studio/v1/ready must return only stable generic public problem details on DB
// failure. The underlying ping error is logged server-side (redacted) with
// request_id and must never appear in the HTTP body.
// Not parallel: temporarily replaces slog.Default for capture.
func TestReadyDoesNotLeakUnderlyingDBError(t *testing.T) {
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

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/ready", nil)
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
			assert.Contains(t, line, "REDACTED")
			break
		}
	}
	require.True(t, found, "expected readiness failure log with request_id=%s; logs=%s", clientRID, logs)
}

// P1-S6 / P1-E4: real router must emit a structured access log for health with
// level/msg plus method, path, status, duration, request_id — and must not leak
// DSN passwords or bearer tokens via the redacting slog path.
// Not parallel: temporarily replaces slog.Default for capture.
func TestRequestAccessLogOnHealth(t *testing.T) {
	pool := testutil.DB(t)

	var logBuf bytes.Buffer
	logger := logging.NewJSONLogger(&logBuf, "info")
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	const (
		sentinelDSN   = "postgres://studio:access-log-dsn-secret-ZZ9@localhost:5432/curriculum_studio?sslmode=disable"
		sentinelToken = "Bearer access-log-token-secret-AA7"
		clientRID     = "rid-access-log-1"
	)

	_, handler := api.New(pool, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/health?token=should-not-log", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", clientRID)
	req.Header.Set("Authorization", sentinelToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, clientRID, resp.Header.Get("X-Request-ID"))

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
	path, _ := access["path"].(string)
	assert.Equal(t, "/studio/v1/health", path)
	assert.NotContains(t, path, "?")
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

	assert.NotContains(t, logs, "access-log-dsn-secret-ZZ9")
	assert.NotContains(t, logs, "access-log-token-secret-AA7")
	assert.NotContains(t, logs, sentinelToken)
	assert.NotContains(t, logs, "should-not-log")
}
