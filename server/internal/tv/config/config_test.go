package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8081, cfg.Port, "the TV port must not clash with the LMS default")
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 5*time.Minute, cfg.GrantTTL)
	assert.Equal(t, 15*time.Minute, cfg.PairingTTL)
	assert.Contains(t, cfg.DatabaseURL, "primer_tv", "the TV schema uses its own database")
	assert.Equal(t, "America/Chicago", cfg.ChannelTimezone, "the household keeps Central time")
	assert.Equal(t, "0.0.0.0:8081", cfg.Addr())
	assert.Equal(t, 10, cfg.ManifestFailMaxAttempts)
	assert.Equal(t, 14, cfg.ManifestFailMaxDays)
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("TV_HOST", "127.0.0.1")
	t.Setenv("TV_PORT", "9000")
	t.Setenv("TV_DATABASE_URL", "postgres://example/tv")
	t.Setenv("TV_JELLYFIN_BASE_URL", "http://jellyfin.local:8096")
	t.Setenv("TV_JELLYFIN_API_KEY", "key-123")
	t.Setenv("TV_JELLYFIN_USER_ID", "user-9")
	t.Setenv("TV_GRANT_TTL", "90s")
	t.Setenv("TV_PAIRING_TTL", "2m")
	t.Setenv("TV_CORS_ORIGINS", "http://a.test,http://b.test")
	t.Setenv("TV_CHANNEL_TIMEZONE", "America/New_York")
	t.Setenv("TV_MANIFEST_FAIL_MAX_ATTEMPTS", "5")
	t.Setenv("TV_MANIFEST_FAIL_MAX_DAYS", "21")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9000", cfg.Addr())
	assert.Equal(t, "postgres://example/tv", cfg.DatabaseURL)
	assert.Equal(t, "http://jellyfin.local:8096", cfg.JellyfinBaseURL)
	assert.Equal(t, "key-123", cfg.JellyfinAPIKey)
	assert.Equal(t, "user-9", cfg.JellyfinUserID)
	assert.Equal(t, 90*time.Second, cfg.GrantTTL)
	assert.Equal(t, 2*time.Minute, cfg.PairingTTL)
	assert.Equal(t, []string{"http://a.test", "http://b.test"}, cfg.CORSOrigins)
	assert.Equal(t, "America/New_York", cfg.ChannelTimezone)
	assert.Equal(t, 5, cfg.ManifestFailMaxAttempts)
	assert.Equal(t, 21, cfg.ManifestFailMaxDays)
}

func TestLoadRejectsMalformedValues(t *testing.T) {
	t.Setenv("TV_PORT", "not-a-number")
	_, err := config.Load()
	assert.Error(t, err)
}

func TestAdminAPIKeyDefaultsToOpen(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.AdminAPIKey, "no key by default; deployments must set one")

	t.Setenv("TV_ADMIN_API_KEY", "s3cret")
	cfg, err = config.Load()
	require.NoError(t, err)
	assert.Equal(t, "s3cret", cfg.AdminAPIKey)
}

func TestPrimerReportingDefaultsToOff(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.PrimerBaseURL, "a channel without an LMS reports nothing")
	assert.Empty(t, cfg.PrimerServiceToken)
	assert.Equal(t, 5*time.Minute, cfg.PrimerReportInterval)
	assert.Equal(t, 100, cfg.PrimerReportBatchSize)
	assert.Equal(t, 15*time.Second, cfg.PrimerTimeout)

	t.Setenv("TV_PRIMER_BASE_URL", "https://primer.example")
	t.Setenv("TV_PRIMER_SERVICE_TOKEN", "s3cret")
	t.Setenv("TV_PRIMER_REPORT_INTERVAL", "30s")
	t.Setenv("TV_PRIMER_REPORT_BATCH_SIZE", "10")
	t.Setenv("TV_PRIMER_TIMEOUT", "3s")

	cfg, err = config.Load()
	require.NoError(t, err)
	assert.Equal(t, "https://primer.example", cfg.PrimerBaseURL)
	assert.Equal(t, "s3cret", cfg.PrimerServiceToken)
	assert.Equal(t, 30*time.Second, cfg.PrimerReportInterval)
	assert.Equal(t, 10, cfg.PrimerReportBatchSize)
	assert.Equal(t, 3*time.Second, cfg.PrimerTimeout)
}
