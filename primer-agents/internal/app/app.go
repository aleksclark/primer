// Package app boots the primer-agents HTTP process: config, migrate, listen,
// and graceful shutdown. cmd/primer-agents is a thin main over Run.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleksclark/primer/agents/internal/api"
	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/config"
	agentsdb "github.com/aleksclark/primer/agents/internal/db"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/logging"
)

// Options customizes process bootstrap for tests.
type Options struct {
	// Config, when set, skips config.Load().
	Config *config.Config
	// Stdout is the log destination (defaults to os.Stdout).
	Stdout io.Writer
	// Listener allows tests to inject a bound listener.
	Listener net.Listener
	// SkipMigrate skips goose up (tests that manage schema themselves).
	SkipMigrate bool
	// ShutdownSignal replaces OS signals in tests.
	ShutdownSignal <-chan struct{}
	// Validator overrides the JWT validator (tests inject a loopback JWKS).
	Validator api.TokenValidator
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

	if !opts.SkipMigrate {
		if err := agentsdb.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	pool, err := agentsdb.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	svc := appservice.New(pool)
	svcAdapter := &appServiceAdapter{svc: svc}

	// Build the JWT validator if Identity is configured. Production requires
	// JWKS/issuer; development/test may omit them (all /agents/v1 routes return
	// 401 but healthz/readyz remain available).
	var validator api.TokenValidator
	if opts.Validator != nil {
		validator = opts.Validator
	} else if cfg.AuthEnabled() {
		validator, err = authn.NewValidator(authn.Options{
			Issuer:  cfg.IdentityIssuer,
			JWKSURL: cfg.IdentityJWKSURL,
		})
		if err != nil {
			return fmt.Errorf("configure authn validator: %w", err)
		}
	}

	handler := api.New(api.Options{
		Pool:      pool,
		Validator: validator,
		Service:   svcAdapter,
		Env:       cfg.Env,
		Logger:    logger,
	})
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
	logger.Info("listening", "addr", addr, "env", cfg.Env, "service", "primer-agents",
		"auth_enabled", validator != nil)

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

// WaitReady polls addr until /readyz returns 200 or the context expires.
func WaitReady(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/readyz", nil)
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

// appServiceAdapter adapts *appservice.Service to api.AgentService by
// converting between the parallel command types of the two packages.
type appServiceAdapter struct{ svc *appservice.Service }

func (a *appServiceAdapter) CreateRun(ctx context.Context, cmd api.CreateRunCmd) (*domain.Run, error) {
	return a.svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: cmd.OwnerNamespace,
		IdempotencyKey: cmd.IdempotencyKey,
		Profile:        cmd.Profile,
		InputContent:   cmd.InputContent,
		InputPreview:   cmd.InputPreview,
		SessionID:      cmd.SessionID,
	})
}
func (a *appServiceAdapter) GetRun(ctx context.Context, id, ns string) (*domain.Run, error) {
	return a.svc.GetRun(ctx, id, ns)
}
func (a *appServiceAdapter) ListRuns(ctx context.Context, ns string, limit int) ([]*domain.Run, error) {
	return a.svc.ListRuns(ctx, ns, limit)
}
func (a *appServiceAdapter) RequestCancel(ctx context.Context, id, ns string, rc *string) (*domain.Run, error) {
	return a.svc.RequestCancel(ctx, id, ns, rc)
}
func (a *appServiceAdapter) ListEvents(ctx context.Context, runID, ns string, afterSeq int64, limit int) ([]*domain.RunEvent, error) {
	return a.svc.ListEvents(ctx, runID, ns, afterSeq, limit)
}
func (a *appServiceAdapter) CreateSession(ctx context.Context, cmd api.CreateSessionCmd) (*domain.Session, error) {
	return a.svc.CreateSession(ctx, appservice.CreateSessionCmd{
		OwnerNamespace: cmd.OwnerNamespace,
		Profile:        cmd.Profile,
		CallerContext:  cmd.CallerContext,
		ExpiresAt:      cmd.ExpiresAt,
	})
}
func (a *appServiceAdapter) GetSession(ctx context.Context, id, ns string) (*domain.Session, error) {
	return a.svc.GetSession(ctx, id, ns)
}
