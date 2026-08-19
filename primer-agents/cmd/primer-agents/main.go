// Command primer-agents is the standalone Primer Agents HTTP service.
// It is a thin main over app.Run; all bootstrap logic lives in internal/app.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aleksclark/primer/agents/internal/app"
	"github.com/aleksclark/primer/agents/internal/logging"
)

func main() {
	// Wire redacting JSON logs before any failure path so "error" attributes
	// cannot leak DSN passwords from early config/listen failures.
	logger := logging.NewJSONLogger(os.Stdout, "info")
	slog.SetDefault(logger)

	if err := app.Run(context.Background(), app.Options{}); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}
