package parent

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This test uses the repository's explicit TASKS_TEST_DATABASE_URL when a
// Postgres harness is present. It does not create a database or migration; the
// table is a fixture for the store contract and is removed by the test.
func TestSQLConfirmationStoreSingleUseAndBindings(t *testing.T) {
	dsn := os.Getenv("TASKS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TASKS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ConfirmationTableSQL); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DROP TABLE IF EXISTS parent_confirmation_previews`)
	_, _ = pool.Exec(ctx, `DELETE FROM parent_confirmation_previews`)

	now := time.Date(2027, 2, 3, 4, 5, 6, 0, time.UTC)
	store := &SQLConfirmationStore{DB: pool, Now: func() time.Time { return now }}
	p, err := store.Issue(ctx, ConfirmationPreview{TenantID: "tenant-a", ActorID: "parent-a", Action: RetireTaskAction("task-1", 2), ExpiresAt: now.Add(5 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Consume(ctx, p.Handle, "tenant-b", "parent-a", p.ActionDigest); !errors.Is(err, ErrConfirmationForeign) {
		t.Fatalf("foreign=%v", err)
	}
	if _, err = store.Consume(ctx, p.Handle, "tenant-a", "parent-a", "00"); !errors.Is(err, ErrConfirmationAltered) {
		t.Fatalf("altered=%v", err)
	}
	if _, err = store.Consume(ctx, p.Handle, "tenant-a", "parent-a", p.ActionDigest); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Consume(ctx, p.Handle, "tenant-a", "parent-a", p.ActionDigest); !errors.Is(err, ErrConfirmationReplay) {
		t.Fatalf("replay=%v", err)
	}
}
