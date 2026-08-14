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

	"github.com/aleksclark/primer/curriculum-studio/internal/app"
	"github.com/aleksclark/primer/curriculum-studio/internal/config"
	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestProcessHealthReadyAndShutdown(t *testing.T) {
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
		Port:                  0,
		Env:                   "test",
		LogLevel:              "info",
		AuthMode:              "jwks",
		ShutdownTimeout:       5 * time.Second,
		HTTPReadHeaderTimeout: 5 * time.Second,
		HTTPReadTimeout:       10 * time.Second,
		HTTPWriteTimeout:      10 * time.Second,
		HTTPIdleTimeout:       15 * time.Second,
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
	hresp, err := http.Get(baseURL + "/studio/v1/health")
	require.NoError(t, err)
	defer hresp.Body.Close()
	assert.Equal(t, http.StatusOK, hresp.StatusCode)
	var hbody map[string]any
	require.NoError(t, json.NewDecoder(hresp.Body).Decode(&hbody))
	assert.Equal(t, "ok", hbody["status"])

	rresp, err := http.Get(baseURL + "/studio/v1/ready")
	require.NoError(t, err)
	defer rresp.Body.Close()
	assert.Equal(t, http.StatusOK, rresp.StatusCode)

	// Goose version ≥ 4 via live DB (D1 migrations 00001–00004)
	pool := testutil.DB(t)
	var n int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM `+studiodb.VersionTable+` WHERE version_id > 0 AND is_applied`).Scan(&n)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 4)

	ver, err := studiodb.Studio.CurrentVersion(context.Background(), url)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ver, int64(4))

	// P1-S6: structured logs without secrets
	logs := logBuf.String()
	assert.Contains(t, logs, `"msg"`)
	assert.NotContains(t, logs, "primer:primer")
	// Harness user/pass may be "studio" which also appears in /studio/v1 paths;
	// assert the credential pair and full DSN do not leak.
	assert.NotContains(t, logs, "studio:studio@")
	assert.NotContains(t, logs, url)
	if u := extractPassword(url); u != "" && u != "studio" && len(u) >= 8 {
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
	url := testutil.URL(t)
	ctx := context.Background()
	require.NoError(t, studiodb.Migrate(ctx, url))
	require.NoError(t, studiodb.Migrate(ctx, url))

	pool := testutil.DB(t)
	var reg *string
	err := pool.QueryRow(ctx, `SELECT to_regclass('public.`+studiodb.VersionTable+`')::text`).Scan(&reg)
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, studiodb.VersionTable, *reg)

	// Shared LMS goose table name must not be used.
	var lmsGoose bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='goose_db_version'
		)`).Scan(&lmsGoose)
	require.NoError(t, err)
	assert.False(t, lmsGoose)

	// Studio schema objects present (anti-cheat vs empty migrate).
	var tenants bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='curriculum_studio' AND table_name='tenants'
		)`).Scan(&tenants)
	require.NoError(t, err)
	assert.True(t, tenants)
}

func TestBinaryFailFastBadConfig(t *testing.T) {
	bin := buildStudioServer(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"STUDIO_ENV=production",
		"STUDIO_DATABASE_URL= ",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "expected non-zero exit: %s", out)
	if ee, ok := err.(*exec.ExitError); ok {
		assert.NotEqual(t, 0, ee.ExitCode())
	}
	assert.NotContains(t, string(out), "listening")
}

// Process regression (P1-S2/E3): production with STUDIO_DATABASE_URL truly
// unset must exit nonzero at config validation — before migrate/listen — with
// no localhost default and no bare DATABASE_URL inheritance.
func TestBinaryFailFastProductionUnsetDatabaseURL(t *testing.T) {
	bin := buildStudioServer(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())

	cmd := exec.Command(bin)
	cmd.Env = scrubEnv(os.Environ(),
		"STUDIO_DATABASE_URL",
		"DATABASE_URL",
		"TEST_DATABASE_URL",
		"STUDIO_TEST_DATABASE_URL",
	)
	cmd.Env = append(cmd.Env,
		"STUDIO_ENV=production",
		"STUDIO_HOST=127.0.0.1",
		fmt.Sprintf("STUDIO_PORT=%d", port),
		"STUDIO_LOG_LEVEL=info",
		// Adversarial ambient LMS DSN must not satisfy Studio.
		"DATABASE_URL=postgres://lms:x@127.0.0.1:5432/primer?sslmode=disable",
	)

	out, err := cmd.CombinedOutput()
	require.Error(t, err, "expected non-zero exit when STUDIO_DATABASE_URL unset: %s", out)
	if ee, ok := err.(*exec.ExitError); ok {
		assert.NotEqual(t, 0, ee.ExitCode())
	}
	combined := string(out)
	lower := strings.ToLower(combined)
	assert.NotContains(t, combined, "listening")
	assert.NotContains(t, lower, "migrate:")
	assert.NotContains(t, lower, "password authentication failed")
	assert.Contains(t, lower, "database url is required")

	conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatalf("process listened on %d despite missing STUDIO_DATABASE_URL; output=%s", port, combined)
	}
}

func TestBinaryFailFastForeignDSN(t *testing.T) {
	bin := buildStudioServer(t)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())

	cmd := exec.Command(bin)
	cmd.Env = scrubEnv(os.Environ(), "STUDIO_DATABASE_URL", "DATABASE_URL")
	cmd.Env = append(cmd.Env,
		"STUDIO_ENV=production",
		"STUDIO_HOST=127.0.0.1",
		fmt.Sprintf("STUDIO_PORT=%d", port),
		"STUDIO_DATABASE_URL=postgres://u:x@127.0.0.1:5432/primer?sslmode=disable",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "expected non-zero exit for foreign DSN: %s", out)
	assert.NotContains(t, string(out), "listening")
	assert.Contains(t, strings.ToLower(string(out)), "forbidden")

	conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatalf("process listened despite foreign DSN")
	}
}

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
	url := testutil.URL(t)
	bin := buildStudioServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"STUDIO_ENV=test",
		"STUDIO_HOST=127.0.0.1",
		fmt.Sprintf("STUDIO_PORT=%d", port),
		"STUDIO_DATABASE_URL="+url,
		"STUDIO_SHUTDOWN_TIMEOUT=5s",
		"STUDIO_LOG_LEVEL=info",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	require.NoError(t, cmd.Start())

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, baseURL))

	resp, err := http.Get(baseURL + "/studio/v1/health")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "ok")

	// ready with real DB
	rresp, err := http.Get(baseURL + "/studio/v1/ready")
	require.NoError(t, err)
	_ = rresp.Body.Close()
	assert.Equal(t, http.StatusOK, rresp.StatusCode)

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
	out := buf.String()
	assert.NotContains(t, out, "super-secret")
	assert.NotContains(t, out, "studio:studio@")
	assert.NotContains(t, out, url)
	if pw := extractPassword(url); pw != "" && pw != "studio" && len(pw) >= 8 {
		assert.NotContains(t, out, pw)
	}
	assert.Contains(t, out, "shutdown complete")
}

func TestHarnessUsesStudioDatabase(t *testing.T) {
	t.Parallel()
	url := testutil.URL(t)
	name, err := studiodb.ParseDatabaseName(url)
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(name), "studio")
	assert.False(t, studiodb.IsForbiddenDBName(name))
}

func buildStudioServer(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// internal/testutil/e2e -> module root
	modRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
	out := filepath.Join(t.TempDir(), "studio-server")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/studio-server")
	cmd.Dir = modRoot
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build studio-server: %s", output)
	return out
}

func extractPassword(dsn string) string {
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
