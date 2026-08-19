// Package testutil provides the primer-agents integration-test harness:
// a PostgreSQL testcontainer (or PRIMER_AGENTS_TEST_DATABASE_URL) migrated
// with the agents migrator, plus a per-test rollback helper.
//
// Ambient bare TEST_DATABASE_URL / DATABASE_URL / STUDIO_* / IDENTITY_* / TV_*
// are never inherited.
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

	agentsdb "github.com/aleksclark/primer/agents/internal/db"
)

// TestDatabaseEnv is the only external DSN override the agents harness honors.
const TestDatabaseEnv = "PRIMER_AGENTS_TEST_DATABASE_URL"

// Harness owns a migrated PostgreSQL pool for the agents schema.
type Harness struct {
	Migrate func(ctx context.Context, url string) error
	DBName  string

	once sync.Once
	err  error
	pool *pgxpool.Pool
	url  string
}

var agents = &Harness{Migrate: agentsdb.Migrate, DBName: "primer_agents_test"}

// DB returns a migrated connection pool backed by a shared PostgreSQL
// testcontainer (or PRIMER_AGENTS_TEST_DATABASE_URL if set).
func DB(t *testing.T) *pgxpool.Pool { return agents.DB(t) }

// Tx begins a transaction on the shared pool and registers rollback on cleanup.
func Tx(t *testing.T) pgx.Tx { return agents.Tx(t) }

// URL returns the DSN used by the shared harness (after first DB() call).
func URL(t *testing.T) string {
	t.Helper()
	_ = agents.DB(t)
	return agents.url
}

// ResolveTestDatabaseURL returns the external agents test DSN when
// PRIMER_AGENTS_TEST_DATABASE_URL is set. Unset means testcontainer path.
// Ambient bare TEST_DATABASE_URL / DATABASE_URL / STUDIO_* / IDENTITY_* / TV_*
// are never read.
func ResolveTestDatabaseURL() (url string, usedExternal bool, err error) {
	raw := strings.TrimSpace(os.Getenv(TestDatabaseEnv))
	if raw == "" {
		return "", false, nil
	}
	if err := agentsdb.ValidateDatabaseURL(raw); err != nil {
		return "", false, fmt.Errorf("%s: %w", TestDatabaseEnv, err)
	}
	name, err := agentsdb.ParseDatabaseName(raw)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", TestDatabaseEnv, err)
	}
	if !agentsdb.IsAgentsSafeTestDBName(name) {
		return "", false, fmt.Errorf("%s: database name %q is not an agents-safe test name (use primer_agents or primer_agents_test)", TestDatabaseEnv, name)
	}
	return raw, true, nil
}

func (h *Harness) DB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	h.once.Do(func() { h.err = h.setup() })
	if h.err != nil {
		t.Fatalf("test database setup: %v", h.err)
	}
	return h.pool
}

func (h *Harness) Tx(t *testing.T) pgx.Tx {
	t.Helper()
	p := h.DB(t)
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
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
			tcpostgres.WithUsername("agents"),
			tcpostgres.WithPassword("agents"),
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
	p, err := agentsdb.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect test db: %w", err)
	}
	h.pool = p
	return nil
}
