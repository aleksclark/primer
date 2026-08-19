// Command migrate applies embedded primer-agents goose migrations.
// It intentionally uses only PRIMER_AGENTS_DATABASE_URL and the agents DSN
// isolation guard; no bare DATABASE_URL fallback is permitted.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aleksclark/primer/agents/internal/config"
	agentsdb "github.com/aleksclark/primer/agents/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := agentsdb.Migrate(context.Background(), cfg.DatabaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "agents migrate: %v\n", err)
		os.Exit(1)
	}
}
