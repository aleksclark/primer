package keys_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
)

func custodyConfig(t *testing.T, env string, auto bool) config.KeyConfig {
	t.Helper()
	var cfg config.KeyConfig
	cfg.Enabled = true
	cfg.AutoBootstrap = auto
	cfg.SetSealSecretForTest("UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE")
	require.NoError(t, cfg.Validate(env))
	return cfg
}

func dedicatedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := dedicatedPoolURL(t)
	return pool
}

func dedicatedPoolURL(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
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
	return pool, url
}

func connectPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func requireGenericSignerFailure(t *testing.T, err error, sig []byte) {
	t.Helper()
	require.Error(t, err)
	assert.Nil(t, sig)
	assert.True(t, errors.Is(err, keys.ErrSignerRevoked) || errors.Is(err, domain.ErrSignerUnavailable), err)
	assert.NotContains(t, err.Error(), `"d"`)
	assert.NotContains(t, strings.ToLower(err.Error()), "private")
	assert.NotContains(t, strings.ToLower(err.Error()), "sealed")
}

func tamperSealedPrivate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, kid string) {
	t.Helper()
	var sealed []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT sealed_private_key FROM signing_keys WHERE kid = $1`, kid).Scan(&sealed))
	require.Greater(t, len(sealed), 8)
	sealed[7] ^= 0xff
	tag, err := pool.Exec(ctx, `UPDATE signing_keys SET sealed_private_key = $2 WHERE kid = $1`, kid, sealed)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag.RowsAffected())
}

func TestCreateInitialActiveAndFetchSigner(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")

	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	require.Equal(t, domain.SigningKeyStatusActive, created.Status)
	require.NotNil(t, created.ActivatedAt)

	signer, meta, err := svc.ActiveSigner(ctx)
	require.NoError(t, err)
	var _ crypto.Signer = signer
	signerPub, err := signer.PublicJWK()
	require.NoError(t, err)
	assert.Equal(t, created.Kid, signerPub.Kid)
	assert.Equal(t, created.Kid, meta.Kid)
	again, againMeta, err := svc.ActiveSigner(ctx)
	require.NoError(t, err)
	assert.Same(t, signer, again)
	assert.Equal(t, meta.Kid, againMeta.Kid)
	_, err = json.Marshal(signer)
	require.Error(t, err)
	_, err = json.Marshal(*signer)
	require.Error(t, err)
	formatted := fmt.Sprintf("%v %#v %v %#v", signer, signer, *signer, *signer)
	assert.NotContains(t, formatted, `"d"`)

	pubs, err := svc.PublicJWKS(ctx)
	require.NoError(t, err)
	require.Len(t, pubs, 1)
	assert.Equal(t, created.Kid, pubs[0].Kid)
	raw, err := json.Marshal(pubs)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"d"`)
	etag, err := keys.PublicSetETag(pubs)
	require.NoError(t, err)
	assert.NotEmpty(t, etag)
}

func TestCreateNextAndPublicSetIncludesOptionalNext(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "development", false), "development")

	active, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	next, err := svc.CreateNext(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.SigningKeyStatusNext, next.Status)
	assert.NotEqual(t, active.Kid, next.Kid)
	assert.Nil(t, next.ActivatedAt)

	pubs, err := svc.PublicJWKS(ctx)
	require.NoError(t, err)
	require.Len(t, pubs, 2)
	etag, err := svc.PublicSetETag(ctx)
	require.NoError(t, err)
	assert.Contains(t, etag, "W/")
}

func TestActiveSignerFailsClosedOnEmptyAndMultiple(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")

	_, _, err := svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable), err)

	_, err = svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DROP INDEX IF EXISTS signing_keys_one_active_uq`)
	require.NoError(t, err)
	mat, err := keys.Generate()
	require.NoError(t, err)
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	sealed, err := keys.Seal(mat, custodyConfig(t, "test", false).SealKey())
	require.NoError(t, err)
	pubJSON, err := json.Marshal(pub)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at)
VALUES ($1, 'ES256', $2::jsonb, $3, 1, 'active', now(), now())`, pub.Kid, pubJSON, sealed)
	require.NoError(t, err)

	_, _, err = svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, domain.ErrSignerUnavailable), err)
}

func TestActiveSignerFailsClosedOnCorruptUnseal(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET sealed_private_key = overlay(sealed_private_key placing E'\\x00' from 8 for 1) WHERE kid = $1`, created.Kid)
	require.NoError(t, err)

	_, _, err = svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, keys.ErrUnsealFailed), err)
	assert.NotContains(t, err.Error(), created.Kid+"private")
	assert.NotContains(t, strings.ToLower(err.Error()), "begin")
}

func TestActiveSignerFailsWhenNotInPublicSetOrTimeInvalid(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET not_before = now() + interval '2 hours' WHERE kid = $1`, created.Kid)
	require.NoError(t, err)
	_, _, err = svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable) || errors.Is(err, domain.ErrInvalid), err)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET not_before = now() - interval '2 hours', not_after = now() - interval '1 hour' WHERE kid = $1`, created.Kid)
	require.NoError(t, err)
	_, _, err = svc.ActiveSigner(ctx)
	require.Error(t, err)
}

func TestProductionDoesNotAutoGenerate(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "production", false)
	svc := keys.NewService(pool, cfg, "production")

	_, _, err := svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable), err)

	_, err = svc.Bootstrap(ctx)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "bootstrap")

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM signing_keys`).Scan(&n))
	assert.Zero(t, n)
}

func TestExplicitDevBootstrapCreatesOneActive(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "development", true), "development")
	created, err := svc.Bootstrap(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.SigningKeyStatusActive, created.Status)
	again, err := svc.Bootstrap(ctx)
	require.NoError(t, err)
	assert.Equal(t, created.Kid, again.Kid)
}

func TestCreateNextThenActivateWhenNoActiveExists(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	next := plantSealedKey(t, pool, custodyConfig(t, "test", false), domain.SigningKeyStatusNext)
	activated, err := svc.ActivateNext(ctx)
	require.NoError(t, err)
	assert.Equal(t, next.Kid, activated.Kid)
	assert.Equal(t, domain.SigningKeyStatusActive, activated.Status)
	require.NotNil(t, activated.ActivatedAt)
}

func TestActivateNextFailsWhenActiveAlreadyExists(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	_, err = svc.CreateNext(ctx)
	require.NoError(t, err)
	_, err = svc.ActivateNext(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrInvalid), err)
}

func TestSignerMustBeRepresentedInPublicSet(t *testing.T) {
	t.Parallel()
	jwk := domain.PublicJWK{
		KTY: "EC",
		CRV: "P-256",
		Use: "sig",
		Alg: domain.SigningAlgES256,
		Kid: "11111111-2222-3333-4444-555555555555",
		X:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Y:   "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
	}
	require.False(t, keys.SignerInPublicSet(jwk, nil))
	require.False(t, keys.SignerInPublicSet(jwk, []domain.PublicJWK{{Kid: jwk.Kid}}))
	require.True(t, keys.SignerInPublicSet(jwk, []domain.PublicJWK{jwk}))
	kidOnlyMatch := jwk
	kidOnlyMatch.X = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	require.False(t, keys.SignerInPublicSet(jwk, []domain.PublicJWK{kidOnlyMatch}))
}

func plantSealedKey(t *testing.T, pool *pgxpool.Pool, cfg config.KeyConfig, status string) domain.PublicJWK {
	t.Helper()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	sealed, err := keys.Seal(mat, cfg.SealKey())
	require.NoError(t, err)
	pubJSON, err := json.Marshal(pub)
	require.NoError(t, err)
	if status == domain.SigningKeyStatusActive {
		_, err = pool.Exec(context.Background(), `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before, activated_at)
VALUES ($1, 'ES256', $2::jsonb, $3, 1, $4, now(), now())`, pub.Kid, pubJSON, sealed, status)
	} else {
		_, err = pool.Exec(context.Background(), `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
VALUES ($1, 'ES256', $2::jsonb, $3, 1, $4, now())`, pub.Kid, pubJSON, sealed, status)
	}
	require.NoError(t, err)
	return pub
}

func TestPublicJWKSAuthenticatesCompleteActiveAndNext(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	svc := keys.NewService(pool, cfg, "test")
	active, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	next, err := svc.CreateNext(ctx)
	require.NoError(t, err)

	pubs, err := svc.PublicJWKS(ctx)
	require.NoError(t, err)
	require.Len(t, pubs, 2)
	assert.Equal(t, active.PublicJWK, pubs[0])
	assert.Equal(t, next.PublicJWK, pubs[1])
}

func TestPublicJWKSAndActiveSignerFailClosedOnPlantedTamperedNext(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	svc := keys.NewService(pool, cfg, "test")
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	next := plantSealedKey(t, pool, cfg, domain.SigningKeyStatusNext)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET sealed_private_key = overlay(sealed_private_key placing E'\\x00' from 8 for 1) WHERE kid = $1`, next.Kid)
	require.NoError(t, err)

	_, err = svc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, keys.ErrUnsealFailed), err)
	assert.NotContains(t, err.Error(), next.Kid+"private")

	_, _, err = svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, keys.ErrUnsealFailed), err)
}

func TestPublicJWKSFailsClosedOnCardinalityTemporalAndDuplicateKids(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	svc := keys.NewService(pool, cfg, "test")

	_, err := svc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable) || errors.Is(err, domain.ErrCorruptSigner), err)

	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	next := plantSealedKey(t, pool, cfg, domain.SigningKeyStatusNext)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET not_before = now() + interval '2 hours' WHERE kid = $1`, next.Kid)
	require.NoError(t, err)
	_, err = svc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable) || errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, domain.ErrInvalid), err)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET not_before = now() - interval '2 hours', not_after = now() - interval '1 hour' WHERE kid = $1`, next.Kid)
	require.NoError(t, err)
	_, err = svc.PublicJWKS(ctx)
	require.Error(t, err)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET not_before = now() - interval '1 hour', not_after = NULL WHERE kid = $1`, next.Kid)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DROP INDEX IF EXISTS signing_keys_one_next_uq`)
	require.NoError(t, err)
	plantSealedKey(t, pool, cfg, domain.SigningKeyStatusNext)
	_, err = svc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner), err)

	_, err = pool.Exec(ctx, `DELETE FROM signing_keys WHERE status = 'next'`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE signing_keys DROP CONSTRAINT IF EXISTS signing_keys_kid_uq`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO signing_keys (kid, alg, public_jwk, sealed_private_key, key_version, status, not_before)
SELECT kid, alg, public_jwk, sealed_private_key, key_version, 'next', now() FROM signing_keys WHERE kid = $1`, created.Kid)
	require.NoError(t, err)
	_, err = svc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner), err)
}

func TestCreateInitialActiveRejectsCorruptExistingActive(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	svc := keys.NewService(pool, cfg, "test")
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET sealed_private_key = overlay(sealed_private_key placing E'\\x00' from 8 for 1) WHERE kid = $1`, created.Kid)
	require.NoError(t, err)
	_, err = svc.CreateInitialActive(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, keys.ErrUnsealFailed), err)

	_, err = pool.Exec(ctx, `DELETE FROM signing_keys`)
	require.NoError(t, err)
	created, err = svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE signing_keys SET not_before = now() + interval '2 hours' WHERE kid = $1`, created.Kid)
	require.NoError(t, err)
	_, err = svc.CreateInitialActive(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable) || errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, domain.ErrInvalid), err)
}

func TestConcurrentInitialBootstrapCreatesExactlyOneActive(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL concurrency harness")
	}
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", true), "test")
	const callers = 8
	start := make(chan struct{})
	results := make(chan *domain.SigningKey, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := svc.CreateInitialActive(ctx)
			if err != nil {
				errs <- err
				return
			}
			results <- got
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	var created []*domain.SigningKey
	for rec := range results {
		created = append(created, rec)
	}
	var firstErr error
	for err := range errs {
		if firstErr == nil {
			firstErr = err
		}
	}
	require.NoError(t, firstErr)
	require.Len(t, created, callers)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = 'active'`).Scan(&n))
	assert.Equal(t, 1, n)
	for _, rec := range created[1:] {
		assert.Equal(t, created[0].Kid, rec.Kid)
	}
}

func TestCreateNextReturnsExistingAuthenticatedNext(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	first, err := svc.CreateNext(ctx)
	require.NoError(t, err)
	require.Equal(t, domain.SigningKeyStatusNext, first.Status)

	again, err := svc.CreateNext(ctx)
	require.NoError(t, err, "retry after an existing next must converge instead of ErrConflict")
	assert.Equal(t, first.Kid, again.Kid)
	assert.Equal(t, domain.SigningKeyStatusNext, again.Status)
	assert.Equal(t, first.PublicJWK, again.PublicJWK)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestConcurrentCreateNextConvergesOnOneKid(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL concurrency harness")
	}
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)

	const callers = 8
	start := make(chan struct{})
	results := make(chan *domain.SigningKey, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := svc.CreateNext(ctx)
			if err != nil {
				errs <- err
				return
			}
			results <- got
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	var created []*domain.SigningKey
	for rec := range results {
		created = append(created, rec)
	}
	var firstErr error
	for err := range errs {
		if firstErr == nil {
			firstErr = err
		}
	}
	require.NoError(t, firstErr)
	require.Len(t, created, callers)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&n))
	assert.Equal(t, 1, n)
	for _, rec := range created[1:] {
		assert.Equal(t, created[0].Kid, rec.Kid)
	}
}

func TestHighCountConcurrentBootstrapAndNextConverge(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL concurrency harness")
	}
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", true), "test")
	const callers = 32
	start := make(chan struct{})
	results := make(chan *domain.SigningKey, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := svc.CreateInitialActive(ctx)
			if err != nil {
				errs <- err
				return
			}
			results <- got
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var created []*domain.SigningKey
	for rec := range results {
		created = append(created, rec)
	}
	for err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, created, callers)
	for _, rec := range created[1:] {
		assert.Equal(t, created[0].Kid, rec.Kid)
	}

	start = make(chan struct{})
	results = make(chan *domain.SigningKey, callers)
	errs = make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := svc.CreateNext(ctx)
			if err != nil {
				errs <- err
				return
			}
			results <- got
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var next []*domain.SigningKey
	for rec := range results {
		next = append(next, rec)
	}
	for err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, next, callers)
	var activeCount, nextCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = 'active'`).Scan(&activeCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&nextCount))
	assert.Equal(t, 1, activeCount)
	assert.Equal(t, 1, nextCount)
	for _, rec := range next[1:] {
		assert.Equal(t, next[0].Kid, rec.Kid)
	}
}

func TestCreateInitialAndNextHonorCanceledContext(t *testing.T) {
	pool := dedicatedPool(t)
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.CreateInitialActive(ctx)
	require.ErrorIs(t, err, context.Canceled)
	_, err = svc.CreateNext(ctx)
	require.ErrorIs(t, err, context.Canceled)

	created, err := svc.CreateInitialActive(context.Background())
	require.NoError(t, err)
	nextCtx, nextCancel := context.WithCancel(context.Background())
	nextCancel()
	_, err = svc.CreateNext(nextCtx)
	require.ErrorIs(t, err, context.Canceled)
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&n))
	assert.Zero(t, n)
	_ = created
}

func TestCreateNextFailsClosedOnMultipleNextRows(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	svc := keys.NewService(pool, cfg, "test")
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	plantSealedKey(t, pool, cfg, domain.SigningKeyStatusNext)
	_, err = pool.Exec(ctx, `DROP INDEX IF EXISTS signing_keys_one_next_uq`)
	require.NoError(t, err)
	plantSealedKey(t, pool, cfg, domain.SigningKeyStatusNext)

	_, err = svc.CreateNext(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, domain.ErrConflict), err)
	assert.NotContains(t, err.Error(), `"d"`)
}

func TestServiceErrorsOmitPrivateBytes(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	svc := keys.NewService(pool, cfg, "test")
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	_, err = svc.CreateNext(ctx)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DROP INDEX IF EXISTS signing_keys_one_next_uq`)
	require.NoError(t, err)
	plantSealedKey(t, pool, cfg, domain.SigningKeyStatusNext)
	_, err = svc.CreateNext(ctx)
	require.Error(t, err)
	formatted := fmt.Sprintf("%v %#v", err, svc)
	assert.NotContains(t, formatted, "PLANT_SEAL_SECRET_VALUE_AAAA")
	assert.NotContains(t, err.Error(), `"d"`)
	_ = created
}

func TestDisabledCustodyOperationsFailBeforeDatabaseAccess(t *testing.T) {
	for _, env := range []string{"test", "production"} {
		pool := dedicatedPool(t)
		cfg := custodyConfig(t, env, false)
		cfg.Enabled = false
		svc := keys.NewService(pool, cfg, env)
		ctx := context.Background()
		want := func(err error) {
			t.Helper()
			require.Error(t, err)
			assert.ErrorIs(t, err, keys.ErrCustodyDisabled)
		}

		_, err := svc.CreateInitialActive(ctx)
		want(err)
		_, _, err = svc.ActiveSigner(ctx)
		want(err)
		_, err = svc.PublicJWKS(ctx)
		want(err)
		_, err = svc.CreateNext(ctx)
		want(err)
		_, err = svc.ActivateNext(ctx)
		want(err)
		pool.Close()
	}
}

func TestNewServiceRejectsProgrammaticInvalidConfiguration(t *testing.T) {
	pool := dedicatedPool(t)
	pool.Close()
	ctx := context.Background()

	invalid := config.KeyConfig{Enabled: true}
	invalidSvc := keys.NewService(pool, invalid, "test")
	_, err := invalidSvc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalid)

	unsupported := keys.NewService(pool, custodyConfig(t, "test", false), "staging")
	_, err = unsupported.PublicJWKS(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalid)

	invalidAuto := custodyConfig(t, "test", false)
	invalidAuto.AutoBootstrap = true
	invalidAuto.Enabled = false
	invalidAutoSvc := keys.NewService(pool, invalidAuto, "test")
	_, err = invalidAutoSvc.PublicJWKS(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalid)
}

func TestManagedSignerRejectsStaleNonActiveRow(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	signer, _, err := svc.ActiveSigner(ctx)
	require.NoError(t, err)
	digest := sha256.Sum256([]byte("pre-retire"))
	_, err = signer.Sign(rand.Reader, digest[:], nil)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
UPDATE signing_keys
SET status='retired', retired_at=now()
WHERE kid=$1`, created.Kid)
	require.NoError(t, err)

	_, err = signer.PublicJWK()
	require.NoError(t, err) // public metadata may still be readable locally
	sig, err := signer.Sign(rand.Reader, digest[:], nil)
	requireGenericSignerFailure(t, err, sig)

	_, _, err = svc.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrSignerUnavailable), err)
}

func TestServiceCloseDestroysOutstandingHolders(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	signer, _, err := svc.ActiveSigner(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.Close())
	_, err = signer.PublicJWK()
	require.Error(t, err)
	digest := sha256.Sum256([]byte("closed"))
	_, err = signer.Sign(rand.Reader, digest[:], nil)
	require.Error(t, err)
}

func TestExternalDBMutationFailsClosedOnNextServiceLoad(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	first := keys.NewService(pool, cfg, "test")
	created, err := first.CreateInitialActive(ctx)
	require.NoError(t, err)
	holder, _, err := first.ActiveSigner(ctx)
	require.NoError(t, err)
	require.NotNil(t, holder)

	_, err = pool.Exec(ctx, `UPDATE signing_keys SET sealed_private_key = overlay(sealed_private_key placing E'\\x00' from 8 for 1) WHERE kid = $1`, created.Kid)
	require.NoError(t, err)

	next := keys.NewService(pool, cfg, "test")
	_, err = next.PublicJWKS(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, keys.ErrUnsealFailed), err)
	_, _, err = next.ActiveSigner(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner) || errors.Is(err, keys.ErrUnsealFailed), err)
}

func TestManagedSignerRejectsCrossProcessStatusChange(t *testing.T) {
	poolA, url := dedicatedPoolURL(t)
	t.Cleanup(poolA.Close)
	poolB := connectPool(t, url)

	ctx := context.Background()
	cfg := custodyConfig(t, "test", false)
	serviceA := keys.NewService(poolA, cfg, "test")
	created, err := serviceA.CreateInitialActive(ctx)
	require.NoError(t, err)
	signer, _, err := serviceA.ActiveSigner(ctx)
	require.NoError(t, err)

	_, err = poolB.Exec(ctx, `
UPDATE signing_keys SET status='retired', retired_at=now() WHERE kid=$1`, created.Kid)
	require.NoError(t, err)

	digest := sha256.Sum256([]byte("cross-process-retirement"))
	sig, err := signer.Sign(rand.Reader, digest[:], nil)
	requireGenericSignerFailure(t, err, sig)
	_, err = signer.Sign(rand.Reader, digest[:], nil)
	require.ErrorIs(t, err, keys.ErrSignerRevoked)
}

func TestManagedSignerRejectsExternalStatusPublicAndSealedCorruption(t *testing.T) {
	for _, mutate := range []struct {
		name string
		fn   func(t *testing.T, pool *pgxpool.Pool, kid string)
	}{
		{
			name: "status",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				_, err := pool.Exec(context.Background(), `UPDATE signing_keys SET status = 'next', activated_at = NULL WHERE kid = $1`, kid)
				require.NoError(t, err)
			},
		},
		{
			name: "public_jwk",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				other, err := keys.Generate()
				require.NoError(t, err)
				t.Cleanup(func() { _ = other.Destroy() })
				public, err := other.PublicJWK()
				require.NoError(t, err)
				public.Kid = kid
				raw, err := public.MarshalJSON()
				require.NoError(t, err)
				_, err = pool.Exec(context.Background(), `UPDATE signing_keys SET public_jwk = $2::jsonb WHERE kid = $1`, kid, raw)
				require.NoError(t, err)
			},
		},
		{
			name: "sealed_private",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				_, err := pool.Exec(context.Background(), `UPDATE signing_keys SET sealed_private_key = overlay(sealed_private_key placing E'\\x00' from 8 for 1) WHERE kid = $1`, kid)
				require.NoError(t, err)
			},
		},
		{
			name: "id",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				_, err := pool.Exec(context.Background(), `UPDATE signing_keys SET id = gen_random_uuid() WHERE kid = $1`, kid)
				require.NoError(t, err)
			},
		},
		{
			name: "created_at",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				_, err := pool.Exec(context.Background(), `UPDATE signing_keys SET created_at = created_at - interval '1 second' WHERE kid = $1`, kid)
				require.NoError(t, err)
			},
		},
		{
			name: "activated_at",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				_, err := pool.Exec(context.Background(), `UPDATE signing_keys SET activated_at = activated_at + interval '1 second' WHERE kid = $1`, kid)
				require.NoError(t, err)
			},
		},
		{
			// Donor 53b693cc93c8cccb15109d90bae90811029af893 row-replacement
			// probe: delete+reinsert must keep created_at and
			// activated_at>=created_at so signing_keys_status_material_ck
			// is not the failure mode. A new id still changes the
			// complete-row fingerprint and must fail closed.
			name: "row_replacement",
			fn: func(t *testing.T, pool *pgxpool.Pool, kid string) {
				t.Helper()
				tag, err := pool.Exec(context.Background(), `
WITH old AS (
    DELETE FROM signing_keys WHERE kid = $1
    RETURNING kid, alg, key_version, public_jwk, sealed_private_key, status, not_before, not_after, created_at, activated_at
)
INSERT INTO signing_keys (kid, alg, key_version, public_jwk, sealed_private_key, status, not_before, not_after, created_at, activated_at)
SELECT kid, alg, key_version, public_jwk, sealed_private_key, status, not_before, not_after, created_at, activated_at FROM old`, kid)
				require.NoError(t, err)
				require.Equal(t, int64(1), tag.RowsAffected())
			},
		},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			pool := dedicatedPool(t)
			ctx := context.Background()
			cfg := custodyConfig(t, "test", false)
			service := keys.NewService(pool, cfg, "test")
			created, err := service.CreateInitialActive(ctx)
			require.NoError(t, err)
			signer, _, err := service.ActiveSigner(ctx)
			require.NoError(t, err)
			mutate.fn(t, pool, created.Kid)

			digest := sha256.Sum256([]byte("external-corruption-" + mutate.name))
			sig, err := signer.Sign(rand.Reader, digest[:], nil)
			requireGenericSignerFailure(t, err, sig)
		})
	}
}

func TestManagedSignerRejectsClosedPoolWithoutSignature(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	service := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	_, err := service.CreateInitialActive(ctx)
	require.NoError(t, err)
	signer, _, err := service.ActiveSigner(ctx)
	require.NoError(t, err)
	pool.Close()

	digest := sha256.Sum256([]byte("closed-pool"))
	sig, err := signer.Sign(rand.Reader, digest[:], nil)
	requireGenericSignerFailure(t, err, sig)
}

func TestManagedSignerRejectsDatabaseLockTimeoutWithoutSignature(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	service := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	created, err := service.CreateInitialActive(ctx)
	require.NoError(t, err)
	signer, _, err := service.ActiveSigner(ctx)
	require.NoError(t, err)

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	t.Cleanup(conn.Release)
	lockTx, err := conn.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lockTx.Rollback(context.Background()) })
	_, err = lockTx.Exec(ctx, `SELECT id FROM signing_keys WHERE kid = $1 FOR UPDATE`, created.Kid)
	require.NoError(t, err)

	digest := sha256.Sum256([]byte("database-timeout"))
	started := time.Now()
	sig, err := signer.Sign(rand.Reader, digest[:], nil)
	requireGenericSignerFailure(t, err, sig)
	assert.LessOrEqual(t, time.Since(started), 2500*time.Millisecond)

	require.NoError(t, lockTx.Rollback(context.Background()))
	recovered, _, err := service.ActiveSigner(ctx)
	require.NoError(t, err)
	assert.NotSame(t, signer, recovered)
	_, err = signer.Sign(rand.Reader, digest[:], nil)
	require.ErrorIs(t, err, keys.ErrSignerRevoked)
	_, err = recovered.Sign(rand.Reader, digest[:], nil)
	require.NoError(t, err)
}

func TestReadyFailsClosedWithoutUsableActive(t *testing.T) {
	pool := dedicatedPool(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t, "test", false), "test")
	require.Error(t, svc.Ready(ctx))
	_, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.Ready(ctx))
}

type blockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return io.ReadFull(rand.Reader, p)
}
