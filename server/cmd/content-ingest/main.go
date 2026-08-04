// Command content-ingest converges Radarr/Sonarr/yt-dlp, Jellyfin, and the TV
// server toward a curated content manifest. See agent_docs/plans/content-ingest.md.
//
//	content-ingest plan    # show the diff (no mutations except review.yaml)
//	content-ingest review  # interactive TUI to pick candidates in review.yaml
//	content-ingest apply   # resolve, acquire, sync, import, report
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aleksclark/primer/server/internal/ingest/config"
	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/radarr"
	"github.com/aleksclark/primer/server/internal/ingest/reconcile"
	"github.com/aleksclark/primer/server/internal/ingest/reviewtui"
	"github.com/aleksclark/primer/server/internal/ingest/sonarr"
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: content-ingest <plan|review|apply>")
	}
	cmd := os.Args[1]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, `content-ingest — converge the media stack toward curriculum/content-manifest.yaml

Commands:
  plan    Show what would change (writes review.yaml candidates + a report)
  review  Interactive TUI: pick the right candidate for each review.yaml entry
  apply   Resolve, acquire, sync, import, and write a report

Typical loop:
  make ingest-plan
  make ingest-review    # answer ambiguous matches
  make ingest-apply

Environment: INGEST_* (see internal/ingest/config). Key vars:
  INGEST_MANIFEST_PATH   default curriculum/content-manifest.yaml
  INGEST_REVIEW_PATH     default curriculum/content-review.yaml
  INGEST_REPORT_DIR      default curriculum/ingest-reports
  INGEST_RADARR_BASE_URL / INGEST_RADARR_API_KEY / INGEST_RADARR_ROOT_FOLDER
  INGEST_RADARR_QUALITY_PROFILE_ID
  INGEST_SONARR_BASE_URL / INGEST_SONARR_API_KEY / INGEST_SONARR_ROOT_FOLDER
  INGEST_SONARR_QUALITY_PROFILE_ID
  INGEST_JELLYFIN_BASE_URL / INGEST_JELLYFIN_API_KEY / INGEST_JELLYFIN_USER_ID
  INGEST_TV_BASE_URL     e.g. http://localhost:8081/api/v1
  INGEST_TV_ADMIN_KEY
  INGEST_YTDLP_OUTPUT_DIR / INGEST_YTDLP_ARCHIVE_PATH / INGEST_YTDLP_PATH
`)
		return nil
	case "review":
		return runReview()
	case "plan", "apply":
		return runReconcile(cmd == "plan")
	default:
		return fmt.Errorf("unknown command %q (want plan, review, or apply)", cmd)
	}
}

func runReview() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	review, err := manifest.LoadReview(cfg.ReviewPath)
	if err != nil {
		return err
	}
	pending := 0
	for _, e := range review.Entries {
		if e.ChosenTMDB == 0 && e.ChosenTVDB == 0 {
			pending++
		}
	}
	if pending == 0 {
		fmt.Printf("No pending entries in %s\n", cfg.ReviewPath)
		return nil
	}

	res, err := reviewtui.Run(review, cfg.ReviewPath)
	if err != nil {
		return err
	}
	if res.Chosen > 0 {
		if err := manifest.SaveReview(cfg.ReviewPath, res.Review); err != nil {
			return err
		}
		fmt.Printf("Saved %d choice(s) to %s\n", res.Chosen, cfg.ReviewPath)
	} else {
		fmt.Println("No choices made; review file unchanged.")
	}
	if res.Skipped > 0 {
		fmt.Printf("Skipped %d entr(y/ies).\n", res.Skipped)
	}
	if res.QuitEarly {
		fmt.Println("Quit early — remaining entries left unanswered.")
	} else if res.Chosen > 0 {
		fmt.Println("Next: make ingest-apply  (resolve will write picks into the manifest)")
	}
	return nil
}

func runReconcile(dryRun bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := manifest.Load(cfg.ManifestPath)
	if err != nil {
		return err
	}
	review, err := manifest.LoadReview(cfg.ReviewPath)
	if err != nil {
		return err
	}

	deps, err := buildDeps(cfg)
	if err != nil {
		return err
	}
	eng := reconcile.New(deps)
	res, err := eng.Run(ctx, m, review, reconcile.Options{DryRun: dryRun})
	if err != nil {
		return err
	}

	fmt.Print(res.Report.Markdown())
	if res.ReportPath != "" {
		fmt.Fprintf(os.Stderr, "report written to %s\n", res.ReportPath)
	}
	if len(res.Report.Errors) > 0 {
		cmd := "apply"
		if dryRun {
			cmd = "plan"
		}
		return fmt.Errorf("%d error(s) during %s", len(res.Report.Errors), cmd)
	}
	return nil
}

func buildDeps(cfg *config.Config) (reconcile.Deps, error) {
	deps := reconcile.Deps{
		RadarrQualityProfileID: cfg.RadarrQualityProfileID,
		RadarrRootFolder:       cfg.RadarrRootFolder,
		RadarrTag:              cfg.RadarrTag,
		SonarrQualityProfileID: cfg.SonarrQualityProfileID,
		SonarrRootFolder:       cfg.SonarrRootFolder,
		SonarrTag:              cfg.SonarrTag,
		YtDlpOutputDir:         cfg.YtDlpOutputDir,
		YtDlpArchivePath:       cfg.YtDlpArchivePath,
		YtDlpBinary:            cfg.YtDlpPath,
		SyncWait:               cfg.SyncWait,
		SyncPollInterval:       cfg.SyncPollInterval,
		ManifestPath:           cfg.ManifestPath,
		ReviewPath:             cfg.ReviewPath,
		ReportDir:              cfg.ReportDir,
		Log:                    slog.Default(),
	}

	if cfg.RadarrBaseURL != "" {
		c, err := radarr.New(radarr.Options{
			BaseURL: cfg.RadarrBaseURL, APIKey: cfg.RadarrAPIKey, Timeout: cfg.HTTPTimeout,
		})
		if err != nil {
			return deps, fmt.Errorf("radarr: %w", err)
		}
		deps.Radarr = c
	} else {
		slog.Warn("radarr not configured; movie resolve/acquire disabled")
	}

	if cfg.SonarrBaseURL != "" {
		c, err := sonarr.New(sonarr.Options{
			BaseURL: cfg.SonarrBaseURL, APIKey: cfg.SonarrAPIKey, Timeout: cfg.HTTPTimeout,
		})
		if err != nil {
			return deps, fmt.Errorf("sonarr: %w", err)
		}
		deps.Sonarr = c
	} else {
		slog.Warn("sonarr not configured; series resolve/acquire disabled")
	}

	if cfg.JellyfinBaseURL != "" {
		c, err := jellyfin.New(jellyfin.Options{
			BaseURL: cfg.JellyfinBaseURL, APIKey: cfg.JellyfinAPIKey, UserID: cfg.JellyfinUserID,
		})
		if err != nil {
			return deps, fmt.Errorf("jellyfin: %w", err)
		}
		deps.Jellyfin = c
	} else {
		slog.Warn("jellyfin not configured; sync/import disabled")
	}

	if cfg.TVBaseURL != "" {
		c, err := tvclient.New(tvclient.Options{
			BaseURL: cfg.TVBaseURL, AdminKey: cfg.TVAdminKey, Timeout: cfg.HTTPTimeout,
		})
		if err != nil {
			return deps, fmt.Errorf("tv client: %w", err)
		}
		deps.TV = c
	} else {
		slog.Warn("tv base url not configured; import disabled")
	}

	if cfg.YtDlpOutputDir != "" {
		deps.YtDlp = ytdlp.ExecRunner{}
	} else {
		slog.Warn("yt-dlp output dir not configured; youtube acquire disabled")
	}

	return deps, nil
}
