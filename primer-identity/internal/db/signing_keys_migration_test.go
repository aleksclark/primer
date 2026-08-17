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

// E00 provenance: IB0 data contract at
// agent_docs/plans/stytch-identity-ib0/03-data-state-and-revocation.md
// plus keep/reshape/drop at
// agent_docs/plans/stytch-identity-ib0/jwks-donor-evidence.md (donor tip
// 53b693cc93c8cccb15109d90bae90811029af893). Migration number is 00006 on
// post-IB1 master; donor 00004 is not copied.

const ib2SigningKeyJWK = `{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","x":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","y":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}`

func ib2SealedPrivate() []byte {
	sealed := make([]byte, 64)
	for i := range sealed {
		sealed[i] = byte(i + 1)
	}
	return sealed
}

func startIB2MigrationDB(t *testing.T) (context.Context, string, *pgxpool.Pool) {
	t.Helper()
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
	return ctx, url, nil
}

func connectIB2MigrationDB(t *testing.T, ctx context.Context, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestIB2SigningKeysMigrationFreshUpgradeDownAndExactContract(t *testing.T) {
	ctx, url, _ := startIB2MigrationDB(t)
	require.NoError(t, db.MigrateTo(ctx, url, 5), "upgrade path starts from IB1 tip")
	pool := connectIB2MigrationDB(t, ctx, url)
	assertNoTable(t, pool, "signing_keys")
	assertNoTable(t, pool, "token_issuance_audit")

	require.NoError(t, db.Migrate(ctx, url), "fresh/upgrade to IB2 signing-key tip")
	assertTable(t, pool, "signing_keys")
	assertNoTable(t, pool, "token_issuance_audit")
	assertNoTable(t, pool, "oauth_refresh_families")

	assertColumn(t, pool, "signing_keys", "id", "uuid", false)
	assertColumn(t, pool, "signing_keys", "kid", "character varying", false)
	assertColumn(t, pool, "signing_keys", "alg", "character varying", false)
	assertColumn(t, pool, "signing_keys", "public_jwk", "jsonb", false)
	assertColumn(t, pool, "signing_keys", "sealed_private_key", "bytea", true)
	assertColumn(t, pool, "signing_keys", "key_version", "smallint", false)
	assertColumn(t, pool, "signing_keys", "status", "character varying", false)
	assertColumn(t, pool, "signing_keys", "not_before", "timestamp with time zone", false)
	assertColumn(t, pool, "signing_keys", "not_after", "timestamp with time zone", true)
	assertColumn(t, pool, "signing_keys", "created_at", "timestamp with time zone", false)
	assertColumn(t, pool, "signing_keys", "activated_at", "timestamp with time zone", true)
	assertColumn(t, pool, "signing_keys", "retired_at", "timestamp with time zone", true)
	assertColumn(t, pool, "signing_keys", "destroyed_at", "timestamp with time zone", true)

	assertNamedCheck(t, pool, "signing_keys", "signing_keys_profile_ck")
	assertNamedCheck(t, pool, "signing_keys", "signing_keys_lifetime_ck")
	assertNamedCheck(t, pool, "signing_keys", "signing_keys_status_material_ck")
	assertNamedUnique(t, pool, "signing_keys", "signing_keys_kid_uq")
	assertIndex(t, pool, "signing_keys_one_active_uq")
	assertIndex(t, pool, "signing_keys_one_next_uq")

	require.NoError(t, db.MigrateDown(ctx, url))
	assertNoTable(t, pool, "signing_keys")
	assertTable(t, pool, "oauth_authorization_codes")
	assertTable(t, pool, "broker_transactions")

	require.NoError(t, db.Migrate(ctx, url))
	assertTable(t, pool, "signing_keys")
	assertNamedCheck(t, pool, "signing_keys", "signing_keys_status_material_ck")

	// Repeat the real PG fresh/upgrade/down + constraint cycle a second time.
	require.NoError(t, db.MigrateDown(ctx, url))
	assertNoTable(t, pool, "signing_keys")
	require.NoError(t, db.Migrate(ctx, url))
	assertTable(t, pool, "signing_keys")
	assertNamedCheck(t, pool, "signing_keys", "signing_keys_profile_ck")
	assertNamedCheck(t, pool, "signing_keys", "signing_keys_lifetime_ck")
	assertNamedCheck(t, pool, "signing_keys", "signing_keys_status_material_ck")
	assertIndex(t, pool, "signing_keys_one_active_uq")
	assertIndex(t, pool, "signing_keys_one_next_uq")
}

func TestIB2SigningKeysUniqueActiveNextAndLifecycleConstraints(t *testing.T) {
	ctx, url, _ := startIB2MigrationDB(t)
	require.NoError(t, db.Migrate(ctx, url))
	pool := connectIB2MigrationDB(t, ctx, url)
	sealed := ib2SealedPrivate()

	_, err := pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at)
VALUES ('aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'active', now(), now())`, ib2SigningKeyJWK, sealed)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at)
VALUES ('bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'active', now(), now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "at most one active signing key")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ('cccccccc-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'next', now())`, ib2SigningKeyJWK, sealed)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ('dddddddd-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'next', now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "at most one next signing key")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES (E'bad\tkid', 'ES256', $1::jsonb, $2, 1, 'retired', now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "control characters in kid must fail")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at, retired_at)
VALUES ('eeeeeeee-bbbb-cccc-dddd-eeeeeeeeeeee', 'RS256', $1::jsonb, $2, 1, 'retired', now(), now(), now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "alg must be ES256")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ('ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 0, 'next', now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "key_version must be >0")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ('11111111-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'pending', now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "status must be next|active|retired|destroyed")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, not_after)
VALUES ('22222222-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'next', now(), now() - interval '1 second')`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "not_after must be after not_before")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ('33333333-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'active', now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "active requires activated_at and sealed material")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at)
VALUES ('44444444-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'next', now(), now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "next must not have activation timestamps")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at, retired_at)
VALUES ('55555555-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'retired', now(), now(), now() - interval '1 second')`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "retired_at must be >= activated_at")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at, retired_at, destroyed_at)
VALUES ('66666666-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'destroyed', now(), now(), now(), now())`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "destroyed must have NULL sealed_private_key")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at, retired_at, destroyed_at)
VALUES ('77777777-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, NULL, 1, 'destroyed', now(), now(), now(), now())`, ib2SigningKeyJWK)
	require.NoError(t, err, "destroyed without ciphertext is the IB7-ready terminal shape")

	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ('aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', 'ES256', $1::jsonb, $2, 1, 'next')`, ib2SigningKeyJWK, sealed)
	require.Error(t, err, "kid must be unique")
}

func TestIB2SigningKeysMigrationSQLHasNoTokenOrHTTPSurface(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("migrations", "00006_ib2_signing_keys.sql"))
	require.NoError(t, err)
	lower := strings.ToLower(string(body))
	require.Contains(t, lower, "create table signing_keys")
	require.Contains(t, lower, "sealed_private_key")
	require.Contains(t, lower, "signing_keys_one_active_uq")
	require.Contains(t, lower, "signing_keys_one_next_uq")
	require.Contains(t, lower, "signing_keys_status_material_ck")
	require.NotContains(t, lower, "token_issuance_audit")
	require.NotContains(t, lower, "oauth/token")
	require.NotContains(t, lower, "jwks.json")
	require.NotContains(t, lower, "session_jwt")
	require.NotContains(t, lower, "00004_signing_keys")

	notes, err := os.ReadFile("SCHEMA.md")
	require.NoError(t, err)
	noteText := string(notes)
	require.Contains(t, noteText, "## 00006_ib2_signing_keys")
	require.Contains(t, noteText, "53b693cc93c8cccb15109d90bae90811029af893")
	require.Contains(t, noteText, "signing_keys_status_material_ck")
	require.Contains(t, noteText, "signer readiness is deferred")
	require.Contains(t, noteText, "integration HTTP lane")
	require.NotContains(t, strings.ToLower(noteText), "oauth/token")
}

func assertNamedCheck(t *testing.T, pool *pgxpool.Pool, table, name string) {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(), `
SELECT EXISTS (
  SELECT 1 FROM pg_constraint c
  JOIN pg_class t ON c.conrelid=t.oid
  WHERE t.relname=$1 AND c.conname=$2 AND c.contype='c'
)`, table, name).Scan(&exists))
	require.True(t, exists, "missing CHECK %s on %s", name, table)
}

func assertNamedUnique(t *testing.T, pool *pgxpool.Pool, table, name string) {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(), `
SELECT EXISTS (
  SELECT 1 FROM pg_constraint c
  JOIN pg_class t ON c.conrelid=t.oid
  WHERE t.relname=$1 AND c.conname=$2 AND c.contype='u'
)`, table, name).Scan(&exists))
	require.True(t, exists, "missing UNIQUE %s on %s", name, table)
}
