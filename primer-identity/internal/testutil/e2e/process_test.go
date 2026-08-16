package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/app"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestProcessHealthReadyAndShutdown(t *testing.T) {
	t.Parallel()
	url := testutil.DatabaseURL(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	baseURL := "http://" + ln.Addr().String()

	var logBuf bytes.Buffer
	shutdown := make(chan struct{})
	errCh := make(chan error, 1)

	cfg := &config.Config{
		DatabaseURL:           url,
		Host:                  "127.0.0.1",
		Port:                  0,
		Env:                   "test",
		LogLevel:              "info",
		Issuer:                "http://localhost:8090",
		ShutdownTimeout:       5 * time.Second,
		HTTPReadHeaderTimeout: 5 * time.Second,
		HTTPMaxBodyBytes:      1 << 20,
	}

	go func() {
		errCh <- app.Run(context.Background(), app.Options{
			Config:         cfg,
			Stdout:         &logBuf,
			Listener:       ln,
			ShutdownSignal: shutdown,
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, baseURL))

	// P1-E1: health + ready
	hresp, err := http.Get(baseURL + "/healthz")
	require.NoError(t, err)
	defer hresp.Body.Close()
	assert.Equal(t, http.StatusOK, hresp.StatusCode)
	var hbody map[string]any
	require.NoError(t, json.NewDecoder(hresp.Body).Decode(&hbody))
	assert.Equal(t, "ok", hbody["status"])

	rresp, err := http.Get(baseURL + "/readyz")
	require.NoError(t, err)
	defer rresp.Body.Close()
	assert.Equal(t, http.StatusOK, rresp.StatusCode)

	// Goose version ≥ 1 via live DB
	pool := testutil.DB(t)
	var n int
	err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_goose_db_version`).Scan(&n)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)

	// P1-S6: structured logs without secrets
	logs := logBuf.String()
	assert.Contains(t, logs, `"msg"`)
	// password from harness DSN must never appear
	assert.NotContains(t, logs, "primer:primer")
	if u := extractPassword(url); u != "" {
		assert.NotContains(t, logs, u)
	}

	// P1-E2: graceful shutdown
	close(shutdown)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("process did not shut down within timeout")
	}
}

func TestMigrateTwiceIsolated(t *testing.T) {
	t.Parallel()
	url := testutil.DatabaseURL(t)
	ctx := context.Background()
	require.NoError(t, db.Migrate(ctx, url))
	require.NoError(t, db.Migrate(ctx, url))

	pool := testutil.DB(t)
	var reg *string
	err := pool.QueryRow(ctx, `SELECT to_regclass('public.identity_goose_db_version')::text`).Scan(&reg)
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, "identity_goose_db_version", *reg)

	// Shared LMS goose table name must not be used.
	var lmsGoose bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='goose_db_version'
		)`).Scan(&lmsGoose)
	require.NoError(t, err)
	assert.False(t, lmsGoose)
}

func TestBinaryFailFastBadConfig(t *testing.T) {
	// Build identity-server and run with production env missing DB URL.
	bin := buildIdentityServer(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"IDENTITY_ENV=production",
		"IDENTITY_DATABASE_URL=",
		"IDENTITY_ISSUER=",
		// Force empty after defaults by using a space that Validate trims.
		"IDENTITY_DATABASE_URL= ",
		"IDENTITY_ISSUER= ",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "expected non-zero exit: %s", out)
	if ee, ok := err.(*exec.ExitError); ok {
		assert.NotEqual(t, 0, ee.ExitCode())
	}
	assert.NotContains(t, string(out), "listening")
}

// Process regression (P1-S2/E4): production with IDENTITY_DATABASE_URL truly
// unset must exit nonzero at config validation — before migrate/listen — with
// no localhost default and no bare DATABASE_URL inheritance.
func TestBinaryFailFastProductionUnsetDatabaseURL(t *testing.T) {
	bin := buildIdentityServer(t)

	// Bind a free port only to prove the child never accepts on it.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())

	cmd := exec.Command(bin)
	cmd.Env = scrubEnv(os.Environ(),
		"IDENTITY_DATABASE_URL",
		"DATABASE_URL",
		"IDENTITY_ISSUER",
		"ISSUER",
	)
	cmd.Env = append(cmd.Env,
		"IDENTITY_ENV=production",
		"IDENTITY_ISSUER=https://id.example",
		"IDENTITY_HOST=127.0.0.1",
		fmt.Sprintf("IDENTITY_PORT=%d", port),
		"IDENTITY_LOG_LEVEL=info",
	)
	// IDENTITY_DATABASE_URL intentionally absent (not empty string).

	out, err := cmd.CombinedOutput()
	require.Error(t, err, "expected non-zero exit when DATABASE_URL unset: %s", out)
	if ee, ok := err.(*exec.ExitError); ok {
		assert.NotEqual(t, 0, ee.ExitCode())
	}
	combined := string(out)
	lower := strings.ToLower(combined)
	assert.NotContains(t, combined, "listening")
	// Must fail at config validation, not later at migrate/connect.
	assert.NotContains(t, lower, "migrate:")
	assert.NotContains(t, lower, "password authentication failed")
	assert.Contains(t, lower, "database url is required")

	// Never listened: connect must fail immediately.
	conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatalf("process listened on %d despite missing DATABASE_URL; output=%s", port, combined)
	}
}

// Process regression (IA-R): production with only hostile bare SECRET/ISSUER
// (and sibling unprefixed names) must fail at config validation before
// migrate/listen. Prefixed IDENTITY_ISSUER / IDENTITY_STYTCH_SECRET are absent.
func TestBinaryFailFastProductionHostileBareSecretAndIssuer(t *testing.T) {
	bin := buildIdentityServer(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())

	const hostileSecret = "hostile-bare-secret-must-not-be-used"
	cmd := exec.Command(bin)
	cmd.Env = scrubEnv(os.Environ(),
		"IDENTITY_DATABASE_URL",
		"IDENTITY_ISSUER",
		"IDENTITY_STYTCH_SECRET",
		"IDENTITY_STYTCH_PROJECT_ID",
		"IDENTITY_STYTCH_ENABLED",
		"IDENTITY_STYTCH_ENV",
		"IDENTITY_STYTCH_BASE_URI",
		"IDENTITY_HOST",
		"IDENTITY_PORT",
		"IDENTITY_ENV",
		"DATABASE_URL",
		"ISSUER",
		"SECRET",
		"PROJECT_ID",
		"ENABLED",
		"ENV",
		"BASE_URI",
		"HOST",
		"PORT",
		"LOG_LEVEL",
		"REQUEST_TIMEOUT",
		"POSITIVE_CACHE_TTL",
		"NEGATIVE_CACHE_TTL",
		"POSITIVE_CACHE_CAPACITY",
		"NEGATIVE_CACHE_CAPACITY",
		"SHUTDOWN_TIMEOUT",
		"HTTP_READ_HEADER_TIMEOUT",
		"HTTP_MAX_BODY_BYTES",
	)
	cmd.Env = append(cmd.Env,
		"IDENTITY_ENV=production",
		"IDENTITY_HOST=127.0.0.1",
		fmt.Sprintf("IDENTITY_PORT=%d", port),
		"IDENTITY_LOG_LEVEL=info",
		"IDENTITY_DATABASE_URL=postgres://identity:hostile-db-pass-must-not-leak@127.0.0.1:1/primer_identity?sslmode=disable",
		"IDENTITY_STYTCH_ENABLED=true",
		"IDENTITY_STYTCH_ENV=live",
		"IDENTITY_STYTCH_PROJECT_ID=project-live-example",
		// IDENTITY_ISSUER and IDENTITY_STYTCH_SECRET intentionally absent.
		"SECRET="+hostileSecret,
		"PROJECT_ID=project-live-hostile-bare",
		"ISSUER=https://hostile-bare.example",
		"ENABLED=true",
		"ENV=production",
		"BASE_URI=https://hostile-bare.stytch.example",
		"HOST=127.0.0.1",
		fmt.Sprintf("PORT=%d", port),
		"LOG_LEVEL=error",
		"REQUEST_TIMEOUT=4s",
		"POSITIVE_CACHE_TTL=16s",
		"NEGATIVE_CACHE_TTL=6s",
		"POSITIVE_CACHE_CAPACITY=10001",
		"NEGATIVE_CACHE_CAPACITY=2001",
	)

	out, err := cmd.CombinedOutput()
	require.Error(t, err, "expected non-zero exit when prefixed issuer/secret are missing: %s", out)
	if ee, ok := err.(*exec.ExitError); ok {
		assert.NotEqual(t, 0, ee.ExitCode())
	}
	combined := string(out)
	lower := strings.ToLower(combined)
	assert.NotContains(t, combined, "listening")
	assert.NotContains(t, combined, hostileSecret)
	assert.NotContains(t, combined, "hostile-db-pass-must-not-leak")
	assert.NotContains(t, lower, "migrate:")
	assert.NotContains(t, lower, "password authentication failed")
	assert.True(t,
		strings.Contains(lower, "issuer is required") || strings.Contains(lower, "credential"),
		"expected issuer or credential fail-closed, got %s", combined,
	)

	conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatalf("process listened on %d despite missing prefixed issuer/secret; output=%s", port, combined)
	}
}

// scrubEnv returns env without keys in drop (case-sensitive KEY= prefix match).
func scrubEnv(environ []string, drop ...string) []string {
	deny := make(map[string]struct{}, len(drop))
	for _, k := range drop {
		deny[k] = struct{}{}
	}
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		key, _, _ := strings.Cut(e, "=")
		if _, bad := deny[key]; bad {
			continue
		}
		out = append(out, e)
	}
	return out
}

func TestBinaryGracefulSIGTERM(t *testing.T) {
	url := testutil.DatabaseURL(t)
	bin := buildIdentityServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"IDENTITY_ENV=test",
		"IDENTITY_HOST=127.0.0.1",
		fmt.Sprintf("IDENTITY_PORT=%d", port),
		"IDENTITY_DATABASE_URL="+url,
		"IDENTITY_ISSUER=http://127.0.0.1:"+fmt.Sprint(port),
		"IDENTITY_SHUTDOWN_TIMEOUT=5s",
		"IDENTITY_LOG_LEVEL=info",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	require.NoError(t, cmd.Start())

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, baseURL))

	// health
	resp, err := http.Get(baseURL + "/healthz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "ok")

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("binary did not exit after SIGTERM")
	}
	assert.NotContains(t, buf.String(), "super-secret")
}

func buildIdentityServer(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// internal/testutil/e2e -> module root
	modRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
	out := filepath.Join(t.TempDir(), "identity-server")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/identity-server")
	cmd.Dir = modRoot
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build identity-server: %s", output)
	return out
}

func extractPassword(dsn string) string {
	// postgres://user:pass@host/db
	i := strings.Index(dsn, "://")
	if i < 0 {
		return ""
	}
	rest := dsn[i+3:]
	at := strings.Index(rest, "@")
	if at < 0 {
		return ""
	}
	userinfo := rest[:at]
	if _, pass, ok := strings.Cut(userinfo, ":"); ok {
		return pass
	}
	return ""
}
