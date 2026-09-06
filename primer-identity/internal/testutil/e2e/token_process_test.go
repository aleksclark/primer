package e2e_test

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
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	tokenProcessIssuer   = "https://id.example.test"
	tokenProcessVerifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVW"
	tokenProcessSeal     = "UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE"
	tokenAssertionType   = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	tokenBasicRealm      = `Basic realm="token", charset="UTF-8"`
)

func tokenProcessConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := brokerProcessConfig(t)
	cfg.Issuer = tokenProcessIssuer
	cfg.Key.Enabled = true
	cfg.Key.AutoBootstrap = true
	cfg.Key.SetSealSecretForTest(tokenProcessSeal)
	cfg.ClientSecretPeppers = encodedSecret(0x51)
	cfg.ClientSecretActiveVersion = 1
	cfg.RefreshTokenPeppers = encodedSecret(0x61)
	cfg.RefreshTokenActiveVersion = 1
	cfg.ClientAssertionPeppers = encodedSecret(0x71)
	cfg.ClientAssertionActiveVersion = 1
	require.NoError(t, cfg.Validate())
	return cfg
}

func startTokenProcess(t *testing.T) *processServer {
	t.Helper()
	return startBrokerProcess(t, tokenProcessConfig(t), scripted(t, successFixture(uniqueLabel("token-ready"), brokerprovider.MethodEmailMagicLink)))
}

func startTokenProcessWithArtifact(t *testing.T, artifact string, extra ...brokerprovider.Fixture) *processServer {
	t.Helper()
	fixtures := append([]brokerprovider.Fixture{successFixture(artifact, brokerprovider.MethodEmailMagicLink)}, extra...)
	return startBrokerProcess(t, tokenProcessConfig(t), scripted(t, fixtures...))
}

func issueProcessCode(t *testing.T, srv *processServer, clientID, redirect, resource, audience, artifact string) string {
	t.Helper()
	client := noFollowClient()
	authz := authorizeWithVerifier(t, client, srv.baseURL, clientID, redirect, resource, audience, uniqueLabel("st"))
	cookie := cookieFrom(authz)
	_ = authz.Body.Close()
	require.NotEmpty(t, cookie)
	// Private, per-request capture: never inspect callback bodies or dump the
	// service log buffer. The diagnostic contains only closed classes/counts.
	requestID := uuid.NewString()
	capture, release := api.CaptureBrokerCallbackForTest(strings.TrimPrefix(srv.baseURL, "http://"), requestID)
	defer release()
	cbReq, err := http.NewRequest(http.MethodGet, srv.baseURL+"/broker/stytch/callback?token="+url.QueryEscape(artifact)+"&type=discovery_magic_link", nil)
	require.NoError(t, err)
	cbReq.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	cbReq.Header.Set("X-Request-ID", requestID)
	cb, err := client.Do(cbReq)
	require.NoError(t, err)
	_ = cb.Body.Close()
	diagnostic, captured := capture.Snapshot()
	require.Equal(t, http.StatusSeeOther, cb.StatusCode, "callback diagnostic captured=%t %s", captured, diagnostic.String())
	loc, err := url.Parse(cb.Header.Get("Location"))
	require.NoError(t, err)
	code := loc.Query().Get("code")
	require.NotEmpty(t, code)
	return code
}

func authorizeWithVerifier(t *testing.T, client *http.Client, baseURL, clientID, redirect, resource, audience, state string) *http.Response {
	t.Helper()
	sum := sha256.Sum256([]byte(tokenProcessVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	q := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect},
		"resource": {resource}, "audience": {audience}, "scope": {"openid"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	resp, err := client.Get(baseURL + "/oauth/authorize?" + q.Encode())
	require.NoError(t, err)
	return resp
}

func postProcessToken(t *testing.T, baseURL string, form url.Values, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/oauth/token", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := noFollowClient().Do(req)
	require.NoError(t, err)
	return resp
}

func publicTokenForm(code, redirect, resource, clientID string) url.Values {
	form := url.Values{
		"grant_type":    {oauth.GrantAuthorizationCode},
		"code":          {code},
		"redirect_uri":  {redirect},
		"resource":      {resource},
		"code_verifier": {tokenProcessVerifier},
	}
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	return form
}

func readJSON(t *testing.T, resp *http.Response) (int, map[string]any, string) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	var parsed map[string]any
	if len(bytes.TrimSpace(body)) > 0 && body[0] == '{' {
		require.NoError(t, json.Unmarshal(body, &parsed))
	}
	return resp.StatusCode, parsed, string(body)
}

func verifyFetchedAccess(t *testing.T, baseURL, access, audience, clientID, subject string) {
	t.Helper()
	jwksResp, err := noFollowClient().Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	raw, err := io.ReadAll(jwksResp.Body)
	_ = jwksResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, jwksResp.StatusCode)
	pubs, err := token.ParseJWKS(raw)
	require.NoError(t, err)
	keyset, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) { return pubs, nil })
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(context.Background()))
	verifier, err := token.NewVerifier(keyset, tokenProcessIssuer, audience, nil, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		return token.ClientRegistration{ClientID: clientID, Audience: audience, SubjectClass: token.KindHuman}, nil
	})
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), access)
	require.NoError(t, err)
	assert.Equal(t, tokenProcessIssuer, got.Issuer)
	assert.Equal(t, audience, got.Audience)
	assert.Equal(t, clientID, got.ClientID)
	assert.Equal(t, subject, got.Subject)
	payload := decodeJWTPayload(t, access)
	assert.Equal(t, audience, payload["aud"])
	assert.Equal(t, clientID, payload["client_id"])
	_, hasAZP := payload["azp"]
	assert.False(t, hasAZP)
	for _, banned := range []string{"roles", "org", "organization_id", "provider", "session_id", "stytch"} {
		_, present := payload[banned]
		assert.False(t, present, banned)
	}
}

func decodeJWTPayload(t *testing.T, compact string) map[string]any {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload
}

func registerConfidentialBasic(t *testing.T, clientID, secret string) (redirect, resource, audience string) {
	t.Helper()
	pool := testutil.DB(t)
	hash, err := secrethash.Hash(secrethash.Peppers(map[int][]byte{1: bytes.Repeat([]byte{0x51}, 32)}), 1, "primer.oauth.client-secret", []byte(secret))
	require.NoError(t, err)
	version := int16(1)
	client, err := repo.CreateOAuthClient(context.Background(), pool, domain.OAuthClient{
		ClientID: clientID, Name: "e2e basic", ClientType: "confidential",
		TokenEndpointAuthMethod: oauth.AuthBasic, AllowedGrants: []string{oauth.GrantAuthorizationCode},
		Enabled: true, ClientSecretHash: hash, ClientSecretPepperVersion: &version,
	})
	require.NoError(t, err)
	redirect = "https://" + clientID + ".example/callback"
	resource = "https://" + clientID + ".example/mcp"
	audience = clientID + "-aud"
	_, err = repo.CreateOAuthClientRedirect(context.Background(), pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: redirect, ResourceURI: resource,
		Audience: audience, AllowedScopes: []string{"openid"}, Enabled: true,
	})
	require.NoError(t, err)
	return redirect, resource, audience
}

func registerPrivateKeyJWT(t *testing.T, clientID string, mat *keys.Material) (redirect, resource, audience string) {
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
		ClientID: clientID, Name: "e2e jwt", ClientType: "confidential",
		TokenEndpointAuthMethod: oauth.AuthPrivateKeyJWT, AllowedGrants: []string{oauth.GrantAuthorizationCode},
		Enabled: true,
	})
	require.NoError(t, err)
	_, err = repo.CreateOAuthClientKey(ctx, tx, domain.OAuthClientKey{
		OAuthClientID: client.ID, Kid: jwk.Kid, JWKJSON: raw, Alg: "ES256", Use: "sig", Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	redirect = "https://" + clientID + ".example/callback"
	resource = "https://" + clientID + ".example/mcp"
	audience = clientID + "-aud"
	_, err = repo.CreateOAuthClientRedirect(context.Background(), pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: redirect, ResourceURI: resource,
		Audience: audience, AllowedScopes: []string{"openid"}, Enabled: true,
	})
	require.NoError(t, err)
	return redirect, resource, audience
}

func mintProcessAssertion(t *testing.T, mat *keys.Material, clientID, aud string) string {
	t.Helper()
	jwk, err := mat.PublicJWK()
	require.NoError(t, err)
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": jwk.Kid, "typ": "JWT"})
	require.NoError(t, err)
	now := time.Now().UTC().Unix()
	payload, err := json.Marshal(map[string]any{
		"iss": clientID, "sub": clientID, "aud": aud,
		"iat": now, "exp": now + 120, "jti": uuid.NewString(),
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

func accountSubjectForClient(t *testing.T, clientID string) string {
	t.Helper()
	var subject string
	err := testutil.DB(t).QueryRow(context.Background(), `
SELECT a.id::text
FROM accounts a
JOIN oauth_grants g ON g.account_id = a.id
JOIN oauth_clients c ON c.id = g.oauth_client_id
WHERE c.client_id = $1
ORDER BY g.granted_at DESC
LIMIT 1`, clientID).Scan(&subject)
	require.NoError(t, err)
	return subject
}

func TestProcessTokenPublicBasicAndPrivateKeyJWTViaFetchedJWKS(t *testing.T) {
	publicID := uniqueLabel("pub")
	artifact := uniqueLabel("pub-art")
	srv := startTokenProcessWithArtifact(t, artifact)
	redirect, resource, audience := registerProcessClient(t, publicID)
	code := issueProcessCode(t, srv, publicID, redirect, resource, audience, artifact)

	ok := postProcessToken(t, srv.baseURL, publicTokenForm(code, redirect, resource, publicID), nil)
	status, body, raw := readJSON(t, ok)
	require.Equal(t, http.StatusOK, status, raw)
	assert.Equal(t, "Bearer", body["token_type"])
	assert.Equal(t, "openid", body["scope"])
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)
	assert.LessOrEqual(t, body["expires_in"].(float64), 900.0)
	verifyFetchedAccess(t, srv.baseURL, access, audience, publicID, accountSubjectForClient(t, publicID))
	assert.NotContains(t, raw, "azp")
	assert.NotContains(t, srv.logs.String(), access)
	assert.NotContains(t, srv.logs.String(), refresh)
	assert.NotContains(t, srv.logs.String(), code)

	var families, tokens, audits int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM oauth_refresh_families f JOIN oauth_clients c ON c.id=f.oauth_client_id WHERE c.client_id=$1),
  (SELECT count(*) FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id JOIN oauth_clients c ON c.id=f.oauth_client_id WHERE c.client_id=$1 AND t.consumed_at IS NULL AND t.sequence=0),
  (SELECT count(*) FROM token_issuance_audit a WHERE a.client_id=$1)`, publicID).Scan(&families, &tokens, &audits))
	assert.Equal(t, 1, families)
	assert.Equal(t, 1, tokens)
	assert.Equal(t, 1, audits)

	secret := "basic-secret-" + uuid.NewString()
	basicID := uniqueLabel("basic")
	basicArtifact := uniqueLabel("basic-art")
	basicSrv := startTokenProcessWithArtifact(t, basicArtifact)
	basicRedirect, basicResource, basicAudience := registerConfidentialBasic(t, basicID, secret)
	basicCode := issueProcessCode(t, basicSrv, basicID, basicRedirect, basicResource, basicAudience, basicArtifact)
	wrong := postProcessToken(t, basicSrv.baseURL, publicTokenForm(basicCode, basicRedirect, basicResource, ""), map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(basicID+":wrong-secret-value-xxxxxxxxxxxxxxxx")),
	})
	wrongStatus, wrongBody, wrongRaw := readJSON(t, wrong)
	assert.Equal(t, http.StatusUnauthorized, wrongStatus)
	assert.Equal(t, oauth.ErrorInvalidClient, wrongBody["error"])
	assert.Equal(t, tokenBasicRealm, wrong.Header.Get("WWW-Authenticate"))
	assert.NotContains(t, wrongRaw, secret)

	okBasic := postProcessToken(t, basicSrv.baseURL, publicTokenForm(basicCode, basicRedirect, basicResource, ""), map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(basicID+":"+secret)),
	})
	basicStatus, basicBody, basicRaw := readJSON(t, okBasic)
	require.Equal(t, http.StatusOK, basicStatus, basicRaw)
	verifyFetchedAccess(t, basicSrv.baseURL, basicBody["access_token"].(string), basicAudience, basicID, accountSubjectForClient(t, basicID))

	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	jwtID := uniqueLabel("jwt")
	jwtArtifact := uniqueLabel("jwt-art")
	jwtSrv := startTokenProcessWithArtifact(t, jwtArtifact)
	jwtRedirect, jwtResource, jwtAudience := registerPrivateKeyJWT(t, jwtID, mat)
	jwtCode := issueProcessCode(t, jwtSrv, jwtID, jwtRedirect, jwtResource, jwtAudience, jwtArtifact)
	assertion := mintProcessAssertion(t, mat, jwtID, tokenProcessIssuer+"/oauth/token")
	jwtForm := publicTokenForm(jwtCode, jwtRedirect, jwtResource, jwtID)
	jwtForm.Set("client_assertion_type", tokenAssertionType)
	jwtForm.Set("client_assertion", assertion)
	okJWT := postProcessToken(t, jwtSrv.baseURL, jwtForm, nil)
	jwtStatus, jwtBody, jwtRaw := readJSON(t, okJWT)
	require.Equal(t, http.StatusOK, jwtStatus, jwtRaw)
	assert.Empty(t, okJWT.Header.Get("WWW-Authenticate"))
	assert.NotContains(t, jwtRaw, assertion)
	verifyFetchedAccess(t, jwtSrv.baseURL, jwtBody["access_token"].(string), jwtAudience, jwtID, accountSubjectForClient(t, jwtID))

	replayForm := publicTokenForm(jwtCode, jwtRedirect, jwtResource, jwtID)
	replayForm.Set("client_assertion_type", tokenAssertionType)
	replayForm.Set("client_assertion", assertion)
	replay := postProcessToken(t, jwtSrv.baseURL, replayForm, nil)
	replayStatus, replayBody, _ := readJSON(t, replay)
	assert.Equal(t, http.StatusUnauthorized, replayStatus)
	assert.Equal(t, oauth.ErrorInvalidClient, replayBody["error"])
	assert.Empty(t, replay.Header.Get("WWW-Authenticate"))
}

func TestProcessTokenConcurrencyReplayBindingsAndUnsupportedGrants(t *testing.T) {
	clientID := uniqueLabel("race")
	artifact := uniqueLabel("race-art")
	srv := startTokenProcessWithArtifact(t, artifact)
	redirect, resource, audience := registerProcessClient(t, clientID)
	code := issueProcessCode(t, srv, clientID, redirect, resource, audience, artifact)
	form := publicTokenForm(code, redirect, resource, clientID)

	const n = 8
	var wg sync.WaitGroup
	statuses := make(chan int, n)
	bodies := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := postProcessToken(t, srv.baseURL, form, nil)
			status, _, raw := readJSON(t, resp)
			statuses <- status
			bodies <- raw
		}()
	}
	wg.Wait()
	close(statuses)
	close(bodies)
	ok := 0
	for status := range statuses {
		if status == http.StatusOK {
			ok++
			continue
		}
		assert.Equal(t, http.StatusBadRequest, status)
	}
	require.Equal(t, 1, ok)
	for body := range bodies {
		assert.NotContains(t, body, code)
	}

	replay := postProcessToken(t, srv.baseURL, form, nil)
	replayStatus, replayBody, _ := readJSON(t, replay)
	assert.Equal(t, http.StatusBadRequest, replayStatus)
	assert.Equal(t, oauth.ErrorInvalidGrant, replayBody["error"])

	freshID := uniqueLabel("bind")
	freshArt := uniqueLabel("bind-art")
	freshSrv := startTokenProcessWithArtifact(t, freshArt)
	freshRedirect, freshResource, freshAudience := registerProcessClient(t, freshID)
	freshCode := issueProcessCode(t, freshSrv, freshID, freshRedirect, freshResource, freshAudience, freshArt)
	wrongPKCE := publicTokenForm(freshCode, freshRedirect, freshResource, freshID)
	wrongPKCE.Set("code_verifier", strings.Repeat("b", 43))
	denied := postProcessToken(t, freshSrv.baseURL, wrongPKCE, nil)
	deniedStatus, deniedBody, _ := readJSON(t, denied)
	assert.Equal(t, http.StatusBadRequest, deniedStatus)
	assert.Equal(t, oauth.ErrorInvalidGrant, deniedBody["error"])

	refresh := url.Values{"grant_type": {oauth.GrantRefreshToken}, "refresh_token": {"opaque"}, "client_id": {freshID}, "resource": {freshResource}}
	refreshResp := postProcessToken(t, freshSrv.baseURL, refresh, nil)
	refreshStatus, refreshBody, _ := readJSON(t, refreshResp)
	assert.Equal(t, http.StatusBadRequest, refreshStatus)
	assert.Equal(t, oauth.ErrorInvalidGrant, refreshBody["error"])

	creds := url.Values{"grant_type": {oauth.GrantClientCredentials}, "client_id": {freshID}, "resource": {freshResource}}
	credsResp := postProcessToken(t, freshSrv.baseURL, creds, nil)
	credsStatus, credsBody, _ := readJSON(t, credsResp)
	assert.Equal(t, http.StatusBadRequest, credsStatus)
	assert.Equal(t, oauth.ErrorInvalidRequest, credsBody["error"])
}

func TestProcessTokenMetadataPublishesRevokeAndReadyRequiresKeyBootstrap(t *testing.T) {
	srv := startTokenProcess(t)
	metaResp, err := noFollowClient().Get(srv.baseURL + "/.well-known/oauth-authorization-server")
	require.NoError(t, err)
	_, meta, raw := readJSON(t, metaResp)
	require.Equal(t, http.StatusOK, metaResp.StatusCode)
	assert.Equal(t, tokenProcessIssuer+"/oauth/token", meta["token_endpoint"])
	assert.Equal(t, tokenProcessIssuer+"/oauth/revoke", meta["revocation_endpoint"])
	assert.Equal(t, []any{"authorization_code", "refresh_token", "client_credentials"}, meta["grant_types_supported"])
	assert.Equal(t, []any{"none", "client_secret_basic", "private_key_jwt"}, meta["revocation_endpoint_auth_methods_supported"])
	assert.Equal(t, []any{"ES256"}, meta["revocation_endpoint_auth_signing_alg_values_supported"])
	assert.Equal(t, "public,max-age=300", metaResp.Header.Get("Cache-Control"))
	assert.Contains(t, raw, "/oauth/revoke")
	assert.Contains(t, raw, "client_credentials")
	assert.Contains(t, raw, `"refresh_token"`)

	revoke, err := noFollowClient().Get(srv.baseURL + "/oauth/revoke")
	require.NoError(t, err)
	_ = revoke.Body.Close()
	assert.True(t, revoke.StatusCode == http.StatusMethodNotAllowed || revoke.StatusCode == http.StatusNotFound, revoke.StatusCode)
	assert.NotEqual(t, http.StatusOK, revoke.StatusCode)

	ready, err := noFollowClient().Get(srv.baseURL + "/readyz")
	require.NoError(t, err)
	_ = ready.Body.Close()
	assert.Equal(t, http.StatusOK, ready.StatusCode)
	var actives int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `SELECT count(*) FROM signing_keys WHERE status='active'`).Scan(&actives))
	assert.GreaterOrEqual(t, actives, 1)

	assert.NotContains(t, strings.ToLower(srv.logs.String()), "live stytch")
	assert.NotContains(t, srv.logs.String(), e2eSecret)
}

// A real app.Run request proves the out-of-band projection is available to the
// process harness without parsing a response body or any process log buffer.
func TestProcessCallbackFailureAttributionIsPrivate(t *testing.T) {
	artifact := uniqueLabel("diagnostic-artifact")
	srv := startTokenProcessWithArtifact(t, artifact)
	requestID := uuid.NewString()
	capture, release := api.CaptureBrokerCallbackForTest(strings.TrimPrefix(srv.baseURL, "http://"), requestID)
	defer release()
	cookie := uniqueLabel("unbound-cookie")
	req, err := http.NewRequest(http.MethodGet, srv.baseURL+"/broker/stytch/callback?token="+url.QueryEscape(artifact)+"&type=discovery_magic_link", nil)
	require.NoError(t, err)
	req.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	req.Header.Set("X-Request-ID", requestID)
	response, err := noFollowClient().Do(req)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	d, ok := capture.Snapshot()
	require.True(t, ok)
	require.Equal(t, "binding", d.Stage())
	require.Equal(t, "not_found", d.Class())
	for _, private := range []string{cookie, artifact, requestID, srv.baseURL} {
		if strings.Contains(d.String(), private) {
			t.Fatal("unsafe callback attribution")
		}
	}
	require.Empty(t, response.Header.Get("Location"))
	require.Empty(t, response.Header.Get("Set-Cookie"))
}

func TestProcessTokenHighCountPublicExchanges(t *testing.T) {
	const n = 16
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			clientID := fmt.Sprintf("hi-%d-%s", i, uuid.NewString()[:8])
			artifact := uniqueLabel("hi")
			srv := startTokenProcessWithArtifact(t, artifact)
			redirect, resource, audience := registerProcessClient(t, clientID)
			code := issueProcessCode(t, srv, clientID, redirect, resource, audience, artifact)
			resp := postProcessToken(t, srv.baseURL, publicTokenForm(code, redirect, resource, clientID), nil)
			status, body, raw := readJSON(t, resp)
			if status != http.StatusOK {
				errCh <- fmt.Errorf("http %d body=%s", status, raw)
				return
			}
			if body["access_token"] == "" {
				errCh <- fmt.Errorf("missing access token")
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

func postProcessRevoke(t *testing.T, baseURL string, form url.Values, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/oauth/revoke", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := noFollowClient().Do(req)
	require.NoError(t, err)
	return resp
}

func assertProcessFamilyRevoked(t *testing.T, clientID string) {
	t.Helper()
	var familyStatus, grantStatus string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status, g.status,
       (SELECT count(*) FROM oauth_refresh_tokens tok WHERE tok.family_id=f.id AND tok.consumed_at IS NULL AND tok.revoked_at IS NULL)
FROM oauth_refresh_families f
JOIN oauth_grants g ON g.id=f.grant_id
JOIN oauth_clients c ON c.id=f.oauth_client_id
WHERE c.client_id=$1`, clientID).Scan(&familyStatus, &grantStatus, &live))
	require.Equal(t, "revoked", familyStatus)
	require.Equal(t, "revoked", grantStatus)
	require.Zero(t, live)
}

func assertProcessFamilyActive(t *testing.T, clientID string) {
	t.Helper()
	var status string
	var live int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(), `
SELECT f.status,
       (SELECT count(*) FROM oauth_refresh_tokens tok WHERE tok.family_id=f.id AND tok.consumed_at IS NULL AND tok.revoked_at IS NULL)
FROM oauth_refresh_families f
JOIN oauth_clients c ON c.id=f.oauth_client_id
WHERE c.client_id=$1`, clientID).Scan(&status, &live))
	require.Equal(t, "active", status)
	require.Equal(t, 1, live)
}

func TestProcessPublicBasicAndPrivateRevokeAfterIssue(t *testing.T) {
	publicID := uniqueLabel("rev-pub")
	artifact := uniqueLabel("rev-pub-art")
	srv := startTokenProcessWithArtifact(t, artifact)
	redirect, resource, audience := registerProcessClient(t, publicID)
	code := issueProcessCode(t, srv, publicID, redirect, resource, audience, artifact)
	issued := postProcessToken(t, srv.baseURL, publicTokenForm(code, redirect, resource, publicID), nil)
	status, body, raw := readJSON(t, issued)
	require.Equal(t, http.StatusOK, status, raw)
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)

	ok := postProcessRevoke(t, srv.baseURL, url.Values{"token": {refresh}, "client_id": {publicID}}, nil)
	okStatus, _, okRaw := readJSON(t, ok)
	require.Equal(t, http.StatusOK, okStatus, okRaw)
	assert.Empty(t, strings.TrimSpace(okRaw))
	assert.Empty(t, ok.Header.Get("Access-Control-Allow-Origin"))
	assert.Empty(t, ok.Header.Get("WWW-Authenticate"))
	assertProcessFamilyRevoked(t, publicID)
	verifyFetchedAccess(t, srv.baseURL, access, audience, publicID, accountSubjectForClient(t, publicID))

	again := postProcessRevoke(t, srv.baseURL, url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {publicID}}, nil)
	againStatus, _, againRaw := readJSON(t, again)
	require.Equal(t, http.StatusOK, againStatus, againRaw)
	assert.Empty(t, strings.TrimSpace(againRaw))

	secret := "basic-secret-" + uuid.NewString()
	basicID := uniqueLabel("rev-basic")
	basicArtifact := uniqueLabel("rev-basic-art")
	basicSrv := startTokenProcessWithArtifact(t, basicArtifact)
	basicRedirect, basicResource, basicAudience := registerConfidentialBasic(t, basicID, secret)
	basicCode := issueProcessCode(t, basicSrv, basicID, basicRedirect, basicResource, basicAudience, basicArtifact)
	basicIssued := postProcessToken(t, basicSrv.baseURL, publicTokenForm(basicCode, basicRedirect, basicResource, ""), map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(basicID+":"+secret)),
	})
	basicStatus, basicBody, basicRaw := readJSON(t, basicIssued)
	require.Equal(t, http.StatusOK, basicStatus, basicRaw)
	basicRefresh, _ := basicBody["refresh_token"].(string)
	require.NotEmpty(t, basicRefresh)

	wrong := postProcessRevoke(t, basicSrv.baseURL, url.Values{"token": {basicRefresh}}, map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(basicID+":wrong-secret-value-xxxxxxxxxxxxxxxx")),
	})
	wrongStatus, wrongBody, wrongRaw := readJSON(t, wrong)
	assert.Equal(t, http.StatusUnauthorized, wrongStatus)
	assert.Equal(t, oauth.ErrorInvalidClient, wrongBody["error"])
	assert.Equal(t, `Basic realm="oauth/revoke", charset="UTF-8"`, wrong.Header.Get("WWW-Authenticate"))
	assert.NotContains(t, wrongRaw, secret)
	assertProcessFamilyActive(t, basicID)

	okBasic := postProcessRevoke(t, basicSrv.baseURL, url.Values{"token": {basicRefresh}}, map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(basicID+":"+secret)),
	})
	okBasicStatus, _, okBasicRaw := readJSON(t, okBasic)
	require.Equal(t, http.StatusOK, okBasicStatus, okBasicRaw)
	assert.Empty(t, okBasic.Header.Get("WWW-Authenticate"))
	assertProcessFamilyRevoked(t, basicID)

	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	jwtID := uniqueLabel("rev-jwt")
	jwtArtifact := uniqueLabel("rev-jwt-art")
	jwtSrv := startTokenProcessWithArtifact(t, jwtArtifact)
	jwtRedirect, jwtResource, jwtAudience := registerPrivateKeyJWT(t, jwtID, mat)
	jwtCode := issueProcessCode(t, jwtSrv, jwtID, jwtRedirect, jwtResource, jwtAudience, jwtArtifact)
	tokenAssertion := mintProcessAssertion(t, mat, jwtID, tokenProcessIssuer+"/oauth/token")
	jwtForm := publicTokenForm(jwtCode, jwtRedirect, jwtResource, jwtID)
	jwtForm.Set("client_assertion_type", tokenAssertionType)
	jwtForm.Set("client_assertion", tokenAssertion)
	jwtIssued := postProcessToken(t, jwtSrv.baseURL, jwtForm, nil)
	jwtStatus, jwtBody, jwtRaw := readJSON(t, jwtIssued)
	require.Equal(t, http.StatusOK, jwtStatus, jwtRaw)
	jwtRefresh, _ := jwtBody["refresh_token"].(string)
	require.NotEmpty(t, jwtRefresh)

	replay := url.Values{"token": {jwtRefresh}, "client_id": {jwtID}, "client_assertion_type": {tokenAssertionType}, "client_assertion": {tokenAssertion}}
	denied := postProcessRevoke(t, jwtSrv.baseURL, replay, nil)
	deniedStatus, deniedBody, deniedRaw := readJSON(t, denied)
	assert.Equal(t, http.StatusUnauthorized, deniedStatus)
	assert.Equal(t, oauth.ErrorInvalidClient, deniedBody["error"])
	assert.Empty(t, denied.Header.Get("WWW-Authenticate"))
	assert.NotContains(t, deniedRaw, tokenAssertion)
	assertProcessFamilyActive(t, jwtID)

	revokeAssertion := mintProcessAssertion(t, mat, jwtID, tokenProcessIssuer+"/oauth/revoke")
	okJWT := postProcessRevoke(t, jwtSrv.baseURL, url.Values{
		"token": {jwtRefresh}, "client_id": {jwtID},
		"client_assertion_type": {tokenAssertionType}, "client_assertion": {revokeAssertion},
	}, nil)
	okJWTStatus, _, okJWTRaw := readJSON(t, okJWT)
	require.Equal(t, http.StatusOK, okJWTStatus, okJWTRaw)
	assert.Empty(t, okJWT.Header.Get("WWW-Authenticate"))
	assert.NotContains(t, okJWTRaw, revokeAssertion)
	assertProcessFamilyRevoked(t, jwtID)
	verifyFetchedAccess(t, jwtSrv.baseURL, jwtBody["access_token"].(string), jwtAudience, jwtID, accountSubjectForClient(t, jwtID))
}

func TestProcessRevokeIsOracleFreeAndRejectsQueryDuplicates(t *testing.T) {
	ownerID := uniqueLabel("rev-owner")
	otherID := uniqueLabel("rev-other")
	artifact := uniqueLabel("rev-oracle")
	srv := startTokenProcessWithArtifact(t, artifact)
	ownerRedirect, ownerResource, ownerAudience := registerProcessClient(t, ownerID)
	otherRedirect, otherResource, otherAudience := registerProcessClient(t, otherID)
	ownerCode := issueProcessCode(t, srv, ownerID, ownerRedirect, ownerResource, ownerAudience, artifact)
	issued := postProcessToken(t, srv.baseURL, publicTokenForm(ownerCode, ownerRedirect, ownerResource, ownerID), nil)
	status, body, raw := readJSON(t, issued)
	require.Equal(t, http.StatusOK, status, raw)
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)

	unknown := postProcessRevoke(t, srv.baseURL, url.Values{"token": {"not-a-refresh"}, "client_id": {ownerID}}, nil)
	unknownStatus, _, unknownRaw := readJSON(t, unknown)
	require.Equal(t, http.StatusOK, unknownStatus, unknownRaw)
	assert.Empty(t, strings.TrimSpace(unknownRaw))

	cross := postProcessRevoke(t, srv.baseURL, url.Values{"token": {refresh}, "client_id": {otherID}}, nil)
	crossStatus, _, _ := readJSON(t, cross)
	require.Equal(t, http.StatusOK, crossStatus)
	assertProcessFamilyActive(t, ownerID)
	_ = otherRedirect
	_ = otherResource
	_ = otherAudience

	accessResp := postProcessRevoke(t, srv.baseURL, url.Values{"token": {access}, "token_type_hint": {"access_token"}, "client_id": {ownerID}}, nil)
	accessStatus, _, _ := readJSON(t, accessResp)
	require.Equal(t, http.StatusOK, accessStatus)
	assertProcessFamilyActive(t, ownerID)

	form := url.Values{"token": {refresh}, "client_id": {ownerID}}
	req, err := http.NewRequest(http.MethodPost, srv.baseURL+"/oauth/revoke?token=x", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	queryResp, err := noFollowClient().Do(req)
	require.NoError(t, err)
	queryStatus, queryBody, _ := readJSON(t, queryResp)
	assert.Equal(t, http.StatusBadRequest, queryStatus)
	assert.Equal(t, oauth.ErrorInvalidRequest, queryBody["error"])
	assertProcessFamilyActive(t, ownerID)
}

func TestProcessConcurrentRevokeAndRestartStayIdempotent(t *testing.T) {
	clientID := uniqueLabel("rev-conc")
	artifact := uniqueLabel("rev-conc-art")
	srv := startTokenProcessWithArtifact(t, artifact)
	redirect, resource, audience := registerProcessClient(t, clientID)
	code := issueProcessCode(t, srv, clientID, redirect, resource, audience, artifact)
	issued := postProcessToken(t, srv.baseURL, publicTokenForm(code, redirect, resource, clientID), nil)
	status, body, raw := readJSON(t, issued)
	require.Equal(t, http.StatusOK, status, raw)
	refresh, _ := body["refresh_token"].(string)
	require.NotEmpty(t, refresh)

	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := postProcessRevoke(t, srv.baseURL, url.Values{"token": {refresh}, "client_id": {clientID}}, nil)
			got, _, bodyRaw := readJSON(t, resp)
			if got != http.StatusOK {
				errCh <- fmt.Errorf("http %d body=%s", got, bodyRaw)
				return
			}
			if strings.TrimSpace(bodyRaw) != "" {
				errCh <- fmt.Errorf("non-empty revoke body %q", bodyRaw)
				return
			}
			errCh <- nil
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	assertProcessFamilyRevoked(t, clientID)

	again := postProcessRevoke(t, srv.baseURL, url.Values{"token": {refresh}, "client_id": {clientID}}, nil)
	againStatus, _, againRaw := readJSON(t, again)
	require.Equal(t, http.StatusOK, againStatus, againRaw)
	assert.Empty(t, strings.TrimSpace(againRaw))
}
