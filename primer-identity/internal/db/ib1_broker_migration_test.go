package db_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/db"
)

func TestIB1MigrationFreshUpgradeDownAndExactContract(t *testing.T) {
	if testing.Short() {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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

	require.NoError(t, db.MigrateTo(ctx, url, 3), "upgrade path starts from pre-IB1 tip")
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	assertNoTable(t, pool, "broker_transactions")
	assertNoTable(t, pool, "oauth_clients")

	require.NoError(t, db.MigrateTo(ctx, url, 5), "fresh/upgrade to IB1 tip")
	for _, table := range []string{"oauth_clients", "oauth_client_redirects", "broker_transactions", "provider_session_associations", "oauth_grants", "oauth_authorization_codes"} {
		assertTable(t, pool, table)
	}
	for _, later := range []string{
		"oauth_client_keys", "oauth_refresh_families", "oauth_refresh_tokens", "signing_keys",
		"token_issuance_audit", "oauth_client_assertion_replays", "webhook_events",
		"webhook_security_events", "oauth_revocations", "identity_audit_events",
		"oauth_service_principals",
	} {
		assertNoTable(t, pool, later)
	}

	assertColumn(t, pool, "broker_transactions", "state_hash", "bytea", false)
	assertColumn(t, pool, "broker_transactions", "state_sealed", "bytea", true)
	assertColumn(t, pool, "broker_transactions", "state_key_version", "smallint", false)
	assertColumn(t, pool, "broker_transactions", "broker_cookie_hash", "bytea", false)
	assertColumn(t, pool, "oauth_authorization_codes", "code_hash", "bytea", false)
	assertColumn(t, pool, "oauth_authorization_codes", "consumed_at", "timestamp with time zone", true)
	assertColumn(t, pool, "provider_session_associations", "provider_member_session_id", "character varying", false)

	assertNamedFK(t, pool, "broker_transactions", "broker_transactions_oauth_client_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "broker_transactions", "broker_transactions_redirect_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "broker_transactions", "broker_transactions_account_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "broker_transactions", "broker_transactions_association_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "broker_transactions", "broker_transactions_association_account_fk", "RESTRICT", true)
	assertNamedFK(t, pool, "provider_session_associations", "provider_session_associations_account_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "provider_session_associations", "provider_session_associations_stytch_mapping_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_grants", "oauth_grants_account_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_grants", "oauth_grants_oauth_client_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_grants", "oauth_grants_association_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_grants", "oauth_grants_association_account_fk", "RESTRICT", true)
	assertNamedFK(t, pool, "oauth_authorization_codes", "oauth_authorization_codes_grant_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_authorization_codes", "oauth_authorization_codes_broker_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_authorization_codes", "oauth_authorization_codes_oauth_client_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "stytch_mappings", "stytch_mappings_account_id_fkey", "RESTRICT", false)

	assertIndex(t, pool, "broker_transactions_active_state_uq")
	assertIndex(t, pool, "broker_transactions_active_cookie_uq")
	assertIndex(t, pool, "broker_transactions_expiry_idx")
	assertIndex(t, pool, "provider_session_associations_id_account_uq")
	assertIndex(t, pool, "provider_session_associations_uq")
	assertIndex(t, pool, "oauth_grants_active_human_uq")
	assertIndex(t, pool, "oauth_authorization_codes_hash_uq")
	assertIndex(t, pool, "oauth_authorization_codes_broker_uq")

	var trigger string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT tgname FROM pg_trigger
WHERE tgname='provider_session_associations_stytch_mapping_tuple_ck'`).Scan(&trigger))
	require.Equal(t, "provider_session_associations_stytch_mapping_tuple_ck", trigger)

	require.NoError(t, db.MigrateDown(ctx, url), "IB1 broker exchange remains reversible")
	assertNoTable(t, pool, "broker_transactions")
	assertNoTable(t, pool, "provider_session_associations")
	assertNoTable(t, pool, "oauth_grants")
	assertNoTable(t, pool, "oauth_authorization_codes")
	assertTable(t, pool, "oauth_clients")
	assertNamedFK(t, pool, "stytch_mappings", "stytch_mappings_account_id_fkey", "CASCADE", false)

	require.NoError(t, db.MigrateTo(ctx, url, 5))
	assertTable(t, pool, "broker_transactions")
	assertNoTable(t, pool, "signing_keys")
	assertNamedFK(t, pool, "stytch_mappings", "stytch_mappings_account_id_fkey", "RESTRICT", false)

	require.NoError(t, db.Migrate(ctx, url), "fresh/upgrade to current Identity tip")
	assertTable(t, pool, "signing_keys")
	for _, table := range []string{
		"oauth_client_keys", "oauth_refresh_families", "oauth_refresh_tokens",
		"oauth_client_assertion_replays", "token_issuance_audit",
	} {
		assertTable(t, pool, table)
	}

	require.NoError(t, db.MigrateDown(ctx, url), "IB2 audit signing-key FK is newest")
	require.NoError(t, db.MigrateDown(ctx, url), "IB2 token_grants remains reversible")
	assertNoTable(t, pool, "oauth_client_keys")
	assertNoTable(t, pool, "oauth_refresh_families")
	assertNoTable(t, pool, "oauth_refresh_tokens")
	assertNoTable(t, pool, "oauth_client_assertion_replays")
	assertNoTable(t, pool, "token_issuance_audit")
	assertTable(t, pool, "signing_keys")
	assertTable(t, pool, "broker_transactions")
	require.NoError(t, db.MigrateDown(ctx, url), "IB2 signing_keys remains reversible")
	assertNoTable(t, pool, "signing_keys")
	assertTable(t, pool, "broker_transactions")

	require.NoError(t, db.Migrate(ctx, url))
	assertTable(t, pool, "broker_transactions")
	assertTable(t, pool, "signing_keys")
	assertTable(t, pool, "oauth_refresh_families")
	assertNamedFK(t, pool, "stytch_mappings", "stytch_mappings_account_id_fkey", "RESTRICT", false)
}

func TestIB1MigrationSQLHasNoRawTokensProviderPayloadOrLaterTables(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("migrations")
	require.NoError(t, err)
	var joined strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if !strings.Contains(e.Name(), "ib1") {
			continue
		}
		body, err := os.ReadFile(filepath.Join("migrations", e.Name()))
		require.NoError(t, err)
		lower := strings.ToLower(string(body))
		joined.WriteString(lower)
		require.NotContains(t, lower, "session_jwt")
		require.NotContains(t, lower, "signing_keys")
		require.NotContains(t, lower, "webhook_events")
		require.NotContains(t, lower, "token_issuance_audit")
		require.NotContains(t, lower, "findorcreatebyemail")
		require.NotContains(t, lower, "on conflict (primary_email)")
	}
	sql := joined.String()
	require.Contains(t, sql, "broker_transactions")
	require.Contains(t, sql, "provider_session_associations")
	require.Contains(t, sql, "oauth_authorization_codes")
}

func assertTable(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(), `
SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, name).Scan(&exists))
	require.True(t, exists, "missing table %s", name)
}

func assertNoTable(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(), `
SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, name).Scan(&exists))
	require.False(t, exists, "unexpected table %s", name)
}

func assertColumn(t *testing.T, pool *pgxpool.Pool, table, column, dataType string, nullable bool) {
	t.Helper()
	var gotType, isNullable string
	require.NoError(t, pool.QueryRow(context.Background(), `
SELECT data_type, is_nullable FROM information_schema.columns
WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&gotType, &isNullable))
	require.Equal(t, dataType, gotType, "%s.%s type", table, column)
	wantNull := "NO"
	if nullable {
		wantNull = "YES"
	}
	require.Equal(t, wantNull, isNullable, "%s.%s nullability", table, column)
}

func assertNamedFK(t *testing.T, pool *pgxpool.Pool, table, name, action string, deferrable bool) {
	t.Helper()
	var confdeltype, confdeferrable string
	err := pool.QueryRow(context.Background(), `
SELECT c.confdeltype, c.condeferrable::text
FROM pg_constraint c
JOIN pg_class t ON c.conrelid=t.oid
WHERE t.relname=$1 AND c.conname=$2 AND c.contype='f'`, table, name).Scan(&confdeltype, &confdeferrable)
	require.NoError(t, err, "missing FK %s on %s", name, table)
	wantAction := "r"
	if action == "CASCADE" {
		wantAction = "c"
	}
	require.Equal(t, wantAction, confdeltype, "%s delete action", name)
	if deferrable {
		require.Equal(t, "true", confdeferrable, "%s must be DEFERRABLE", name)
	}
}

func assertIndex(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname=$1)`, name).Scan(&exists))
	require.True(t, exists, "missing index %s", name)
}
