package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	revokeHTTPEndpoint = "https://identity.example.test/oauth/revoke"
	revokeBasicRealm   = `Basic realm="oauth/revoke", charset="UTF-8"`
)

func TestPublicClientRevokeReturnsEmpty200AndPersists(t *testing.T) {
	now := time.Date(2026, 8, 17, 21, 0, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)
	issued := exchangeHTTPPublic(t, handler, fx)

	rr := postRevokeOn(handler, url.Values{
		"token":     {issued.refresh},
		"client_id": {fx.client.ClientID},
	}, nil)
	assertRevokeSuccess(t, rr)
	assert.Empty(t, strings.TrimSpace(rr.Body.String()))
	assertHTTPFamilyRevoked(t, fx.grant.ID)

	again := postRevokeOn(handler, url.Values{
		"token":           {issued.refresh},
		"token_type_hint": {"refresh_token"},
		"client_id":       {fx.client.ClientID},
	}, nil)
	assertRevokeSuccess(t, again)
	assert.NotContains(t, rr.Body.String(), issued.refresh)
	assert.NotContains(t, rr.Body.String(), issued.access)
}

func TestRevokeUnknownCrossClientAndAccessTokenAreOracleFree(t *testing.T) {
	now := time.Date(2026, 8, 17, 21, 5, 0, 0, time.UTC)
	owner := issuedHTTPPublicCode(t, now)
	other := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, owner)
	issued := exchangeHTTPPublic(t, handler, owner)

	unknown := postRevokeOn(handler, url.Values{"token": {"not-a-refresh"}, "client_id": {owner.client.ClientID}}, nil)
	assertRevokeSuccess(t, unknown)

	cross := postRevokeOn(handler, url.Values{"token": {issued.refresh}, "client_id": {other.client.ClientID}}, nil)
	assertRevokeSuccess(t, cross)
	assertHTTPFamilyActive(t, owner.grant.ID)

	access := postRevokeOn(handler, url.Values{
		"token": {issued.access}, "token_type_hint": {"access_token"}, "client_id": {owner.client.ClientID},
	}, nil)
	assertRevokeSuccess(t, access)
	assertHTTPFamilyActive(t, owner.grant.ID)

	ignoredHint := postRevokeOn(handler, url.Values{
		"token": {issued.refresh}, "token_type_hint": {"id_token"}, "client_id": {owner.client.ClientID},
	}, nil)
	assertRevokeSuccess(t, ignoredHint)
	assertHTTPFamilyRevoked(t, owner.grant.ID)
}

func TestRevokeBasicChallengeAndPrivateKeyJWTNoBearer(t *testing.T) {
	now := time.Date(2026, 8, 17, 21, 10, 0, 0, time.UTC)
	secret := "basic-secret-" + uuid.NewString()
	basicFX := issuedHTTPConfidentialBasicCode(t, now, secret)
	handler := newTokenAPI(t, basicFX)
	issued := exchangeHTTPBasic(t, handler, basicFX, secret)

	wrong := postRevokeOn(handler, url.Values{"token": {issued.refresh}}, map[string]string{
		"Authorization": basicAuth(basicFX.client.ClientID, "wrong-secret-value-xxxxxxxxxxxxxxxx"),
	})
	assertRevokeError(t, wrong, http.StatusUnauthorized, oauth.ErrorInvalidClient, true)
	assert.Equal(t, revokeBasicRealm, wrong.Header().Get("WWW-Authenticate"))
	assertHTTPFamilyActive(t, basicFX.grant.ID)

	ok := postRevokeOn(handler, url.Values{"token": {issued.refresh}}, map[string]string{
		"Authorization": basicAuth(basicFX.client.ClientID, secret),
	})
	assertRevokeSuccess(t, ok)
	assert.Empty(t, ok.Header().Get("WWW-Authenticate"))
	assertHTTPFamilyRevoked(t, basicFX.grant.ID)

	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	jwtFX := issuedHTTPPrivateKeyJWTCode(t, now, mat)
	jwtHandler := newTokenAPI(t, jwtFX)
	tokenAssertion := mintHTTPClientAssertion(t, mat, jwtFX.client.ClientID, tokenHTTPEndpoint, now, 2*time.Minute)
	jwtIssued := exchangeHTTPPrivate(t, jwtHandler, jwtFX, tokenAssertion)

	replay := url.Values{
		"token": {jwtIssued.refresh}, "client_id": {jwtFX.client.ClientID},
		"client_assertion_type": {tokenAssertionType}, "client_assertion": {tokenAssertion},
	}
	denied := postRevokeOn(jwtHandler, replay, nil)
	assertRevokeError(t, denied, http.StatusUnauthorized, oauth.ErrorInvalidClient, false)
	assert.Empty(t, denied.Header().Get("WWW-Authenticate"))
	assert.NotContains(t, denied.Body.String(), tokenAssertion)
	assertHTTPFamilyActive(t, jwtFX.grant.ID)

	revokeAssertion := mintHTTPClientAssertion(t, mat, jwtFX.client.ClientID, revokeHTTPEndpoint, now, 2*time.Minute)
	form := url.Values{
		"token": {jwtIssued.refresh}, "client_id": {jwtFX.client.ClientID},
		"client_assertion_type": {tokenAssertionType}, "client_assertion": {revokeAssertion},
	}
	okJWT := postRevokeOn(jwtHandler, form, nil)
	assertRevokeSuccess(t, okJWT)
	assert.Empty(t, okJWT.Header().Get("WWW-Authenticate"))
	assertHTTPFamilyRevoked(t, jwtFX.grant.ID)
}

func TestRevokeRejectsDuplicateUnknownQueryUTF8AndWrongMedia(t *testing.T) {
	now := time.Date(2026, 8, 17, 21, 15, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)
	issued := exchangeHTTPPublic(t, handler, fx)
	form := url.Values{"token": {issued.refresh}, "client_id": {fx.client.ClientID}}

	dup := postRawRevoke(handler, form.Encode()+"&token="+url.QueryEscape(issued.refresh), "application/x-www-form-urlencoded", nil)
	assertRevokeError(t, dup, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	unknown := url.Values{"token": {issued.refresh}, "client_id": {fx.client.ClientID}, "foo": {"bar"}}
	assertRevokeError(t, postRevokeOn(handler, unknown, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	withQuery := httptest.NewRequest(http.MethodPost, "/oauth/revoke?token=x", strings.NewReader(form.Encode()))
	withQuery.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assertRevokeError(t, serve(handler, withQuery), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	jsonCT := postRawRevoke(handler, form.Encode(), "application/json", nil)
	assert.Equal(t, http.StatusUnsupportedMediaType, jsonCT.Code)

	charset := postRawRevoke(handler, form.Encode(), "application/x-www-form-urlencoded; charset=UTF-8", nil)
	assert.Equal(t, http.StatusUnsupportedMediaType, charset.Code)

	oversize := strings.Repeat("a", 16*1024+1)
	tooBig := postRawRevoke(handler, "token="+oversize+"&client_id="+fx.client.ClientID, "application/x-www-form-urlencoded", nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, tooBig.Code)

	invalidUTF8 := postRawRevoke(handler, "token=\xff\xfe&client_id="+fx.client.ClientID, "application/x-www-form-urlencoded", nil)
	assertRevokeError(t, invalidUTF8, http.StatusBadRequest, oauth.ErrorInvalidRequest, false)

	control := url.Values{"token": {"abc\x01def"}, "client_id": {fx.client.ClientID}}
	assertRevokeError(t, postRevokeOn(handler, control, nil), http.StatusBadRequest, oauth.ErrorInvalidRequest, false)
	assertHTTPFamilyActive(t, fx.grant.ID)
}

func TestGETRevokeIsNeverSuccess(t *testing.T) {
	now := time.Date(2026, 8, 17, 21, 20, 0, 0, time.UTC)
	fx := issuedHTTPPublicCode(t, now)
	handler := newTokenAPI(t, fx)
	rr := serve(handler, httptest.NewRequest(http.MethodGet, "/oauth/revoke", nil))
	assert.True(t, rr.Code == http.StatusMethodNotAllowed || rr.Code == http.StatusNotFound, rr.Code)
	assert.NotEqual(t, http.StatusOK, rr.Code)
	if rr.Code == http.StatusMethodNotAllowed {
		assert.Contains(t, rr.Header().Get("Allow"), http.MethodPost)
	}
}

func TestRevokeSuccessSetsProductionHSTSAndLeavesAccessJWTValid(t *testing.T) {
	now := time.Date(2026, 8, 17, 21, 25, 0, 0, time.UTC)
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
	issued := exchangeHTTPPublic(t, handler, fx)
	rr := postRevokeOn(handler, url.Values{"token": {issued.refresh}, "client_id": {fx.client.ClientID}}, nil)
	assertRevokeSuccess(t, rr)
	assert.Equal(t, "max-age=31536000; includeSubDomains", rr.Header().Get("Strict-Transport-Security"))
	assert.Empty(t, strings.TrimSpace(rr.Body.String()))
	assertHTTPFamilyRevoked(t, fx.grant.ID)

	pubs, err := fx.keys.PublicJWKS(context.Background())
	require.NoError(t, err)
	keyset, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) { return pubs, nil })
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(context.Background()))
	verifier, err := token.NewVerifier(keyset, tokenHTTPIssuer, fx.redirect.Audience, frozenHTTPClock{now: now}, nil)
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), issued.access)
	require.NoError(t, err)
	assert.Equal(t, fx.client.ClientID, got.ClientID)
	assert.Equal(t, fx.redirect.Audience, got.Audience)
}

func TestNewOpenAPIRevokeWithoutOAuthIsTemporarilyUnavailable(t *testing.T) {
	_, handler := api.NewOpenAPI()
	rr := postRevokeOn(handler, url.Values{"token": {"schema-only-must-not-revoke"}, "client_id": {"schema-only-client"}}, nil)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), oauth.ErrorTemporarilyUnavail)
	assert.NotContains(t, rr.Body.String(), "schema-only-must-not-revoke")
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

type httpIssuedTokens struct {
	access  string
	refresh string
}

func exchangeHTTPPublic(t *testing.T, handler http.Handler, fx httpTokenFixture) httpIssuedTokens {
	t.Helper()
	rr := postTokenOn(handler, publicCodeForm(fx), nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	return parseIssued(t, rr)
}

func exchangeHTTPBasic(t *testing.T, handler http.Handler, fx httpTokenFixture, secret string) httpIssuedTokens {
	t.Helper()
	rr := postTokenOn(handler, publicCodeForm(fx), map[string]string{"Authorization": basicAuth(fx.client.ClientID, secret)})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	return parseIssued(t, rr)
}

func exchangeHTTPPrivate(t *testing.T, handler http.Handler, fx httpTokenFixture, assertion string) httpIssuedTokens {
	t.Helper()
	form := publicCodeForm(fx)
	form.Set("client_assertion_type", tokenAssertionType)
	form.Set("client_assertion", assertion)
	rr := postTokenOn(handler, form, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	return parseIssued(t, rr)
}

func parseIssued(t *testing.T, rr *httptest.ResponseRecorder) httpIssuedTokens {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)
	return httpIssuedTokens{access: access, refresh: refresh}
}

func postRevokeOn(handler http.Handler, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	return postRawRevoke(handler, form.Encode(), "application/x-www-form-urlencoded", headers)
}

func postRawRevoke(handler http.Handler, body, contentType string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return serve(handler, req)
}

func assertRevokeSuccess(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
	assert.NotContains(t, rr.Body.String(), "access_token")
	assert.NotContains(t, rr.Body.String(), "refresh_token")
	assert.NotContains(t, rr.Body.String(), "$schema")
}

func assertRevokeError(t *testing.T, rr *httptest.ResponseRecorder, status int, code string, wantChallenge bool) {
	t.Helper()
	require.Equal(t, status, rr.Code, rr.Body.String())
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	if wantChallenge {
		assert.Equal(t, revokeBasicRealm, rr.Header().Get("WWW-Authenticate"))
	} else {
		assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
	}
	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, code, body["error"])
}

func assertHTTPFamilyActive(t *testing.T, grantID uuid.UUID) {
	t.Helper()
	var status string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status,
       (SELECT count(*) FROM oauth_refresh_tokens tok WHERE tok.family_id=f.id AND tok.consumed_at IS NULL AND tok.revoked_at IS NULL)
FROM oauth_refresh_families f WHERE f.grant_id=$1`, grantID).Scan(&status, &live))
	require.Equal(t, "active", status)
	require.Equal(t, 1, live)
}

func assertHTTPFamilyRevoked(t *testing.T, grantID uuid.UUID) {
	t.Helper()
	var familyStatus, grantStatus string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status, g.status,
       (SELECT count(*) FROM oauth_refresh_tokens tok WHERE tok.family_id=f.id AND tok.consumed_at IS NULL AND tok.revoked_at IS NULL)
FROM oauth_refresh_families f JOIN oauth_grants g ON g.id=f.grant_id WHERE f.grant_id=$1`, grantID).Scan(&familyStatus, &grantStatus, &live))
	require.Equal(t, "revoked", familyStatus)
	require.Equal(t, "revoked", grantStatus)
	require.Zero(t, live)
}
