// Command migrate applies (or rolls back) database migrations for either the
// LMS or the TV schema.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/aleksclark/primer/server/internal/config"
	"github.com/aleksclark/primer/server/internal/db"
	tvconfig "github.com/aleksclark/primer/server/internal/tv/config"
	tvdb "github.com/aleksclark/primer/server/internal/tv/db"
)

// schema bundles a service's migration entry points with its database URL
// lookup, so the same command drives both schemas.
type schema struct {
	up   func(context.Context, string) error
	down func(context.Context, string) error
	url  func() (string, error)
}

var schemas = map[string]schema{
	"lms": {
		up:   db.Migrate,
		down: db.MigrateDown,
		url: func() (string, error) {
			cfg, err := config.Load()
			if err != nil {
				return "", err
			}
			return cfg.DatabaseURL, nil
		},
	},
	"tv": {
		up:   tvdb.Migrate,
		down: tvdb.MigrateDown,
		url: func() (string, error) {
			cfg, err := tvconfig.Load()
			if err != nil {
				return "", err
			}
			return cfg.DatabaseURL, nil
		},
	},
}

func main() {
	service := flag.String("service", "lms", "schema to migrate (lms or tv)")
	flag.Parse()

	s, ok := schemas[*service]
	if !ok {
		slog.Error("unknown service", "service", *service, "want", "lms or tv")
		os.Exit(2)
	}

	url, err := s.url()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	direction := "up"
	if args := flag.Args(); len(args) > 0 {
		direction = args[0]
	}

	ctx := context.Background()
	switch direction {
	case "up":
		err = s.up(ctx, url)
	case "down":
		err = s.down(ctx, url)
	default:
		slog.Error("usage: migrate [-service lms|tv] [up|down]")
		os.Exit(2)
	}
	if err != nil {
		slog.Error("migrate", "service", *service, "direction", direction, "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied", "service", *service, "direction", direction)
}
