package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/config"
)

func TestLoadDefaults(t *testing.T) {
	// Unset overrides so struct defaults apply. t.Setenv("") would win over default.
	for _, k := range []string{
		"INGEST_MANIFEST_PATH", "INGEST_REVIEW_PATH", "INGEST_REPORT_DIR",
		"INGEST_RADARR_BASE_URL", "INGEST_SONARR_BASE_URL",
		"INGEST_JELLYFIN_BASE_URL", "INGEST_TV_BASE_URL",
		"INGEST_RADARR_TAG", "INGEST_SONARR_TAG", "INGEST_YTDLP_PATH",
		"INGEST_HTTP_TIMEOUT",
	} {
		t.Helper()
		_ = os.Unsetenv(k)
	}

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "curriculum/content-manifest.yaml", cfg.ManifestPath)
	assert.Equal(t, "curriculum/content-review.yaml", cfg.ReviewPath)
	assert.Equal(t, "primer", cfg.RadarrTag)
	assert.Equal(t, "primer", cfg.SonarrTag)
	assert.Equal(t, "yt-dlp", cfg.YtDlpPath)
	assert.Equal(t, 30*time.Second, cfg.HTTPTimeout)
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("INGEST_MANIFEST_PATH", "/tmp/m.yaml")
	t.Setenv("INGEST_RADARR_BASE_URL", "http://radarr.test")
	t.Setenv("INGEST_RADARR_QUALITY_PROFILE_ID", "7")
	t.Setenv("INGEST_HTTP_TIMEOUT", "5s")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/m.yaml", cfg.ManifestPath)
	assert.Equal(t, "http://radarr.test", cfg.RadarrBaseURL)
	assert.Equal(t, 7, cfg.RadarrQualityProfileID)
	assert.Equal(t, 5*time.Second, cfg.HTTPTimeout)
}
