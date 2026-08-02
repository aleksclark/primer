package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // register "pgx" sql driver
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect opens a pgx connection pool to the given database URL.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrator applies a set of embedded goose migrations, tracking applied
// versions in its own table. Each service owns a Migrator so independent
// schemas can live in the same PostgreSQL instance without sharing version
// bookkeeping.
type Migrator struct {
	fsys  fs.FS
	table string
}

// NewMigrator builds a Migrator over the migrations directory of fsys. The
// table name records applied versions; pass an empty string for the goose
// default.
func NewMigrator(fsys fs.FS, table string) *Migrator {
	return &Migrator{fsys: fsys, table: table}
}

// Up applies all pending up migrations against the given database URL.
func (m *Migrator) Up(ctx context.Context, url string) error {
	return m.with(ctx, url, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// Down rolls back a single migration. Used primarily in tooling/tests.
func (m *Migrator) Down(ctx context.Context, url string) error {
	return m.with(ctx, url, func(p *goose.Provider) error {
		_, err := p.Down(ctx)
		return err
	})
}

func (m *Migrator) with(ctx context.Context, url string, fn func(*goose.Provider) error) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open sql db: %w", err)
	}
	defer sqlDB.Close()

	migrations, err := fs.Sub(m.fsys, "migrations")
	if err != nil {
		return fmt.Errorf("sub fs: %w", err)
	}
	var opts []goose.ProviderOption
	if m.table != "" {
		opts = append(opts, goose.WithTableName(m.table))
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations, opts...)
	if err != nil {
		return fmt.Errorf("new goose provider: %w", err)
	}
	if err := fn(provider); err != nil {
		return fmt.Errorf("run migration: %w", err)
	}
	return nil
}

// LMS is the migrator for the LMS schema.
var LMS = NewMigrator(migrationsFS, "")

// Migrate applies all pending LMS up migrations.
func Migrate(ctx context.Context, url string) error { return LMS.Up(ctx, url) }

// MigrateDown rolls back a single LMS migration.
func MigrateDown(ctx context.Context, url string) error { return LMS.Down(ctx, url) }
