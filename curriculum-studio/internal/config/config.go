// Package config loads Curriculum Studio runtime configuration from the
// environment using the STUDIO_ prefix so the service can run beside LMS, TV,
// and Primer Identity without colliding settings.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

// EnvPrefix namespaces every Studio setting (e.g. STUDIO_DATABASE_URL).
const EnvPrefix = "STUDIO"

// Config holds all Studio runtime configuration, populated from the environment.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string for the Studio DB only.
	// Required and non-empty in every environment (no localhost default).
	// Loaded only from STUDIO_DATABASE_URL — bare DATABASE_URL is ignored so
	// ambient LMS/host DSNs cannot silently satisfy Studio config.
	// split_words (without envconfig alt) yields STUDIO_DATABASE_URL only.
	DatabaseURL string `split_words:"true"`
	// Host is the address the HTTP server binds to.
	Host string `envconfig:"HOST" default:"0.0.0.0"`
	// Port is the TCP port the HTTP server listens on.
	Port int `envconfig:"PORT" default:"8088"`
	// Env is the deployment environment name: development|test|production.
	Env string `envconfig:"ENV" default:"development"`
	// LogLevel is the slog level name (debug|info|warn|error).
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	// AuthMode is reserved for S2+ (jwks|test). Unused by the S1 shell.
	AuthMode string `envconfig:"AUTH_MODE" default:"jwks"`
	// ArtifactStoreDir is optional filesystem root for later export bytes (S13).
	ArtifactStoreDir string `envconfig:"ARTIFACT_STORE_DIR"`
	// ShutdownTimeout bounds graceful HTTP shutdown after SIGINT/SIGTERM.
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`
	// HTTPReadHeaderTimeout bounds how long the server waits for request headers.
	HTTPReadHeaderTimeout time.Duration `envconfig:"HTTP_READ_HEADER_TIMEOUT" default:"10s"`
	// HTTPReadTimeout bounds full request read (headers + body).
	HTTPReadTimeout time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"30s"`
	// HTTPWriteTimeout bounds response write time.
	HTTPWriteTimeout time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"30s"`
	// HTTPIdleTimeout bounds keep-alive idle connections.
	HTTPIdleTimeout time.Duration `envconfig:"HTTP_IDLE_TIMEOUT" default:"60s"`
	// HTTPMaxBodyBytes caps request body size for future write endpoints.
	HTTPMaxBodyBytes int64 `envconfig:"HTTP_MAX_BODY_BYTES" default:"1048576"`
}

// Load reads Studio configuration from the environment and validates it.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process(EnvPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("load studio config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Addr returns the host:port bind address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Validate enforces fail-fast rules for Studio configuration.
func (c *Config) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "development", "test", "production":
		c.Env = strings.ToLower(strings.TrimSpace(c.Env))
	default:
		return fmt.Errorf("studio config: env must be development|test|production, got %q", c.Env)
	}

	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.AuthMode = strings.ToLower(strings.TrimSpace(c.AuthMode))
	if c.AuthMode == "" {
		c.AuthMode = "jwks"
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("studio config: database url is required")
	}
	// Port 0 is allowed for tests that inject an already-bound listener.
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("studio config: port out of range: %d", c.Port)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("studio config: shutdown timeout must be positive")
	}
	if c.HTTPReadHeaderTimeout <= 0 {
		return fmt.Errorf("studio config: http read header timeout must be positive")
	}
	if c.HTTPReadTimeout <= 0 {
		return fmt.Errorf("studio config: http read timeout must be positive")
	}
	if c.HTTPWriteTimeout <= 0 {
		return fmt.Errorf("studio config: http write timeout must be positive")
	}
	if c.HTTPIdleTimeout <= 0 {
		return fmt.Errorf("studio config: http idle timeout must be positive")
	}
	if c.HTTPMaxBodyBytes <= 0 {
		return fmt.Errorf("studio config: http max body bytes must be positive")
	}
	// Same pgx-parsed forbidden-name validator as library Connect/Migrate —
	// no forked deny list in config.
	if err := studiodb.ValidateDatabaseURL(c.DatabaseURL); err != nil {
		return fmt.Errorf("studio config: %w", err)
	}
	return nil
}
