package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tvdb "github.com/aleksclark/primer/server/internal/tv/db"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
)

func TestMigrationsCreateEveryTable(t *testing.T) {
	t.Parallel()
	// The harness migrates on first use, so reaching here proves the up
	// migration applied cleanly.
	pool := tvtestutil.DB(t)
	ctx := context.Background()

	tables := []string{
		"devices",
		"media_items",
		"availability_windows",
		"play_ledger",
		"schedule_entries",
		"play_grants",
		"playback_sessions",
		"primer_reports",
		"content_manifest_entries",
	}
	for _, table := range tables {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1)`, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist", table)
	}
}

func TestMigrationsUseADedicatedVersionTable(t *testing.T) {
	t.Parallel()
	pool := tvtestutil.DB(t)

	// A separate goose table is what lets the TV and LMS schemas share a
	// PostgreSQL instance without clobbering each other's history.
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = $1)`, tvdb.VersionTable).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "the TV schema tracks versions in %s", tvdb.VersionTable)
	assert.NotEqual(t, "goose_db_version", tvdb.VersionTable)
}

func TestMigrateReportsBadConnectionStrings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	assert.Error(t, tvdb.Migrate(ctx, "not-a-valid-url"))
	assert.Error(t, tvdb.MigrateDown(ctx, "not-a-valid-url"))
}
