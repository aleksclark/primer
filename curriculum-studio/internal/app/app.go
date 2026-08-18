// Package app boots the Curriculum Studio HTTP process: config, migrate, listen,
// and graceful shutdown. cmd/studio-server is a thin main over Run.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/config"
	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
	"github.com/aleksclark/primer/curriculum-studio/internal/logging"
)

// Options customizes process bootstrap for tests.
type Options struct {
	// Config, when set, skips config.Load().
	Config *config.Config
	// Stdout is the log destination (defaults to os.Stdout).
	Stdout io.Writer
	// Listener allows tests to inject a bound listener.
	// When nil, the server listens on cfg.Addr().
	Listener net.Listener
	// SkipMigrate skips goose up (tests that manage schema themselves).
	SkipMigrate bool
	// ShutdownSignal, when set, is used instead of OS signals.
	ShutdownSignal <-chan struct{}
}

// Result is returned after a successful Run that has shut down.
type Result struct {
	Addr string
}

// Run loads config (unless provided), migrates, serves HTTP, and shuts down
// gracefully on SIGINT/SIGTERM or Options.ShutdownSignal.
func Run(ctx context.Context, opts Options) error {
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}

	cfg := opts.Config
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}
	} else if err := cfg.Validate(); err != nil {
		return err
	}

	logger := logging.NewJSONLogger(out, cfg.LogLevel)
	slog.SetDefault(logger)

	if !opts.SkipMigrate {
		if err := studiodb.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	pool, err := studiodb.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var validator *authn.Validator
	if cfg.JWKSURL != "" && cfg.Issuer != "" {
		validator, err = authn.NewValidator(authn.Options{
			Issuer:   cfg.Issuer,
			Audience: cfg.Audience,
			JWKSURL:  cfg.JWKSURL,
		})
		if err != nil {
			return fmt.Errorf("configure auth validator: %w", err)
		}
	}
	_, handler := api.New(pool, api.Options{Validator: validator})

	// Bound body size for all routes (health is tiny; future writes stay capped).
	bounded := http.MaxBytesHandler(handler, cfg.HTTPMaxBodyBytes)

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           bounded,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	ln := opts.Listener
	if ln == nil {
		ln, err = net.Listen("tcp", cfg.Addr())
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	addr := ln.Addr().String()
	logger.Info("listening", "addr", addr, "env", cfg.Env)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCtx := ctx
	stop := func() {}
	if opts.ShutdownSignal == nil {
		sigCtx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	}
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-sigCtx.Done():
	case <-opts.ShutdownSignal:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("shutdown complete", "addr", addr)
	return nil
}

// WaitReady polls baseURL until /studio/v1/ready returns 200 or timeout.
func WaitReady(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/studio/v1/ready", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
