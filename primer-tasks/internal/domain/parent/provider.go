package parent

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProviderMode string

const (
	ProviderDisabled   ProviderMode = "disabled"
	ProviderScripted   ProviderMode = "scripted"
	ProviderBedrock    ProviderMode = "bedrock"
	ProviderOpenRouter ProviderMode = "openrouter"
)

// ProviderConfig is server-side configuration for the future Fantasy runtime.
// This slice owns validation and policy; it does not create a provider client
// or read credentials into an agent/tool context.
type ProviderConfig struct {
	Environment          string
	Mode                 ProviderMode
	Primary              ProviderMode
	Fallback             ProviderMode
	BedrockRegion        string
	BedrockModel         string
	OpenRouterBaseURL    string
	OpenRouterModel      string
	MaxSteps             int
	MaxTokens            int
	MaxDuration          time.Duration
	MaxRetries           int
	MaxConcurrentTenants int
	ActiveTools          []string
}

func LoadProviderConfig() (ProviderConfig, error) {
	env := envValue("TASKS_ENV", "development")
	mode := ProviderMode(envValue("TASKS_AGENT_MODE", envValue("TASKS_MODEL_PROVIDER", string(ProviderDisabled))))
	primary := ProviderMode(envValue("TASKS_AGENT_PRIMARY", string(ProviderBedrock)))
	fallback := ProviderMode(envValue("TASKS_AGENT_FALLBACK", string(ProviderOpenRouter)))
	c := ProviderConfig{
		Environment: env, Mode: mode, Primary: primary, Fallback: fallback,
		BedrockRegion:        envValue("TASKS_BEDROCK_REGION", "us-east-1"),
		BedrockModel:         os.Getenv("TASKS_BEDROCK_MODEL"),
		OpenRouterBaseURL:    strings.TrimRight(envValue("TASKS_OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"), "/"),
		OpenRouterModel:      os.Getenv("TASKS_OPENROUTER_MODEL"),
		MaxSteps:             envInt("TASKS_AGENT_MAX_STEPS", 12),
		MaxTokens:            envInt("TASKS_AGENT_MAX_TOKENS", 4096),
		MaxDuration:          time.Duration(envInt("TASKS_AGENT_MAX_SECONDS", 120)) * time.Second,
		MaxRetries:           envInt("TASKS_AGENT_MAX_RETRIES", 2),
		MaxConcurrentTenants: envInt("TASKS_AGENT_MAX_CONCURRENT_TENANTS", 4),
		ActiveTools:          splitTools(os.Getenv("TASKS_AGENT_ACTIVE_TOOLS")),
	}
	return c, c.Validate()
}

func (c ProviderConfig) Validate() error {
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return fmt.Errorf("agent environment is invalid")
	}
	switch c.Mode {
	case ProviderDisabled:
		// Disabled is explicit.  Callers must continue through the ordinary
		// phase-2 manual services; this mode never reports agent success.
	case ProviderScripted:
		if c.Environment == "production" {
			return fmt.Errorf("scripted provider is forbidden in production")
		}
	case ProviderBedrock, ProviderOpenRouter:
		if err := validateProvider(c.Primary); err != nil {
			return err
		}
		if c.Fallback != "" && c.Fallback != ProviderDisabled {
			if err := validateProvider(c.Fallback); err != nil {
				return err
			}
			if c.Fallback == c.Primary {
				return fmt.Errorf("agent fallback must differ from primary")
			}
		}
	default:
		return fmt.Errorf("unknown agent mode %q", c.Mode)
	}
	if c.Mode != ProviderDisabled && c.Mode != ProviderScripted && c.Primary == ProviderScripted {
		return fmt.Errorf("scripted cannot be a live provider")
	}
	if c.MaxSteps <= 0 || c.MaxSteps > 100 || c.MaxTokens <= 0 || c.MaxTokens > 1_000_000 || c.MaxDuration <= 0 || c.MaxDuration > time.Hour || c.MaxRetries < 0 || c.MaxRetries > 10 || c.MaxConcurrentTenants <= 0 || c.MaxConcurrentTenants > 100 {
		return fmt.Errorf("agent limits are outside safe bounds")
	}
	if c.Mode != ProviderDisabled && c.Mode != ProviderScripted {
		u, err := url.Parse(c.OpenRouterBaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("OpenRouter base URL must be an absolute HTTP(S) URL")
		}
		if c.Environment == "production" && (u.Scheme != "https" || strings.Contains(u.Hostname(), "localhost") || strings.HasPrefix(u.Hostname(), "127.")) {
			return fmt.Errorf("production cannot use a local or insecure OpenRouter URL")
		}
	}
	if c.Mode != ProviderDisabled && len(c.ActiveTools) == 0 {
		return fmt.Errorf("active tool allowlist is required when agent is enabled")
	}
	if len(c.ActiveTools) > 0 {
		if _, err := NewToolSet(c.ActiveTools); err != nil {
			return err
		}
	}
	return nil
}
func validateProvider(p ProviderMode) error {
	if p != ProviderBedrock && p != ProviderOpenRouter {
		return fmt.Errorf("provider %q is not a live provider", p)
	}
	return nil
}

// ProviderUnavailable is intentionally a distinct result.  Disabled mode is
// not a scripted reply and has no authority to mutate anything.
type ProviderUnavailable struct{ Mode ProviderMode }

func (e ProviderUnavailable) Error() string {
	return fmt.Sprintf("parent agent provider is %s; use manual phase-2 controls", e.Mode)
}
func (c ProviderConfig) Availability() error {
	if c.Mode == ProviderDisabled {
		return ProviderUnavailable{Mode: c.Mode}
	}
	return nil
}

func envValue(k, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(k)); value != "" {
		return value
	}
	return fallback
}
func envInt(k string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(k))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return n
}
func splitTools(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
