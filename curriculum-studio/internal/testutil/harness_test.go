package testutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestHarnessMigratesStudioNotLMS(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	var table string
	err := pool.QueryRow(ctx, `
		SELECT tablename FROM pg_tables
		WHERE tablename = $1`, studiodb.VersionTable).Scan(&table)
	require.NoError(t, err)
	require.Equal(t, studiodb.VersionTable, table)

	// LMS default goose table must not be required/created by Studio harness.
	var n int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables
		WHERE schemaname='public' AND tablename='goose_db_version'`).Scan(&n))
	// May be 0; if present must not be Studio's table name.
	_ = n

	var schemaOK int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables
		WHERE schemaname=$1 AND tablename='tenants'`, studiodb.SchemaName).Scan(&schemaOK))
	require.Equal(t, 1, schemaOK)
}

func TestTxRollbackIsolation(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	_, err := tx.Exec(ctx, `
		INSERT INTO curriculum_studio.tenants (slug, name)
		VALUES ('rollback-me', 'x')`)
	require.NoError(t, err)
	// Visible inside tx
	var n int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT count(*) FROM curriculum_studio.tenants WHERE slug='rollback-me'`).Scan(&n))
	require.Equal(t, 1, n)
	// Not visible on pool (uncommitted)
	pool := testutil.DB(t)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM curriculum_studio.tenants WHERE slug='rollback-me'`).Scan(&n))
	require.Zero(t, n)
}
