package config

import (
	"fmt"

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
