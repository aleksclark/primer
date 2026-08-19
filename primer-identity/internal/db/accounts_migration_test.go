package db_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestNoStudentTableOrProvider(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	ctx := context.Background()

	// No students table in Identity schema.
	var studentsExists bool
	err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'students'
)`).Scan(&studentsExists)
	require.NoError(t, err)
	assert.False(t, studentsExists, "students table must not exist in Identity v1")

	// Inventory public tables — none should be student-related.
	rows, err := pool.Query(ctx, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' ORDER BY table_name`)
	require.NoError(t, err)
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
		assert.NotContains(t, strings.ToLower(name), "student")
	}
	require.NoError(t, rows.Err())
	assert.Contains(t, tables, "accounts")
	assert.Contains(t, tables, "external_identities")
	assert.Contains(t, tables, "credentials_password")

	// Provider check constraint must not include student.
	var checkDef string
	err = pool.QueryRow(ctx, `
SELECT pg_get_constraintdef(c.oid)
FROM pg_constraint c
JOIN pg_class t ON c.conrelid = t.oid
WHERE t.relname = 'external_identities' AND c.contype = 'c'
  AND pg_get_constraintdef(c.oid) ILIKE '%provider%'`).Scan(&checkDef)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(checkDef), "student")
	assert.Contains(t, checkDef, "google")
	assert.Contains(t, checkDef, "password")
	assert.Contains(t, checkDef, "breakglass")
}

func TestAccountsSchemaConstraints(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	ctx := context.Background()

	for _, table := range []string{"accounts", "external_identities", "credentials_password"} {
		var exists bool
		err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = $1
)`, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s", table)
	}

	// Unique on (provider, provider_subject).
	var uniq string
	err := pool.QueryRow(ctx, `
SELECT pg_get_constraintdef(c.oid)
FROM pg_constraint c
JOIN pg_class t ON c.conrelid = t.oid
WHERE t.relname = 'external_identities' AND c.contype = 'u'`).Scan(&uniq)
	require.NoError(t, err)
	assert.Contains(t, uniq, "provider")
	assert.Contains(t, uniq, "provider_subject")

	// primary_email must NOT be unique.
	var emailUniq int
	err = pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_constraint c
JOIN pg_class t ON c.conrelid = t.oid
WHERE t.relname = 'accounts' AND c.contype = 'u'
  AND pg_get_constraintdef(c.oid) ILIKE '%primary_email%'`).Scan(&emailUniq)
	require.NoError(t, err)
	assert.Zero(t, emailUniq, "primary_email must not have a unique constraint")
}

func TestAccountsMigrationFreshAndUpgrade(t *testing.T) {
	// Dedicated container so MigrateDown does not disturb the shared harness.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("primer_identity_test"),
		tcpostgres.WithUsername("primer"),
		tcpostgres.WithPassword("primer"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Fresh: apply all migrations.
	require.NoError(t, db.Migrate(ctx, url))

	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var accounts bool
	require.NoError(t, pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'accounts'
)`).Scan(&accounts))
	assert.True(t, accounts)

	// Roll down IB2, IB1, and baseline migrations one at a time; each is reversible.
	// 00002..00009 = 8 downs leave foundation (00001) in place.
	for range 10 {
		require.NoError(t, db.MigrateDown(ctx, url))
	}
	var stytchMappings bool
	require.NoError(t, pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'stytch_mappings'
)`).Scan(&stytchMappings))
	assert.False(t, stytchMappings)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'accounts'
)`).Scan(&accounts))
	assert.False(t, accounts, "full rollback should drop accounts")

	var meta bool
	require.NoError(t, pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'schema_meta'
)`).Scan(&meta))
	assert.True(t, meta, "foundation remains after IB1/baseline rollback")

	require.NoError(t, db.Migrate(ctx, url), "upgrade from phase 1 must apply all later migrations")
	require.NoError(t, pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'accounts'
)`).Scan(&accounts))
	assert.True(t, accounts)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'stytch_mappings'
)`).Scan(&stytchMappings))
	assert.True(t, stytchMappings)
}

func TestAntiCheat_NoEmailConflictMergeInMigrations(t *testing.T) {
	t.Parallel()
	// Read committed migration SQL and ban email-merge ON CONFLICT patterns.
	entries, err := os.ReadDir("migrations")
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join("migrations", e.Name()))
		require.NoError(t, err)
		lower := strings.ToLower(string(body))
		assert.NotContains(t, lower, "on conflict (primary_email)", e.Name())
		assert.NotContains(t, lower, "on conflict(primary_email)", e.Name())
		assert.NotContains(t, lower, "unique (primary_email)", e.Name())
		assert.NotContains(t, lower, "unique(primary_email)", e.Name())
	}
}
