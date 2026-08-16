package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/db"
)

func TestStytchMappingMigrationConstraintsAndDownAreAdditive(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL migration harness")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("primer_identity_test"),
		tcpostgres.WithUsername("primer"),
		tcpostgres.WithPassword("primer"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, url))
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var hasTable bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.stytch_mappings') IS NOT NULL`).Scan(&hasTable))
	require.True(t, hasTable)

	var columns []string
	rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_name='stytch_mappings' ORDER BY ordinal_position`)
	require.NoError(t, err)
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		columns = append(columns, column)
	}
	rows.Close()
	require.Equal(t, []string{"id", "account_id", "project_id", "organization_id", "member_id", "created_at", "updated_at"}, columns)
	require.NotContains(t, columns, "email")

	var accountID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts (display_name) VALUES ('mapping test') RETURNING id`).Scan(&accountID))
	_, err = pool.Exec(ctx, `INSERT INTO stytch_mappings (account_id, project_id, organization_id, member_id) VALUES ($1, 'p', 'o', 'm')`, accountID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO stytch_mappings (account_id, project_id, organization_id, member_id) VALUES ($1, 'p', 'o', 'm')`, accountID)
	require.Error(t, err, "the tuple must be unique")

	for _, values := range []string{"'', 'o', 'm'", "'p', '', 'm'", "'p', 'o', ''"} {
		_, err = pool.Exec(ctx, `INSERT INTO stytch_mappings (account_id, project_id, organization_id, member_id) VALUES ($1, `+values+`)`, accountID)
		require.Error(t, err, "empty tuple component must fail: %s", values)
	}
	for _, values := range []string{
		"repeat('x', 256), 'o', 'm'",
		"'p', repeat('x', 256), 'm'",
		"'p', 'o', repeat('x', 256)",
	} {
		_, err = pool.Exec(ctx, `INSERT INTO stytch_mappings (account_id, project_id, organization_id, member_id) VALUES ($1, `+values+`)`, accountID)
		require.Error(t, err, "oversized tuple component must fail: %s", values)
	}

	var indexNames []string
	rows, err = pool.Query(ctx, `SELECT indexname FROM pg_indexes WHERE tablename='stytch_mappings' ORDER BY indexname`)
	require.NoError(t, err)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		indexNames = append(indexNames, name)
	}
	rows.Close()
	require.Contains(t, indexNames, "stytch_mappings_tuple_uidx")
	require.Contains(t, indexNames, "stytch_mappings_account_id_idx")
	require.Contains(t, indexNames, "stytch_mappings_project_org_idx")

	_, err = pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, accountID)
	require.NoError(t, err)
	var mappingCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM stytch_mappings`).Scan(&mappingCount))
	require.Zero(t, mappingCount, "account cascade must remove mappings")

	require.NoError(t, db.MigrateDown(ctx, url))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname='stytch_mappings'`).Scan(&mappingCount))
	require.Zero(t, mappingCount)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname='accounts'`).Scan(&mappingCount))
	require.Equal(t, 1, mappingCount, "down removes only the new table")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname='schema_meta'`).Scan(&mappingCount))
	require.Equal(t, 1, mappingCount)
}
