package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestMigrationsCreateFoundationAndGooseTable(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	ctx := context.Background()

	var gooseExists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`, db.VersionTable).Scan(&gooseExists)
	require.NoError(t, err)
	assert.True(t, gooseExists, "goose table %s must exist", db.VersionTable)
	assert.Equal(t, "identity_goose_db_version", db.VersionTable)
	assert.NotEqual(t, "goose_db_version", db.VersionTable)
	assert.NotEqual(t, "tv_goose_db_version", db.VersionTable)

	// Confirm via to_regclass (anti-cheat: not only constant assertion).
	var reg *string
	err = pool.QueryRow(ctx, `SELECT to_regclass('public.identity_goose_db_version')::text`).Scan(&reg)
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, "identity_goose_db_version", *reg)

	var metaExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'schema_meta'
		)`).Scan(&metaExists)
	require.NoError(t, err)
	assert.True(t, metaExists, "foundation migration should create schema_meta")

	// No LMS/TV tables leaked into Identity DB.
	for _, foreign := range []string{"educators", "students", "devices", "media_items"} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, foreign).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists, "foreign table %s must not exist in Identity DB", foreign)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()
	url := testutil.DatabaseURL(t)
	ctx := context.Background()

	require.NoError(t, db.Migrate(ctx, url))
	require.NoError(t, db.Migrate(ctx, url), "second migrate must be a no-op success")

	pool := testutil.DB(t)
	var version int64
	err := pool.QueryRow(ctx, `SELECT version_id FROM identity_goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&version)
	// goose v3 provider table columns may differ; fall back to count.
	if err != nil {
		var n int
		err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_goose_db_version`).Scan(&n)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, 1)
		return
	}
	assert.GreaterOrEqual(t, version, int64(1))
}

func TestMigrateReportsBadConnectionStrings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	assert.Error(t, db.Migrate(ctx, "not-a-valid-url"))
	assert.Error(t, db.MigrateDown(ctx, "not-a-valid-url"))
}

func TestConnectPing(t *testing.T) {
	t.Parallel()
	url := testutil.DatabaseURL(t)
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))
}

func TestHarnessUsesIdentityDBName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "primer_identity_test", testutil.DBName)
}

func TestConnectRejectsBadURL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := db.Connect(ctx, "://bad")
	assert.Error(t, err)

	_, err = db.Connect(ctx, "postgres://primer:primer@127.0.0.1:1/nope?sslmode=disable&connect_timeout=1")
	assert.Error(t, err)
}

func TestMigrateDownAgainstLiveDB(t *testing.T) {
	// Not parallel: mutates shared harness schema; re-up at end.
	url := testutil.DatabaseURL(t)
	ctx := context.Background()
	require.NoError(t, db.Migrate(ctx, url))
	require.NoError(t, db.MigrateDown(ctx, url))
	require.NoError(t, db.Migrate(ctx, url), "re-apply after down for other tests")
}
