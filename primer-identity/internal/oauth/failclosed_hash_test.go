package oauth

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
	"github.com/aleksclark/primer/identity/internal/token"
)

func TestAssertionPepperFailureIsTemporarilyUnavailableAndLeavesNoReplay(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 10, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })

	fx := issuedInternalPrivateKeyJWTCode(t, now, clientMat)
	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config:  Config{Issuer: internalIssuer, TokenEndpoint: internalTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.NoError(t, err)
	breakAssertionPeppers(svc)

	assertion := mintInternalClientAssertion(t, clientMat, fx.client.ClientID, internalTokenEndpoint, now, 2*time.Minute)
	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.Error(t, err)
	assert.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(err))
	assert.Empty(t, resp.AccessToken)
	assert.Empty(t, resp.RefreshToken)
	assertNoSecretLeak(t, err, assertion, fx.rawCode)

	assertNoIssuanceResidue(t, fx)
	var replays int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_client_assertion_replays WHERE oauth_client_id=$1`, fx.client.ID).Scan(&replays))
	require.Zero(t, replays)
}

func TestAccessJTIPepperFailureRollsBackIssuanceAndAudit(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 12, 0, 0, time.UTC)
	fx := issuedInternalPublicCode(t, now)
	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config:  Config{Issuer: internalIssuer, TokenEndpoint: internalTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.NoError(t, err)
	breakAssertionPeppers(svc)

	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
	}, ClientAuth{Method: AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(err))
	assert.Empty(t, resp.AccessToken)
	assert.Empty(t, resp.RefreshToken)
	assertNoSecretLeak(t, err, fx.rawCode)

	assertNoIssuanceResidue(t, fx)
}

func breakAssertionPeppers(svc *Service) {
	for version := range svc.secrets.AssertionPeppers {
		svc.secrets.AssertionPeppers[version] = []byte{0x01}
	}
}

func assertNoIssuanceResidue(t *testing.T, fx issuedInternalFixture) {
	t.Helper()
	pool := testutil.DB(t)
	var families, tokens, audits int
	var consumed *time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id WHERE f.grant_id=$1`, fx.grant.ID).Scan(&tokens))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1`, fx.grant.ID).Scan(&audits))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.Zero(t, families)
	require.Zero(t, tokens)
	require.Zero(t, audits)
	require.Nil(t, consumed)
}

func assertNoSecretLeak(t *testing.T, err error, secrets ...string) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	lower := strings.ToLower(msg)
	assert.NotContains(t, lower, "pepper")
	assert.NotContains(t, lower, "sha256")
	assert.NotContains(t, lower, "sha-256")
	assert.NotContains(t, msg, "jti")
	for _, secret := range secrets {
		if secret != "" {
			assert.NotContains(t, msg, secret)
		}
	}
}

func issuedInternalPrivateKeyJWTCode(t *testing.T, now time.Time, mat *keys.Material) issuedInternalFixture {
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
		TokenEndpointAuthMethod: AuthPrivateKeyJWT, AllowedGrants: []string{GrantAuthorizationCode},
		Enabled: true,
	})
	require.NoError(t, err)
	_, err = repo.CreateOAuthClientKey(ctx, tx, domain.OAuthClientKey{
		OAuthClientID: client.ID, Kid: jwk.Kid, JWKJSON: raw, Alg: "ES256", Use: "sig", Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return issuedInternalCustomClient(t, now, *client)
}

func issuedInternalCustomClient(t *testing.T, now time.Time, client domain.OAuthClient) issuedInternalFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := Secrets{
		AuthorizationCodePeppers:       map[int][]byte{1: pepperBytes(0x41)},
		AuthorizationCodeActiveVersion: 1,
		ClientSecretPeppers:            map[int][]byte{1: pepperBytes(0x51)},
		ClientSecretActiveVersion:      1,
		RefreshTokenPeppers:            map[int][]byte{1: pepperBytes(0x61)},
		RefreshTokenActiveVersion:      1,
		AssertionPeppers:               map[int][]byte{1: pepperBytes(0x71)},
		AssertionActiveVersion:         1,
	}
	redirect, err := repo.CreateOAuthClientRedirect(ctx, pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: "https://example.test/callback", ResourceURI: "https://resource.example.test",
		Audience: "test", AllowedScopes: []string{"openid"}, Enabled: true,
	})
	require.NoError(t, err)
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
	broker := factory.BrokerTransaction(t, pool, &client, redirect)
	grant, err := repo.CreateOAuthGrant(ctx, pool, domain.OAuthGrant{
		AccountID: &account.ID, OAuthClientID: client.ID, ProviderSessionAssociationID: &assoc.ID,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, Scopes: []string{"openid"},
		SubjectClass: "human", Status: "active", GrantedAt: now, NotAfter: now.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	rawCode := "code-" + uuid.NewString()
	codeHash, err := secrethash.Hash(secrethash.Peppers(secrets.AuthorizationCodePeppers), secrets.AuthorizationCodeActiveVersion, CodeHashContext, []byte(rawCode))
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(internalVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code, err := repo.CreateAuthorizationCode(ctx, pool, domain.OAuthAuthorizationCode{
		CodeHash: codeHash, PepperVersion: int16(secrets.AuthorizationCodeActiveVersion),
		GrantID: grant.ID, BrokerTransactionID: broker.ID, OAuthClientID: client.ID,
		RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: challenge, PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	})
	require.NoError(t, err)
	var cfg config.KeyConfig
	cfg.Enabled = true
	cfg.SetSealSecretForTest("UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE")
	require.NoError(t, cfg.Validate("test"))
	keySvc := keys.NewService(pool, cfg, "test")
	t.Cleanup(func() { _ = keySvc.Close() })
	_, err = keySvc.CreateInitialActive(ctx)
	require.NoError(t, err)
	return issuedInternalFixture{
		client: &client, redirect: redirect, account: account, grant: grant, code: code,
		rawCode: rawCode, secrets: secrets, keys: keySvc,
	}
}

func mintInternalClientAssertion(t *testing.T, mat *keys.Material, clientID, aud string, now time.Time, ttl time.Duration) string {
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

func TestPrivateKeyJWTReplayRollsBackWhenLaterIssuanceFails(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 20, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedInternalPrivateKeyJWTCode(t, now, clientMat)
	assertion := mintInternalClientAssertion(t, clientMat, fx.client.ClientID, internalTokenEndpoint, now, 2*time.Minute)

	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config:  Config{Issuer: internalIssuer, TokenEndpoint: internalTokenEndpoint, AccessTTL: 15 * time.Minute},
		afterSign: func(tx pgx.Tx, issued token.IssuedToken) error {
			_, err := tx.Exec(context.Background(), `
CREATE TEMP TABLE oauth_replay_commit_fail (
  k int PRIMARY KEY DEFERRABLE INITIALLY DEFERRED
) ON COMMIT DROP`)
			require.NoError(t, err)
			_, err = tx.Exec(context.Background(), `INSERT INTO oauth_replay_commit_fail(k) VALUES (1)`)
			require.NoError(t, err)
			_, err = tx.Exec(context.Background(), `INSERT INTO oauth_replay_commit_fail(k) VALUES (1)`)
			require.NoError(t, err)
			return nil
		},
	})
	require.NoError(t, err)

	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.Error(t, err)
	assert.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(err))
	assert.Empty(t, resp.AccessToken)
	assertNoIssuanceResidue(t, fx)
	require.Zero(t, assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointToken))

	svc.after = nil
	retry, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.NoError(t, err)
	require.NotEmpty(t, retry.AccessToken)
	require.Equal(t, int64(1), assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointToken))
}

func TestInvalidGrantDoesNotConsumePrivateKeyJWTReplay(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 22, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedInternalPrivateKeyJWTCode(t, now, clientMat)
	assertion := mintInternalClientAssertion(t, clientMat, fx.client.ClientID, internalTokenEndpoint, now, 2*time.Minute)
	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config:  Config{Issuer: internalIssuer, TokenEndpoint: internalTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.NoError(t, err)

	_, err = svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: "missing-" + uuid.NewString(), ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.Error(t, err)
	assert.Equal(t, ErrorInvalidGrant, ErrorCodeOf(err))
	require.Zero(t, assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointToken))

	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.Equal(t, int64(1), assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointToken))
}

func TestRevokeUnknownTokenCommitsReplayInSameTransaction(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 24, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedInternalPrivateKeyJWTCode(t, now, clientMat)
	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config: Config{
			Issuer:             internalIssuer,
			TokenEndpoint:      internalTokenEndpoint,
			RevocationEndpoint: "https://identity.example.test/oauth/revoke",
			AccessTTL:          15 * time.Minute,
		},
	})
	require.NoError(t, err)
	assertion := mintInternalClientAssertion(t, clientMat, fx.client.ClientID, "https://identity.example.test/oauth/revoke", now, 2*time.Minute)
	require.NoError(t, svc.Revoke(context.Background(), RevokeRequest{
		Token: strings.Repeat("A", 43), ClientID: fx.client.ClientID,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion}))
	require.Equal(t, int64(1), assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointRevocation))
	require.Zero(t, assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointToken))

	err = svc.Revoke(context.Background(), RevokeRequest{
		Token: strings.Repeat("B", 43), ClientID: fx.client.ClientID,
		ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
	}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: fx.client.ClientID, Assertion: assertion})
	require.Error(t, err)
	assert.Equal(t, ErrorInvalidClient, ErrorCodeOf(err))
}

func TestConcurrentPrivateKeyJWTReplayHasOneWinner(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 26, 0, 0, time.UTC)
	clientMat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientMat.Destroy() })
	fx := issuedInternalPrivateKeyJWTCode(t, now, clientMat)
	fx2 := issuedInternalPrivateKeyJWTCode(t, now, clientMat)
	assertion := mintInternalClientAssertion(t, clientMat, fx.client.ClientID, internalTokenEndpoint, now, 2*time.Minute)
	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config:  Config{Issuer: internalIssuer, TokenEndpoint: internalTokenEndpoint, AccessTTL: 15 * time.Minute},
	})
	require.NoError(t, err)

	const n = 8
	type outcome struct {
		resp TokenResponse
		err  error
	}
	out := make(chan outcome, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := fx.rawCode
			clientID := fx.client.ClientID
			redirect := fx.redirect.RedirectURI
			resource := fx.redirect.ResourceURI
			if i%2 == 1 {
				code = fx2.rawCode
				clientID = fx2.client.ClientID
				redirect = fx2.redirect.RedirectURI
				resource = fx2.redirect.ResourceURI
			}
			resp, err := svc.Exchange(context.Background(), ExchangeRequest{
				GrantType: GrantAuthorizationCode, Code: code, ClientID: clientID,
				RedirectURI: redirect, Resource: resource, CodeVerifier: internalVerifier,
				ClientAssertionType: assertionTypeJWTBearer, ClientAssertion: assertion,
			}, ClientAuth{Method: AuthPrivateKeyJWT, ClientID: clientID, Assertion: assertion})
			out <- outcome{resp: resp, err: err}
		}(i)
	}
	wg.Wait()
	close(out)
	var ok, invalidClient int
	for got := range out {
		if got.err == nil {
			ok++
			require.NotEmpty(t, got.resp.AccessToken)
			continue
		}
		require.Equal(t, ErrorInvalidClient, ErrorCodeOf(got.err))
		invalidClient++
	}
	require.Equal(t, 1, ok)
	require.Equal(t, n-1, invalidClient)
	require.Equal(t, int64(1), assertionReplayCount(t, fx.client.ID, domain.AssertionEndpointToken)+assertionReplayCount(t, fx2.client.ID, domain.AssertionEndpointToken))
}

func assertionReplayCount(t *testing.T, clientID uuid.UUID, endpoint string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_client_assertion_replays WHERE oauth_client_id=$1 AND endpoint_kind=$2`,
		clientID, endpoint).Scan(&n))
	return n
}
