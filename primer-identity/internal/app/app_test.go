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

	"github.com/aleksclark/primer/identity/internal/app"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestRunLoadsConfigAndServes(t *testing.T) {
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
		Port:                  ln.Addr().(*net.TCPAddr).Port,
		Env:                   "test",
		LogLevel:              "debug",
		Issuer:                "http://id.test",
		ShutdownTimeout:       3 * time.Second,
		HTTPReadHeaderTimeout: 3 * time.Second,
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

	close(shutdown)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run exit")
	}
	assert.Contains(t, logBuf.String(), "listening")
	assert.Contains(t, logBuf.String(), "shutdown complete")
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DatabaseURL:           "postgres://u:p@localhost:5432/primer", // forbidden
		Host:                  "127.0.0.1",
		Port:                  8090,
		Env:                   "production",
		LogLevel:              "info",
		Issuer:                "https://id.example",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1024,
	}
	err := app.Run(context.Background(), app.Options{Config: cfg})
	require.Error(t, err)
}

func TestRunFailsOnBadDSN(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DatabaseURL:           "postgres://primer:primer@127.0.0.1:1/primer_identity?sslmode=disable&connect_timeout=1",
		Host:                  "127.0.0.1",
		Port:                  0,
		Env:                   "test",
		LogLevel:              "info",
		Issuer:                "http://id.test",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1024,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := app.Run(ctx, app.Options{Config: cfg, SkipMigrate: true})
	require.Error(t, err)
}

func TestWaitReadyTimesOut(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := app.WaitReady(ctx, "http://127.0.0.1:1")
	require.Error(t, err)
}
