// Package config loads primer-agents runtime configuration from the
// environment using the PRIMER_AGENTS_ prefix so the service can run beside
// LMS, TV, Identity, and Studio without colliding settings.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"

	agentsdb "github.com/aleksclark/primer/agents/internal/db"
)

// EnvPrefix namespaces every agents setting (e.g. PRIMER_AGENTS_DATABASE_URL).
const EnvPrefix = "PRIMER_AGENTS"

// Config holds all primer-agents runtime configuration, populated from the
// environment. No field falls back to a bare DATABASE_URL or any other
// service's environment variable.
type Config struct {
	// DatabaseURL is the PostgreSQL DSN for the agents DB only.
	// Required and non-empty in every environment.
	// Only PRIMER_AGENTS_DATABASE_URL is read; bare DATABASE_URL,
	// STUDIO_DATABASE_URL, IDENTITY_DATABASE_URL, and TV_DATABASE_URL are ignored.
	DatabaseURL string `split_words:"true"`

	Host     string `envconfig:"HOST" default:"0.0.0.0"`
	Port     int    `envconfig:"PORT" default:"8091"`
	Env      string `envconfig:"ENV" default:"development"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	ShutdownTimeout       time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`
	HTTPReadHeaderTimeout time.Duration `envconfig:"HTTP_READ_HEADER_TIMEOUT" default:"10s"`
	HTTPReadTimeout       time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"30s"`
	HTTPWriteTimeout      time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"30s"`
	HTTPIdleTimeout       time.Duration `envconfig:"HTTP_IDLE_TIMEOUT" default:"60s"`
	HTTPMaxBodyBytes      int64         `envconfig:"HTTP_MAX_BODY_BYTES" default:"1048576"`

	// WorkerEnabled enables the background job worker (Phase 2+).
	WorkerEnabled bool `envconfig:"WORKER_ENABLED" default:"false"`

	// Provider fields — reserved for Phase 2 runtime composition.
	// Credentials are never defaulted; absence is safe in Phase 1.
	ProviderMode      string `envconfig:"PROVIDER_MODE"`
	ProviderBaseURL   string `envconfig:"PROVIDER_BASE_URL"`
	ProviderModel     string `envconfig:"PROVIDER_MODEL"`
	ProviderSecretRef string `envconfig:"PROVIDER_SECRET_REF"`

	// Identity endpoint fields — reserved for Phase 3 authentication.
	// Absence is allowed in Phase 1 (no auth routes yet).
	IdentityJWKSURL string `envconfig:"IDENTITY_JWKS_URL"`
	IdentityIssuer  string `envconfig:"IDENTITY_ISSUER"`
}

// Load reads agents configuration from the environment and validates it.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process(EnvPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("load agents config: %w", err)
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

// Validate enforces fail-fast rules for agents configuration.
func (c *Config) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "development", "test", "production":
		c.Env = strings.ToLower(strings.TrimSpace(c.Env))
	default:
		return fmt.Errorf("agents config: env must be development|test|production, got %q", c.Env)
	}

	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	if c.DatabaseURL == "" {
		return fmt.Errorf("agents config: PRIMER_AGENTS_DATABASE_URL is required (no fallback to DATABASE_URL)")
	}

	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("agents config: port out of range: %d", c.Port)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("agents config: shutdown timeout must be positive")
	}
	if c.HTTPReadHeaderTimeout <= 0 {
		return fmt.Errorf("agents config: http read header timeout must be positive")
	}
	if c.HTTPReadTimeout <= 0 {
		return fmt.Errorf("agents config: http read timeout must be positive")
	}
	if c.HTTPWriteTimeout <= 0 {
		return fmt.Errorf("agents config: http write timeout must be positive")
	}
	if c.HTTPIdleTimeout <= 0 {
		return fmt.Errorf("agents config: http idle timeout must be positive")
	}
	if c.HTTPMaxBodyBytes <= 0 {
		return fmt.Errorf("agents config: http max body bytes must be positive")
	}

	if err := agentsdb.ValidateDatabaseURL(c.DatabaseURL); err != nil {
		return fmt.Errorf("agents config: %w", err)
	}

	return nil
}
