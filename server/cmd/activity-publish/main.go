// Command activity-publish upserts curriculum standards and publishes immutable
// activity revisions from curriculum/ into the LMS database.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aleksclark/primer/server/internal/config"
	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	activities := flag.String("activities", "", "path to curriculum/activities (default: ../curriculum/activities from cwd)")
	standards := flag.String("standards", "", "path to curriculum/standards")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	actDir, stdDir := *activities, *standards
	if actDir == "" || stdDir == "" {
		root := findRepoRoot()
		if actDir == "" {
			actDir = filepath.Join(root, "curriculum", "activities")
		}
		if stdDir == "" {
			stdDir = filepath.Join(root, "curriculum", "standards")
		}
	}

	res, err := curriculum.Publish(ctx, pool, curriculum.PublishOptions{
		ActivitiesDir: actDir,
		StandardsDir:  stdDir,
		Now:           time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	fmt.Printf("published: standards=%d activities=%d revisions=%d\n",
		res.StandardsUpserted, res.Activities, res.Revisions)
	return nil
}

func findRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := wd
	for {
		if st, err := os.Stat(filepath.Join(dir, "curriculum", "activities")); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd
		}
		dir = parent
	}
}
