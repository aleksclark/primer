// Package config holds content-ingest runtime configuration. It is read from
// the environment with an INGEST_ prefix so the tool can run alongside the LMS
// and TV servers without colliding settings.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// EnvPrefix namespaces every ingest setting (e.g. INGEST_MANIFEST_PATH).
const EnvPrefix = "INGEST"

// Config holds all content-ingest runtime configuration.
type Config struct {
	// ManifestPath is the desired-state YAML (committed to git).
	ManifestPath string `envconfig:"MANIFEST_PATH" default:"curriculum/content-manifest.yaml"`
	// ReviewPath is the human-resolution working file for ambiguous lookups.
	ReviewPath string `envconfig:"REVIEW_PATH" default:"curriculum/content-review.yaml"`
	// ReportDir is where per-run markdown reports are written.
	ReportDir string `envconfig:"REPORT_DIR" default:"curriculum/ingest-reports"`

	// RadarrBaseURL is the root URL of the Radarr instance.
	RadarrBaseURL string `envconfig:"RADARR_BASE_URL"`
	// RadarrAPIKey authenticates against Radarr.
	RadarrAPIKey string `envconfig:"RADARR_API_KEY"`
	// RadarrRootFolder is where Radarr stores movies.
	RadarrRootFolder string `envconfig:"RADARR_ROOT_FOLDER"`
	// RadarrQualityProfileID selects the 1080p H.264/H.265 profile.
	RadarrQualityProfileID int `envconfig:"RADARR_QUALITY_PROFILE_ID"`
	// RadarrTag is applied to every movie this tool adds (created if missing).
	RadarrTag string `envconfig:"RADARR_TAG" default:"primer"`

	// SonarrBaseURL is the root URL of the Sonarr instance.
	SonarrBaseURL string `envconfig:"SONARR_BASE_URL"`
	// SonarrAPIKey authenticates against Sonarr.
	SonarrAPIKey string `envconfig:"SONARR_API_KEY"`
	// SonarrRootFolder is where Sonarr stores series.
	SonarrRootFolder string `envconfig:"SONARR_ROOT_FOLDER"`
	// SonarrQualityProfileID selects the 1080p H.264/H.265 profile.
	SonarrQualityProfileID int `envconfig:"SONARR_QUALITY_PROFILE_ID"`
	// SonarrTag is applied to every series this tool adds (created if missing).
	SonarrTag string `envconfig:"SONARR_TAG" default:"primer"`

	// JellyfinBaseURL is the root URL of the Jellyfin server.
	JellyfinBaseURL string `envconfig:"JELLYFIN_BASE_URL"`
	// JellyfinAPIKey authenticates admin calls against Jellyfin.
	JellyfinAPIKey string `envconfig:"JELLYFIN_API_KEY"`
	// JellyfinUserID scopes library browsing to a Jellyfin user.
	JellyfinUserID string `envconfig:"JELLYFIN_USER_ID"`

	// TVBaseURL is the root of the TV server admin API (including /api/v1).
	TVBaseURL string `envconfig:"TV_BASE_URL"`
	// TVAdminKey authenticates against the TV admin API (X-Admin-Key).
	TVAdminKey string `envconfig:"TV_ADMIN_KEY"`

	// YtDlpPath is the yt-dlp binary.
	YtDlpPath string `envconfig:"YTDLP_PATH" default:"yt-dlp"`
	// YtDlpOutputDir is the library root yt-dlp writes into (Jellyfin-scanned).
	YtDlpOutputDir string `envconfig:"YTDLP_OUTPUT_DIR"`
	// YtDlpArchivePath is the --download-archive file (idempotent downloads).
	YtDlpArchivePath string `envconfig:"YTDLP_ARCHIVE_PATH" default:"curriculum/ytdlp-archive.txt"`

	// HTTPTimeout bounds a single upstream HTTP call.
	HTTPTimeout time.Duration `envconfig:"HTTP_TIMEOUT" default:"30s"`
	// SyncWait is how long to wait for a Jellyfin library scan to finish.
	SyncWait time.Duration `envconfig:"SYNC_WAIT" default:"2m"`
	// SyncPollInterval is how often to poll Jellyfin for scan completion.
	SyncPollInterval time.Duration `envconfig:"SYNC_POLL_INTERVAL" default:"2s"`
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process(EnvPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("load ingest config: %w", err)
	}
	return &cfg, nil
}
