package app_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/app"
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
