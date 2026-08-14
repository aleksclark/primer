package db

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ForbiddenDBNames are database names Studio refuses by default so an
// operator cannot accidentally apply Studio migrations or open pools against
// LMS/TV/Identity DBs. Comparison is case-insensitive after normalization.
var ForbiddenDBNames = []string{
	"primer",
	"primer_test",
	"primer_tv",
	"primer_tv_test",
	"tv",
	"tv_test",
	"primer_identity",
	"primer_identity_test",
}

// AllowedStudioDBNames are the canonical Studio product/test database names.
// Validation does not require the name to be in this list (custom names are
// fine); it only refuses ForbiddenDBNames. Tests may assert these remain free.
var AllowedStudioDBNames = []string{
	"curriculum_studio",
	"curriculum_studio_test",
}

// ParseDatabaseName extracts the PostgreSQL database name from a DSN using the
// same pgxpool config path used for real connections. This covers URI,
// keyword/libpq, double-slash path, and case forms that naive url.Parse misses.
func ParseDatabaseName(dsn string) (string, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	name := normalizeDatabaseName(cfg.ConnConfig.Database)
	if name == "" {
		return "", fmt.Errorf("database name missing in DSN")
	}
	return name, nil
}

// ValidateDatabaseURL refuses empty DSNs and ForbiddenDBNames. Callers that
// open pools or run migrations must use this (or Config.Validate) so library
// entrypoints cannot bypass CLI/config guards.
func ValidateDatabaseURL(dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("database url is required")
	}
	name, err := ParseDatabaseName(dsn)
	if err != nil {
		return err
	}
	if IsForbiddenDBName(name) {
		return fmt.Errorf("refusing Studio connect/migrate against forbidden database name %q", name)
	}
	return nil
}

// IsForbiddenDBName reports whether name (already or not yet normalized) is on
// the Studio isolation deny list.
func IsForbiddenDBName(name string) bool {
	n := normalizeDatabaseName(name)
	if n == "" {
		return false
	}
	for _, banned := range ForbiddenDBNames {
		if n == normalizeDatabaseName(banned) {
			return true
		}
	}
	return false
}

func normalizeDatabaseName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "/")
	name = strings.TrimSpace(name)
	return strings.ToLower(name)
}
