// Package studiodbschema owns the Curriculum Studio SQL migration files as an
// embeddable filesystem. The Go migrator in internal/db consumes this FS so the
// SQL under db/migrations remains the single source of truth (no duplicated
// copies under internal/).
package studiodbschema

import "embed"

// MigrationsFS contains goose SQL files under migrations/*.sql.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
