// Package db owns the TV server's schema. Its migrations track applied
// versions in a dedicated table so the TV schema can share a PostgreSQL
// instance with the LMS without either service disturbing the other's
// migration history.
package db

import (
	"context"
	"embed"

	"github.com/aleksclark/primer/server/internal/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// VersionTable is the goose bookkeeping table for the TV schema.
const VersionTable = "tv_goose_db_version"

// migrator applies the embedded TV migrations.
var migrator = db.NewMigrator(migrationsFS, VersionTable)

// Migrate applies all pending TV up migrations.
func Migrate(ctx context.Context, url string) error { return migrator.Up(ctx, url) }

// MigrateDown rolls back a single TV migration.
func MigrateDown(ctx context.Context, url string) error { return migrator.Down(ctx, url) }
