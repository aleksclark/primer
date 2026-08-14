// Package config loads Primer Identity runtime configuration from the
// environment using the IDENTITY_ prefix so the service can run beside LMS,
// TV, and Curriculum Studio without colliding settings.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kelseyhightower/envconfig"
)

// EnvPrefix namespaces every Identity setting (e.g. IDENTITY_DATABASE_URL).
const EnvPrefix = "IDENTITY"

// Config holds all Identity runtime configuration, populated from the environment.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string for the Identity DB only.
	// Required and non-empty in every environment (no localhost default).
	// Loaded only from IDENTITY_DATABASE_URL — bare DATABASE_URL is ignored so
	// ambient LMS/host DSNs cannot silently satisfy Identity config.
	// split_words (without envconfig alt) yields IDENTITY_DATABASE_URL only;
	// envconfig's Alt fallback would otherwise inherit bare DATABASE_URL.
	DatabaseURL string `split_words:"true"`
	// Host is the address the HTTP server binds to.
	Host string `envconfig:"HOST" default:"0.0.0.0"`
	// Port is the TCP port the HTTP server listens on.
	Port int `envconfig:"PORT" default:"8090"`
	// Env is the deployment environment name: development|test|production.
	Env string `envconfig:"ENV" default:"development"`
	// LogLevel is the slog level name (debug|info|warn|error).
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	// Issuer is the OIDC issuer URL. Required non-empty in every environment.
	Issuer string `envconfig:"ISSUER"`
	// ShutdownTimeout bounds graceful HTTP shutdown after SIGINT/SIGTERM.
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`
	// HTTPReadHeaderTimeout bounds how long the server waits for request headers.
	HTTPReadHeaderTimeout time.Duration `envconfig:"HTTP_READ_HEADER_TIMEOUT" default:"10s"`
	// HTTPMaxBodyBytes caps request body size for future write endpoints.
	HTTPMaxBodyBytes int64 `envconfig:"HTTP_MAX_BODY_BYTES" default:"1048576"`
}

// Load reads Identity configuration from the environment and validates it.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process(EnvPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("load identity config: %w", err)
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

// Validate enforces fail-fast rules for Identity configuration.
func (c *Config) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "development", "test", "production":
		c.Env = strings.ToLower(strings.TrimSpace(c.Env))
	default:
		return fmt.Errorf("identity config: env must be development|test|production, got %q", c.Env)
	}

	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.Issuer = strings.TrimSpace(c.Issuer)

	if c.DatabaseURL == "" {
		return fmt.Errorf("identity config: database url is required")
	}
	if c.Issuer == "" {
		return fmt.Errorf("identity config: issuer is required")
	}
	// Port 0 is allowed for tests that inject an already-bound listener.
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("identity config: port out of range: %d", c.Port)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("identity config: shutdown timeout must be positive")
	}
	if c.HTTPReadHeaderTimeout <= 0 {
		return fmt.Errorf("identity config: http read header timeout must be positive")
	}
	if c.HTTPMaxBodyBytes <= 0 {
		return fmt.Errorf("identity config: http max body bytes must be positive")
	}
	if err := validateIdentityDatabaseURL(c.DatabaseURL); err != nil {
		return err
	}
	return nil
}

// Forbidden database path names that belong to other Primer products.
// Identity must never silently reuse LMS/TV/Studio DSNs.
var forbiddenDBNames = map[string]struct{}{
	"primer":            {},
	"primer_tv":         {},
	"primer_test":       {},
	"curriculum_studio": {},
	"studio":            {},
	"tv":                {},
}

// validateIdentityDatabaseURL parses the DSN with the same pgx/pgxpool config
// path used for connections, then rejects empty and reserved product DB names.
// This covers URI, keyword/libpq, and slash-normalized forms that url.Parse misses.
func validateIdentityDatabaseURL(raw string) error {
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return fmt.Errorf("identity config: parse database url: %w", err)
	}
	name := normalizeDatabaseName(cfg.ConnConfig.Database)
	if name == "" {
		return fmt.Errorf("identity config: database url missing database name")
	}
	if _, bad := forbiddenDBNames[name]; bad {
		return fmt.Errorf("identity config: database name %q is reserved for another Primer product; use primer_identity", name)
	}
	// Reject obvious LMS default path reuse even when host differs.
	if name == "goose_db_version" {
		return fmt.Errorf("identity config: invalid database name %q", name)
	}
	return nil
}

func normalizeDatabaseName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "/")
	name = strings.TrimSpace(name)
	return strings.ToLower(name)
}
