package db

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ForbiddenDBNames are database names the agents service refuses by default so
// an operator cannot accidentally apply agents migrations or open pools against
// LMS, TV, Identity, or Studio DBs. Comparison is case-insensitive after
// normalization.
var ForbiddenDBNames = []string{
	"primer",
	"primer_test",
	"primer_tv",
	"primer_tv_test",
	"tv",
	"tv_test",
	"primer_identity",
	"primer_identity_test",
	"curriculum_studio",
	"curriculum_studio_test",
	"studio",
}

// AllowedAgentsDBNames are the canonical agents product/test database names.
// ValidateDatabaseURL does not require the name to be in this list (custom
// non-reserved names are fine); it only refuses ForbiddenDBNames.
var AllowedAgentsDBNames = []string{
	"primer_agents",
	"primer_agents_test",
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

// ValidateDatabaseURL refuses empty DSNs and ForbiddenDBNames. Every
// entrypoint that opens a pool or runs migrations must call this so library
// callers cannot bypass config/CLI guards.
func ValidateDatabaseURL(dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("database url is required")
	}
	name, err := ParseDatabaseName(dsn)
	if err != nil {
		return err
	}
	if IsForbiddenDBName(name) {
		return fmt.Errorf("refusing agents connect/migrate against forbidden database name %q", name)
	}
	return nil
}

// IsForbiddenDBName reports whether name (already normalized or not) is on
// the agents isolation deny list.
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

// IsAgentsSafeTestDBName reports whether name is acceptable for the
// PRIMER_AGENTS_TEST_DATABASE_URL harness override.
func IsAgentsSafeTestDBName(name string) bool {
	n := normalizeDatabaseName(name)
	if n == "" {
		return false
	}
	for _, allowed := range AllowedAgentsDBNames {
		if n == normalizeDatabaseName(allowed) {
			return true
		}
	}
	if strings.HasPrefix(n, "primer_agents") {
		return !IsForbiddenDBName(n)
	}
	return false
}

func normalizeDatabaseName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "/")
	name = strings.TrimSpace(name)
	return strings.ToLower(name)
}
