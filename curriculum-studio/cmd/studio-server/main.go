// Command studio-server runs the Curriculum Studio HTTP service.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aleksclark/primer/curriculum-studio/internal/app"
	"github.com/aleksclark/primer/curriculum-studio/internal/logging"
)

func main() {
	// Wire redacting JSON logs before any failure path so "error" attrs
	// cannot leak DSN passwords from early config/listen failures.
	logger := logging.NewJSONLogger(os.Stdout, "info")
	slog.SetDefault(logger)

	if err := app.Run(context.Background(), app.Options{}); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}
