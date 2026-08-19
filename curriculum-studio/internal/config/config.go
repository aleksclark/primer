// Package config loads Curriculum Studio runtime configuration from the
// environment using the STUDIO_ prefix so the service can run beside LMS, TV,
// and Primer Identity without colliding settings.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

// AudienceCurriculumStudio is the only accepted JWT audience for Studio.
const AudienceCurriculumStudio = "curriculum-studio"

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
	// AuthMode is the credential-free validator path: jwks|test.
	// Production refuses test mode and loopback/test Identity providers.
	AuthMode string `envconfig:"AUTH_MODE" default:"jwks"`
	// JWKSURL is the Identity (or test-double) JWKS document Studio validates against.
	// Studio never mints keys or tokens; it only fetches public JWKS.
	JWKSURL string `envconfig:"JWKS_URL"`
	// Issuer is the expected JWT iss (Identity issuer or loopback test issuer).
	Issuer string `envconfig:"ISSUER"`
	// Audience is the expected JWT aud. Frozen to curriculum-studio.
	Audience string `envconfig:"AUDIENCE" default:"curriculum-studio"`
	// AcceptServiceTokenAlias enables the migration-only X-Service-Token JWT
	// alias. It is disabled by default and never accepts static secrets.
	AcceptServiceTokenAlias bool `envconfig:"ACCEPT_SERVICE_TOKEN_ALIAS" default:"false"`
	// ArtifactStoreDir is optional filesystem root for later export bytes (S13).
	ArtifactStoreDir string `envconfig:"ARTIFACT_STORE_DIR"`
	// MCPEnabled controls whether the /mcp Streamable HTTP endpoint is registered.
	// Default: true in non-production; must be explicitly set in production.
	MCPEnabled bool `envconfig:"MCP_ENABLED" default:"true"`
	// MCPOriginAllowlist is a comma-separated list of allowed Origin header values.
	// Empty in dev/test (no browser-origin restriction); required non-empty in production.
	MCPOriginAllowlist string `envconfig:"MCP_ORIGIN_ALLOWLIST"`
	// MCPMaxBodyBytes caps the MCP request body. The explicit default keeps the
	// transport limit independent of the SDK's changing defaults.
	MCPMaxBodyBytes int64 `envconfig:"MCP_MAX_BODY_BYTES" default:"4194304"`
	// MCPRequestTimeout caps each tool invocation (0 → no per-request timeout).
	MCPRequestTimeout time.Duration `envconfig:"MCP_REQUEST_TIMEOUT" default:"0"`
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
	switch c.AuthMode {
	case "jwks", "test":
	default:
		return fmt.Errorf("studio config: auth mode must be jwks|test, got %q", c.AuthMode)
	}
	if c.Env == "production" && c.AuthMode == "test" {
		return fmt.Errorf("studio config: forbidden test auth in production")
	}

	c.JWKSURL = strings.TrimSpace(c.JWKSURL)
	c.Issuer = strings.TrimSpace(c.Issuer)
	c.Audience = strings.TrimSpace(c.Audience)
	if c.Audience == "" {
		c.Audience = AudienceCurriculumStudio
	}
	if c.Audience != AudienceCurriculumStudio {
		return fmt.Errorf("studio config: audience must be %s, got %q", AudienceCurriculumStudio, c.Audience)
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
	if c.MCPMaxBodyBytes == 0 {
		c.MCPMaxBodyBytes = 4 << 20
	}
	if c.MCPMaxBodyBytes < 0 {
		return fmt.Errorf("studio config: mcp max body bytes cannot be negative")
	}
	if c.MCPRequestTimeout < 0 {
		return fmt.Errorf("studio config: mcp request timeout cannot be negative")
	}
	// Same pgx-parsed forbidden-name validator as library Connect/Migrate —
	// no forked deny list in config.
	if err := studiodb.ValidateDatabaseURL(c.DatabaseURL); err != nil {
		return fmt.Errorf("studio config: %w", err)
	}
	if err := validateIdentityEndpoints(c.Env, c.AuthMode, c.JWKSURL, c.Issuer); err != nil {
		return err
	}
	if c.Env == "production" && c.MCPEnabled && len(strings.TrimSpace(c.MCPOriginAllowlist)) == 0 {
		return fmt.Errorf("studio config: mcp origin allowlist is required when mcp is enabled in production")
	}
	return nil
}

func validateIdentityEndpoints(env, _ string, jwksURL, issuer string) error {
	if jwksURL == "" && issuer == "" {
		// A bare development/test process may serve health while its external
		// Identity fixture is brought up. Protected routes still fail closed
		// until a validator is configured.
		if env == "production" {
			return fmt.Errorf("studio config: jwks url is required in production")
		}
		return nil
	}
	if jwksURL == "" {
		return fmt.Errorf("studio config: jwks url is required when issuer is set")
	}
	if issuer == "" {
		return fmt.Errorf("studio config: issuer is required when jwks url is set")
	}
	jwks, err := parseIdentityURL("jwks url", jwksURL)
	if err != nil {
		return err
	}
	iss, err := parseIdentityURL("issuer", issuer)
	if err != nil {
		return err
	}
	if env == "production" {
		if jwks.Scheme != "https" {
			return fmt.Errorf("studio config: jwks url must use https in production")
		}
		if iss.Scheme != "https" {
			return fmt.Errorf("studio config: issuer must use https in production")
		}
		if isForbiddenProductionHost(jwks.Hostname()) || isForbiddenProductionHost(iss.Hostname()) {
			return fmt.Errorf("studio config: forbidden test/loopback identity provider in production")
		}
	}
	return nil
}

func parseIdentityURL(field, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("studio config: %s is invalid", field)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return nil, fmt.Errorf("studio config: %s scheme must be http or https", field)
	}
	return u, nil
}

func isForbiddenProductionHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "test" || strings.HasSuffix(h, ".test") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
