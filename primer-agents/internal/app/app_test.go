package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/app"
	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/authn/jwttest"
	"github.com/aleksclark/primer/agents/internal/config"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

func TestRunServesHealthAndReady(t *testing.T) {
	t.Parallel()
	url := testutil.URL(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	baseURL := "http://" + ln.Addr().String()

	var logBuf bytes.Buffer
	shutdown := make(chan struct{})
	errCh := make(chan error, 1)

	cfg := &config.Config{
		DatabaseURL:           url,
		Host:                  "127.0.0.1",
		Port:                  ln.Addr().(*net.TCPAddr).Port,
		Env:                   "test",
		LogLevel:              "debug",
		ShutdownTimeout:       3 * time.Second,
		HTTPReadHeaderTimeout: 3 * time.Second,
		HTTPReadTimeout:       5 * time.Second,
		HTTPWriteTimeout:      5 * time.Second,
		HTTPIdleTimeout:       10 * time.Second,
		HTTPMaxBodyBytes:      1024,
		WorkerEnabled:         true,
	}

	go func() {
		errCh <- app.Run(context.Background(), app.Options{
			Config:         cfg,
			Stdout:         &logBuf,
			Listener:       ln,
			SkipMigrate:    true, // harness already migrated
			ShutdownSignal: shutdown,
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, baseURL))

	resp, err := http.Get(baseURL + "/healthz")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(baseURL + "/readyz")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	close(shutdown)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to exit")
	}

	logs := logBuf.String()
	assert.Contains(t, logs, "listening")
	assert.Contains(t, logs, "shutdown complete")
	// Redaction: DSN password must never appear in logs.
	assert.NotContains(t, logs, "agents:") // password portion of DSN
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DatabaseURL:           "postgres://u:x@localhost:5432/primer", // forbidden
		Port:                  8091,
		Env:                   "production",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPReadTimeout:       time.Second,
		HTTPWriteTimeout:      time.Second,
		HTTPIdleTimeout:       time.Second,
		HTTPMaxBodyBytes:      1024,
	}
	err := app.Run(context.Background(), app.Options{Config: cfg})
	require.Error(t, err)
}

func TestRunFailsOnUnreachableDSN(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DatabaseURL:           "postgres://agents:x@127.0.0.1:1/primer_agents?sslmode=disable&connect_timeout=1",
		Host:                  "127.0.0.1",
		Port:                  0,
		Env:                   "test",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPReadTimeout:       time.Second,
		HTTPWriteTimeout:      time.Second,
		HTTPIdleTimeout:       time.Second,
		HTTPMaxBodyBytes:      1024,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := app.Run(ctx, app.Options{Config: cfg, SkipMigrate: true})
	require.Error(t, err)
}

func TestRunFailsOnListenConflict(t *testing.T) {
	t.Parallel()
	url := testutil.URL(t)

	hold, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer hold.Close()
	port := hold.Addr().(*net.TCPAddr).Port

	cfg := &config.Config{
		DatabaseURL:           url,
		Host:                  "127.0.0.1",
		Port:                  port,
		Env:                   "test",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPReadTimeout:       time.Second,
		HTTPWriteTimeout:      time.Second,
		HTTPIdleTimeout:       time.Second,
		HTTPMaxBodyBytes:      1024,
	}
	err = app.Run(context.Background(), app.Options{
		Config:      cfg,
		SkipMigrate: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listen")
}

func TestWaitReadyTimesOut(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := app.WaitReady(ctx, "http://127.0.0.1:1")
	require.Error(t, err)
}

func TestRunLogsAreRedactedNoDSNPassword(t *testing.T) {
	t.Parallel()
	url := testutil.URL(t)
	// inject a sentinel password into the DSN so we can confirm it's scrubbed
	const sentinel = "REDACT-ME-SECRET-99"
	_ = sentinel // DSN password already comes from testutil (not sentinel)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	shutdown := make(chan struct{})
	errCh := make(chan error, 1)
	var logBuf bytes.Buffer

	cfg := &config.Config{
		DatabaseURL:           url,
		Host:                  "127.0.0.1",
		Port:                  ln.Addr().(*net.TCPAddr).Port,
		Env:                   "test",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPReadTimeout:       time.Second,
		HTTPWriteTimeout:      time.Second,
		HTTPIdleTimeout:       time.Second,
		HTTPMaxBodyBytes:      1024,
	}
	go func() {
		errCh <- app.Run(context.Background(), app.Options{
			Config:         cfg,
			Stdout:         &logBuf,
			Listener:       ln,
			SkipMigrate:    true,
			ShutdownSignal: shutdown,
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, "http://"+ln.Addr().String()))

	close(shutdown)
	<-errCh

	logs := logBuf.String()
	// URL from testcontainer has "agents:agents@..." — password must not appear raw.
	assert.NotContains(t, logs, ":agents@")
}

// TestRunServesAuthenticatedAPI exercises the full process with real auth so the
// appServiceAdapter delegation paths are covered.
func TestRunServesAuthenticatedAPI(t *testing.T) {
	t.Parallel()
	dbURL := testutil.URL(t)
	now := time.Now().UTC().Truncate(time.Second)

	key := generateTestKey(t)
	jwks := startJWKSServer(t, key)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	baseURL := "http://" + ln.Addr().String()

	cfg := &config.Config{
		DatabaseURL:           dbURL,
		Host:                  "127.0.0.1",
		Port:                  ln.Addr().(*net.TCPAddr).Port,
		Env:                   "test",
		LogLevel:              "debug",
		ShutdownTimeout:       3 * time.Second,
		HTTPReadHeaderTimeout: 3 * time.Second,
		HTTPReadTimeout:       5 * time.Second,
		HTTPWriteTimeout:      5 * time.Second,
		HTTPIdleTimeout:       10 * time.Second,
		HTTPMaxBodyBytes:      65536,
		IdentityJWKSURL:       jwks.URL + "/jwks",
		IdentityIssuer:        "https://identity.example.test",
	}

	tok := mintTestToken(t, key, now)
	v, err := authn.NewValidator(authn.Options{
		Issuer:  cfg.IdentityIssuer,
		JWKSURL: cfg.IdentityJWKSURL,
		Now:     func() time.Time { return now },
	})
	require.NoError(t, err)

	shutdown := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background(), app.Options{
			Config:         cfg,
			Listener:       ln,
			SkipMigrate:    true,
			ShutdownSignal: shutdown,
			Validator:      v,
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, baseURL))

	client := &http.Client{Timeout: 5 * time.Second}

	// Create a run through the full API (covers CreateRun adapter).
	reqBody := `{"profile":"tutor"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/agents/v1/runs",
		strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "full-process-key-1")
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var runBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&runBody))
	_ = resp.Body.Close()
	runID := runBody["id"].(string)

	// Get the run (covers GetRun adapter).
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/agents/v1/runs/"+runID, nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	_ = resp2.Body.Close()

	// List runs (covers ListRuns adapter).
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/agents/v1/runs", nil)
	req3.Header.Set("Authorization", "Bearer "+tok)
	resp3, err := client.Do(req3)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp3.StatusCode)
	_ = resp3.Body.Close()

	// Create a session (covers CreateSession adapter).
	req4, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/agents/v1/sessions",
		strings.NewReader(`{"profile":"admin"}`))
	req4.Header.Set("Authorization", "Bearer "+tok)
	req4.Header.Set("Content-Type", "application/json")
	resp4, err := client.Do(req4)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp4.StatusCode)
	var sessBody map[string]any
	require.NoError(t, json.NewDecoder(resp4.Body).Decode(&sessBody))
	_ = resp4.Body.Close()
	sessID := sessBody["id"].(string)

	// Get session (covers GetSession adapter).
	req5, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/agents/v1/sessions/"+sessID, nil)
	req5.Header.Set("Authorization", "Bearer "+tok)
	resp5, err := client.Do(req5)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp5.StatusCode)
	_ = resp5.Body.Close()

	// Create job (covers CreateJob adapter).
	req6, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/agents/v1/jobs",
		strings.NewReader(`{"jobType":"test"}`))
	req6.Header.Set("Authorization", "Bearer "+tok)
	req6.Header.Set("Content-Type", "application/json")
	req6.Header.Set("Idempotency-Key", "job-proc-1")
	resp6, err := client.Do(req6)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp6.StatusCode)
	_ = resp6.Body.Close()

	// Student session (covers CreateStudentSession adapter).
	req7, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/agents/v1/student/sessions",
		strings.NewReader(`{}`))
	req7.Header.Set("Authorization", "Bearer "+studentTok(t, key, now))
	req7.Header.Set("Content-Type", "application/json")
	resp7, err := client.Do(req7)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp7.StatusCode)
	_ = resp7.Body.Close()

	// Cancel the run (covers RequestCancel adapter).
	reqCancel, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/agents/v1/runs/"+runID+"/cancel",
		strings.NewReader(`{}`))
	reqCancel.Header.Set("Authorization", "Bearer "+tok)
	reqCancel.Header.Set("Content-Type", "application/json")
	respCancel, err := client.Do(reqCancel)
	require.NoError(t, err)
	_ = respCancel.Body.Close()

	// List events (covers ListEvents adapter).
	reqEvts, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/agents/v1/runs/"+runID+"/events", nil)
	reqEvts.Header.Set("Authorization", "Bearer "+tok)
	respEvts, err := client.Do(reqEvts)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respEvts.StatusCode)
	_ = respEvts.Body.Close()

	// Append a session turn (covers AppendTurn adapter).
	reqTurn, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/agents/v1/sessions/"+sessID+"/turns",
		strings.NewReader(`{"idempotencyKey":"t1","profile":"admin","expectedRevision":1}`))
	reqTurn.Header.Set("Authorization", "Bearer "+tok)
	reqTurn.Header.Set("Content-Type", "application/json")
	respTurn, err := client.Do(reqTurn)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respTurn.StatusCode)
	_ = respTurn.Body.Close()

	// List turns (covers ListTurns adapter).
	reqTurns, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/agents/v1/sessions/"+sessID+"/turns", nil)
	reqTurns.Header.Set("Authorization", "Bearer "+tok)
	respTurns, err := client.Do(reqTurns)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respTurns.StatusCode)
	_ = respTurns.Body.Close()

	// Create schedule (covers CreateSchedule adapter).
	reqSched, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/agents/v1/schedules",
		strings.NewReader(`{"profile":"job","cronExpr":"1h","jobType":"t","timezone":"UTC","maxCatchUp":1}`))
	reqSched.Header.Set("Authorization", "Bearer "+tok)
	reqSched.Header.Set("Content-Type", "application/json")
	respSched, err := client.Do(reqSched)
	require.NoError(t, err)
	var schedBody map[string]any
	require.NoError(t, json.NewDecoder(respSched.Body).Decode(&schedBody))
	_ = respSched.Body.Close()
	schedID := schedBody["id"].(string)

	// List schedules (covers ListSchedules adapter).
	reqScheds, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/agents/v1/schedules", nil)
	reqScheds.Header.Set("Authorization", "Bearer "+tok)
	respScheds, err := client.Do(reqScheds)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respScheds.StatusCode)
	_ = respScheds.Body.Close()

	// Disable schedule (covers SetScheduleEnabled adapter).
	reqDisable, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/agents/v1/schedules/"+schedID+"/disable",
		strings.NewReader(`{}`))
	reqDisable.Header.Set("Authorization", "Bearer "+tok)
	reqDisable.Header.Set("Content-Type", "application/json")
	respDisable, err := client.Do(reqDisable)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respDisable.StatusCode)
	_ = respDisable.Body.Close()

	// Append student turn (covers AppendStudentTurn adapter).
	// Use the same student token throughout so the session + turn share namespace.
	stuToken := studentTok(t, key, now)
	reqStSess, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/agents/v1/student/sessions", strings.NewReader(`{}`))
	reqStSess.Header.Set("Authorization", "Bearer "+stuToken)
	reqStSess.Header.Set("Content-Type", "application/json")
	respStSess, err := client.Do(reqStSess)
	require.NoError(t, err)
	var studentSessBody map[string]any
	require.NoError(t, json.NewDecoder(respStSess.Body).Decode(&studentSessBody))
	_ = respStSess.Body.Close()
	studentSessID := studentSessBody["id"].(string)
	reqStTurn, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/agents/v1/student/sessions/"+studentSessID+"/turns",
		strings.NewReader(`{"idempotencyKey":"st-1","expectedRevision":1}`))
	reqStTurn.Header.Set("Authorization", "Bearer "+stuToken)
	reqStTurn.Header.Set("Content-Type", "application/json")
	respStTurn, err := client.Do(reqStTurn)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respStTurn.StatusCode)
	_ = respStTurn.Body.Close()

	close(shutdown)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// ── test helpers ──────────────────────────────────────────────────────────────

func generateTestKey(t *testing.T) *jwttest.Keypair {
	t.Helper()
	return jwttest.GenerateKey(t)
}

func startJWKSServer(t *testing.T, key *jwttest.Keypair) *httptest.Server {
	t.Helper()
	doc, _ := json.Marshal(jwttest.JWKSDoc(key))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mintTestToken(t *testing.T, key *jwttest.Keypair, now time.Time) string {
	t.Helper()
	sub := "identity:" + uuid.NewString()
	claims := jwttest.ValidHumanClaims(now, sub)
	claims.Scope = jwttest.AllScopes
	return jwttest.Mint(t, key, claims)
}

func studentTok(t *testing.T, key *jwttest.Keypair, now time.Time) string {
	t.Helper()
	sub := "identity:" + uuid.NewString()
	claims := jwttest.ValidHumanClaims(now, sub)
	claims.Scope = authn.ScopeStudentSession
	return jwttest.Mint(t, key, claims)
}
