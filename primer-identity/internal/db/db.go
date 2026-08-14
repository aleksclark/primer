// Package db owns the Identity service's PostgreSQL access and embedded goose
// migrations. The goose version table is identity-specific so Identity never
// shares migration bookkeeping with LMS, TV, or Studio.
//
// DSN isolation: Connect, Migrator.with (therefore Up/Down), Migrate, and
// MigrateDown all call ValidateDatabaseURL before any dial or goose work so
// library callers cannot bypass config/CLI gates.
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

// VersionTable is the goose bookkeeping table for the Identity schema.
const VersionTable = "identity_goose_db_version"

// Connect opens a pgx connection pool to the given database URL and pings it.
// Forbidden LMS/TV/Studio database names are always refused before dial.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if err := ValidateDatabaseURL(url); err != nil {
		return nil, err
	}
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

// Migrator applies embedded goose migrations against a dedicated version table.
type Migrator struct {
	fsys  fs.FS
	table string
}

// NewMigrator builds a Migrator over the migrations directory of fsys.
func NewMigrator(fsys fs.FS, table string) *Migrator {
	return &Migrator{fsys: fsys, table: table}
}

// Up applies all pending up migrations against the given database URL.
// Forbidden database names are refused before any goose work.
func (m *Migrator) Up(ctx context.Context, url string) error {
	return m.with(ctx, url, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// Down rolls back a single migration.
// Forbidden database names are refused before any goose work.
func (m *Migrator) Down(ctx context.Context, url string) error {
	return m.with(ctx, url, func(p *goose.Provider) error {
		_, err := p.Down(ctx)
		return err
	})
}

func (m *Migrator) with(ctx context.Context, url string, fn func(*goose.Provider) error) error {
	if err := ValidateDatabaseURL(url); err != nil {
		return err
	}
	if m == nil || m.fsys == nil {
		return fmt.Errorf("migrator: nil filesystem")
	}
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

var migrator = NewMigrator(migrationsFS, VersionTable)

// Migrate applies all pending Identity up migrations.
// Forbidden LMS/TV/Studio database names are always refused.
func Migrate(ctx context.Context, url string) error { return migrator.Up(ctx, url) }

// MigrateDown rolls back a single Identity migration.
// Forbidden LMS/TV/Studio database names are always refused.
func MigrateDown(ctx context.Context, url string) error { return migrator.Down(ctx, url) }
