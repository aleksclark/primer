// Package agentsdbschema owns the primer-agents SQL migration files as an
// embeddable filesystem. The Go migrator in internal/db consumes this FS so
// the SQL under db/migrations remains the single source of truth.
package agentsdbschema

import "embed"

// MigrationsFS contains goose SQL files under migrations/*.sql.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
