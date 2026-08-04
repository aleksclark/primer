// Command tv-server runs the Primer TV channel HTTP API: catalog,
// availability, watch-once ledger, play grants, and playback sessions.
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

	basedb "github.com/aleksclark/primer/server/internal/db"
	"github.com/aleksclark/primer/server/internal/tv/api"
	"github.com/aleksclark/primer/server/internal/tv/config"
	tvdb "github.com/aleksclark/primer/server/internal/tv/db"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/aleksclark/primer/server/internal/tv/primer"
	"github.com/aleksclark/primer/server/internal/tv/spa"
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

	if err := tvdb.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	pool, err := basedb.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	media, err := jellyfin.New(jellyfin.Options{
		BaseURL: cfg.JellyfinBaseURL,
		APIKey:  cfg.JellyfinAPIKey,
		UserID:  cfg.JellyfinUserID,
	})
	if err != nil {
		return err
	}
	if err := media.Ping(ctx); err != nil {
		// A cold Jellyfin should not stop the TV server from booting: the admin
		// UI needs to come up so the operator can see and fix the problem.
		slog.Warn("jellyfin unreachable at startup", "error", err)
	}

	if cfg.AdminAPIKey == "" {
		// Worth shouting about: without a key anyone who can reach the port can
		// read pairing codes and pair their own device.
		slog.Warn("admin api key is not set; the admin API is unauthenticated")
	}

	reporter := primerReporter(cfg)

	_, handler := api.New(pool, api.Options{
		CORSOrigins:             cfg.CORSOrigins,
		Jellyfin:                media,
		AdminKey:                cfg.AdminAPIKey,
		GrantTTL:                cfg.GrantTTL,
		PairingTTL:              cfg.PairingTTL,
		ChannelTimezone:         cfg.ChannelTimezone,
		Primer:                  reporter,
		ReleaseDir:              cfg.ReleaseDir,
		ManifestFailMaxAttempts: cfg.ManifestFailMaxAttempts,
		ManifestFailMaxDays:     cfg.ManifestFailMaxDays,
	})

	if reporter != nil {
		go reporter.Run(ctx, pool, cfg.PrimerReportInterval)
	}

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

// primerReporter builds the instructional-hours reporter, or nil when no LMS
// is configured. Reporting off is a supported state, not an error: the channel
// works on its own, so an unconfigured LMS gets a warning and silence rather
// than a failed boot or a loop failing every five minutes.
func primerReporter(cfg *config.Config) *primer.Reporter {
	client, err := primer.New(primer.Options{
		BaseURL:      cfg.PrimerBaseURL,
		ServiceToken: cfg.PrimerServiceToken,
		Timeout:      cfg.PrimerTimeout,
	})
	if errors.Is(err, primer.ErrNotConfigured) {
		slog.Warn("primer base url is not set; watched sessions will not be reported as instructional time")
		return nil
	}
	if err != nil {
		slog.Warn("primer client is misconfigured; reporting is disabled", "error", err)
		return nil
	}
	if cfg.PrimerServiceToken == "" {
		slog.Warn("primer service token is not set; the LMS will reject the ingest unless it too is unconfigured")
	}
	return &primer.Reporter{
		Ingest:    client,
		Location:  api.ChannelLocation(cfg.ChannelTimezone),
		BatchSize: cfg.PrimerReportBatchSize,
	}
}
