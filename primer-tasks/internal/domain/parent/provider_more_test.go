package parent

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validProviderConfig() ProviderConfig {
	return ProviderConfig{Environment: "test", Mode: ProviderBedrock, Primary: ProviderBedrock, Fallback: ProviderOpenRouter, OpenRouterBaseURL: "https://openrouter.example/v1", MaxSteps: 4, MaxTokens: 100, MaxDuration: time.Minute, MaxRetries: 1, MaxConcurrentTenants: 2, ActiveTools: []string{ToolListStudents}}
}

func TestProviderValidationRejectsUnsafeAndIncompleteConfigurations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProviderConfig)
	}{
		{"bad environment", func(c *ProviderConfig) { c.Environment = "staging" }},
		{"scripted production", func(c *ProviderConfig) { c.Environment, c.Mode = "production", ProviderScripted }},
		{"unknown mode", func(c *ProviderConfig) { c.Mode = ProviderMode("wat") }},
		{"invalid primary", func(c *ProviderConfig) { c.Primary = ProviderScripted }},
		{"same fallback", func(c *ProviderConfig) { c.Fallback = c.Primary }},
		{"invalid url", func(c *ProviderConfig) { c.OpenRouterBaseURL = "not a url" }},
		{"production insecure url", func(c *ProviderConfig) { c.Environment, c.OpenRouterBaseURL = "production", "http://localhost:8080" }},
		{"missing active tools", func(c *ProviderConfig) { c.ActiveTools = nil }},
		{"unknown active tool", func(c *ProviderConfig) { c.ActiveTools = []string{"not-a-tool"} }},
		{"too many steps", func(c *ProviderConfig) { c.MaxSteps = 101 }},
		{"too many tokens", func(c *ProviderConfig) { c.MaxTokens = 1000001 }},
		{"zero duration", func(c *ProviderConfig) { c.MaxDuration = 0 }},
		{"too many retries", func(c *ProviderConfig) { c.MaxRetries = 11 }},
		{"zero concurrency", func(c *ProviderConfig) { c.MaxConcurrentTenants = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProviderConfig()
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("unsafe provider configuration accepted")
			}
		})
	}
	cfg := ProviderConfig{Environment: "test", Mode: ProviderDisabled, MaxSteps: 1, MaxTokens: 1, MaxDuration: time.Second, MaxConcurrentTenants: 1}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if !errors.As(cfg.Availability(), new(ProviderUnavailable)) {
		t.Fatal("disabled provider did not report ProviderUnavailable")
	}
	if validProviderConfig().Availability() != nil {
		t.Fatal("enabled provider reported unavailable")
	}
	if !strings.Contains(ProviderUnavailable{Mode: ProviderScripted}.Error(), "scripted") {
		t.Fatal("provider error lost mode")
	}
}

func TestProviderEnvironmentParsingAndToolSplit(t *testing.T) {
	for _, key := range []string{"TASKS_ENV", "TASKS_AGENT_MODE", "TASKS_MODEL_PROVIDER", "TASKS_AGENT_PRIMARY", "TASKS_AGENT_FALLBACK", "TASKS_AGENT_ACTIVE_TOOLS", "TASKS_AGENT_MAX_STEPS", "TASKS_AGENT_MAX_TOKENS", "TASKS_AGENT_MAX_SECONDS", "TASKS_AGENT_MAX_RETRIES", "TASKS_AGENT_MAX_CONCURRENT_TENANTS"} {
		t.Setenv(key, "")
	}
	t.Setenv("TASKS_ENV", "production")
	t.Setenv("TASKS_AGENT_MODE", "disabled")
	cfg, err := LoadProviderConfig()
	if err != nil || cfg.Environment != "production" || cfg.Mode != ProviderDisabled {
		t.Fatalf("config=%+v err=%v", cfg, err)
	}
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	t.Setenv("TASKS_ENV", "test")
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", " list_students, list_tasks ,, ")
	cfg, err = LoadProviderConfig()
	if err != nil || len(cfg.ActiveTools) != 2 {
		t.Fatalf("split config=%+v err=%v", cfg, err)
	}
	t.Setenv("TASKS_AGENT_MAX_STEPS", "not-an-int")
	if _, err = LoadProviderConfig(); err == nil {
		t.Fatal("invalid integer accepted")
	}
}
