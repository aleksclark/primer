package repo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

// P2-E1: Connect+Ping success; invalid URL fails.
func TestP2E1_ConnectPingAndInvalidURL(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	require.NoError(t, pool.Ping(ctx))
	stats := pool.Stat()
	require.GreaterOrEqual(t, stats.MaxConns(), int32(1))

	_, err := studiodb.Connect(ctx, "not-a-url://%%%")
	require.Error(t, err)

	// Fresh connect against harness URL then close releases connections.
	url := testutil.URL(t)
	p2, err := studiodb.Connect(ctx, url)
	require.NoError(t, err)
	require.NoError(t, p2.Ping(ctx))
	p2.Close()
	err = p2.Ping(ctx)
	require.Error(t, err)
	require.ErrorIs(t, repo.MapError(err), repo.ErrClosed)
}

// P2-E2: WithTx commit inserts tenant; error path rolls back.
func TestP2E2_WithTxCommitAndRollback(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	slugOK := "tx-ok-" + uuid.NewString()[:8]
	slugBad := "tx-bad-" + uuid.NewString()[:8]

	err := repo.WithTx(ctx, pool, func(q repo.Querier) error {
		_, err := q.Exec(ctx, `
			INSERT INTO curriculum_studio.tenants (slug, name)
			VALUES ($1, $2)`, slugOK, "OK Tenant")
		return err
	})
	require.NoError(t, err)

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM curriculum_studio.tenants WHERE slug=$1`, slugOK).Scan(&n))
	require.Equal(t, 1, n)

	err = repo.WithTx(ctx, pool, func(q repo.Querier) error {
		if _, err := q.Exec(ctx, `
			INSERT INTO curriculum_studio.tenants (slug, name)
			VALUES ($1, $2)`, slugBad, "Bad Tenant"); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	require.Error(t, err)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM curriculum_studio.tenants WHERE slug=$1`, slugBad).Scan(&n))
	require.Zero(t, n, "insert must not be visible after rollback")
}

// P2-E3: parallel tests with Tx() isolation on unique slugs.
func TestP2E3_ParallelTxIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		i := i
		t.Run(fmt.Sprintf("worker-%d", i), func(t *testing.T) {
			t.Parallel()
			defer wg.Done()
			ctx := context.Background()
			tx := testutil.Tx(t)
			// each worker inserts the same slug; isolation is via per-test tx rollback
			// Use a fixed conflicting slug intentionally — isolation via rollback.
			_, err := tx.Exec(ctx, `
				INSERT INTO curriculum_studio.tenants (slug, name)
				VALUES ($1, $2)`, "shared-parallel-slug", fmt.Sprintf("T-%d", i))
			require.NoError(t, err)
			var n int
			require.NoError(t, tx.QueryRow(ctx,
				`SELECT count(*) FROM curriculum_studio.tenants WHERE slug=$1`, "shared-parallel-slug").Scan(&n))
			require.Equal(t, 1, n)
		})
	}
	// Note: t.Parallel subtests + WaitGroup is awkward; rely on test harness.
	_ = wg
}

// P2-E4: NewFactory smoke.
func TestP2E4_FactorySmoke(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	f := repo.NewFactory(pool)
	require.NotNil(t, f)
	require.NotNil(t, f.Q)
	require.NoError(t, f.Ping(ctx))
	h := f.Health()
	require.NotNil(t, h)
	require.NoError(t, h.Ready(ctx))

	tx := testutil.Tx(t)
	f2 := repo.NewFactory(tx)
	require.NoError(t, f2.Ping(ctx))
}

// P2-E5: ban in-memory production repository types in non-test packages.
func TestP2E5_NoInMemoryProductionRepos(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	banned := []string{
		"type MemoryTenantRepo",
		"type memoryTenantRepo",
		"map[uuid.UUID]*Tenant",
		"inmemory.New",
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "testdata" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if len(path) > 8 && path[len(path)-8:] == "_test.go" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(b)
		for _, needle := range banned {
			require.NotContainsf(t, body, needle, "banned production pattern in %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestWithTxOnExistingTxRunsDirect(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	slug := "direct-" + uuid.NewString()[:8]
	err := repo.WithTx(ctx, tx, func(q repo.Querier) error {
		_, err := q.Exec(ctx, `
			INSERT INTO curriculum_studio.tenants (slug, name) VALUES ($1,$2)`, slug, "Direct")
		return err
	})
	require.NoError(t, err)
	var n int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT count(*) FROM curriculum_studio.tenants WHERE slug=$1`, slug).Scan(&n))
	require.Equal(t, 1, n)
}

func TestMapErrorClasses(t *testing.T) {
	t.Parallel()
	require.Nil(t, repo.MapError(nil))
	require.ErrorIs(t, repo.MapError(pgx.ErrNoRows), repo.ErrNotFound)

	err := repo.MapError(&pgconn.PgError{Code: "23505", ConstraintName: "tenants_slug_key"})
	require.ErrorIs(t, err, repo.ErrConflict)

	err = repo.MapError(&pgconn.PgError{Code: "23503", ConstraintName: "fk"})
	require.ErrorIs(t, err, repo.ErrForeignKey)

	err = repo.MapError(&pgconn.PgError{Code: "23514", ConstraintName: "chk"})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	err = repo.MapError(errors.New("closed pool"))
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestFactoryNilPanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { repo.NewFactory(nil) })
}

func TestHealthNil(t *testing.T) {
	t.Parallel()
	var f *repo.Factory
	require.ErrorIs(t, f.Ping(context.Background()), repo.ErrClosed)
	var h *repo.HealthRepo
	require.ErrorIs(t, h.Ready(context.Background()), repo.ErrClosed)
}

func TestSavepointContinuesAfterConstraintError(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := testutil.NewSavepointQuerier(tx)
	slug := "sp-" + uuid.NewString()[:8]
	_, err := q.Exec(ctx, `
		INSERT INTO curriculum_studio.tenants (slug, name) VALUES ($1,$2)`, slug, "A")
	require.NoError(t, err)
	_, err = q.Exec(ctx, `
		INSERT INTO curriculum_studio.tenants (slug, name) VALUES ($1,$2)`, slug, "dup")
	require.Error(t, err)
	require.ErrorIs(t, repo.MapError(err), repo.ErrConflict)
	// Outer tx still usable.
	_, err = q.Exec(ctx, `
		INSERT INTO curriculum_studio.tenants (slug, name) VALUES ($1,$2)`, slug+"-2", "B")
	require.NoError(t, err)
}

func TestRaceyPings(t *testing.T) {
	pool := testutil.DB(t)
	f := repo.NewFactory(pool)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.Ping(context.Background())
		}()
	}
	wg.Wait()
}

func TestWithTxBeginError(t *testing.T) {
	ctx := context.Background()
	// Dedicated pool so we do not close the shared harness.
	p, err := studiodb.Connect(ctx, testutil.URL(t))
	require.NoError(t, err)
	p.Close()
	err = repo.WithTx(ctx, p, func(q repo.Querier) error { return nil })
	require.Error(t, err)
}

func TestMapErrorPassthrough(t *testing.T) {
	t.Parallel()
	err := errors.New("something else")
	require.Equal(t, err, repo.MapError(err))
}
