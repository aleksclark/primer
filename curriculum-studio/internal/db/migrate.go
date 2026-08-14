package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	studiodbschema "github.com/aleksclark/primer/curriculum-studio/db"
)

// Migrator applies embedded goose migrations against a Studio database,
// tracking versions in VersionTable (studio_goose_db_version).
type Migrator struct {
	fsys  fs.FS
	table string
}

// NewMigrator builds a Migrator. fsys must contain a "migrations" directory of
// goose SQL files. Empty table uses goose default (not recommended for Studio).
func NewMigrator(fsys fs.FS, table string) *Migrator {
	return &Migrator{fsys: fsys, table: table}
}

// Studio is the canonical Curriculum Studio migrator. SQL sources live only
// under curriculum-studio/db/migrations (embedded via studiodbschema).
var Studio = NewMigrator(studiodbschema.MigrationsFS, VersionTable)

// Migrate applies all pending Studio up migrations.
func Migrate(ctx context.Context, databaseURL string) error {
	return Studio.Up(ctx, databaseURL)
}

// Up applies all pending up migrations.
func (m *Migrator) Up(ctx context.Context, databaseURL string) error {
	return m.with(ctx, databaseURL, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// down rolls back a single migration. Unexported so callers cannot bypass
// DownWithPolicy / Config.AllowDown live guards.
func (m *Migrator) down(ctx context.Context, databaseURL string) error {
	return m.with(ctx, databaseURL, func(p *goose.Provider) error {
		_, err := p.Down(ctx)
		return err
	})
}

// Status returns applied migration version rows (goose provider status).
func (m *Migrator) Status(ctx context.Context, databaseURL string) ([]*goose.MigrationStatus, error) {
	var out []*goose.MigrationStatus
	err := m.with(ctx, databaseURL, func(p *goose.Provider) error {
		st, err := p.Status(ctx)
		if err != nil {
			return err
		}
		out = st
		return nil
	})
	return out, err
}

// CurrentVersion returns the highest applied goose version, or 0 if none.
func (m *Migrator) CurrentVersion(ctx context.Context, databaseURL string) (int64, error) {
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

// DownWithPolicy applies one down step only when cfg.AllowDown() is true.
// This is the sole exported destructive down entrypoint for Studio.
func (m *Migrator) DownWithPolicy(ctx context.Context, databaseURL string, cfg Config) error {
	if !cfg.AllowDown() {
		return fmt.Errorf("refusing migrate down: STUDIO_MIGRATIONS_LIVE is set (break-glass: STUDIO_MIGRATE_BREAK_GLASS_DOWN=true)")
	}
	return m.down(ctx, databaseURL)
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
