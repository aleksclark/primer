package db

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ForbiddenDBNames are database names Identity refuses by default so an
// operator cannot accidentally apply Identity migrations or open pools against
// LMS/TV/Studio DBs. Comparison is case-insensitive after normalization.
//
// Includes production and test aliases observed in the monorepo (including
// foreign test names such as curriculum_studio_test / primer_tv_test / tv_test
// that previously slipped past config-only gates).
var ForbiddenDBNames = []string{
	"primer",
	"primer_test",
	"primer_tv",
	"primer_tv_test",
	"tv",
	"tv_test",
	"curriculum_studio",
	"curriculum_studio_test",
	"studio",
}

// AllowedIdentityDBNames are the canonical Identity product/test database names.
// Library ValidateDatabaseURL does not require the name to be in this list
// (non-reserved ephemeral names are allowed and documented); the test harness
// override path is stricter and only accepts Identity-safe names.
var AllowedIdentityDBNames = []string{
	"primer_identity",
	"primer_identity_test",
}

// ParseDatabaseName extracts the PostgreSQL database name from a DSN using the
// same pgxpool config path used for real connections. This covers URI,
// keyword/libpq, double-slash path, and case/space forms that naive url.Parse misses.
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
// open pools or run migrations must use this so library entrypoints cannot
// bypass CLI/config guards.
//
// Policy:
//   - Empty / unparseable / missing DB name → error
//   - Name in ForbiddenDBNames → error ("forbidden")
//   - primer_identity / primer_identity_test → allowed
//   - Other non-reserved names (e.g. identity_scratch_42) → allowed at the
//     library/config boundary so disposable operator DBs work; the integration
//     test harness override path additionally requires an Identity-safe name.
func ValidateDatabaseURL(dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("database url is required")
	}
	name, err := ParseDatabaseName(dsn)
	if err != nil {
		return err
	}
	if IsForbiddenDBName(name) {
		return fmt.Errorf("refusing Identity connect/migrate against forbidden database name %q", name)
	}
	return nil
}

// IsForbiddenDBName reports whether name (already or not yet normalized) is on
// the Identity isolation deny list.
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

// IsIdentitySafeTestDBName reports whether name is acceptable for the
// IDENTITY_TEST_DATABASE_URL harness override (canonical Identity names or
// primer_identity* prefix that is not otherwise forbidden).
func IsIdentitySafeTestDBName(name string) bool {
	n := normalizeDatabaseName(name)
	if n == "" {
		return false
	}
	for _, allowed := range AllowedIdentityDBNames {
		if n == normalizeDatabaseName(allowed) {
			return true
		}
	}
	if strings.HasPrefix(n, "primer_identity") {
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
