package oauth_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

const testRevocationEndpoint = "https://identity.example.test/oauth/revoke"

func TestPublicClientRevokesOwnedRefreshFamilyAndGrant(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})

	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         fx.rawCode,
		ClientID:     fx.client.ClientID,
		RedirectURI:  fx.redirect.RedirectURI,
		Resource:     fx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, issued.RefreshToken)
	raw, err := base64.RawURLEncoding.DecodeString(issued.RefreshToken)
	require.NoError(t, err)
	require.Len(t, raw, 32)

	err = svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token: issued.RefreshToken,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)

	pool := testutil.DB(t)
	var familyStatus, grantStatus, reason string
	var familyRevoked, tokenRevoked *time.Time
	var liveTokens int
	require.NoError(t, pool.QueryRow(context.Background(), `
SELECT f.status, f.revoke_reason_code, f.revoked_at, g.status, t.revoked_at,
       (SELECT count(*) FROM oauth_refresh_tokens lt WHERE lt.family_id=f.id AND lt.consumed_at IS NULL AND lt.revoked_at IS NULL)
FROM oauth_refresh_families f
JOIN oauth_grants g ON g.id=f.grant_id
JOIN oauth_refresh_tokens t ON t.family_id=f.id
WHERE f.grant_id=$1`, fx.grant.ID).Scan(&familyStatus, &reason, &familyRevoked, &grantStatus, &tokenRevoked, &liveTokens))
	assert.Equal(t, "revoked", familyStatus)
	assert.Equal(t, "revoked", grantStatus)
	assert.Equal(t, "client_revoked", reason)
	require.NotNil(t, familyRevoked)
	require.NotNil(t, tokenRevoked)
	assert.Zero(t, liveTokens)

	require.NoError(t, svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token:         issued.RefreshToken,
		TokenTypeHint: "refresh_token",
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID}))
	assert.NotContains(t, issued.String(), issued.RefreshToken)
}

func newRevokeTestService(t *testing.T, secrets oauth.Secrets, clock frozenClock) *oauth.Service {
	t.Helper()
	svc, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: sharedTestKeys(t)},
		Secrets: secrets,
		Clock:   clock,
		Config: oauth.Config{
			Issuer:             testIssuer,
			TokenEndpoint:      testTokenEndpoint,
			RevocationEndpoint: testRevocationEndpoint,
			AccessTTL:          15 * time.Minute,
		},
	})
	require.NoError(t, err)
	return svc
}

func TestUnknownHintIsIgnoredAndUnknownTokenIsIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 5, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})

	err := svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token:         strings.Repeat("A", 43),
		TokenTypeHint: "id_token",
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)

	err = svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token:         "not-a-refresh-token",
		TokenTypeHint: "access_token",
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
}

func TestCrossClientRefreshIsOracleFree(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 10, 0, 0, time.UTC)
	owner := issuedPublicCode(t, now)
	other := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, owner.secrets, frozenClock{now: now})
	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         owner.rawCode,
		ClientID:     owner.client.ClientID,
		RedirectURI:  owner.redirect.RedirectURI,
		Resource:     owner.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: owner.client.ClientID})
	require.NoError(t, err)

	err = svc.Revoke(context.Background(), oauth.RevokeRequest{Token: issued.RefreshToken}, oauth.ClientAuth{
		Method: oauth.AuthNone, ClientID: other.client.ClientID,
	})
	require.NoError(t, err)

	var familyStatus string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status,
       (SELECT count(*) FROM oauth_refresh_tokens t WHERE t.family_id=f.id AND t.consumed_at IS NULL AND t.revoked_at IS NULL)
FROM oauth_refresh_families f WHERE f.grant_id=$1`, owner.grant.ID).Scan(&familyStatus, &live))
	assert.Equal(t, "active", familyStatus)
	assert.Equal(t, 1, live)
}

func TestInvalidClientStillFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 15, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
	err := svc.Revoke(context.Background(), oauth.RevokeRequest{Token: "opaque"}, oauth.ClientAuth{
		Method: oauth.AuthNone, ClientID: "missing-client-" + uuid.NewString()[:8],
	})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
}

func TestConfidentialBasicRevokeAndWrongSecret(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 20, 0, 0, time.UTC)
	plaintext := "basic-secret-" + uuid.NewString()
	fx := issuedConfidentialBasicCode(t, now, plaintext)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, RedirectURI: fx.redirect.RedirectURI,
		Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthBasic, ClientID: fx.client.ClientID, Secret: plaintext})
	require.NoError(t, err)

	err = svc.Revoke(context.Background(), oauth.RevokeRequest{Token: issued.RefreshToken}, oauth.ClientAuth{
		Method: oauth.AuthBasic, ClientID: fx.client.ClientID, Secret: "wrong-secret-value-xxxxxxxxxxxxxxxx",
	})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
	assertFamilyActive(t, fx.grant.ID)

	require.NoError(t, svc.Revoke(context.Background(), oauth.RevokeRequest{Token: issued.RefreshToken}, oauth.ClientAuth{
		Method: oauth.AuthBasic, ClientID: fx.client.ClientID, Secret: plaintext,
	}))
	assertFamilyRevoked(t, fx.grant.ID)
}

func TestPrivateKeyJWTRevokeUsesRevocationAudienceAndSeparateReplay(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 25, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedPrivateKeyJWTCode(t, now, clientMat)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
	tokenAssertion := mintClientAssertion(t, clientMat, fx.client.ClientID, testTokenEndpoint, now, 2*time.Minute)
	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     tokenAssertion,
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: tokenAssertion})
	require.NoError(t, err)

	err = svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token: issued.RefreshToken, ClientID: fx.client.ClientID,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     tokenAssertion,
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: tokenAssertion})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
	assertFamilyActive(t, fx.grant.ID)

	revokeAssertion := mintClientAssertion(t, clientMat, fx.client.ClientID, testRevocationEndpoint, now, 2*time.Minute)
	require.NoError(t, svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token: issued.RefreshToken, ClientID: fx.client.ClientID,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     revokeAssertion,
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: revokeAssertion}))
	assertFamilyRevoked(t, fx.grant.ID)

	fx2 := issuedPrivateKeyJWTCode(t, now, clientMat)
	issued2, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx2.rawCode, ClientID: fx2.client.ClientID,
		RedirectURI: fx2.redirect.RedirectURI, Resource: fx2.redirect.ResourceURI, CodeVerifier: testVerifier,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     mintClientAssertion(t, clientMat, fx2.client.ClientID, testTokenEndpoint, now, 2*time.Minute),
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx2.client.ClientID, Assertion: mintClientAssertion(t, clientMat, fx2.client.ClientID, testTokenEndpoint, now, 2*time.Minute)})
	require.NoError(t, err)
	_ = issued2
	err = svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token: issued.RefreshToken, ClientID: fx2.client.ClientID,
		ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		ClientAssertion:     revokeAssertion,
	}, oauth.ClientAuth{Method: oauth.AuthPrivateKeyJWT, ClientID: fx2.client.ClientID, Assertion: revokeAssertion})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidClient, oauth.ErrorCodeOf(err))
}

func TestRefreshPepperRotationRevokesOldHash(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 30, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)

	rotated := fx.secrets
	rotated.RefreshTokenPeppers = map[int][]byte{
		1: append([]byte(nil), fx.secrets.RefreshTokenPeppers[1]...),
		2: bytes.Repeat([]byte{0x62}, 32),
	}
	rotated.RefreshTokenActiveVersion = 2
	rotatedSvc := newRevokeTestService(t, rotated, frozenClock{now: now})
	require.NoError(t, rotatedSvc.Revoke(context.Background(), oauth.RevokeRequest{Token: issued.RefreshToken}, oauth.ClientAuth{
		Method: oauth.AuthNone, ClientID: fx.client.ClientID,
	}))
	assertFamilyRevoked(t, fx.grant.ID)
}

func TestAccessTokenPresentationHasNoEffect(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 35, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NoError(t, svc.Revoke(context.Background(), oauth.RevokeRequest{
		Token: issued.AccessToken, TokenTypeHint: "access_token",
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID}))
	assertFamilyActive(t, fx.grant.ID)
}

func TestConcurrentRevokeHasExactlyOneTerminalFamily(t *testing.T) {
	now := time.Date(2026, 8, 17, 20, 40, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
	issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
		GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)

	const n = 16
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- svc.Revoke(context.Background(), oauth.RevokeRequest{Token: issued.RefreshToken}, oauth.ClientAuth{
				Method: oauth.AuthNone, ClientID: fx.client.ClientID,
			})
		}()
	}
	wg.Wait()
	close(errCh)
	for got := range errCh {
		require.NoError(t, got)
	}
	assertFamilyRevoked(t, fx.grant.ID)
	var versions int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT version FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&versions))
	assert.Equal(t, int64(1), int64(versions))
}

func TestHighCountPublicRevokes(t *testing.T) {
	base := time.Date(2026, 8, 17, 20, 50, 0, 0, time.UTC)
	const n = 32
	type pair struct {
		grantID uuid.UUID
		token   string
		client  string
		svc     *oauth.Service
	}
	pairs := make([]pair, n)
	for i := 0; i < n; i++ {
		now := base.Add(time.Duration(i) * time.Second)
		fx := issuedPublicCode(t, now)
		svc := newRevokeTestService(t, fx.secrets, frozenClock{now: now})
		issued, err := svc.Exchange(context.Background(), oauth.ExchangeRequest{
			GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
			RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier,
		}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
		require.NoError(t, err)
		pairs[i] = pair{grantID: fx.grant.ID, token: issued.RefreshToken, client: fx.client.ClientID, svc: svc}
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- pairs[i].svc.Revoke(context.Background(), oauth.RevokeRequest{Token: pairs[i].token}, oauth.ClientAuth{
				Method: oauth.AuthNone, ClientID: pairs[i].client,
			})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	for _, p := range pairs {
		assertFamilyRevoked(t, p.grantID)
	}
}

func assertFamilyActive(t *testing.T, grantID uuid.UUID) {
	t.Helper()
	var status string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status,
       (SELECT count(*) FROM oauth_refresh_tokens t WHERE t.family_id=f.id AND t.consumed_at IS NULL AND t.revoked_at IS NULL)
FROM oauth_refresh_families f WHERE f.grant_id=$1`, grantID).Scan(&status, &live))
	require.Equal(t, "active", status)
	require.Equal(t, 1, live)
}

func assertFamilyRevoked(t *testing.T, grantID uuid.UUID) {
	t.Helper()
	var familyStatus, grantStatus string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status, g.status,
       (SELECT count(*) FROM oauth_refresh_tokens t WHERE t.family_id=f.id AND t.consumed_at IS NULL AND t.revoked_at IS NULL)
FROM oauth_refresh_families f JOIN oauth_grants g ON g.id=f.grant_id WHERE f.grant_id=$1`, grantID).Scan(&familyStatus, &grantStatus, &live))
	require.Equal(t, "revoked", familyStatus)
	require.Equal(t, "revoked", grantStatus)
	require.Zero(t, live)
}
