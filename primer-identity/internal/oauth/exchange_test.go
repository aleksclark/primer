package oauth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	testIssuer        = "https://identity.example.test"
	testTokenEndpoint = "https://identity.example.test/oauth/token"
	testVerifier      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVW"
)

type frozenClock struct{ now time.Time }

func (c frozenClock) Now() time.Time { return c.now }

type keyServiceSignerSource struct{ svc *keys.Service }

func (s keyServiceSignerSource) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	signer, meta, err := s.svc.ActiveSigner(ctx)
	if err != nil {
		return nil, nil, err
	}
	return signer, meta, nil
}

func (s keyServiceSignerSource) CurrentForTx(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error) {
	return s.svc.ActiveSignerForTx(ctx, tx)
}

func TestPublicRefreshRotatesAndReuseTerminalizes(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	now := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)
	require.NoError(t, pool.QueryRow(ctx, `UPDATE oauth_clients SET allowed_grants=ARRAY['authorization_code','refresh_token'] WHERE id=$1 RETURNING id`, fx.client.ID).Scan(&fx.client.ID))
	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	first, err := svc.Exchange(ctx, oauth.ExchangeRequest{GrantType: oauth.GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID, RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: testVerifier}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	second, err := svc.Exchange(ctx, oauth.ExchangeRequest{GrantType: oauth.GrantRefreshToken, RefreshToken: first.RefreshToken, ClientID: fx.client.ClientID, Resource: fx.redirect.ResourceURI}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEqual(t, first.RefreshToken, second.RefreshToken)
	require.NotEmpty(t, second.AccessToken)
	_, err = svc.Exchange(ctx, oauth.ExchangeRequest{GrantType: oauth.GrantRefreshToken, RefreshToken: first.RefreshToken, ClientID: fx.client.ClientID, Resource: fx.redirect.ResourceURI}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))
	var status, grantStatus string
	var live int
	require.NoError(t, pool.QueryRow(ctx, `SELECT f.status,g.status,(SELECT count(*) FROM oauth_refresh_tokens t WHERE t.family_id=f.id AND t.consumed_at IS NULL AND t.revoked_at IS NULL) FROM oauth_refresh_families f JOIN oauth_grants g ON g.id=f.grant_id WHERE f.grant_id=$1`, fx.grant.ID).Scan(&status, &grantStatus, &live))
	require.Equal(t, domain.RefreshFamilyStatusReuseDetected, status)
	require.Equal(t, "revoked", grantStatus)
	require.Zero(t, live)
}

func TestPublicClientAuthorizationCodeExchangeIssuesJWTAndRefresh(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	now := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	fx := issuedPublicCode(t, now)

	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	resp, err := svc.Exchange(ctx, oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         fx.rawCode,
		ClientID:     fx.client.ClientID,
		RedirectURI:  fx.redirect.RedirectURI,
		Resource:     fx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.Equal(t, "Bearer", resp.TokenType)
	require.Equal(t, "openid", resp.Scope)
	require.NotEmpty(t, resp.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)
	require.Greater(t, resp.ExpiresIn, 0)
	require.LessOrEqual(t, resp.ExpiresIn, 900)
	require.NotContains(t, resp.String(), resp.AccessToken)
	require.NotContains(t, resp.String(), resp.RefreshToken)
	require.NotContains(t, resp.GoString(), "eyJ")

	keyset, err := token.NewKeySet(func(ctx context.Context) ([]domain.PublicJWK, error) {
		return fx.keys.PublicJWKS(ctx)
	})
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(ctx))
	verifier, err := token.NewVerifier(keyset, testIssuer, fx.redirect.Audience, frozenClock{now: now}, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		return token.ClientRegistration{ClientID: clientID, Audience: fx.redirect.Audience, SubjectClass: token.KindHuman}, nil
	})
	require.NoError(t, err)
	got, err := verifier.Verify(ctx, resp.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, token.KindHuman, got.Kind)
	assert.Equal(t, fx.account.ID.String(), got.Subject)
	assert.Equal(t, fx.client.ClientID, got.ClientID)
	assert.Equal(t, fx.redirect.Audience, got.Audience)
	assert.NotEqual(t, fx.client.ID.String(), got.ClientID)

	header := decodeSegment(t, resp.AccessToken, 0)
	payload := decodeSegment(t, resp.AccessToken, 1)
	assert.Equal(t, "ES256", header["alg"])
	assert.Equal(t, "at+jwt", header["typ"])
	assert.Equal(t, fx.client.ClientID, payload["client_id"])
	_, hasAZP := payload["azp"]
	assert.False(t, hasAZP)
	_, hasProvider := payload["provider"]
	assert.False(t, hasProvider)

	var families, tokens, audits int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id WHERE f.grant_id=$1 AND t.consumed_at IS NULL AND t.revoked_at IS NULL AND t.sequence=0`, fx.grant.ID).Scan(&tokens))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1 AND client_id=$2`, fx.grant.ID, fx.client.ClientID).Scan(&audits))
	require.Equal(t, 1, families)
	require.Equal(t, 1, tokens)
	require.Equal(t, 1, audits)

	var storedClient string
	var codeHash []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT client_id, authorization_code_hash FROM token_issuance_audit WHERE grant_id=$1`, fx.grant.ID).Scan(&storedClient, &codeHash))
	require.Equal(t, fx.client.ClientID, storedClient)
	require.Equal(t, fx.codeHash, codeHash)
	require.NotEqual(t, []byte(fx.rawCode), codeHash)

	var consumed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.NotNil(t, consumed)
	require.True(t, consumed.UTC().Equal(now), "consumed_at=%s want service clock=%s", consumed.UTC(), now)

	_, err = svc.Exchange(ctx, oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         fx.rawCode,
		ClientID:     fx.client.ClientID,
		RedirectURI:  fx.redirect.RedirectURI,
		Resource:     fx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))
}

func TestPublicClientAuthorizationCodeExchangeHonorsFrozenClockIndependentOfHostDB(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	now := time.Date(1999, 1, 2, 3, 4, 5, 0, time.UTC)
	fx := issuedPublicCode(t, now)

	svc := newTestService(t, fx.secrets, frozenClock{now: now})
	resp, err := svc.Exchange(ctx, oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         fx.rawCode,
		ClientID:     fx.client.ClientID,
		RedirectURI:  fx.redirect.RedirectURI,
		Resource:     fx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)

	var consumed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.NotNil(t, consumed)
	require.True(t, consumed.UTC().Equal(now), "consumed_at=%s want frozen=%s", consumed.UTC(), now)

	future := time.Date(2035, 6, 15, 12, 0, 0, 0, time.UTC)
	futureFx := issuedPublicCode(t, future)
	futureSvc := newTestService(t, futureFx.secrets, frozenClock{now: future})
	futureResp, err := futureSvc.Exchange(ctx, oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         futureFx.rawCode,
		ClientID:     futureFx.client.ClientID,
		RedirectURI:  futureFx.redirect.RedirectURI,
		Resource:     futureFx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: futureFx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, futureResp.AccessToken)
	var futureConsumed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, futureFx.code.ID).Scan(&futureConsumed))
	require.NotNil(t, futureConsumed)
	require.True(t, futureConsumed.UTC().Equal(future), "consumed_at=%s want future=%s", futureConsumed.UTC(), future)
}

func TestPublicClientAuthorizationCodeExchangeRejectsExpiredRelativeToServiceClock(t *testing.T) {
	ctx := context.Background()
	issuedAt := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	expiresAt := issuedAt.Add(60 * time.Second)
	fx := issuedPublicCode(t, issuedAt)

	svc := newTestService(t, fx.secrets, frozenClock{now: expiresAt.Add(time.Nanosecond)})
	_, err := svc.Exchange(ctx, oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         fx.rawCode,
		ClientID:     fx.client.ClientID,
		RedirectURI:  fx.redirect.RedirectURI,
		Resource:     fx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, oauth.ErrorInvalidGrant, oauth.ErrorCodeOf(err))

	boundary := newTestService(t, fx.secrets, frozenClock{now: expiresAt})
	resp, err := boundary.Exchange(ctx, oauth.ExchangeRequest{
		GrantType:    oauth.GrantAuthorizationCode,
		Code:         fx.rawCode,
		ClientID:     fx.client.ClientID,
		RedirectURI:  fx.redirect.RedirectURI,
		Resource:     fx.redirect.ResourceURI,
		CodeVerifier: testVerifier,
	}, oauth.ClientAuth{Method: oauth.AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
}

type issuedFixture struct {
	client   *domain.OAuthClient
	redirect *domain.OAuthClientRedirect
	account  *domain.Account
	grant    *domain.OAuthGrant
	code     *domain.OAuthAuthorizationCode
	rawCode  string
	codeHash []byte
	secrets  oauth.Secrets
	keys     *keys.Service
}

func issuedPublicCode(t *testing.T, now time.Time) issuedFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := testSecrets()
	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
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
	broker := factory.BrokerTransaction(t, pool, client, redirect)
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
		rawCode: rawCode, codeHash: codeHash, secrets: secrets, keys: sharedTestKeys(t),
	}
}

func newTestService(t *testing.T, secrets oauth.Secrets, clock token.Clock) *oauth.Service {
	t.Helper()
	svc, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: sharedTestKeys(t)},
		Secrets: secrets,
		Clock:   clock,
		Config: oauth.Config{
			Issuer:        testIssuer,
			TokenEndpoint: testTokenEndpoint,
			AccessTTL:     15 * time.Minute,
		},
	})
	require.NoError(t, err)
	return svc
}

var (
	testKeysOnce sync.Once
	testKeys     *keys.Service
	testKeysErr  error
)

func sharedTestKeys(t *testing.T) *keys.Service {
	t.Helper()
	testKeysOnce.Do(func() {
		var cfg config.KeyConfig
		cfg.Enabled = true
		cfg.SetSealSecretForTest("UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE")
		if err := cfg.Validate("test"); err != nil {
			testKeysErr = err
			return
		}
		svc := keys.NewService(testutil.DB(t), cfg, "test")
		_, testKeysErr = svc.CreateInitialActive(context.Background())
		if testKeysErr != nil {
			_ = svc.Close()
			return
		}
		testKeys = svc
	})
	require.NoError(t, testKeysErr)
	require.NotNil(t, testKeys)
	return testKeys
}

func testSecrets() oauth.Secrets {
	pepper := func(b byte) []byte {
		out := make([]byte, 32)
		for i := range out {
			out[i] = b
		}
		return out
	}
	return oauth.Secrets{
		AuthorizationCodePeppers:       map[int][]byte{1: pepper(0x41)},
		AuthorizationCodeActiveVersion: 1,
		ClientSecretPeppers:            map[int][]byte{1: pepper(0x51)},
		ClientSecretActiveVersion:      1,
		RefreshTokenPeppers:            map[int][]byte{1: pepper(0x61)},
		RefreshTokenActiveVersion:      1,
		AssertionPeppers:               map[int][]byte{1: pepper(0x71)},
		AssertionActiveVersion:         1,
	}
}

func decodeSegment(t *testing.T, compact string, idx int) map[string]any {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Greater(t, len(parts), idx)
	raw, err := base64.RawURLEncoding.DecodeString(parts[idx])
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	return obj
}
