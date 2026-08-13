// Command course-publish loads a course.json and publishes an immutable
// curriculum revision into the LMS database. Activity revisions must already
// exist (publish activities first via activity-publish).
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
	course := flag.String("course", "", "path to course.json (default: content/technology/basic_linux/course.json)")
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

	path := *course
	if path == "" {
		path = filepath.Join(findRepoRoot(), "content", "technology", "basic_linux", "course.json")
	}

	res, err := curriculum.PublishCourse(ctx, pool, path, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Printf("published course: slug=%s curriculum=%s revision=%d activities=%d\n",
		res.Curriculum.Slug, res.Curriculum.ID, res.Revision.Revision, res.Activities)
	return nil
}

func findRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := wd
	for {
		if st, err := os.Stat(filepath.Join(dir, "content", "technology", "basic_linux")); err == nil && st.IsDir() {
			return dir
		}
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
