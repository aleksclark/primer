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

	"github.com/go-chi/chi/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/artifacts"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/config"
	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
	"github.com/aleksclark/primer/curriculum-studio/internal/logging"
	studiomcp "github.com/aleksclark/primer/curriculum-studio/internal/mcp"
	"github.com/aleksclark/primer/curriculum-studio/internal/outbox"
	"github.com/aleksclark/primer/curriculum-studio/internal/workflow"
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
	// Keep API-created webhook secrets available to the in-process worker. This
	// credential-free default is intentionally process-local; production must
	// replace it with a durable secret manager keyed by secret_ref.
	secrets := outbox.NewMemorySecrets()
	var store artifacts.Store
	switch cfg.ArtifactStore {
	case "fs":
		fs, err := artifacts.NewFsStore(cfg.ArtifactStoreDir)
		if err != nil {
			return fmt.Errorf("configure artifact store: %w", err)
		}
		defer fs.Close()
		store = fs
	case "s3":
		store, err = artifacts.NewS3Store(artifacts.S3Options{Endpoint: cfg.ArtifactS3Endpoint, Bucket: cfg.ArtifactS3Bucket, Region: cfg.ArtifactS3Region, AccessKey: cfg.ArtifactS3AccessKey, SecretKey: cfg.ArtifactS3SecretKey})
		if err != nil {
			return fmt.Errorf("configure artifact store: %w", err)
		}
	}
	_, apiHandler := api.New(pool, api.Options{
		Validator:               validator,
		AcceptServiceTokenAlias: cfg.AcceptServiceTokenAlias,
		MatStub:                 cfg.MatStub,
		Secrets:                 secrets,
		Artifacts:               store,
	})

	// Mount /mcp Streamable HTTP endpoint when enabled.
	// The MCP handler is not wrapped by MaxBytesHandler because it applies its
	// own body limit (MCPMaxBodyBytes → SDK MaxRequestBodyBytes).
	router := chi.NewMux()
	if cfg.MCPEnabled {
		mcpHandler := studiomcp.New(studiomcp.Options{
			Config: studiomcp.MCPConfig{
				Enabled:         cfg.MCPEnabled,
				OriginAllowlist: cfg.MCPOriginAllowlist,
				MaxBodyBytes:    cfg.MCPMaxBodyBytes,
				RequestTimeout:  cfg.MCPRequestTimeout,
			},
			Services:  studiomcp.NewServicesFromQuerier(pool),
			Validator: validator,
			Querier:   pool,
		})
		router.Mount("/mcp", mcpHandler)
		logger.Info("mcp endpoint registered", "path", "/mcp")
	}
	// Mount all other studio routes; MaxBytesHandler wraps the REST/grpc surface.
	router.Mount("/", http.MaxBytesHandler(apiHandler, cfg.HTTPMaxBodyBytes))

	// The REST subtree is already bounded above. Do not wrap the whole router
	// again: doing so would silently apply the REST 1 MiB limit to /mcp and
	// defeat MCPMaxBodyBytes.
	handler := http.Handler(router)

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
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

	worker, err := outbox.NewWorker(pool, outbox.Config{
		Owner:           "studio-server-" + addr,
		PollInterval:    250 * time.Millisecond,
		DeliveryTimeout: 5 * time.Second,
		LeaseTTL:        30 * time.Second,
		Secrets:         secrets,
		Logger:          logger,
	})
	if err != nil {
		return fmt.Errorf("outbox worker: %w", err)
	}
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	// The S12 runner is deliberately provider-free. The legacy stub remains a
	// test-only API escape hatch and is never run alongside the real worker.
	if !cfg.MatStub {
		model, err := workflow.NewModel(cfg.ModelProvider)
		if err != nil {
			return err
		}
		go workflow.NewRunner(pool, model).Run(workerCtx)
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if werr := worker.Run(workerCtx); werr != nil && !errors.Is(werr, context.Canceled) {
			logger.Error("outbox worker stopped", "error", werr)
		}
	}()

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
	stopWorker()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	stopWorker()
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
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
