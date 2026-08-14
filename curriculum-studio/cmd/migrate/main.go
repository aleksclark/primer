// Command migrate applies Curriculum Studio goose migrations.
//
// Usage:
//
//	STUDIO_DATABASE_URL=postgres://... go run ./cmd/migrate [up|down|status]
//	go run ./cmd/migrate -check-freeze
//
// Environment:
//
//	STUDIO_DATABASE_URL              required for up/down/status
//	STUDIO_MIGRATIONS_LIVE           when true, down and -write-freeze are refused
//	                                 (down needs break-glass; write-freeze has none)
//	STUDIO_MIGRATE_BREAK_GLASS_DOWN  allow one down on live envs
//	STUDIO_DB_MAX_CONNS              optional pool hint (reserved for service)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

func main() {
	checkFreeze := flag.Bool("check-freeze", false, "verify baseline migration hashes against baseline_manifest.json")
	writeFreeze := flag.Bool("write-freeze", false, "regenerate baseline_manifest.json from current migration files (pre-live only)")
	migrationsDir := flag.String("migrations", "", "path to db/migrations (default: module db/migrations)")
	flag.Parse()

	migDir := *migrationsDir
	if migDir == "" {
		migDir = defaultMigrationsDir()
	}

	liveMarker := studiodb.LiveMarkerPathForMigrations(migDir)
	envLive := os.Getenv("STUDIO_MIGRATIONS_LIVE")

	if *writeFreeze {
		if err := studiodb.GuardWriteFreeze(studiodb.Truthy(envLive), liveMarker); err != nil {
			slog.Error("write freeze refused", "error", err)
			os.Exit(1)
		}
		m, err := studiodb.BuildManifest(migDir)
		if err != nil {
			slog.Error("build freeze manifest", "error", err)
			os.Exit(1)
		}
		out := filepath.Join(filepath.Dir(migDir), studiodb.BaselineManifestName)
		if err := studiodb.WriteManifest(out, m); err != nil {
			slog.Error("write freeze manifest", "error", err)
			os.Exit(1)
		}
		fmt.Println("wrote", out)
		return
	}

	if *checkFreeze {
		manifestPath := filepath.Join(filepath.Dir(migDir), studiodb.BaselineManifestName)
		m, err := studiodb.LoadManifest(manifestPath)
		if err != nil {
			slog.Error("load freeze manifest", "error", err)
			os.Exit(1)
		}
		// check-freeze stays available when live; classification only tightens messages.
		live := studiodb.ClassifyLive(envLive, liveMarker)
		if err := studiodb.VerifyManifest(migDir, m, live); err != nil {
			slog.Error("freeze check", "error", err)
			os.Exit(1)
		}
		fmt.Println("freeze check ok")
		return
	}

	direction := "up"
	if args := flag.Args(); len(args) > 0 {
		direction = args[0]
	}

	cfg, err := studiodb.LoadConfig()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	// Destructive down must honor the same dual-signal live model as write-freeze
	// (env truthy OR migration-root marker). Break-glass remains env-only/auditable
	// and does not enable manifest writes.
	studiodb.ApplyLiveMarker(&cfg, migDir)

	ctx := context.Background()
	switch direction {
	case "up":
		err = studiodb.Migrate(ctx, cfg.DatabaseURL)
	case "down":
		err = studiodb.Studio.DownWithPolicy(ctx, cfg.DatabaseURL, cfg)
	case "status":
		st, stErr := studiodb.Studio.Status(ctx, cfg.DatabaseURL)
		if stErr != nil {
			err = stErr
			break
		}
		for _, row := range st {
			fmt.Printf("%v	%v	%s\n", row.Source.Version, row.State, row.Source.Path)
		}
		return
	default:
		slog.Error("usage: migrate [-check-freeze|-write-freeze] [up|down|status]")
		os.Exit(2)
	}
	if err != nil {
		slog.Error("migrate", "direction", direction, "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied", "direction", direction)
}

func defaultMigrationsDir() string {
	// Prefer cwd-relative curriculum-studio layout when run from module root.
	candidates := []string{
		filepath.Join("db", "migrations"),
		filepath.Join("curriculum-studio", "db", "migrations"),
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		// cmd/migrate -> ../../db/migrations
		candidates = append([]string{filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")}, candidates...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return filepath.Join("db", "migrations")
}
