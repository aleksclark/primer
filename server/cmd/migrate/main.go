// Command migrate applies (or rolls back) database migrations.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aleksclark/primer/server/internal/config"
	"github.com/aleksclark/primer/server/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	ctx := context.Background()
	switch direction {
	case "up":
		err = db.Migrate(ctx, cfg.DatabaseURL)
	case "down":
		err = db.MigrateDown(ctx, cfg.DatabaseURL)
	default:
		slog.Error("usage: migrate [up|down]")
		os.Exit(2)
	}
	if err != nil {
		slog.Error("migrate", "direction", direction, "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied", "direction", direction)
}
