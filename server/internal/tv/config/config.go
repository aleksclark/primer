// Package config holds the TV server's runtime configuration. It is read from
// the environment with a TV_ prefix so the TV server and the LMS server can
// run side by side without their settings colliding.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// EnvPrefix namespaces every TV setting (e.g. TV_DATABASE_URL).
const EnvPrefix = "TV"

// Config holds all TV runtime configuration, populated from the environment.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string for the TV schema.
	DatabaseURL string `envconfig:"DATABASE_URL" default:"postgres://primer:primer@localhost:5432/primer_tv?sslmode=disable"`
	// Host is the address the HTTP server binds to.
	Host string `envconfig:"HOST" default:"0.0.0.0"`
	// Port is the TCP port the HTTP server listens on.
	Port int `envconfig:"PORT" default:"8081"`
	// Env is the deployment environment name.
	Env string `envconfig:"ENV" default:"development"`
	// CORSOrigins is the list of allowed CORS origins for the admin SPA.
	CORSOrigins []string `envconfig:"CORS_ORIGINS" default:"http://localhost:5174"`
	// JellyfinBaseURL is the root URL of the Jellyfin server.
	JellyfinBaseURL string `envconfig:"JELLYFIN_BASE_URL" default:"http://localhost:8096"`
	// JellyfinAPIKey authenticates admin calls against Jellyfin.
	JellyfinAPIKey string `envconfig:"JELLYFIN_API_KEY"`
	// JellyfinUserID scopes library browsing to a Jellyfin user.
	JellyfinUserID string `envconfig:"JELLYFIN_USER_ID"`
	// AdminAPIKey guards the admin API, which issues device pairing codes.
	// Empty leaves it open, which is only safe for local development.
	AdminAPIKey string `envconfig:"ADMIN_API_KEY"`
	// GrantTTL is how long a play grant stays redeemable. Grants only need to
	// survive long enough for the client to start playback.
	GrantTTL time.Duration `envconfig:"GRANT_TTL" default:"5m"`
	// PairingTTL is how long an unclaimed pairing code stays valid.
	PairingTTL time.Duration `envconfig:"PAIRING_TTL" default:"15m"`
	// ChannelTimezone is the IANA zone the programmed grid's calendar days are
	// bucketed in. The grid is authored and watched in one household's local
	// time, so "Tuesday morning" has to mean the same thing on every device
	// regardless of where the server is hosted.
	ChannelTimezone string `envconfig:"CHANNEL_TIMEZONE" default:"America/Chicago"`
	// PrimerBaseURL is the root of the Primer LMS deployment that instructional
	// time is reported to. Empty switches reporting off entirely, which is the
	// supported configuration for a channel running without an LMS.
	PrimerBaseURL string `envconfig:"PRIMER_BASE_URL"`
	// PrimerServiceToken authenticates against the LMS ingest endpoint.
	PrimerServiceToken string `envconfig:"PRIMER_SERVICE_TOKEN"`
	// PrimerReportInterval is how often finished viewings are pushed to the
	// LMS. Instructional hours are only ever read by day, so this is slow on
	// purpose.
	PrimerReportInterval time.Duration `envconfig:"PRIMER_REPORT_INTERVAL" default:"5m"`
	// PrimerReportBatchSize bounds one reporting pass.
	PrimerReportBatchSize int `envconfig:"PRIMER_REPORT_BATCH_SIZE" default:"100"`
	// PrimerTimeout bounds a single call to the LMS.
	PrimerTimeout time.Duration `envconfig:"PRIMER_TIMEOUT" default:"15s"`
	// ReleaseDir holds the APK published for sideloading plus a `version` file
	// naming its versionCode. The box has no app store, so this is the whole
	// distribution channel. Empty switches self-update off.
	ReleaseDir string `envconfig:"RELEASE_DIR"`

	// ManifestFailMaxAttempts is how many content-ingest acquisition attempts
	// a missing title may receive before the TV server marks it failed for
	// human intervention (buy/rip). Zero disables the attempt threshold.
	ManifestFailMaxAttempts int `envconfig:"MANIFEST_FAIL_MAX_ATTEMPTS" default:"10"`
	// ManifestFailMaxDays is how many calendar days after the first attempt a
	// title may stay missing before it is marked failed. Zero disables the
	// day threshold. Either threshold alone is enough to fail an entry.
	ManifestFailMaxDays int `envconfig:"MANIFEST_FAIL_MAX_DAYS" default:"14"`
}

// Load reads TV configuration from the environment.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process(EnvPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("load tv config: %w", err)
	}
	return &cfg, nil
}

// Addr returns the host:port bind address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
