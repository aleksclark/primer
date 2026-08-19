package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	agentsdbschema "github.com/aleksclark/primer/agents/db"
)

// VersionTable is the goose version bookkeeping table for the agents service.
// It must never collide with LMS/TV/Identity/Studio goose tables.
const VersionTable = "agents_goose_db_version"

// Migrator applies embedded goose migrations against an agents database,
// tracking versions in VersionTable.
type Migrator struct {
	fsys  fs.FS
	table string
}

// NewMigrator builds a Migrator. fsys must contain a "migrations" directory of
// goose SQL files. Empty table uses goose default (not recommended).
func NewMigrator(fsys fs.FS, table string) *Migrator {
	return &Migrator{fsys: fsys, table: table}
}

// Agents is the canonical agents service migrator.
var Agents = NewMigrator(agentsdbschema.MigrationsFS, VersionTable)

// Migrate applies all pending agents up migrations.
// Forbidden database names are always refused.
func Migrate(ctx context.Context, databaseURL string) error {
	if err := ValidateDatabaseURL(databaseURL); err != nil {
		return err
	}
	return Agents.Up(ctx, databaseURL)
}

// Up applies all pending up migrations after DSN isolation checks.
func (m *Migrator) Up(ctx context.Context, databaseURL string) error {
	if err := ValidateDatabaseURL(databaseURL); err != nil {
		return err
	}
	return m.with(ctx, databaseURL, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// CurrentVersion returns the highest applied goose version, or 0 if none.
func (m *Migrator) CurrentVersion(ctx context.Context, databaseURL string) (int64, error) {
	if err := ValidateDatabaseURL(databaseURL); err != nil {
		return 0, err
	}
	var ver int64
	err := m.with(ctx, databaseURL, func(p *goose.Provider) error {
		v, err := p.GetDBVersion(ctx)
		if err != nil {
			return err
		}
		ver = v
		return nil
	})
	return ver, err
}

func (m *Migrator) with(ctx context.Context, databaseURL string, fn func(*goose.Provider) error) error {
	if m == nil || m.fsys == nil {
		return fmt.Errorf("migrator: nil filesystem")
	}
	sqlDB, err := sql.Open("pgx", databaseURL)
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
