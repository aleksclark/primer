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
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestIB2TokenGrantsMigrationFreshUpgradeDownAndExactContract(t *testing.T) {
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

	require.NoError(t, db.MigrateTo(ctx, url, 6), "upgrade path starts from IB1 plus signing-key tip")
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	for _, later := range []string{
		"oauth_client_keys", "oauth_refresh_families", "oauth_refresh_tokens",
		"oauth_client_assertion_replays", "token_issuance_audit",
	} {
		assertNoTable(t, pool, later)
	}
	assertTable(t, pool, "signing_keys")

	require.NoError(t, db.Migrate(ctx, url), "fresh/upgrade to IB2 token-grants tip")
	for _, table := range []string{
		"oauth_client_keys", "oauth_refresh_families", "oauth_refresh_tokens",
		"oauth_client_assertion_replays", "token_issuance_audit", "signing_keys",
	} {
		assertTable(t, pool, table)
	}
	for _, later := range []string{
		"webhook_events", "webhook_security_events",
		"oauth_revocations", "identity_audit_events", "oauth_service_principals",
	} {
		assertNoTable(t, pool, later)
	}

	assertColumn(t, pool, "oauth_client_keys", "kid", "character varying", false)
	assertColumn(t, pool, "oauth_client_keys", "jwk_json", "jsonb", false)
	assertColumn(t, pool, "oauth_client_keys", "alg", "character varying", false)
	assertColumn(t, pool, "oauth_client_keys", "not_before", "timestamp with time zone", true)
	assertColumn(t, pool, "oauth_client_keys", "disabled_at", "timestamp with time zone", true)
	assertColumn(t, pool, "oauth_refresh_families", "absolute_expires_at", "timestamp with time zone", false)
	assertColumn(t, pool, "oauth_refresh_families", "idle_expires_at", "timestamp with time zone", false)
	assertColumn(t, pool, "oauth_refresh_tokens", "token_hash", "bytea", false)
	assertColumn(t, pool, "oauth_refresh_tokens", "pepper_version", "smallint", false)
	assertColumn(t, pool, "oauth_refresh_tokens", "consumed_at", "timestamp with time zone", true)
	assertColumn(t, pool, "oauth_client_assertion_replays", "jti_hash", "bytea", false)
	assertColumn(t, pool, "oauth_client_assertion_replays", "audience", "character varying", false)
	assertColumn(t, pool, "oauth_client_assertion_replays", "issued_at", "timestamp with time zone", false)
	assertColumn(t, pool, "token_issuance_audit", "authorization_code_id", "uuid", true)
	assertColumn(t, pool, "token_issuance_audit", "authorization_code_hash", "bytea", true)
	assertColumn(t, pool, "token_issuance_audit", "subject_ref", "character varying", false)
	assertColumn(t, pool, "token_issuance_audit", "client_id", "character varying", false)
	assertColumn(t, pool, "token_issuance_audit", "resource_uri", "character varying", false)
	assertColumn(t, pool, "token_issuance_audit", "kid", "character varying", false)

	assertNamedFK(t, pool, "oauth_client_keys", "oauth_client_keys_oauth_client_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_refresh_families", "oauth_refresh_families_grant_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_refresh_families", "oauth_refresh_families_oauth_client_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_refresh_tokens", "oauth_refresh_tokens_family_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_refresh_tokens", "oauth_refresh_tokens_replaced_by_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "oauth_client_assertion_replays", "oauth_client_assertion_replays_oauth_client_fk", "RESTRICT", false)
	assertNamedFK(t, pool, "token_issuance_audit", "token_issuance_audit_grant_fk", "RESTRICT", false)
	assertNamedFKSetNull(t, pool, "token_issuance_audit", "token_issuance_audit_code_fk")

	assertIndex(t, pool, "oauth_client_keys_client_kid_uq")
	assertIndex(t, pool, "oauth_refresh_tokens_hash_uq")
	assertIndex(t, pool, "oauth_refresh_tokens_family_sequence_uq")
	assertIndex(t, pool, "oauth_refresh_tokens_one_live_uq")
	assertIndex(t, pool, "oauth_client_assertion_replays_uq")
	assertIndex(t, pool, "oauth_client_assertion_replays_expiry_idx")

	var trigger string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT tgname FROM pg_trigger
WHERE tgname='oauth_clients_private_key_presence_ck'`).Scan(&trigger))
	require.Equal(t, "oauth_clients_private_key_presence_ck", trigger)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT tgname FROM pg_trigger
WHERE tgname='oauth_refresh_family_lifecycle_ck'`).Scan(&trigger))
	require.Equal(t, "oauth_refresh_family_lifecycle_ck", trigger)

	require.NoError(t, db.MigrateDown(ctx, url))
	for _, table := range []string{
		"oauth_client_keys", "oauth_refresh_families", "oauth_refresh_tokens",
		"oauth_client_assertion_replays", "token_issuance_audit",
	} {
		assertNoTable(t, pool, table)
	}
	assertTable(t, pool, "signing_keys")
	assertTable(t, pool, "oauth_authorization_codes")
	assertTable(t, pool, "oauth_clients")

	require.NoError(t, db.Migrate(ctx, url))
	assertTable(t, pool, "oauth_refresh_families")
	assertNamedFKSetNull(t, pool, "token_issuance_audit", "token_issuance_audit_code_fk")
	assertIB2ConstraintRejects(t, pool)
}

func TestIB2TokenGrantsMigrationSQLHasNoRawTokensOrLaterTables(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("migrations")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	require.Equal(t, []string{
		"00001_foundation.sql",
		"00002_accounts.sql",
		"00003_stytch_mappings.sql",
		"00004_ib1_oauth_clients.sql",
		"00005_ib1_broker_exchange.sql",
		"00006_ib2_signing_keys.sql",
		"00007_ib2_token_grants.sql",
	}, names, "old migrations inventory may grow only by the expected IB2 versions")
	body, err := os.ReadFile(filepath.Join("migrations", "00007_ib2_token_grants.sql"))
	require.NoError(t, err)
	lower := strings.ToLower(string(body))
	require.Contains(t, lower, "oauth_client_assertion_replays")
	require.Contains(t, lower, "oauth_refresh_families")
	require.Contains(t, lower, "oauth_refresh_tokens")
	require.Contains(t, lower, "token_issuance_audit")
	require.Contains(t, lower, "oauth_client_keys")
	require.NotContains(t, lower, "session_jwt")
	require.NotContains(t, lower, "signing_keys")
	require.NotContains(t, lower, "webhook_events")
	require.NotContains(t, lower, "oauth_service_principals")
	require.NotContains(t, lower, "findorcreatebyemail")
	require.NotContains(t, filepath.Base("00007_ib2_token_grants.sql"), "00006")
}

func assertNamedFKSetNull(t *testing.T, pool *pgxpool.Pool, table, name string) {
	t.Helper()
	var confdeltype string
	err := pool.QueryRow(context.Background(), `
SELECT c.confdeltype
FROM pg_constraint c
JOIN pg_class t ON c.conrelid=t.oid
WHERE t.relname=$1 AND c.conname=$2 AND c.contype='f'`, table, name).Scan(&confdeltype)
	require.NoError(t, err, "missing FK %s on %s", name, table)
	require.Equal(t, "n", confdeltype, "%s must be ON DELETE SET NULL", name)
}

func assertIB2ConstraintRejects(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	q := testutil.NewSavepointQuerier(tx)

	var clientID string
	require.NoError(t, q.QueryRow(ctx, `
INSERT INTO oauth_clients(client_id,name,client_type,token_endpoint_auth_method,allowed_grants)
VALUES('public-ib2-'||substr(gen_random_uuid()::text,1,8),'n','public','none',ARRAY['authorization_code'])
RETURNING id`).Scan(&clientID))

	_, err = q.Exec(ctx, `
INSERT INTO oauth_client_keys(oauth_client_id,kid,jwk_json)
VALUES($1,'k1','{"kty":"EC","crv":"P-256","x":"a","y":"b","d":"secret"}'::jsonb)`, clientID)
	require.Error(t, err, "private JWK material must fail")
	require.NotContains(t, err.Error(), "25P02", "unexpected aborted-tx must not hide the CHECK failure")

	_, err = q.Exec(ctx, `
INSERT INTO oauth_client_assertion_replays(oauth_client_id,endpoint_kind,jti_hash,audience,issued_at,expires_at,consumed_at)
VALUES($1,'token',decode(repeat('aa',16),'hex'),'https://identity.example/oauth/token', now(), now()+interval '10 minutes', now())`, clientID)
	require.Error(t, err, "assertion TTL above 5 minutes must fail")
	require.NotContains(t, err.Error(), "25P02")

	_, err = q.Exec(ctx, `
INSERT INTO oauth_refresh_families(grant_id,oauth_client_id,resource_uri,status,absolute_expires_at,idle_expires_at,last_rotated_at)
VALUES(gen_random_uuid(),$1,'https://resource.example','active', now()+interval '120 days', now()+interval '1 day', now())`, clientID)
	require.Error(t, err, "absolute lifetime above 90 days must fail")
	require.NotContains(t, err.Error(), "25P02")

	var familyCK string
	require.NoError(t, q.QueryRow(ctx, `
SELECT pg_get_constraintdef(c.oid)
FROM pg_constraint c JOIN pg_class t ON c.conrelid=t.oid
WHERE t.relname='oauth_refresh_families' AND c.conname='oauth_refresh_families_status_lifetime_ck'`).Scan(&familyCK))
	require.Contains(t, familyCK, "14 days")
	require.Contains(t, familyCK, "90 days")
}
