package oauth_test

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
	"github.com/aleksclark/primer/identity/internal/token"
)

func TestRefreshIsUnsupportedAndClientCredentialsRequiresAuth(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})

	_, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{GrantType: oauth.GrantRefreshToken, RefreshToken: "opaque"}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorUnsupportedGrantType, oauth.ErrorCodeOf(err))

	_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{GrantType: oauth.GrantClientCredentials, Resource: fx.redirect.ResourceURI}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidRequest, oauth.ErrorCodeOf(err))
}

func TestWrongBindingsAndPKCEAreInvalidGrant(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 5, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	base := oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}
	auth := oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID}

	wrongRedirect := base
	wrongRedirect.RedirectURI = "https://other.example/callback"
	_, err := svc.Exchange(context.Background(), wrongRedirect, auth)
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))

	wrongResource := base
	wrongResource.Resource = "https://other.example/resource"
	_, err = svc.Exchange(context.Background(), wrongResource, auth)
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))

	wrongPKCE := base
	wrongPKCE.CodeVerifier = strings.Repeat("b", 43)
	_, err = svc.Exchange(context.Background(), wrongPKCE, auth)
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))

	var consumed *time.Time
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.Nil(t, consumed)
}

func TestInternalUUIDClientIDIsRejected(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 6, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	_, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ID.String(),
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ID.String()})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
}

func TestConfidentialBasicExchangeAndWrongSecret(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 10, 0, 0, time.UTC)
	plaintext := "basic-secret-" + uuid.NewString()
	fx := issuedConfidentialBasicCode(t, now, plaintext)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})

	_, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, RedirectURI: fx.redirect.RedirectURI,
		Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthBasic, ClientID: fx.client.ClientID, Secret: "wrong-secret-value-xxxxxxxxxxxxxxxx"})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))

	resp, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, RedirectURI: fx.redirect.RedirectURI,
		Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthBasic, ClientID: fx.client.ClientID, Secret: plaintext})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)
	assert.NotContains(t, resp.AccessToken, plaintext)
	assert.NotContains(t, resp.RefreshToken, plaintext)
}

func TestPrivateKeyJWTExchangeAndReplayDenied(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 15, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedPrivateKeyJWTCode(t, now, clientMat)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	assertion := mintClientAssertion(t, clientMat, fx.client.ClientID, testTokenEndpoint, now, 2*time.Minute)

	req := oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     assertion,
	}
	resp, err := svc.Exchange(context.Background(), req, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)

	fx2 := issuedPrivateKeyJWTCode(t, now, clientMat)
	_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx2.rawCode, ClientID: fx2.client.ClientID,
		RedirectURI: fx2.redirect.RedirectURI, Resource: fx2.redirect.ResourceURI, CodeVerifier: testVerifier,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     assertion,
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx2.client.ClientID, Assertion: assertion})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
}

func TestPrivateKeyJWTPostExpSkewRecordsRetentionAndDeniesReplay(t *testing.T) {
	// Presentation at JWT exp+59s is still accepted by the parser. Ledger must
	// retain through exp+AssertionClockSkew so purge-at-exp cannot open a
	// replay window, and lifetime-edge record failures stay invalid_client.
	iat := time.Date(2026, 8, 17, 16, 15, 0, 0, time.UTC)
	jwtTTL := 2 * time.Minute
	jwtExp := iat.Add(jwtTTL)
	presentAt := jwtExp.Add(59 * time.Second)

	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedPrivateKeyJWTCode(t, presentAt, clientMat)
	svc := newTestService(t, fx.secrets, frozenClock{now: presentAt})
	assertion := mintClientAssertion(t, clientMat, fx.client.ClientID, testTokenEndpoint, iat, jwtTTL)

	req := oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     assertion,
	}
	resp, err := svc.Exchange(context.Background(), req, oauth.ClientAuth{
		Method: oauth.AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)

	var storedExp time.Time
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT expires_at FROM oauth_client_assertion_replays
WHERE oauth_client_id=$1 AND endpoint_kind=$2
ORDER BY consumed_at DESC LIMIT 1`, fx.client.ID, domain.AssertionEndpointToken).Scan(&storedExp))
	require.True(t, storedExp.Equal(jwtExp.Add(token.AssertionClockSkew)) || storedExp.After(jwtExp),
		"retention must outlive JWT exp; got %s jwtExp %s", storedExp, jwtExp)
	require.True(t, !storedExp.Before(jwtExp.Add(token.AssertionClockSkew)),
		"retention must be at least exp+skew; got %s want >= %s", storedExp, jwtExp.Add(token.AssertionClockSkew))

	// Purge at JWT exp must leave the ledger row (still inside skew window).
	purged, err := svc.PurgeExpiredAssertionReplays(context.Background(), jwtExp)
	require.NoError(t, err)
	require.Zero(t, purged)

	// Same client + same jti after purge-at-exp is still invalid_client (replay retained).
	fx2 := issuedCodeOnClient(t, presentAt, fx)
	_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx2.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     assertion,
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
	assert.NotEqual(t, oauth.ErrorTemporarilyUnavail, oauth.ErrorCodeOf(err))

	// After retention + ε the row is purgeable.
	purged, err = svc.PurgeExpiredAssertionReplays(context.Background(), jwtExp.Add(token.AssertionClockSkew).Add(time.Second))
	require.NoError(t, err)
	require.GreaterOrEqual(t, purged, int64(1))
}

func TestConcurrentCodeExchangeHasExactlyOneSuccess(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 20, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	req := oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}
	auth := oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID}

	const n = 16
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	okCh := make(chan oauth.TokenResponse, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := svc.Exchange(context.Background(), req, auth)
			if err != nil {
				errCh <- err
				return
			}
			okCh <- resp
		}()
	}
	wg.Wait()
	close(errCh)
	close(okCh)
	require.Len(t, okCh, 1)
	for err := range errCh {
		code := oauth.ErrorCodeOf(err)
		assert.True(t, code == oauth.ErrorInvalidGrant || code == oauth.ErrorTemporarilyUnavail, err)
	}
}

func TestSignerFailureRollsBackCodeAndFamily(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 25, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  failingSignerSource{},
		Secrets: fx.secrets,
		Clock:   frozenClock{now: now},
		Config:  oauth.Config{Issuer: testIssuer, TokenEndpoint: testTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.NoError(t, err)
	_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorTemporarilyUnavail, oauth.ErrorCodeOf(err))

	var families, audits int
	var consumed *time.Time
	pool := testutil.DB(t)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1`, fx.grant.ID).Scan(&audits))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.Zero(t, families)
	require.Zero(t, audits)
	require.Nil(t, consumed)
}

func TestAuthorizationCodePepperRotationRedeemsOldHash(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 33, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	rotated := fx.secrets
	rotated.AuthorizationCodePeppers = map[int][]byte{
		1: append([]byte(nil), fx.secrets.AuthorizationCodePeppers[1]...),
		2: func() []byte {
			out := make([]byte, 32)
			for i := range out {
				out[i] = 0x42
			}
			return out
		}(),
	}
	rotated.AuthorizationCodeActiveVersion = 2
	svc := newTestService(t, rotated, frozenClock{now: now})
	resp, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
}

func TestCanceledContextDoesNotConsumeCode(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 30, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	pool := testutil.DB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := svc.Exchange(ctx, oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorTemporarilyUnavail, oauth.ErrorCodeOf(err))
	assert.Empty(t, resp.AccessToken)
	assert.Empty(t, resp.RefreshToken)
	var families int
	var consumed *time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.Zero(t, families)
	require.Nil(t, consumed)
}

func TestLostResponseConsumesCode(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 35, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	resp, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)

	_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))
}

func TestProviderCapShortensAccessTTL(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 40, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT 1`).Scan(new(int)))
	_, err := testutil.DB(t).Exec(context.Background(), `UPDATE provider_session_associations SET provider_expires_at=$1 WHERE id=(SELECT provider_session_association_id FROM oauth_grants WHERE id=$2)`, now.Add(90*time.Second), fx.grant.ID)
	require.NoError(t, err)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	resp, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	assert.Equal(t, 90, resp.ExpiresIn)
}

func TestConfiguredTTLAbove900FailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 45, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	_, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenClock{now: now},
		Config:  oauth.Config{Issuer: testIssuer, TokenEndpoint: testTokenEndpoint, AccessTTL: 901 * time.Second},
	})
	require.Error(t, err)
}

func TestCodePurgePreservesCopiedAudit(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 50, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	_, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)

	pool := testutil.DB(t)
	old := now.Add(-25 * time.Hour)
	_, err = pool.Exec(context.Background(), `UPDATE oauth_authorization_codes SET issued_at=$1::timestamptz, expires_at=$1::timestamptz + interval '30 seconds' WHERE id=$2`, old, fx.code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `UPDATE broker_transactions SET created_at=$1::timestamptz, expires_at=$1::timestamptz + interval '1 minute' WHERE id=$2`, old, fx.code.BrokerTransactionID)
	require.NoError(t, err)
	_, err = repo.PurgeBrokerTransactions(context.Background(), pool, now)
	require.NoError(t, err)

	var codeID *uuid.UUID
	var hash []byte
	var clientID string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT authorization_code_id, authorization_code_hash, client_id FROM token_issuance_audit WHERE grant_id=$1`, fx.grant.ID).Scan(&codeID, &hash, &clientID))
	require.Nil(t, codeID)
	require.Equal(t, fx.codeHash, hash)
	require.Equal(t, fx.client.ClientID, clientID)
}

func TestAssertionReplayPurgeIsBounded(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 0, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	purged, err := svc.PurgeExpiredAssertionReplays(context.Background(), now)
	require.NoError(t, err)
	require.GreaterOrEqual(t, purged, int64(0))
}

func TestHighCountPublicExchanges(t *testing.T) {
	base := time.Date(2026, 8, 17, 17, 5, 0, 0, time.UTC)
	const n = 64
	fxs := make([]issuedFixture, n)
	for i := 0; i < n; i++ {
		fxs[i] = issuedPublicCode(t, base.Add(time.Duration(i)*time.Minute))
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			now := base.Add(time.Duration(i) * time.Minute)
			fx := fxs[i]
			tracer := newTracingSignerSource(fx.keys)
			svc, err := oauth.NewService(oauth.Dependencies{
				Pool:    testutil.DB(t),
				Signer:  tracer,
				Secrets: fx.secrets,
				Clock:   frozenClock{now: now},
				Config: oauth.Config{
					Issuer:        testIssuer,
					TokenEndpoint: testTokenEndpoint,
					AccessTTL:     15 * time.Minute,
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("new service: %w", err)
				return
			}
			_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{
				GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
				RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
			}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
			if err != nil {
				errCh <- fmt.Errorf("exchange: %w [root=%v]", err, tracer.root())
				return
			}
			errCh <- nil
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestBoundedPoolHighCountPublicExchanges(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL concurrency harness")
	}
	base := time.Date(2026, 8, 17, 17, 40, 0, 0, time.UTC)
	for _, maxConns := range []int32{8, 16, 32} {
		t.Run(fmt.Sprintf("maxconns-%d", maxConns), func(t *testing.T) {
			runBoundedPublicExchanges(t, base, maxConns, 64)
			runBoundedPublicExchanges(t, base.Add(time.Hour), maxConns, 128)
		})
	}
}

func runBoundedPublicExchanges(t *testing.T, base time.Time, maxConns int32, n int) {
	t.Helper()
	pool := limitedTestPool(t, maxConns)
	fxs := make([]issuedFixture, n)
	for i := 0; i < n; i++ {
		fxs[i] = issuedPublicCode(t, base.Add(time.Duration(i)*time.Second))
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	var current, currentTx atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			now := base.Add(time.Duration(i) * time.Second)
			fx := fxs[i]
			tracer := newTracingSignerSource(fx.keys)
			svc, err := oauth.NewService(oauth.Dependencies{
				Pool:    pool,
				Signer:  tracer,
				Secrets: fx.secrets,
				Clock:   frozenClock{now: now},
				Config: oauth.Config{
					Issuer:        testIssuer,
					TokenEndpoint: testTokenEndpoint,
					AccessTTL:     15 * time.Minute,
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("new service: %w", err)
				return
			}
			_, err = svc.Exchange(context.Background(), oauth.ExchangeRequest{
				GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
				RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
			}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
			current.Add(tracer.current.Load())
			currentTx.Add(tracer.currentTx.Load())
			errCh <- err
		}(i)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(errCh)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(45 * time.Second):
		t.Fatal("bounded public exchanges did not complete")
	}
	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, int64(0), current.Load(), "issuance must not check out a nested ActiveSigner")
	assert.GreaterOrEqual(t, currentTx.Load(), int64(n), "each successful exchange prepares a transaction-bound signer")
	assert.LessOrEqual(t, currentTx.Load(), int64(n)*8, "signer checkouts stay within serializable retry bounds")
	assert.LessOrEqual(t, pool.Stat().MaxConns(), maxConns)
}

func limitedTestPool(t *testing.T, maxConns int32) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testutil.DatabaseURL(t))
	require.NoError(t, err)
	cfg.MaxConns = maxConns
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestServiceHasNoHTTPOrRefreshRedemption(t *testing.T) {
	_, ok := any((*oauth.Service)(nil)).(interface {
		ServeHTTP(any, any)
	})
	assert.False(t, ok)
	_, ok = any((*oauth.Service)(nil)).(interface {
		RedeemRefresh(context.Context, string) (oauth.TokenResponse, error)
	})
	assert.False(t, ok)
}

func TestSecretsAreCopySafeAndFailClosed(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 10, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	secrets := fx.secrets
	svc, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: secrets,
		Clock:   frozenClock{now: now},
		Config:  oauth.Config{Issuer: testIssuer, TokenEndpoint: testTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.NoError(t, err)
	for i := range secrets.AuthorizationCodePeppers[1] {
		secrets.AuthorizationCodePeppers[1][i] = 0xAA
	}
	resp, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)

	_, err = oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: oauth.Secrets{},
		Clock:   frozenClock{now: now},
		Config:  oauth.Config{Issuer: testIssuer, TokenEndpoint: testTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.Error(t, err)
}

func issuedConfidentialBasicCode(t *testing.T, now time.Time, plaintext string) issuedFixture {
	t.Helper()
	secrets := testSecrets()
	hash, err := secrethash.Hash(secrethash.Peppers(secrets.ClientSecretPeppers), secrets.ClientSecretActiveVersion, "primer.oauth.client-secret", []byte(plaintext))
	require.NoError(t, err)
	version := int16(secrets.ClientSecretActiveVersion)
	return issuedCustomClient(t, now, domain.OAuthClient{
		ClientID: "basic-" + uuid.NewString()[:8], Name: "Basic client", ClientType: "confidential",
		TokenEndpointAuthMethod: oauth.AuthBasic, AllowedGrants: []string{oauth.GrantAuthorizationCode},
		Enabled: true, ClientSecretHash: hash, ClientSecretPepperVersion: &version,
	}, nil)
}

func issuedPrivateKeyJWTCode(t *testing.T, now time.Time, mat *keys.Material) issuedFixture {
	t.Helper()
	jwk, err := mat.PublicJWK()
	require.NoError(t, err)
	raw, err := json.Marshal(jwk)
	require.NoError(t, err)
	pool := testutil.DB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	client, err := repo.CreateOAuthClient(ctx, tx, domain.OAuthClient{
		ClientID: "pkjwt-" + uuid.NewString()[:8], Name: "JWT client", ClientType: "confidential",
		TokenEndpointAuthMethod: oauth.AuthPrivateKeyJWT, AllowedGrants: []string{oauth.GrantAuthorizationCode},
		Enabled: true,
	})
	require.NoError(t, err)
	_, err = repo.CreateOAuthClientKey(ctx, tx, domain.OAuthClientKey{
		OAuthClientID: client.ID, Kid: jwk.Kid, JWKJSON: raw, Alg: "ES256", Use: "sig", Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return issuedCustomClient(t, now, *client, nil)
}

// issuedCodeOnClient mints a fresh authorization code on an existing client/redirect
// pair so private_key_jwt replay can reuse the same oauth_client_id.
func issuedCodeOnClient(t *testing.T, now time.Time, base issuedFixture) issuedFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := base.secrets
	client := base.client
	redirect := base.redirect
	account := factory.Account(t, pool)
	mapping, err := repo.CreateStytchMapping(ctx, pool, account.ID, domain.StytchPrincipal{
		ProjectID: "proj-" + uuid.NewString()[:8], OrganizationID: "org-" + uuid.NewString()[:8], MemberID: "member-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)
	assoc, err := repo.CreateProviderSessionAssociation(ctx, pool, domain.ProviderSessionAssociation{
		AccountID: account.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-" + uuid.NewString()[:8],
		ProviderExpiresAt: now.Add(2 * time.Hour), Status: "active", LastValidatedAt: now,
	})
	require.NoError(t, err)
	broker, err := repo.CreateBrokerTransaction(ctx, pool, domain.CreateBrokerTransactionInput{
		OAuthClientID: client.ID, RedirectID: redirect.ID, StateHash: uniqueTestHash("state"),
		StatePepperVersion: 1, StateSealed: []byte(strings.Repeat("x", 30)), StateKeyVersion: 1, StateLength: 1,
		PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256", RequestedScopes: []string{"openid"},
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, BrokerCookieHash: uniqueTestHash("cookie"),
		BrokerCookiePepperVersion: 1, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	grant, err := repo.CreateOAuthGrant(ctx, pool, domain.OAuthGrant{
		AccountID: &account.ID, OAuthClientID: client.ID, ProviderSessionAssociationID: &assoc.ID,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, Scopes: []string{"openid"},
		SubjectClass: "human", Status: "active", GrantedAt: now, NotAfter: now.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	rawCode := "code-" + uuid.NewString()
	codeHash, err := secrethash.Hash(secrethash.Peppers(secrets.AuthorizationCodePeppers), secrets.AuthorizationCodeActiveVersion, oauth.CodeHashContext, []byte(rawCode))
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(testVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code, err := repo.CreateAuthorizationCode(ctx, pool, domain.OAuthAuthorizationCode{
		CodeHash: codeHash, PepperVersion: int16(secrets.AuthorizationCodeActiveVersion),
		GrantID: grant.ID, BrokerTransactionID: broker.ID, OAuthClientID: client.ID,
		RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: challenge, PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	})
	require.NoError(t, err)
	return issuedFixture{
		client: client, redirect: redirect, account: account, grant: grant, code: code,
		rawCode: rawCode, codeHash: codeHash, secrets: secrets, keys: base.keys,
	}
}

func issuedCustomClient(t *testing.T, now time.Time, clientIn domain.OAuthClient, after func(*testing.T, *domain.OAuthClient)) issuedFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := testSecrets()
	client := &clientIn
	if client.ID == uuid.Nil {
		created, err := repo.CreateOAuthClient(ctx, pool, clientIn)
		require.NoError(t, err)
		client = created
	}
	if after != nil {
		after(t, client)
	}
	redirect, err := repo.CreateOAuthClientRedirect(ctx, pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: "https://example.test/callback", ResourceURI: "https://resource.example.test",
		Audience: "test", AllowedScopes: []string{"openid"}, Enabled: true,
	})
	require.NoError(t, err)
	fx := issuedPublicCode(t, now)
	// Rebuild on the confidential client rather than the public factory client.
	_ = fx
	account := fx.account
	mapping, err := repo.CreateStytchMapping(ctx, pool, account.ID, domain.StytchPrincipal{
		ProjectID: "proj-" + uuid.NewString()[:8], OrganizationID: "org-" + uuid.NewString()[:8], MemberID: "member-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)
	assoc, err := repo.CreateProviderSessionAssociation(ctx, pool, domain.ProviderSessionAssociation{
		AccountID: account.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-" + uuid.NewString()[:8],
		ProviderExpiresAt: now.Add(2 * time.Hour), Status: "active", LastValidatedAt: now,
	})
	require.NoError(t, err)
	broker, err := repo.CreateBrokerTransaction(ctx, pool, domain.CreateBrokerTransactionInput{
		OAuthClientID: client.ID, RedirectID: redirect.ID, StateHash: uniqueTestHash("state"),
		StatePepperVersion: 1, StateSealed: []byte(strings.Repeat("x", 30)), StateKeyVersion: 1, StateLength: 1,
		PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256", RequestedScopes: []string{"openid"},
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, BrokerCookieHash: uniqueTestHash("cookie"),
		BrokerCookiePepperVersion: 1, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	grant, err := repo.CreateOAuthGrant(ctx, pool, domain.OAuthGrant{
		AccountID: &account.ID, OAuthClientID: client.ID, ProviderSessionAssociationID: &assoc.ID,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, Scopes: []string{"openid"},
		SubjectClass: "human", Status: "active", GrantedAt: now, NotAfter: now.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	rawCode := "code-" + uuid.NewString()
	codeHash, err := secrethash.Hash(secrethash.Peppers(secrets.AuthorizationCodePeppers), secrets.AuthorizationCodeActiveVersion, oauth.CodeHashContext, []byte(rawCode))
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(testVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code, err := repo.CreateAuthorizationCode(ctx, pool, domain.OAuthAuthorizationCode{
		CodeHash: codeHash, PepperVersion: int16(secrets.AuthorizationCodeActiveVersion),
		GrantID: grant.ID, BrokerTransactionID: broker.ID, OAuthClientID: client.ID,
		RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: challenge, PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	})
	require.NoError(t, err)
	return issuedFixture{
		client: client, redirect: redirect, account: account, grant: grant, code: code,
		rawCode: rawCode, codeHash: codeHash, secrets: secrets, keys: fx.keys,
	}
}

func uniqueTestHash(label string) []byte {
	sum := sha256.Sum256([]byte(label + uuid.NewString()))
	return append([]byte(nil), sum[:]...)
}

func mintClientAssertion(t *testing.T, mat *keys.Material, clientID, aud string, now time.Time, ttl time.Duration) string {
	t.Helper()
	jwk, err := mat.PublicJWK()
	require.NoError(t, err)
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": jwk.Kid, "typ": "JWT"})
	require.NoError(t, err)
	iat := now.UTC().Unix()
	payload, err := json.Marshal(map[string]any{
		"iss": clientID, "sub": clientID, "aud": aud,
		"iat": iat, "exp": iat + int64(ttl/time.Second), "jti": uuid.NewString(),
	})
	require.NoError(t, err)
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(signingInput))
	der, err := mat.Sign(rand.Reader, sum[:], crypto.SHA256)
	require.NoError(t, err)
	var parsed struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(der, &parsed)
	require.NoError(t, err)
	require.Empty(t, rest)
	n := elliptic.P256().Params().N
	half := new(big.Int).Rsh(new(big.Int).Set(n), 1)
	if parsed.S.Cmp(half) > 0 {
		parsed.S = new(big.Int).Sub(n, parsed.S)
	}
	sig := make([]byte, 64)
	r := parsed.R.Bytes()
	s := parsed.S.Bytes()
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):], s)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

type failingSignerSource struct{}

func (failingSignerSource) Current(context.Context) (token.Signer, *domain.SigningKey, error) {
	return nil, nil, errors.New("signer down")
}

type tracingSignerSource struct {
	svc       *keys.Service
	last      atomic.Value
	current   atomic.Int64
	currentTx atomic.Int64
}

func newTracingSignerSource(svc *keys.Service) *tracingSignerSource {
	return &tracingSignerSource{svc: svc}
}

func (s *tracingSignerSource) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	s.current.Add(1)
	signer, meta, err := s.svc.ActiveSigner(ctx)
	if err != nil {
		s.last.Store(err)
		return nil, nil, err
	}
	return tracingSigner{Signer: signer, last: &s.last}, meta, nil
}

func (s *tracingSignerSource) CurrentForTx(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error) {
	s.currentTx.Add(1)
	signer, meta, err := s.svc.ActiveSignerForTx(ctx, tx)
	if err != nil {
		s.last.Store(err)
		return nil, nil, err
	}
	return tracingSigner{Signer: signer, last: &s.last}, meta, nil
}

func (s *tracingSignerSource) root() error {
	got, _ := s.last.Load().(error)
	return got
}

type tracingSigner struct {
	token.Signer
	last *atomic.Value
}

func (s tracingSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	sig, err := s.Signer.Sign(rand, digest, opts)
	if err != nil && s.last != nil {
		s.last.Store(err)
	}
	return sig, err
}
