package keys

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
)

// Donor 53b693cc93c8cccb15109d90bae90811029af893 commit/fencing probes,
// reshaped for IB2 lane A: no Retire/Destroy admin surface (IB7).

func TestCreateInitialCommitFailureDoesNotClaimSuccess(t *testing.T) {
	pool := commitErrorPool(t)
	svc := NewService(pool, commitErrorConfig(t), "test")

	planted := errors.New("planted rolled-back initial commit failure")
	svc.testCreateInitialCommit = func(ctx context.Context, tx pgx.Tx) error {
		require.NoError(t, tx.Rollback(ctx))
		return planted
	}
	_, err := svc.CreateInitialActive(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, planted)
	require.NotContains(t, strings.ToLower(err.Error()), "private")

	var active int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM signing_keys WHERE status = 'active'`).Scan(&active))
	require.Zero(t, active)
	_, _, err = svc.ActiveSigner(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrSignerUnavailable)
}

func TestCreateNextCommitSuccessWithErrorReconcilesCommittedCandidate(t *testing.T) {
	pool := commitErrorPool(t)
	svc := NewService(pool, commitErrorConfig(t), "test")
	_, err := svc.CreateInitialActive(context.Background())
	require.NoError(t, err)

	planted := errors.New("planted successful commit response loss")
	svc.testCreateNextCommit = func(ctx context.Context, tx pgx.Tx) error {
		require.NoError(t, tx.Commit(ctx))
		return planted
	}
	next, err := svc.CreateNext(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, next.Kid)

	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&count))
	require.Equal(t, 1, count)
	svc.testCreateNextCommit = nil
	again, err := svc.CreateNext(context.Background())
	require.NoError(t, err)
	require.Equal(t, next.Kid, again.Kid)
}

func TestCreateNextCommitRollbackWithErrorDoesNotClaimSuccess(t *testing.T) {
	pool := commitErrorPool(t)
	svc := NewService(pool, commitErrorConfig(t), "test")
	_, err := svc.CreateInitialActive(context.Background())
	require.NoError(t, err)

	planted := errors.New("planted rolled-back commit failure")
	svc.testCreateNextCommit = func(ctx context.Context, tx pgx.Tx) error {
		require.NoError(t, tx.Rollback(ctx))
		return planted
	}
	_, err = svc.CreateNext(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, planted)

	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&count))
	require.Zero(t, count)
}

func TestManagedSignerReleasesValidationRowBeforeBlockedRevocation(t *testing.T) {
	poolA, url := commitErrorPoolURL(t)
	t.Cleanup(poolA.Close)
	poolB := connectInternalPool(t, url)

	cfg := commitErrorConfig(t)
	serviceA := NewService(poolA, cfg, "test")
	created, err := serviceA.CreateInitialActive(context.Background())
	require.NoError(t, err)
	signer, _, err := serviceA.ActiveSigner(context.Background())
	require.NoError(t, err)
	signerA := *signer
	signerB := *signer

	_, err = poolB.Exec(context.Background(), `
		UPDATE signing_keys SET created_at = created_at - interval '1 second' WHERE kid = $1`, created.Kid)
	require.NoError(t, err)

	randomStarted := make(chan struct{})
	releaseRandom := make(chan struct{})
	digest := sha256.Sum256([]byte("validation-row-release"))
	type signResult struct {
		signature []byte
		err       error
	}
	signADone := make(chan signResult, 1)
	signBDone := make(chan signResult, 1)
	go func() {
		signature, signErr := signerA.Sign(&internalBlockingReader{started: randomStarted, release: releaseRandom}, digest[:], crypto.SHA256)
		signADone <- signResult{signature: signature, err: signErr}
	}()
	select {
	case <-randomStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("signer A did not stall in randomness")
	}

	validationEntered := make(chan struct{})
	allowRevocation := make(chan struct{})
	serviceA.testBeforeSignerValidationFailure = func() {
		select {
		case <-validationEntered:
		default:
			close(validationEntered)
		}
		<-allowRevocation
	}
	go func() {
		signature, signErr := signerB.Sign(rand.Reader, digest[:], crypto.SHA256)
		signBDone <- signResult{signature: signature, err: signErr}
	}()
	select {
	case <-validationEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("signer B did not enter validation failure")
	}

	// IB2 has no Retire admin. A second process must still be able to take
	// FOR UPDATE on the same row while signer B is blocked in revocation.
	repairDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), managedSignerDBTimeout+500*time.Millisecond)
		defer cancel()
		conn, acquireErr := poolB.Acquire(ctx)
		if acquireErr != nil {
			repairDone <- acquireErr
			return
		}
		defer conn.Release()
		tx, beginErr := conn.Begin(ctx)
		if beginErr != nil {
			repairDone <- beginErr
			return
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		_, lockErr := tx.Exec(ctx, `SELECT id FROM signing_keys WHERE kid = $1 FOR UPDATE`, created.Kid)
		if lockErr != nil {
			repairDone <- lockErr
			return
		}
		repairDone <- tx.Commit(ctx)
	}()
	select {
	case repairErr := <-repairDone:
		require.NoError(t, repairErr)
	case <-time.After(managedSignerDBTimeout + time.Second):
		t.Fatal("cross-process row lock remained blocked by signer B revocation")
	}
	close(allowRevocation)
	select {
	case <-signBDone:
		t.Fatal("signer B destruction completed while signer A still held Material RLock")
	default:
	}

	close(releaseRandom)
	signA := <-signADone
	signB := <-signBDone
	requireGenericInternalSignerFailure(t, signA.err, signA.signature)
	requireGenericInternalSignerFailure(t, signB.err, signB.signature)
	_, err = signerA.Sign(rand.Reader, digest[:], crypto.SHA256)
	require.ErrorIs(t, err, ErrSignerRevoked)
	_, err = signerB.Sign(rand.Reader, digest[:], crypto.SHA256)
	require.ErrorIs(t, err, ErrSignerRevoked)
}

func TestCreateInitialActiveGeneratesBeforeTransactionAndConverges(t *testing.T) {
	poolA, url := commitErrorPoolURL(t)
	t.Cleanup(poolA.Close)
	poolB := connectInternalPool(t, url)

	cfg := commitErrorConfig(t)
	serviceA := NewService(poolA, cfg, "test")
	serviceB := NewService(poolB, cfg, "test")
	candidate, err := serviceA.generateRecord(domain.SigningKeyStatusActive)
	require.NoError(t, err)
	started := make(chan struct{})
	release := make(chan struct{})
	serviceA.testGenerateRecord = func(status string) (repo.SigningKeyRecord, error) {
		require.Equal(t, domain.SigningKeyStatusActive, status)
		close(started)
		<-release
		return candidate, nil
	}

	result := make(chan *domain.SigningKey, 1)
	errResult := make(chan error, 1)
	go func() {
		got, createErr := serviceA.CreateInitialActive(context.Background())
		result <- got
		errResult <- createErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("initial active generation did not start")
	}

	createCtx, cancel := context.WithTimeout(context.Background(), managedSignerDBTimeout)
	startedAt := time.Now()
	created, err := serviceB.CreateInitialActive(createCtx)
	cancel()
	require.NoError(t, err)
	require.Less(t, time.Since(startedAt), managedSignerDBTimeout)
	close(release)
	createErr := <-errResult
	require.NoError(t, createErr)
	converged := <-result
	require.Equal(t, created.Kid, converged.Kid)
	require.NotEmpty(t, candidate.SealedPrivateKey)
	require.True(t, bytes.Equal(candidate.SealedPrivateKey, make([]byte, len(candidate.SealedPrivateKey))), "unused candidate must be zeroed")
}

func TestCreateNextGeneratesBeforeTransactionAndFailsSafelyAfterActiveGone(t *testing.T) {
	poolA, url := commitErrorPoolURL(t)
	t.Cleanup(poolA.Close)
	poolB := connectInternalPool(t, url)

	cfg := commitErrorConfig(t)
	serviceA := NewService(poolA, cfg, "test")
	active, err := serviceA.CreateInitialActive(context.Background())
	require.NoError(t, err)
	candidate, err := serviceA.generateRecord(domain.SigningKeyStatusNext)
	require.NoError(t, err)
	started := make(chan struct{})
	release := make(chan struct{})
	serviceA.testGenerateRecord = func(status string) (repo.SigningKeyRecord, error) {
		require.Equal(t, domain.SigningKeyStatusNext, status)
		close(started)
		<-release
		return candidate, nil
	}

	result := make(chan *domain.SigningKey, 1)
	errResult := make(chan error, 1)
	go func() {
		got, createErr := serviceA.CreateNext(context.Background())
		result <- got
		errResult <- createErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("next generation did not start")
	}

	deleteCtx, cancel := context.WithTimeout(context.Background(), managedSignerDBTimeout)
	startedAt := time.Now()
	_, err = poolB.Exec(deleteCtx, `DELETE FROM signing_keys WHERE kid = $1`, active.Kid)
	cancel()
	require.NoError(t, err)
	require.Less(t, time.Since(startedAt), managedSignerDBTimeout)
	close(release)
	createErr := <-errResult
	require.Error(t, createErr)
	require.Nil(t, <-result)
	require.ErrorIs(t, createErr, domain.ErrSignerUnavailable)
	require.True(t, bytes.Equal(candidate.SealedPrivateKey, make([]byte, len(candidate.SealedPrivateKey))), "unused candidate must be zeroed")
}

func TestCreateInitialAndNextHonorCanceledContextBeforeRetry(t *testing.T) {
	pool := commitErrorPool(t)
	svc := NewService(pool, commitErrorConfig(t), "test")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.CreateInitialActive(ctx)
	require.ErrorIs(t, err, context.Canceled)

	_, err = svc.CreateInitialActive(context.Background())
	require.NoError(t, err)

	nextCtx, nextCancel := context.WithCancel(context.Background())
	nextCancel()
	_, err = svc.CreateNext(nextCtx)
	require.ErrorIs(t, err, context.Canceled)

	var next int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM signing_keys WHERE status = 'next'`).Scan(&next))
	require.Zero(t, next)
}

func TestManagedSignerRevalidatesCompleteRowBeforeReturningSignature(t *testing.T) {
	pool := commitErrorPool(t)
	svc := NewService(pool, commitErrorConfig(t), "test")
	created, err := svc.CreateInitialActive(context.Background())
	require.NoError(t, err)
	signer, _, err := svc.ActiveSigner(context.Background())
	require.NoError(t, err)

	digest := sha256.Sum256([]byte("db-revalidation-ok"))
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	require.NoError(t, err)
	require.NotEmpty(t, sig)

	_, err = pool.Exec(context.Background(), `UPDATE signing_keys SET created_at = created_at - interval '1 second' WHERE kid = $1`, created.Kid)
	require.NoError(t, err)
	sig, err = signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	requireGenericInternalSignerFailure(t, err, sig)
}

func requireGenericInternalSignerFailure(t *testing.T, err error, sig []byte) {
	t.Helper()
	require.Error(t, err)
	require.Nil(t, sig)
	require.ErrorIs(t, err, ErrSignerRevoked)
}

type internalBlockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *internalBlockingReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return io.ReadFull(rand.Reader, p)
}

func commitErrorConfig(t *testing.T) config.KeyConfig {
	t.Helper()
	cfg := config.KeyConfig{Enabled: true}
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	cfg.SetSealSecretForTest(base64.RawURLEncoding.EncodeToString(raw))
	for i := range raw {
		raw[i] = 0
	}
	require.NoError(t, cfg.Validate("test"))
	return cfg
}

func commitErrorPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := commitErrorPoolURL(t)
	return pool
}

func commitErrorPoolURL(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
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
	require.NoError(t, db.Migrate(ctx, url))
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, url
}

func connectInternalPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}
