// Package testutil provides the Curriculum Studio integration-test harness:
// a PostgreSQL testcontainer (or STUDIO_TEST_DATABASE_URL) migrated with the
// Studio migrator, plus per-test transaction rollback and savepoint wrappers.
//
// Pattern mirrors server/internal/testutil but is Studio-owned and never
// applies LMS migrations. Ambient bare TEST_DATABASE_URL / DATABASE_URL are
// never inherited.
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

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

// TestDatabaseEnv is the only external DSN override the Studio harness honors.
const TestDatabaseEnv = "STUDIO_TEST_DATABASE_URL"

// Harness owns a migrated PostgreSQL pool for the Studio schema.
type Harness struct {
	// Migrate applies the schema under test to a database URL.
	Migrate func(ctx context.Context, url string) error
	// DBName is the database created inside the container.
	DBName string

	once sync.Once
	err  error
	pool *pgxpool.Pool
	url  string
}

// studio is the shared harness for Curriculum Studio tests.
var studio = &Harness{Migrate: studiodb.Migrate, DBName: "curriculum_studio_test"}

// DB returns a migrated connection pool backed by a shared PostgreSQL
// testcontainer (or STUDIO_TEST_DATABASE_URL if set).
func DB(t *testing.T) *pgxpool.Pool { return studio.DB(t) }

// Tx begins a transaction on the shared Studio pool and registers rollback.
func Tx(t *testing.T) pgx.Tx { return studio.Tx(t) }

// URL returns the DSN used by the shared harness (after first DB() call).
func URL(t *testing.T) string {
	t.Helper()
	_ = studio.DB(t)
	return studio.url
}

// ResolveTestDatabaseURL returns the external Studio test DSN when
// STUDIO_TEST_DATABASE_URL is set. Unset means the caller should start a
// testcontainer (empty url, usedExternal=false). Ambient bare
// TEST_DATABASE_URL / DATABASE_URL / IDENTITY_* are never read.
// Supplied external DSNs must pass Studio isolation and use a Studio-safe name.
func ResolveTestDatabaseURL() (url string, usedExternal bool, err error) {
	raw := strings.TrimSpace(os.Getenv(TestDatabaseEnv))
	if raw == "" {
		return "", false, nil
	}
	if err := studiodb.ValidateDatabaseURL(raw); err != nil {
		return "", false, fmt.Errorf("%s: %w", TestDatabaseEnv, err)
	}
	name, err := studiodb.ParseDatabaseName(raw)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", TestDatabaseEnv, err)
	}
	if !isStudioSafeTestDBName(name) {
		return "", false, fmt.Errorf("%s: database name %q is not a Studio-safe test name (use curriculum_studio or curriculum_studio_test)", TestDatabaseEnv, name)
	}
	return raw, true, nil
}

func isStudioSafeTestDBName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, allowed := range studiodb.AllowedStudioDBNames {
		if n == strings.ToLower(allowed) {
			return true
		}
	}
	if strings.HasPrefix(n, "curriculum_studio") {
		return !studiodb.IsForbiddenDBName(n)
	}
	return false
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

// Tx begins a transaction on the harness pool and registers a rollback on
// test cleanup. Repositories accept it via repo.Querier.
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
		container, err := tcpostgres.Run(ctx,
			"postgres:17-alpine",
			tcpostgres.WithDatabase(h.DBName),
			tcpostgres.WithUsername("studio"),
			tcpostgres.WithPassword("studio"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).WithStartupTimeout(60*time.Second),
			),
		)
		if err != nil {
			return fmt.Errorf("start postgres container: %w", err)
		}
		url, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			return fmt.Errorf("container connection string: %w", err)
		}
	}
	h.url = url

	if err := h.Migrate(ctx, url); err != nil {
		return fmt.Errorf("migrate test db: %w", err)
	}

	p, err := studiodb.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect test db: %w", err)
	}
	h.pool = p
	return nil
}
