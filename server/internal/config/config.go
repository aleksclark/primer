package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration, populated from the environment.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string `envconfig:"DATABASE_URL" default:"postgres://primer:primer@localhost:5432/primer?sslmode=disable"`
	// Host is the address the HTTP server binds to.
	Host string `envconfig:"HOST" default:"0.0.0.0"`
	// Port is the TCP port the HTTP server listens on.
	Port int `envconfig:"PORT" default:"8080"`
	// Env is the deployment environment name.
	Env string `envconfig:"ENV" default:"development"`
	// CORSOrigins is the list of allowed CORS origins for the admin SPA.
	CORSOrigins []string `envconfig:"CORS_ORIGINS" default:"http://localhost:5173"`
	// ServiceToken authenticates other Primer services pushing data into the
	// LMS — today, the TV server reporting instructional time. Empty leaves
	// the ingest open, which is only safe for local development.
	ServiceToken string `envconfig:"SERVICE_TOKEN"`

	// TutorProvider selects the coaching backend: fake (default) or bedrock.
	// Bedrock requires TUTOR_BEDROCK_URL (or falls back to fake).
	TutorProvider string `envconfig:"TUTOR_PROVIDER" default:"fake"`
	// TutorEnabled gates student tutoring globally (default true).
	// Per-student off switch: student.Notes containing "tutor:off".
	TutorEnabled bool `envconfig:"TUTOR_ENABLED" default:"true"`
	// TutorBedrockURL is an optional Bedrock Runtime invoke URL or signing proxy.
	TutorBedrockURL string `envconfig:"TUTOR_BEDROCK_URL"`
	// TutorBedrockAPIKey is an optional bearer token for a signing proxy.
	TutorBedrockAPIKey string `envconfig:"TUTOR_BEDROCK_API_KEY"`
	// TutorBedrockModel is recorded for diagnostics / proxy routing.
	TutorBedrockModel string `envconfig:"TUTOR_BEDROCK_MODEL"`

	// ArtifactStoreDir is the filesystem root for session evidence bytes and
	// approved fixture bundles. Empty disables byte upload (metadata-only).
	ArtifactStoreDir string `envconfig:"ARTIFACT_STORE_DIR" default:""`

	// AgentRuntimeEnabled enables the authenticated, process-local MAF preview
	// runtime. It is disabled by default and is not a durability guarantee.
	AgentRuntimeEnabled bool `envconfig:"AGENT_RUNTIME_ENABLED" default:"false"`
	// AgentRuntimeBaseURL is an OpenAI-compatible endpoint. It is required when
	// AgentRuntimeEnabled is true; no billable endpoint is assumed by default.
	AgentRuntimeBaseURL   string        `envconfig:"AGENT_RUNTIME_BASE_URL" default:""`
	AgentRuntimeAPIKey    string        `envconfig:"AGENT_RUNTIME_API_KEY" default:""`
	AgentRuntimeModel     string        `envconfig:"AGENT_RUNTIME_MODEL" default:""`
	AgentRuntimeRunBudget time.Duration `envconfig:"AGENT_RUNTIME_RUN_BUDGET" default:"2m"`
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &cfg, nil
}

// Addr returns the host:port bind address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
