// Package config loads primer-agents runtime configuration from the
// environment using the PRIMER_AGENTS_ prefix so the service can run beside
// LMS, TV, Identity, and Studio without colliding settings.
package config

import (
	"fmt"
	"net/url"
	"os"
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

	// Provider fields — ordinary startup never composes a billable provider.
	// Credentials are never defaulted and these fields are not provider keys.
	ProviderMode      string `envconfig:"PROVIDER_MODE"`
	ProviderBaseURL   string `envconfig:"PROVIDER_BASE_URL"`
	ProviderModel     string `envconfig:"PROVIDER_MODEL"`
	ProviderSecretRef string `envconfig:"PROVIDER_SECRET_REF"`

	// LiveLLM fields are deliberately namespaced and opt-in. The API key is
	// read separately only when LiveLLMEnabled is true; ordinary config.Load
	// never inspects an ambient provider variable.
	LiveLLMEnabled  bool          `envconfig:"LIVE_LLM" default:"false"`
	LiveLLMModel    string        `envconfig:"LIVE_LLM_MODEL" default:"gpt-4o-mini"`
	LiveLLMBaseURL  string        `envconfig:"LIVE_LLM_BASE_URL"`
	LiveLLMTimeout  time.Duration `envconfig:"LIVE_LLM_TIMEOUT" default:"20s"`
	LiveLLMMaxCalls int           `envconfig:"LIVE_LLM_MAX_CALLS" default:"1"`

	// Identity endpoint fields for JWT validation (Phase 3+).
	// In production both must be HTTPS non-loopback URLs; absence disables
	// authentication (development/test only).
	IdentityJWKSURL string `envconfig:"IDENTITY_JWKS_URL"`
	IdentityIssuer  string `envconfig:"IDENTITY_ISSUER"`
}

// AuthEnabled reports whether Identity JWT validation is configured.
func (c *Config) AuthEnabled() bool {
	return strings.TrimSpace(c.IdentityJWKSURL) != "" &&
		strings.TrimSpace(c.IdentityIssuer) != ""
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

	if err := c.validateIdentityConfig(); err != nil {
		return err
	}
	if err := c.validateLiveLLMConfig(); err != nil {
		return err
	}

	return nil
}

// LiveLLMAPIKey returns the explicitly namespaced live key only after the
// caller has opted in. It intentionally never falls back to an ambient
// provider credential.
func (c *Config) LiveLLMAPIKey() string {
	if c == nil || !c.LiveLLMEnabled {
		return ""
	}
	return strings.TrimSpace(os.Getenv("PRIMER_AGENTS_LIVE_LLM_API_KEY"))
}

func (c *Config) validateLiveLLMConfig() error {
	if !c.LiveLLMEnabled {
		return nil
	}
	if c.Env == "production" {
		return fmt.Errorf("agents config: live billable LLM is forbidden in production")
	}
	if strings.TrimSpace(c.LiveLLMAPIKey()) == "" {
		return fmt.Errorf("agents config: PRIMER_AGENTS_LIVE_LLM_API_KEY is required when PRIMER_AGENTS_LIVE_LLM=1")
	}
	if strings.TrimSpace(c.LiveLLMModel) != "gpt-4o-mini" {
		return fmt.Errorf("agents config: live LLM model must be the fixed cheap model gpt-4o-mini")
	}
	if c.LiveLLMMaxCalls != 1 {
		return fmt.Errorf("agents config: live LLM max calls must be exactly 1")
	}
	if c.LiveLLMTimeout <= 0 || c.LiveLLMTimeout > 30*time.Second {
		return fmt.Errorf("agents config: live LLM timeout must be between 1ns and 30s")
	}
	raw := strings.TrimSpace(c.LiveLLMBaseURL)
	if raw == "" {
		return fmt.Errorf("agents config: PRIMER_AGENTS_LIVE_LLM_BASE_URL is required when live LLM is enabled (no billable URL default)")
	}
	if !ApprovedLiveLLMBaseURL(raw) {
		return fmt.Errorf("agents config: live LLM base URL must be an allowlisted HTTPS OpenAI-compatible endpoint")
	}
	return nil
}

// Approved live LLM HTTPS endpoints. Direct OpenAI plus the existing Primer
// Cloudflare AI Gateway OpenAI route used by crush/pi. No loopback, no
// arbitrary hosts, no query/fragment.
const (
	liveOpenAIBaseURL = "https://api.openai.com/v1"
	liveCFAIGOpenAI   = "https://gateway.ai.cloudflare.com/v1/a9d106d880527eaecdaf7835b792849d/curri-gateway/openai"
)

// ApprovedLiveLLMBaseURL reports whether raw is an explicit allowlisted
// HTTPS OpenAI-compatible provider URL.
func ApprovedLiveLLMBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	switch strings.TrimRight(u.String(), "/") {
	case liveOpenAIBaseURL, liveCFAIGOpenAI:
		return true
	default:
		return false
	}
}

// validateIdentityConfig enforces fail-closed rules for the JWKS/issuer pair.
// Production requires HTTPS non-loopback URLs. Development/test may omit both
// (auth middleware returns 401 on all protected routes when unconfigured).
func (c *Config) validateIdentityConfig() error {
	jwks := strings.TrimSpace(c.IdentityJWKSURL)
	issuer := strings.TrimSpace(c.IdentityIssuer)
	if jwks == "" && issuer == "" {
		if c.Env == "production" {
			return fmt.Errorf("agents config: PRIMER_AGENTS_IDENTITY_JWKS_URL and PRIMER_AGENTS_IDENTITY_ISSUER are required in production")
		}
		return nil
	}
	if (jwks == "") != (issuer == "") {
		return fmt.Errorf("agents config: PRIMER_AGENTS_IDENTITY_JWKS_URL and PRIMER_AGENTS_IDENTITY_ISSUER must both be set or both be unset")
	}
	if c.Env == "production" {
		if !strings.HasPrefix(jwks, "https://") {
			return fmt.Errorf("agents config: PRIMER_AGENTS_IDENTITY_JWKS_URL must use https in production")
		}
		if !strings.HasPrefix(issuer, "https://") {
			return fmt.Errorf("agents config: PRIMER_AGENTS_IDENTITY_ISSUER must use https in production")
		}
		if isForbiddenProductionHost(jwks) || isForbiddenProductionHost(issuer) {
			return fmt.Errorf("agents config: loopback/test Identity provider is forbidden in production")
		}
	}
	return nil
}

func isForbiddenProductionHost(raw string) bool {
	h := strings.ToLower(raw)
	return strings.Contains(h, "localhost") || strings.Contains(h, "127.0.0.1") ||
		strings.Contains(h, ".test") || strings.Contains(h, "::1")
}
