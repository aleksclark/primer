package db

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// VersionTable is the goose version bookkeeping table for Curriculum Studio.
// It must never collide with LMS/TV default goose_db_version.
const VersionTable = "studio_goose_db_version"

// SchemaName is the PostgreSQL schema that owns all Studio domain tables.
const SchemaName = "curriculum_studio"

// ForbiddenDBNames are database names Studio migrate refuses by default so an
// operator cannot accidentally apply Studio migrations onto LMS/TV DBs.
var ForbiddenDBNames = []string{
	"primer",
	"primer_test",
	"primer_tv",
	"primer_tv_test",
	"tv",
	"tv_test",
}

// Config is the minimal Studio database configuration used by migrate and the
// connection pool (Phase 2).
type Config struct {
	// DatabaseURL is the Studio PostgreSQL DSN (STUDIO_DATABASE_URL only).
	DatabaseURL string
	// MaxConns is the pgx pool max connections (0 = driver default).
	MaxConns int32
	// MigrationsLive marks an environment where destructive down migrations
	// are refused unless BreakGlassDown is also set.
	MigrationsLive bool
	// BreakGlassDown permits a single down step on a live-classified env.
	// Documented in MIGRATION_POLICY.md; never set by default.
	BreakGlassDown bool
	// GuardForbiddenDBNames enables refusal when the DSN database name is in
	// ForbiddenDBNames (default true for migrate).
	GuardForbiddenDBNames bool
}

// LoadConfig reads Studio DB config exclusively from Studio-prefixed env vars.
// It never falls back to DATABASE_URL / LMS / TV settings.
func LoadConfig() (Config, error) {
	cfg := Config{
		DatabaseURL:           strings.TrimSpace(os.Getenv("STUDIO_DATABASE_URL")),
		GuardForbiddenDBNames: true,
	}
	if v := strings.TrimSpace(os.Getenv("STUDIO_DB_MAX_CONNS")); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("STUDIO_DB_MAX_CONNS: invalid value %q", v)
		}
		cfg.MaxConns = int32(n)
	}
	if truthy(os.Getenv("STUDIO_MIGRATIONS_LIVE")) {
		cfg.MigrationsLive = true
	}
	if truthy(os.Getenv("STUDIO_MIGRATE_BREAK_GLASS_DOWN")) {
		cfg.BreakGlassDown = true
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks required fields and optional DSN isolation guards.
func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("STUDIO_DATABASE_URL is required (no fallback to DATABASE_URL)")
	}
	if c.GuardForbiddenDBNames {
		name, err := databaseName(c.DatabaseURL)
		if err != nil {
			return fmt.Errorf("parse STUDIO_DATABASE_URL: %w", err)
		}
		for _, banned := range ForbiddenDBNames {
			if strings.EqualFold(name, banned) {
				return fmt.Errorf("refusing Studio migrate against forbidden database name %q", name)
			}
		}
	}
	return nil
}

// AllowDown reports whether a destructive down migration is permitted.
func (c Config) AllowDown() bool {
	if !c.MigrationsLive {
		return true
	}
	return c.BreakGlassDown
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func databaseName(dsn string) (string, error) {
	// Accept both URL form and key=value libpq form.
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		name := strings.TrimPrefix(u.Path, "/")
		if i := strings.IndexByte(name, '?'); i >= 0 {
			name = name[:i]
		}
		if name == "" {
			return "", fmt.Errorf("database name missing in URL path")
		}
		return name, nil
	}
	// libpq keywords
	for _, part := range strings.Fields(dsn) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && kv[0] == "dbname" {
			return kv[1], nil
		}
	}
	return "", fmt.Errorf("could not determine database name from DSN")
}
