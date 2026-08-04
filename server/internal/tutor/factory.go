package tutor

import (
	"fmt"
	"strings"
)

// Provider names accepted by NewFromConfig.
const (
	ProviderFake    = "fake"
	ProviderBedrock = "bedrock"
	ProviderEcho    = "echo" // tests / local only
)

// Config selects and configures the tutor stack used by the API.
type Config struct {
	// Provider is fake|bedrock|echo (default fake).
	Provider string
	// Enabled gates tutoring globally (default true when using DefaultConfig).
	Enabled bool
	// Policy overrides defaults when non-zero fields are set via ApplyDefaults.
	Policy PolicyConfig
	// Bedrock is used when Provider is bedrock.
	Bedrock BedrockConfig
	// Inner overrides provider construction (tests).
	Inner Provider
}

// DefaultConfig returns fake provider with tutoring enabled.
func DefaultConfig() Config {
	return Config{
		Provider: ProviderFake,
		Enabled:  true,
		Policy:   DefaultPolicy(),
	}
}

// NewFromConfig builds a PolicyService around the selected provider.
// Unknown providers return an error. Bedrock without URL falls back to fake
// so tests and default deploys never require AWS.
func NewFromConfig(cfg Config) (*PolicyService, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if name == "" {
		name = ProviderFake
	}

	policy := cfg.Policy
	if policy.MaxSentences <= 0 {
		policy.MaxSentences = 2
	}
	if policy.MaxMessagesPerSession <= 0 {
		policy.MaxMessagesPerSession = 20
	}
	if policy.Timeout <= 0 {
		policy.Timeout = DefaultPolicy().Timeout
	}
	// StripCodeLines defaults true unless explicitly disabled via zero+flag pattern:
	// callers should set Policy from DefaultPolicy() then tweak.
	if !cfg.Policy.StripCodeLines && cfg.Policy.MaxSentences == 0 &&
		cfg.Policy.MaxMessagesPerSession == 0 && cfg.Policy.Timeout == 0 {
		policy.StripCodeLines = true
	}
	policy.Enabled = cfg.Enabled

	var inner Provider
	if cfg.Inner != nil {
		inner = cfg.Inner
	} else {
		switch name {
		case ProviderFake:
			inner = NewFake()
		case ProviderEcho:
			inner = &EchoProvider{}
		case ProviderBedrock:
			if strings.TrimSpace(cfg.Bedrock.URL) == "" {
				inner = NewFake()
			} else {
				bp, err := NewBedrock(cfg.Bedrock)
				if err != nil {
					return nil, err
				}
				inner = bp
			}
		default:
			return nil, fmt.Errorf("tutor: unknown provider %q", cfg.Provider)
		}
	}

	return NewPolicy(inner, policy), nil
}
