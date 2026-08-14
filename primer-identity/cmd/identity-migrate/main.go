// Command identity-migrate applies Primer Identity goose migrations only.
//
// Usage:
//
//	identity-migrate [up|down]
//
// Configuration is loaded from IDENTITY_* environment variables.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/logging"
)

func main() {
	flag.Parse()
	direction := "up"
	if args := flag.Args(); len(args) > 0 {
		direction = args[0]
	}

	logger := logging.NewJSONLogger(os.Stdout, "info")
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	switch direction {
	case "up":
		err = db.Migrate(ctx, cfg.DatabaseURL)
	case "down":
		err = db.MigrateDown(ctx, cfg.DatabaseURL)
	default:
		slog.Error("usage: identity-migrate [up|down]")
		os.Exit(2)
	}
	if err != nil {
		slog.Error("migrate", "direction", direction, "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied", "direction", direction, "table", db.VersionTable)
}
