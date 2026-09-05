package db

import (
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tasks_schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		var done bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks_schema_migrations WHERE version=$1)`, e.Name()).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		b, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(b)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO tasks_schema_migrations(version) VALUES($1)`, e.Name())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", e.Name(), err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func DSN() string {
	if v := os.Getenv("TASKS_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://tasks@localhost:5432/primer_tasks?sslmode=disable"
}
func SafeDatabaseName(dsn string) error {
	for _, bad := range []string{"lms", "tv", "studio", "identity"} {
		if strings.Contains(strings.ToLower(dsn), bad) {
			return fmt.Errorf("unsafe Tasks database name")
		}
	}
	return nil
}
