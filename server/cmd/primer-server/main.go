// Command primer-server runs the Primer LMS HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/config"
	"github.com/aleksclark/primer/server/internal/db"
	"github.com/aleksclark/primer/server/internal/spa"
	"github.com/aleksclark/primer/server/internal/tutor"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.ServiceToken == "" {
		// Worth shouting about: without a token anyone who can reach the port
		// can inflate the student's instructional hours.
		slog.Warn("service token is not set; the instruction log ingest is unauthenticated")
	}

	tutorCfg := tutor.DefaultConfig()
	tutorCfg.Provider = cfg.TutorProvider
	tutorCfg.Enabled = cfg.TutorEnabled
	tutorCfg.Bedrock = tutor.BedrockConfig{
		URL:    cfg.TutorBedrockURL,
		APIKey: cfg.TutorBedrockAPIKey,
		Model:  cfg.TutorBedrockModel,
	}
	tutorSvc, err := tutor.NewFromConfig(tutorCfg)
	if err != nil {
		return fmt.Errorf("tutor: %w", err)
	}
	slog.Info("tutor configured", "provider", tutorSvc.ProviderName(), "enabled", tutorSvc.Enabled())

	var artStore *artifacts.Store
	if cfg.ArtifactStoreDir != "" {
		s, err := artifacts.NewStore(cfg.ArtifactStoreDir)
		if err != nil {
			return fmt.Errorf("artifact store: %w", err)
		}
		artStore = s
		slog.Info("artifact store configured", "root", s.Root)
	} else {
		slog.Warn("ARTIFACT_STORE_DIR is not set; artifact byte upload is disabled")
	}

	_, handler := api.New(pool, api.Options{
		CORSOrigins:       cfg.CORSOrigins,
		ServiceToken:      cfg.ServiceToken,
		Tutor:             tutorSvc,
		TutorProviderName: tutorSvc.ProviderName(),
		TutorEnabled:      tutorSvc.Enabled(),
		ArtifactStore:     artStore,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", handler))
	mux.Handle("/", spa.Handler())

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
