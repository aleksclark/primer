// Package testutil provides the integration-test harness: a PostgreSQL
// testcontainer shared per test binary, with each test running inside its
// own transaction that is rolled back on cleanup. This keeps tests fast,
// isolated, and safe to run concurrently.
package testutil

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/server/internal/db"
)

// Harness owns a migrated PostgreSQL pool for one schema. Services define a
// package-level Harness so the LMS and TV schemas can be exercised
// independently while sharing this container bootstrap.
type Harness struct {
	// Migrate applies the schema under test to a database URL.
	Migrate func(ctx context.Context, url string) error
	// DBName is the database created inside the container.
	DBName string

	once sync.Once
	err  error
	pool *pgxpool.Pool
}

// lms is the harness for the LMS schema.
var lms = &Harness{Migrate: db.Migrate, DBName: "primer_test"}

// DB returns a migrated connection pool backed by a shared PostgreSQL
// testcontainer (or TEST_DATABASE_URL if set). The first caller pays the
// startup cost; subsequent callers reuse the pool.
func DB(t *testing.T) *pgxpool.Pool { return lms.DB(t) }

// Tx begins a transaction on the shared LMS pool and registers a rollback on
// test cleanup.
func Tx(t *testing.T) pgx.Tx { return lms.Tx(t) }

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
// test cleanup. Repositories accept it via the repo.Querier interface, so all
// writes made through it vanish when the test ends.
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

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
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
			return fmt.Errorf("start postgres container: %w", err)
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
	return nil
}
