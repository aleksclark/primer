// Package testutil provides the Identity integration-test harness: a
// PostgreSQL testcontainer with DB name primer_identity_test and Identity
// migrations only. It never loads LMS/TV/Studio migrations.
//
// IDENTITY_TEST_DATABASE_URL is the only external DSN override. Ambient bare
// TEST_DATABASE_URL / DATABASE_URL are never inherited. Foreign overrides fail
// loudly; unset uses a real testcontainer.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/db"
)

// TestDatabaseEnv is the only external DSN override the Identity harness honors.
const TestDatabaseEnv = "IDENTITY_TEST_DATABASE_URL"

// DBName is the Identity-specific test database name.
const DBName = "primer_identity_test"

// Harness owns a migrated PostgreSQL pool for the Identity schema.
type Harness struct {
	Migrate func(ctx context.Context, url string) error
	DBName  string

	once sync.Once
	err  error
	pool *pgxpool.Pool
	url  string
}

// identity is the package-level harness for Identity tests.
var identity = &Harness{Migrate: db.Migrate, DBName: DBName}

// DB returns a migrated connection pool backed by a shared PostgreSQL
// testcontainer (or IDENTITY_TEST_DATABASE_URL if set). Docker is required
// when the env override is unset — there is no in-memory substitute.
func DB(t *testing.T) *pgxpool.Pool { return identity.DB(t) }

// DatabaseURL returns the live Identity test database URL.
func DatabaseURL(t *testing.T) string {
	t.Helper()
	_ = identity.DB(t)
	return identity.url
}

// Tx begins a transaction on the shared Identity pool and rolls it back on cleanup.
func Tx(t *testing.T) pgx.Tx { return identity.Tx(t) }

// ResolveTestDatabaseURL returns the external Identity test DSN when
// IDENTITY_TEST_DATABASE_URL is set. Unset means the caller should start a
// testcontainer (empty url, usedExternal=false). Ambient bare
// TEST_DATABASE_URL / DATABASE_URL / STUDIO_* are never read.
// Supplied external DSNs must pass Identity isolation and use an Identity-safe name.
func ResolveTestDatabaseURL() (url string, usedExternal bool, err error) {
	raw := strings.TrimSpace(os.Getenv(TestDatabaseEnv))
	if raw == "" {
		return "", false, nil
	}
	if err := db.ValidateDatabaseURL(raw); err != nil {
		return "", false, fmt.Errorf("%s: %w", TestDatabaseEnv, err)
	}
	name, err := db.ParseDatabaseName(raw)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", TestDatabaseEnv, err)
	}
	if !db.IsIdentitySafeTestDBName(name) {
		return "", false, fmt.Errorf("%s: database name %q is not an Identity-safe test name (use primer_identity or primer_identity_test)", TestDatabaseEnv, name)
	}
	return raw, true, nil
}

// DB returns the harness's migrated pool, starting the container on first use.
func (h *Harness) DB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	h.once.Do(func() { h.err = h.setup() })
	if h.err != nil {
		t.Fatalf("test database setup: %v", h.err)
	}
	return h.pool
}

// Tx begins a transaction on the harness pool and registers a rollback on cleanup.
func (h *Harness) Tx(t *testing.T) pgx.Tx {
	t.Helper()
	p := h.DB(t)
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})
	return tx
}

func (h *Harness) setup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	url, usedExternal, err := ResolveTestDatabaseURL()
	if err != nil {
		return err
	}
	if !usedExternal {
		// Fail closed: require Docker rather than silently skipping.
		container, err := tcpostgres.Run(ctx,
			"postgres:17-alpine",
			tcpostgres.WithDatabase(h.DBName),
			tcpostgres.WithUsername("primer"),
			tcpostgres.WithPassword("primer"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).WithStartupTimeout(60*time.Second),
			),
		)
		if err != nil {
			return fmt.Errorf("start postgres container (Docker required for Identity durability tests): %w", err)
		}
		url, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			return fmt.Errorf("container connection string: %w", err)
		}
	}

	if err := h.Migrate(ctx, url); err != nil {
		return fmt.Errorf("migrate test db: %w", err)
	}

	p, err := db.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect test db: %w", err)
	}
	h.pool = p
	h.url = url
	return nil
}
