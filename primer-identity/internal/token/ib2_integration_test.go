package token_test

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/token"
)

type keyServiceSignerSource struct{ svc *keys.Service }

func (s keyServiceSignerSource) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	signer, meta, err := s.svc.ActiveSigner(ctx)
	if err != nil {
		return nil, nil, err
	}
	return signer, meta, nil
}

func custodyConfig(t *testing.T) config.KeyConfig {
	t.Helper()
	var cfg config.KeyConfig
	cfg.Enabled = true
	cfg.SetSealSecretForTest("UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE")
	require.NoError(t, cfg.Validate("test"))
	return cfg
}

func TestIssueVerifyWithRealKeyServiceAndPostgres(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t), "test")
	t.Cleanup(func() { _ = svc.Close() })
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)

	clock := frozenClock{now: time.Date(2026, 8, 16, 16, 0, 0, 0, time.UTC)}
	minter, err := token.NewMinter(keyServiceSignerSource{svc: svc}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(ctx, humanInput(t), commitOK)
	require.NoError(t, err)
	require.NotEmpty(t, issued.Compact)
	assert.Equal(t, created.Kid, issued.Kid)
	assert.Equal(t, 15*time.Minute, issued.Lifetime)

	keyset, err := token.NewKeySet(func(ctx context.Context) ([]domain.PublicJWK, error) {
		return svc.PublicJWKS(ctx)
	})
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(ctx))
	verifier, err := token.NewVerifier(keyset, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)
	got, err := verifier.Verify(ctx, issued.Compact)
	require.NoError(t, err)
	assert.Equal(t, token.KindHuman, got.Kind)
	assert.Equal(t, issued.JTI, got.JTI)
}

func TestIssueHumanFailsWhenActiveSignerRevokedMidflight(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	base := newTestSigner(t)
	revoked := &revocableSigner{Signer: base}
	minter, err := token.NewMinter(staticSignerSource{signer: revoked}, testIssuer, clock)
	require.NoError(t, err)
	revoked.fail.Store(true)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, issued.Compact)
}

type revocableSigner struct {
	token.Signer
	fail atomic.Bool
}

func (s *revocableSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if s.fail.Load() {
		return nil, io.ErrClosedPipe
	}
	return s.Signer.Sign(rand, digest, opts)
}

func TestHighCountIssueVerify(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	minter, err := token.NewMinter(staticSignerSource{signer: signer}, testIssuer, clock)
	require.NoError(t, err)
	src := &staticKeySource{keys: map[string]domain.PublicJWK{jwk.Kid: jwk}}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)
	const n = 200
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := verifier.Verify(context.Background(), issued.Compact); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestHighCountIssueVerifyWithKeyService(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	svc := keys.NewService(pool, custodyConfig(t), "test")
	t.Cleanup(func() { _ = svc.Close() })
	created, err := svc.CreateInitialActive(ctx)
	require.NoError(t, err)

	clock := frozenClock{now: time.Date(2026, 8, 16, 16, 30, 0, 0, time.UTC)}
	tracer := &tokenTracingSignerSource{svc: svc}
	minter, err := token.NewMinter(tracer, testIssuer, clock)
	require.NoError(t, err)
	keyset, err := token.NewKeySet(func(ctx context.Context) ([]domain.PublicJWK, error) {
		return svc.PublicJWKS(ctx)
	})
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(ctx))
	verifier, err := token.NewVerifier(keyset, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)

	const n = 64
	start := make(chan struct{})
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			issued, err := minter.IssueHuman(ctx, humanInput(t), commitOK)
			if err != nil {
				errCh <- fmt.Errorf("issue: %w [root=%v]", err, tracer.root())
				return
			}
			if issued.Kid != created.Kid {
				errCh <- fmt.Errorf("kid mismatch got=%s want=%s", issued.Kid, created.Kid)
				return
			}
			if _, err := verifier.Verify(ctx, issued.Compact); err != nil {
				errCh <- fmt.Errorf("verify: %w", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

type tokenTracingSignerSource struct {
	svc  *keys.Service
	last atomic.Value
}

func (s *tokenTracingSignerSource) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	signer, meta, err := s.svc.ActiveSigner(ctx)
	if err != nil {
		s.last.Store(err)
		return nil, nil, err
	}
	return tokenTracingSigner{Signer: signer, last: &s.last}, meta, nil
}

func (s *tokenTracingSignerSource) root() error {
	got, _ := s.last.Load().(error)
	return got
}

type tokenTracingSigner struct {
	token.Signer
	last *atomic.Value
}

func (s tokenTracingSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	sig, err := s.Signer.Sign(rand, digest, opts)
	if err != nil && s.last != nil {
		s.last.Store(err)
	}
	return sig, err
}

func TestUnknownKidRefreshFailureIsUnavailable(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, _, _ := issueHuman(t, clock)
	src := &staticKeySource{refreshErr: io.ErrUnexpectedEOF}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)
	principal, err := verifier.Verify(context.Background(), issued.Compact)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, principal)
	assert.Equal(t, 1, src.refreshCount())
}

func TestNewKeySetRejectsNilLoader(t *testing.T) {
	_, err := token.NewKeySet(nil)
	require.ErrorIs(t, err, token.ErrUnavailable)
}

func TestParseJWKSRejectsEmptyAndTrailing(t *testing.T) {
	_, err := token.ParseJWKS(nil)
	require.ErrorIs(t, err, token.ErrInvalid)
	_, err = token.ParseJWKS([]byte(`{"keys":[]}`))
	require.ErrorIs(t, err, token.ErrInvalid)
	_, err = token.ParseJWKS([]byte(`{"keys":[],"extra":1}`))
	require.ErrorIs(t, err, token.ErrInvalid)
}
