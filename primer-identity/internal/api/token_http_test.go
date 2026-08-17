package api_test

import (
	"bytes"
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
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/logging"
	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	tokenHTTPIssuer   = "https://identity.example.test"
	tokenHTTPEndpoint = "https://identity.example.test/oauth/token"
	tokenHTTPVerifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVW"
)

func TestPublicClientTokenExchangeReturnsExactJSONAndValidatesJWKS(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 0, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)

	form := url.Values{
		"grant_type":    {oauth.GrantAuthorizationCode},
		"code":          {fx.rawCode},
		"redirect_uri":  {fx.redirect.RedirectURI},
		"resource":      {fx.redirect.ResourceURI},
		"code_verifier": {tokenHTTPVerifier},
		"client_id":     {fx.client.ClientID},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := serve(handler, req)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rr.Header().Get("WWW-Authenticate"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, []string{"access_token", "expires_in", "refresh_token", "scope", "token_type"}, sortedKeys(body))
	assert.Equal(t, "Bearer", body["token_type"])
	assert.Equal(t, "openid", body["scope"])
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)
	expires, ok := body["expires_in"].(float64)
	require.True(t, ok)
	assert.Greater(t, expires, 0.0)
	assert.LessOrEqual(t, expires, 900.0)
	assert.NotContains(t, rr.Body.String(), "$schema")
	assert.NotContains(t, rr.Body.String(), "id_token")
	assert.NotContains(t, rr.Body.String(), fx.client.ID.String())
	assert.NotContains(t, rr.Body.String(), "azp")

	pubs, err := fx.keys.PublicJWKS(context.Background())
	require.NoError(t, err)
	keyset, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) { return pubs, nil })
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(context.Background()))
	verifier, err := token.NewVerifier(keyset, tokenHTTPIssuer, fx.redirect.Audience, frozenHTTPClock{now: now}, nil)
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), access)
	require.NoError(t, err)
	assert.Equal(t, fx.client.ClientID, got.ClientID)
	assert.Equal(t, fx.redirect.Audience, got.Audience)
	assert.Equal(t, fx.account.ID.String(), got.Subject)
	assert.NotEqual(t, fx.client.ID.String(), got.ClientID)

	pool := testutil.DB(t)
	var families, tokens, audits int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id WHERE f.grant_id=$1 AND t.consumed_at IS NULL AND t.sequence=0`, fx.grant.ID).Scan(&tokens))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1 AND client_id=$2`, fx.grant.ID, fx.client.ClientID).Scan(&audits))
	assert.Equal(t, 1, families)
	assert.Equal(t, 1, tokens)
	assert.Equal(t, 1, audits)
}

type httpTokenFixture struct {
	client   *domain.OAuthClient
	redirect *domain.OAuthClientRedirect
	account  *domain.Account
	grant    *domain.OAuthGrant
	code     *domain.OAuthAuthorizationCode
	rawCode  string
	secrets  oauth.Secrets
	keys     *keys.Service
	now      time.Time
}

type frozenHTTPClock struct{ now time.Time }

func (c frozenHTTPClock) Now() time.Time { return c.now }

type httpKeySigner struct{ svc *keys.Service }

func (s httpKeySigner) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	return s.svc.ActiveSigner(ctx)
}

func (s httpKeySigner) CurrentForTx(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error) {
	return s.svc.ActiveSignerForTx(ctx, tx)
}

func newTokenAPI(t *testing.T, fx httpTokenFixture) http.Handler {
	t.Helper()
	svc, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  httpKeySigner{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenHTTPClock{now: fx.now},
		Config: oauth.Config{
			Issuer:        tokenHTTPIssuer,
			TokenEndpoint: tokenHTTPEndpoint,
			AccessTTL:     15 * time.Minute,
		},
	})
	require.NoError(t, err)
	_, handler := api.New(testutil.DB(t), api.Options{
		OAuth:  svc,
		Issuer: tokenHTTPIssuer,
		JWKS:   fx.keys,
	})
	currentTokenHandler = handler
	return handler
}

func issuedHTTPPublicCode(t *testing.T, now time.Time) httpTokenFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := httpTokenSecrets()
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
	sum := sha256.Sum256([]byte(tokenHTTPVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code, err := repo.CreateAuthorizationCode(ctx, pool, domain.OAuthAuthorizationCode{
		CodeHash: codeHash, PepperVersion: int16(secrets.AuthorizationCodeActiveVersion),
		GrantID: grant.ID, BrokerTransactionID: broker.ID, OAuthClientID: client.ID,
		RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: challenge, PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	})
	require.NoError(t, err)
	return httpTokenFixture{
		client: client, redirect: redirect, account: account, grant: grant, code: code,
		rawCode: rawCode, secrets: secrets, keys: sharedHTTPKeys(t), now: now,
	}
}

func httpTokenSecrets() oauth.Secrets {
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

func sharedHTTPKeys(t *testing.T) *keys.Service {
	t.Helper()
	var cfg config.KeyConfig
	cfg.Enabled = true
	cfg.SetSealSecretForTest("UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE")
	require.NoError(t, cfg.Validate("test"))
	svc := keys.NewService(testutil.DB(t), cfg, "test")
	t.Cleanup(func() { _ = svc.Close() })
	_, err := svc.CreateInitialActive(context.Background())
	require.NoError(t, err)
	return svc
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

const (
	tokenAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	tokenBasicRealm    = `Basic realm="token", charset="UTF-8"`
	tokenMaxSecretLen  = 256
)

func TestConfidentialBasicExchangeAndWrongSecretChallenge(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 10, 0, 0, time.UTC)
	secret := "basic-secret-" + uuid.NewString()
	fx := issuedHTTPConfidentialBasicCode(t, now, secret)
	_ = newTokenAPI(t, fx)

	wrong := postToken(publicCodeForm(fx), map[string]string{
		"Authorization": basicAuth(fx.client.ClientID, "wrong-secret-value-xxxxxxxxxxxxxxxx"),
	})
	assertTokenError(t, wrong, http.StatusUnauthorized, oauth.ErrorInvalidClient, true)
	assert.Equal(t, tokenBasicRealm, wrong.Header().Get("WWW-Authenticate"))
	assert.NotContains(t, wrong.Body.String(), secret)
	assert.NotContains(t, wrong.Body.String(), "wrong-secret")

	ok := postToken(publicCodeForm(fx), map[string]string{
		"Authorization": basicAuth(fx.client.ClientID, secret),
	})
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	assertTokenSuccessHeaders(t, ok, false)
	assertExactTokenSuccess(t, ok)
	assert.Empty(t, ok.Header().Get("WWW-Authenticate"))
}

func TestBasicAuthRejectsMalformedHeaderAndEmptyPassword(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 11, 0, 0, time.UTC)
	secret := "basic-secret-" + uuid.NewString()
	fx := issuedHTTPConfidentialBasicCode(t, now, secret)
	handler := newTokenAPI(t, fx)
	form := publicCodeForm(fx)

	cases := []string{
		"Bearer " + base64.StdEncoding.EncodeToString([]byte(fx.client.ClientID+":"+secret)),
		"Basic",
		"Basic " + base64.StdEncoding.EncodeToString([]byte(fx.client.ClientID)),
		"Basic " + base64.StdEncoding.EncodeToString([]byte(fx.client.ClientID+":")),
		"Basic !!!not-base64!!!",
		"Basic " + base64.StdEncoding.EncodeToString([]byte(fx.client.ClientID+":"+secret)) + " extra",
		"Basic " + base64.RawStdEncoding.EncodeToString([]byte(fx.client.ClientID+":"+secret)),
		"Basic " + base64.StdEncoding.EncodeToString(append([]byte(fx.client.ClientID+":"+secret), 0xff)),
		"Basic " + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 129)+":"+secret)),
		"Basic " + base64.StdEncoding.EncodeToString([]byte(fx.client.ClientID+":"+strings.Repeat("s", tokenMaxSecretLen+1))),
	}
	for _, auth := range cases {
		rr := postToken(form, map[string]string{"Authorization": auth})
		assertTokenError(t, rr, http.StatusUnauthorized, oauth.ErrorInvalidClient, true)
		assert.Equal(t, tokenBasicRealm, rr.Header().Get("WWW-Authenticate"), auth)
		assert.NotContains(t, rr.Body.String(), secret)
	}

	dup := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	dup.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	dup.Header.Add("Authorization", basicAuth(fx.client.ClientID, secret))
	dup.Header.Add("Authorization", basicAuth(fx.client.ClientID, secret))
	rr := serve(handler, dup)
	assertTokenError(t, rr, http.StatusUnauthorized, oauth.ErrorInvalidClient, true)
	assert.Equal(t, tokenBasicRealm, rr.Header().Get("WWW-Authenticate"))
}

func TestFormAssertionAndBasicAreMutuallyExclusive(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 12, 0, 0, time.UTC)
	secret := "basic-secret-" + uuid.NewString()
	fx := issuedHTTPConfidentialBasicCode(t, now, secret)
	_ = newTokenAPI(t, fx)
	form := publicCodeForm(fx)
	form.Set("client_assertion_type", tokenAssertionType)
	form.Set("client_assertion", "eyJhbGciOiJFUzI1NiJ9.e30.sig")
	rr := postToken(form, map[string]string{"Authorization": basicAuth(fx.client.ClientID, secret)})
	assertTokenError(t, rr, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)
	assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
	assertCodeUnconsumed(t, fx)
}

func TestPrivateKeyJWTExchangeAndDurableReplay(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 15, 0, 0, time.UTC)
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	fx := issuedHTTPPrivateKeyJWTCode(t, now, mat)
	_ = newTokenAPI(t, fx)
	assertion := mintHTTPClientAssertion(t, mat, fx.client.ClientID, tokenHTTPEndpoint, now, 2*time.Minute)

	form := publicCodeForm(fx)
	form.Set("client_assertion_type", tokenAssertionType)
	form.Set("client_assertion", assertion)
	ok := postToken(form, nil)
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	assertTokenSuccessHeaders(t, ok, false)
	assertExactTokenSuccess(t, ok)
	assert.Empty(t, ok.Header().Get("WWW-Authenticate"))
	assert.NotContains(t, ok.Body.String(), assertion)

	fx2 := issuedHTTPPrivateKeyJWTCode(t, now, mat)
	replay := publicCodeForm(fx2)
	replay.Set("client_assertion_type", tokenAssertionType)
	replay.Set("client_assertion", assertion)
	denied := postToken(replay, nil)
	assertTokenError(t, denied, http.StatusUnauthorized, oauth.ErrorInvalidClient, false)
	assert.Empty(t, denied.Header().Get("WWW-Authenticate"))
	assert.NotContains(t, denied.Body.String(), assertion)
	assertCodeUnconsumed(t, fx2)
}

func TestPrivateKeyJWTRequiresExactFieldsAndRejectsAuthorization(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 16, 0, 0, time.UTC)
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	fx := issuedHTTPPrivateKeyJWTCode(t, now, mat)
	_ = newTokenAPI(t, fx)
	assertion := mintHTTPClientAssertion(t, mat, fx.client.ClientID, tokenHTTPEndpoint, now, 2*time.Minute)

	missingType := publicCodeForm(fx)
	missingType.Set("client_assertion", assertion)
	assertTokenError(t, postToken(missingType, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	wrongType := publicCodeForm(fx)
	wrongType.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:saml2-bearer")
	wrongType.Set("client_assertion", assertion)
	assertTokenError(t, postToken(wrongType, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	missingID := publicCodeForm(fx)
	missingID.Del("client_id")
	missingID.Set("client_assertion_type", tokenAssertionType)
	missingID.Set("client_assertion", assertion)
	assertTokenError(t, postToken(missingID, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	withAuth := publicCodeForm(fx)
	withAuth.Set("client_assertion_type", tokenAssertionType)
	withAuth.Set("client_assertion", assertion)
	denied := postToken(withAuth, map[string]string{"Authorization": basicAuth(fx.client.ClientID, "unused")})
	assertTokenError(t, denied, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)
	assert.Empty(t, denied.Header().Get("WWW-Authenticate"))
	assertCodeUnconsumed(t, fx)
}

func TestPublicClientRejectsAssertionAndAuthorization(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 17, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	_ = newTokenAPI(t, fx)

	withAuth := postToken(publicCodeForm(fx), map[string]string{
		"Authorization": basicAuth(fx.client.ClientID, "public-must-not-use-basic"),
	})
	assertTokenError(t, withAuth, http.StatusUnauthorized, oauth.ErrorInvalidClient, true)

	withAssertion := publicCodeForm(fx)
	withAssertion.Set("client_assertion_type", tokenAssertionType)
	withAssertion.Set("client_assertion", "eyJhbGciOiJFUzI1NiJ9.e30.sig")
	assertTokenError(t, postToken(withAssertion, nil), http.StatusUnauthorized, oauth.ErrorInvalidClient, false)
	assertCodeUnconsumed(t, fx)
}

func TestConfidentialMethodsMustBeExact(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 18, 0, 0, time.UTC)
	secret := "basic-secret-" + uuid.NewString()
	basicFX := issuedHTTPConfidentialBasicCode(t, now, secret)
	_ = newTokenAPI(t, basicFX)
	none := postToken(publicCodeForm(basicFX), nil)
	assertTokenError(t, none, http.StatusUnauthorized, oauth.ErrorInvalidClient, false)
	assert.Empty(t, none.Header().Get("WWW-Authenticate"))

	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	jwtFX := issuedHTTPPrivateKeyJWTCode(t, now, mat)
	_ = newTokenAPI(t, jwtFX)
	asBasic := postToken(publicCodeForm(jwtFX), map[string]string{
		"Authorization": basicAuth(jwtFX.client.ClientID, "not-a-secret"),
	})
	assertTokenError(t, asBasic, http.StatusUnauthorized, oauth.ErrorInvalidClient, true)
	assertCodeUnconsumed(t, jwtFX)
}

func TestTokenRejectsDuplicateUnknownQueryAndWrongMedia(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 20, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)
	form := publicCodeForm(fx)

	dup := postRaw(handler, form.Encode()+"&code="+url.QueryEscape(fx.rawCode), "application/x-www-form-urlencoded", nil)
	assertTokenError(t, dup, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	unknown := publicCodeForm(fx)
	unknown.Set("foo", "bar")
	assertTokenError(t, postToken(unknown, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	withQuery := httptest.NewRequest(http.MethodPost, "/oauth/token?grant_type=authorization_code", strings.NewReader(form.Encode()))
	withQuery.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assertTokenError(t, serve(handler, withQuery), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	jsonCT := postRaw(handler, form.Encode(), "application/json", nil)
	assert.Equal(t, http.StatusUnsupportedMediaType, jsonCT.Code)
	assert.NotContains(t, jsonCT.Body.String(), fx.rawCode)

	charset := postRaw(handler, form.Encode(), "application/x-www-form-urlencoded; charset=UTF-8", nil)
	assert.Equal(t, http.StatusUnsupportedMediaType, charset.Code)
	assertCodeUnconsumed(t, fx)
}

func TestTokenRejectsOversizeInvalidUTF8AndControlFields(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 21, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)

	over := strings.Repeat("a", 32*1024+1)
	tooBig := postRaw(handler, "grant_type=authorization_code&code="+over, "application/x-www-form-urlencoded", nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, tooBig.Code)

	invalidUTF8 := postRaw(handler, "grant_type=authorization_code&code=\xff\xfe", "application/x-www-form-urlencoded", nil)
	assertTokenError(t, invalidUTF8, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	control := publicCodeForm(fx)
	control.Set("code", fx.rawCode+"\x01")
	assertTokenError(t, postToken(control, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)
	assertCodeUnconsumed(t, fx)
}

func TestUnsupportedGrantsHaveZeroDBSideEffects(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 22, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	_ = newTokenAPI(t, fx)

	refresh := url.Values{
		"grant_type":    {oauth.GrantRefreshToken},
		"refresh_token": {"opaque-refresh-must-not-persist"},
		"resource":      {fx.redirect.ResourceURI},
		"client_id":     {fx.client.ClientID},
	}
	assertTokenError(t, postToken(refresh, nil), http.StatusBadRequest, oauth.ErrorUnsupportedGrantType, false)

	creds := url.Values{
		"grant_type": {oauth.GrantClientCredentials},
		"resource":   {fx.redirect.ResourceURI},
		"client_id":  {fx.client.ClientID},
	}
	assertTokenError(t, postToken(creds, nil), http.StatusBadRequest, oauth.ErrorUnsupportedGrantType, false)
	assertCodeUnconsumed(t, fx)
	assertZeroIssuance(t, fx)
}

func TestPKCEBindingReplayAndTransientHTTPMatrix(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 23, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	_ = newTokenAPI(t, fx)
	base := publicCodeForm(fx)

	wrongRedirect := cloneForm(base)
	wrongRedirect.Set("redirect_uri", "https://other.example/callback")
	assertTokenError(t, postToken(wrongRedirect, nil), http.StatusBadRequest, oauth.ErrorInvalidGrant, false)

	wrongResource := cloneForm(base)
	wrongResource.Set("resource", "https://other.example/resource")
	assertTokenError(t, postToken(wrongResource, nil), http.StatusBadRequest, oauth.ErrorInvalidGrant, false)

	wrongPKCE := cloneForm(base)
	wrongPKCE.Set("code_verifier", strings.Repeat("b", 43))
	assertTokenError(t, postToken(wrongPKCE, nil), http.StatusBadRequest, oauth.ErrorInvalidGrant, false)
	assertCodeUnconsumed(t, fx)

	ok := postToken(base, nil)
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	replay := postToken(base, nil)
	assertTokenError(t, replay, http.StatusBadRequest, oauth.ErrorInvalidGrant, false)

	failing := issuedHTTPPublicCode(t, now)
	failingHandler := newTokenAPIWithSigner(t, failing, failingHTTPSigner{})
	denied := postTokenOn(failingHandler, publicCodeForm(failing), nil)
	assertTokenError(t, denied, http.StatusServiceUnavailable, oauth.ErrorTemporarilyUnavail, false)
	assert.NotContains(t, denied.Body.String(), "access_token")
	assert.NotContains(t, denied.Body.String(), failing.rawCode)
	assertCodeUnconsumed(t, failing)
	assertZeroIssuance(t, failing)
}

func TestTokenSuccessOmitsInternalFieldsAndSetsProductionHSTS(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 24, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	svc := mustTokenService(t, fx, httpKeySigner{svc: fx.keys})
	_, handler := api.New(testutil.DB(t), api.Options{
		OAuth:  svc,
		Issuer: tokenHTTPIssuer,
		JWKS:   fx.keys,
		BrokerHTTP: api.BrokerHTTPOptions{
			Production: true,
		},
	})
	rr := postTokenOn(handler, publicCodeForm(fx), nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertTokenSuccessHeaders(t, rr, true)
	assertExactTokenSuccess(t, rr)
	assert.NotContains(t, rr.Body.String(), "id_token")
	assert.NotContains(t, rr.Body.String(), "azp")
	assert.NotContains(t, rr.Body.String(), "provider")
	assert.NotContains(t, rr.Body.String(), fx.client.ID.String())
	assert.NotContains(t, rr.Body.String(), fx.account.ID.String())
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestGETTokenIsNeverSuccess(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 25, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)
	rr := serve(handler, httptest.NewRequest(http.MethodGet, "/oauth/token", nil))
	assert.True(t, rr.Code == http.StatusMethodNotAllowed || rr.Code == http.StatusNotFound, rr.Code)
	assert.NotEqual(t, http.StatusOK, rr.Code)
	assert.NotContains(t, rr.Body.String(), "access_token")
	assert.NotContains(t, rr.Body.String(), fx.rawCode)
	if rr.Code == http.StatusMethodNotAllowed {
		assert.Contains(t, rr.Header().Get("Allow"), http.MethodPost)
	}
}

func TestConcurrentHTTPCodeExchangeHasOneSuccess(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 26, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)
	form := publicCodeForm(fx)

	const n = 8
	var wg sync.WaitGroup
	codes := make(chan int, n)
	bodies := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := postTokenOn(handler, form, nil)
			codes <- rr.Code
			bodies <- rr.Body.String()
		}()
	}
	wg.Wait()
	close(codes)
	close(bodies)

	ok := 0
	for code := range codes {
		if code == http.StatusOK {
			ok++
			continue
		}
		assert.Equal(t, http.StatusBadRequest, code)
	}
	require.Equal(t, 1, ok)
	for body := range bodies {
		assert.NotContains(t, body, fx.rawCode)
	}
}

func TestTokenErrorsAndLogsNeverIncludeSecrets(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 27, 0, 0, time.UTC)
	secret := "basic-log-secret-" + uuid.NewString()
	fx := issuedHTTPConfidentialBasicCode(t, now, secret)
	var logBuf bytes.Buffer
	logger := logging.NewJSONLogger(&logBuf, "info")
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })
	_ = newTokenAPI(t, fx)
	assertion := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9." + strings.Repeat("A", 40) + "." + strings.Repeat("B", 40)

	form := publicCodeForm(fx)
	form.Set("client_assertion_type", tokenAssertionType)
	form.Set("client_assertion", assertion)
	rr := postToken(form, map[string]string{"Authorization": basicAuth(fx.client.ClientID, secret)})
	assertTokenError(t, rr, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)
	scanSecrets(t, rr.Body.String(), fx.rawCode, secret, assertion)
	scanSecrets(t, logBuf.String(), fx.rawCode, secret, assertion)
}

func TestHighCountPublicHTTPExchanges(t *testing.T) {
	base := time.Date(2026, 8, 17, 18, 30, 0, 0, time.UTC)
	const n = 64
	fxs := make([]httpTokenFixture, n)
	for i := 0; i < n; i++ {
		fxs[i] = issuedHTTPPublicCode(t, base.Add(time.Duration(i)*time.Minute))
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fx := fxs[i]
			tracer := newHTTPTracingSigner(fx.keys)
			svc, err := oauth.NewService(oauth.Dependencies{
				Pool:    testutil.DB(t),
				Signer:  tracer,
				Secrets: fx.secrets,
				Clock:   frozenHTTPClock{now: fx.now},
				Config: oauth.Config{
					Issuer:        tokenHTTPIssuer,
					TokenEndpoint: tokenHTTPEndpoint,
					AccessTTL:     15 * time.Minute,
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("new service: %w [root=%v]", err, tracer.root())
				return
			}
			_, handler := api.New(testutil.DB(t), api.Options{
				OAuth:  svc,
				Issuer: tokenHTTPIssuer,
				JWKS:   fx.keys,
			})
			rr := postTokenOn(handler, publicCodeForm(fx), nil)
			if rr.Code != http.StatusOK {
				errCh <- fmt.Errorf("http %d body=%s [root=%v]", rr.Code, rr.Body.String(), tracer.root())
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

func TestBoundedPoolHighCountPublicHTTPExchanges(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL concurrency harness")
	}
	base := time.Date(2026, 8, 17, 18, 45, 0, 0, time.UTC)
	for _, maxConns := range []int32{8, 16, 32} {
		t.Run(fmt.Sprintf("maxconns-%d", maxConns), func(t *testing.T) {
			runBoundedHTTPExchanges(t, base, maxConns, 64)
			runBoundedHTTPExchanges(t, base.Add(time.Hour), maxConns, 128)
		})
	}
}

func runBoundedHTTPExchanges(t *testing.T, base time.Time, maxConns int32, n int) {
	t.Helper()
	pool := limitedHTTPPool(t, maxConns)
	fxs := make([]httpTokenFixture, n)
	for i := 0; i < n; i++ {
		fxs[i] = issuedHTTPPublicCode(t, base.Add(time.Duration(i)*time.Second))
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	var current, currentTx atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fx := fxs[i]
			tracer := newHTTPTracingSigner(fx.keys)
			svc, err := oauth.NewService(oauth.Dependencies{
				Pool:    pool,
				Signer:  tracer,
				Secrets: fx.secrets,
				Clock:   frozenHTTPClock{now: fx.now},
				Config: oauth.Config{
					Issuer:        tokenHTTPIssuer,
					TokenEndpoint: tokenHTTPEndpoint,
					AccessTTL:     15 * time.Minute,
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("new service: %w", err)
				return
			}
			_, handler := api.New(pool, api.Options{
				OAuth:  svc,
				Issuer: tokenHTTPIssuer,
				JWKS:   fx.keys,
			})
			rr := postTokenOn(handler, publicCodeForm(fx), nil)
			current.Add(tracer.current.Load())
			currentTx.Add(tracer.currentTx.Load())
			if rr.Code != http.StatusOK {
				errCh <- fmt.Errorf("http %d body=%s", rr.Code, rr.Body.String())
				return
			}
			errCh <- nil
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
		t.Fatal("bounded HTTP exchanges did not complete")
	}
	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, int64(0), current.Load(), "HTTP issuance must not check out a nested ActiveSigner")
	assert.GreaterOrEqual(t, currentTx.Load(), int64(n), "each successful HTTP exchange prepares a transaction-bound signer")
	assert.LessOrEqual(t, currentTx.Load(), int64(n)*8, "HTTP signer checkouts stay within serializable retry bounds")
	assert.LessOrEqual(t, pool.Stat().MaxConns(), maxConns)
}

func limitedHTTPPool(t *testing.T, maxConns int32) *pgxpool.Pool {
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

func publicCodeForm(fx httpTokenFixture) url.Values {
	return url.Values{
		"grant_type":    {oauth.GrantAuthorizationCode},
		"code":          {fx.rawCode},
		"redirect_uri":  {fx.redirect.RedirectURI},
		"resource":      {fx.redirect.ResourceURI},
		"code_verifier": {tokenHTTPVerifier},
		"client_id":     {fx.client.ClientID},
	}
}

func cloneForm(in url.Values) url.Values {
	out := url.Values{}
	for k, vals := range in {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

func postToken(form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	return postRawOn(currentTokenHandler, form.Encode(), "application/x-www-form-urlencoded", headers)
}

func postTokenOn(handler http.Handler, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	return postRawOn(handler, form.Encode(), "application/x-www-form-urlencoded", headers)
}

func postRaw(handler http.Handler, body, contentType string, headers map[string]string) *httptest.ResponseRecorder {
	return postRawOn(handler, body, contentType, headers)
}

var currentTokenHandler http.Handler

func postRawOn(handler http.Handler, body, contentType string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if handler == nil {
		handler = currentTokenHandler
	}
	return serve(handler, req)
}

func assertTokenError(t *testing.T, rr *httptest.ResponseRecorder, status int, code string, wantChallenge bool) {
	t.Helper()
	require.Equal(t, status, rr.Code, rr.Body.String())
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	if wantChallenge {
		assert.Equal(t, tokenBasicRealm, rr.Header().Get("WWW-Authenticate"))
	} else {
		assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
	}
	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, code, body["error"])
	assert.NotContains(t, rr.Body.String(), "access_token")
	assert.NotContains(t, rr.Body.String(), "$schema")
}

func assertTokenSuccessHeaders(t *testing.T, rr *httptest.ResponseRecorder, wantHSTS bool) {
	t.Helper()
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	if wantHSTS {
		assert.Equal(t, "max-age=31536000; includeSubDomains", rr.Header().Get("Strict-Transport-Security"))
	}
}

func assertExactTokenSuccess(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, []string{"access_token", "expires_in", "refresh_token", "scope", "token_type"}, sortedKeys(body))
	assert.Equal(t, "Bearer", body["token_type"])
	assert.NotEmpty(t, body["access_token"])
	assert.NotEmpty(t, body["refresh_token"])
}

func assertCodeUnconsumed(t *testing.T, fx httpTokenFixture) {
	t.Helper()
	var consumed *time.Time
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.Nil(t, consumed)
}

func assertZeroIssuance(t *testing.T, fx httpTokenFixture) {
	t.Helper()
	var families, audits int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1`, fx.grant.ID).Scan(&audits))
	require.Zero(t, families)
	require.Zero(t, audits)
}

func scanSecrets(t *testing.T, haystack string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		assert.NotContains(t, haystack, secret)
	}
}

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func mustTokenService(t *testing.T, fx httpTokenFixture, signer token.SignerSource) *oauth.Service {
	t.Helper()
	svc, err := oauth.NewService(oauth.Dependencies{
		Pool:    testutil.DB(t),
		Signer:  signer,
		Secrets: fx.secrets,
		Clock:   frozenHTTPClock{now: fx.now},
		Config: oauth.Config{
			Issuer:        tokenHTTPIssuer,
			TokenEndpoint: tokenHTTPEndpoint,
			AccessTTL:     15 * time.Minute,
		},
	})
	require.NoError(t, err)
	return svc
}

func newTokenAPIWithSigner(t *testing.T, fx httpTokenFixture, signer token.SignerSource) http.Handler {
	t.Helper()
	svc := mustTokenService(t, fx, signer)
	_, handler := api.New(testutil.DB(t), api.Options{
		OAuth:  svc,
		Issuer: tokenHTTPIssuer,
		JWKS:   fx.keys,
	})
	currentTokenHandler = handler
	return handler
}

type failingHTTPSigner struct{}

func (failingHTTPSigner) Current(context.Context) (token.Signer, *domain.SigningKey, error) {
	return nil, nil, errors.New("signer down")
}

type httpTracingSigner struct {
	svc       *keys.Service
	last      atomic.Value
	current   atomic.Int64
	currentTx atomic.Int64
}

func newHTTPTracingSigner(svc *keys.Service) *httpTracingSigner {
	return &httpTracingSigner{svc: svc}
}

func (s *httpTracingSigner) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	s.current.Add(1)
	signer, meta, err := s.svc.ActiveSigner(ctx)
	if err != nil {
		s.last.Store(err)
		return nil, nil, err
	}
	return httpTracingCryptoSigner{Signer: signer, last: &s.last}, meta, nil
}

func (s *httpTracingSigner) CurrentForTx(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error) {
	s.currentTx.Add(1)
	signer, meta, err := s.svc.ActiveSignerForTx(ctx, tx)
	if err != nil {
		s.last.Store(err)
		return nil, nil, err
	}
	return httpTracingCryptoSigner{Signer: signer, last: &s.last}, meta, nil
}

func (s *httpTracingSigner) root() error {
	got, _ := s.last.Load().(error)
	return got
}

type httpTracingCryptoSigner struct {
	token.Signer
	last *atomic.Value
}

func (s httpTracingCryptoSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	sig, err := s.Signer.Sign(rand, digest, opts)
	if err != nil && s.last != nil {
		s.last.Store(err)
	}
	return sig, err
}

func issuedHTTPConfidentialBasicCode(t *testing.T, now time.Time, plaintext string) httpTokenFixture {
	t.Helper()
	secrets := httpTokenSecrets()
	hash, err := secrethash.Hash(secrethash.Peppers(secrets.ClientSecretPeppers), secrets.ClientSecretActiveVersion, "primer.oauth.client-secret", []byte(plaintext))
	require.NoError(t, err)
	version := int16(secrets.ClientSecretActiveVersion)
	return issuedHTTPCustomClient(t, now, domain.OAuthClient{
		ClientID: "basic-" + uuid.NewString()[:8], Name: "Basic client", ClientType: "confidential",
		TokenEndpointAuthMethod: oauth.AuthBasic, AllowedGrants: []string{oauth.GrantAuthorizationCode},
		Enabled: true, ClientSecretHash: hash, ClientSecretPepperVersion: &version,
	})
}

func issuedHTTPPrivateKeyJWTCode(t *testing.T, now time.Time, mat *keys.Material) httpTokenFixture {
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
	return issuedHTTPCustomClient(t, now, *client)
}

func issuedHTTPCustomClient(t *testing.T, now time.Time, clientIn domain.OAuthClient) httpTokenFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := httpTokenSecrets()
	client := &clientIn
	if client.ID == uuid.Nil {
		created, err := repo.CreateOAuthClient(ctx, pool, clientIn)
		require.NoError(t, err)
		client = created
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
	sum := sha256.Sum256([]byte(tokenHTTPVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code, err := repo.CreateAuthorizationCode(ctx, pool, domain.OAuthAuthorizationCode{
		CodeHash: codeHash, PepperVersion: int16(secrets.AuthorizationCodeActiveVersion),
		GrantID: grant.ID, BrokerTransactionID: broker.ID, OAuthClientID: client.ID,
		RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: challenge, PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	})
	require.NoError(t, err)
	return httpTokenFixture{
		client: client, redirect: redirect, account: account, grant: grant, code: code,
		rawCode: rawCode, secrets: secrets, keys: sharedHTTPKeys(t), now: now,
	}
}

func mintHTTPClientAssertion(t *testing.T, mat *keys.Material, clientID, aud string, now time.Time, ttl time.Duration) string {
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
