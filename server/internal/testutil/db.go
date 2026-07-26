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

var (
	setupOnce sync.Once
	setupErr  error
	pool      *pgxpool.Pool
)

// DB returns a migrated connection pool backed by a shared PostgreSQL
// testcontainer (or TEST_DATABASE_URL if set). The first caller pays the
// startup cost; subsequent callers reuse the pool.
func DB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	setupOnce.Do(func() { setupErr = setup() })
	if setupErr != nil {
		t.Fatalf("test database setup: %v", setupErr)
	}
	return pool
}

// Tx begins a transaction on the shared pool and registers a rollback on
// test cleanup. Repositories accept it via the repo.Querier interface, so
// all writes made through it vanish when the test ends.
func Tx(t *testing.T) pgx.Tx {
	t.Helper()
	p := DB(t)
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

func setup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		container, err := tcpostgres.Run(ctx,
			"postgres:17-alpine",
			tcpostgres.WithDatabase("primer_test"),
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

	if err := db.Migrate(ctx, url); err != nil {
		return fmt.Errorf("migrate test db: %w", err)
	}

	p, err := db.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect test db: %w", err)
	}
	pool = p
	return nil
}
