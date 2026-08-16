package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

const (
	testOrigin = "https://id.example"
	testIssuer = "https://id.example"
	pkce       = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
)

func brokerSecrets(t *testing.T) config.BrokerSecretSet {
	t.Helper()
	mk := func(b byte) map[int][]byte {
		raw := make([]byte, 32)
		for i := range raw {
			raw[i] = b
		}
		return map[int][]byte{1: raw}
	}
	return config.BrokerSecretSet{
		StateSealKeys: mk(0x11), StateSealActiveVersion: 1,
		StateHashPeppers: mk(0x22), StateHashActiveVersion: 1,
		BrokerCookiePeppers: mk(0x33), BrokerCookieActiveVersion: 1,
		AuthorizationCodePeppers: mk(0x44), AuthorizationCodeActiveVersion: 1,
	}
}

func uniqueLabel(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func uniqueTuple() (project, org, member, session string) {
	suffix := uuid.NewString()[:8]
	return "project-" + suffix, "organization-" + suffix, "member-" + suffix, "member-session-" + suffix
}

func newBrokerService(t *testing.T, provider brokerprovider.Provider) *broker.Service {
	t.Helper()
	svc, err := broker.NewService(broker.ServiceConfig{
		Pool: testutil.DB(t), Secrets: brokerSecrets(t), Provider: provider, Issuer: testIssuer,
	})
	require.NoError(t, err)
	return svc
}

func registerHTTPClient(t *testing.T, clientID string) (redirect, resource, audience string) {
	t.Helper()
	pool := testutil.DB(t)
	client, err := repo.CreateOAuthClient(context.Background(), pool, domain.OAuthClient{
		ClientID: clientID, Name: "IB1 HTTP client", ClientType: "public",
		TokenEndpointAuthMethod: "none", AllowedGrants: []string{"authorization_code"},
		Enabled: true,
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

func scripted(t *testing.T, fixtures ...brokerprovider.Fixture) *brokerprovider.ScriptedProvider {
	t.Helper()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{Fixtures: fixtures})
	require.NoError(t, err)
	return p
}

func successFixture(artifact string, method brokerprovider.Method) brokerprovider.Fixture {
	project, org, member, session := uniqueTuple()
	return brokerprovider.Fixture{
		Artifact: artifact, Method: method, Outcome: brokerprovider.OutcomeAuthenticated,
		ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func newBrokerAPI(t *testing.T, svc *broker.Service) (http.Handler, huma.API) {
	t.Helper()
	humaAPI, handler := api.New(testutil.DB(t), api.Options{
		Broker: svc,
		BrokerHTTP: api.BrokerHTTPOptions{
			AllowedOrigin:      testOrigin,
			InsecureTestCookie: true,
		},
	})
	return handler, humaAPI
}

func authorizeQuery(clientID, redirect, resource, audience, state string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"resource":              {resource},
		"audience":              {audience},
		"scope":                 {"openid"},
		"state":                 {state},
		"code_challenge":        {pkce},
		"code_challenge_method": {"S256"},
	}
}

func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func readBody(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	body, err := io.ReadAll(rr.Result().Body)
	require.NoError(t, err)
	return string(body)
}

func cookieValue(rr *httptest.ResponseRecorder) string {
	for _, c := range rr.Result().Cookies() {
		if c.Name == broker.BrokerCookieName {
			return c.Value
		}
	}
	// httptest may not parse __Host- cookies the same on every path; fall back.
	for _, h := range rr.Result().Header.Values("Set-Cookie") {
		if strings.HasPrefix(h, broker.BrokerCookieName+"=") {
			part := strings.SplitN(h, ";", 2)[0]
			return strings.TrimPrefix(part, broker.BrokerCookieName+"=")
		}
	}
	return ""
}

func setCookieHeader(value string) string {
	return broker.BrokerCookieName + "=" + value
}

func assertBrokerSecurityHeaders(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-referrer", rr.Header().Get("Referrer-Policy"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Credentials"))
}

func assertHasCSP(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	csp := rr.Header().Get("Content-Security-Policy")
	require.NotEmpty(t, csp)
	assert.NotContains(t, csp, "*")
	assert.NotContains(t, strings.ToLower(csp), "unsafe-eval")
}

func doAuthorize(t *testing.T, handler http.Handler, q url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	return serve(handler, req)
}

func TestAuthorizeSetsBrokerCookieAndRedirectsToLogin(t *testing.T) {
	artifact := uniqueLabel("http-authz")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("http")
	redirect, resource, audience := registerHTTPClient(t, clientID)

	rr := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("state")))
	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/broker/stytch/login", rr.Header().Get("Location"))
	assertBrokerSecurityHeaders(t, rr)

	raw := strings.Join(rr.Result().Header.Values("Set-Cookie"), "\n")
	require.Contains(t, raw, broker.BrokerCookieName+"=")
	assert.Contains(t, raw, "HttpOnly")
	assert.Contains(t, raw, "SameSite=Lax")
	assert.Contains(t, raw, "Path=/")
	assert.NotContains(t, raw, "Secure") // InsecureTestCookie
	require.NotEmpty(t, cookieValue(rr))
	assert.LessOrEqual(t, cookieMaxAge(t, raw), 600)
	assert.Greater(t, cookieMaxAge(t, raw), 0)
}

func cookieMaxAge(t *testing.T, header string) int {
	t.Helper()
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "max-age=") {
			var n int
			_, err := fmt.Sscanf(part[8:], "%d", &n)
			require.NoError(t, err)
			return n
		}
	}
	t.Fatalf("max-age missing from %q", header)
	return 0
}

func TestAuthorizeRejectsDuplicateUnknownOpenRedirectControlAndOversize(t *testing.T) {
	artifact := uniqueLabel("http-reject")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("rej")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	base := authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("state"))

	cases := []struct {
		name string
		raw  string
	}{
		{"duplicate client_id", "/oauth/authorize?" + base.Encode() + "&client_id=other"},
		{"unknown parameter", "/oauth/authorize?" + base.Encode() + "&foo=bar"},
		{"open redirect", "/oauth/authorize?" + authorizeQuery(clientID, "https://evil.example/callback", resource, audience, uniqueLabel("st")).Encode()},
		{"control character", "/oauth/authorize?" + strings.ReplaceAll(base.Encode(), "openid", "open%00id")},
		{"oversize state", "/oauth/authorize?" + authorizeQuery(clientID, redirect, resource, audience, strings.Repeat("s", 1025)).Encode()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.raw, nil)
			rr := serve(handler, req)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assertBrokerSecurityHeaders(t, rr)
			assertHasCSP(t, rr)
			body := readBody(t, rr)
			assert.NotContains(t, body, "evil.example")
			assert.NotContains(t, strings.ToLower(body), "https://"+clientID)
			assert.NotEqual(t, "/broker/stytch/login", rr.Header().Get("Location"))
			assert.NotContains(t, rr.Header().Get("Location"), "evil.example")
			assert.Empty(t, cookieValue(rr))
		})
	}
}

func TestProductionCookieIsHostPrefixedSecureAndInsecureTestCookieRejected(t *testing.T) {
	artifact := uniqueLabel("http-secure")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	assert.Panics(t, func() {
		api.New(testutil.DB(t), api.Options{
			Broker: svc,
			BrokerHTTP: api.BrokerHTTPOptions{
				AllowedOrigin:      testOrigin,
				InsecureTestCookie: true,
				Production:         true,
			},
		})
	})

	_, handler := api.New(testutil.DB(t), api.Options{
		Broker: svc,
		BrokerHTTP: api.BrokerHTTPOptions{
			AllowedOrigin: testOrigin,
			Production:    true,
		},
	})
	clientID := uniqueLabel("sec")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	rr := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("state")))
	assert.Equal(t, http.StatusSeeOther, rr.Code)
	raw := strings.Join(rr.Result().Header.Values("Set-Cookie"), "\n")
	assert.Contains(t, raw, "Secure")
	assert.Contains(t, raw, broker.BrokerCookieName+"=")
}

func TestLoginRequiresBrokerCookieAndReturnsSafeDiscovery(t *testing.T) {
	artifact := uniqueLabel("http-login")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("login")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("state")))
	require.Equal(t, http.StatusSeeOther, started.Code)
	cookie := cookieValue(started)
	require.NotEmpty(t, cookie)

	missing := serve(handler, httptest.NewRequest(http.MethodGet, "/broker/stytch/login", nil))
	assert.Equal(t, http.StatusBadRequest, missing.Code)
	assertBrokerSecurityHeaders(t, missing)
	assertHasCSP(t, missing)
	assert.NotContains(t, readBody(t, missing), cookie)

	req := httptest.NewRequest(http.MethodGet, "/broker/stytch/login", nil)
	req.Header.Set("Cookie", setCookieHeader(cookie))
	req.Header.Set("Accept", "application/json")
	ok := serve(handler, req)
	assert.Equal(t, http.StatusOK, ok.Code)
	assertBrokerSecurityHeaders(t, ok)
	assertHasCSP(t, ok)
	body := readBody(t, ok)
	assert.NotContains(t, body, "stytch")
	assert.NotContains(t, strings.ToLower(body), "secret")
	assert.NotContains(t, strings.ToLower(body), "jwt")
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	assert.NotEmpty(t, payload["csrf"])

	htmlReq := httptest.NewRequest(http.MethodGet, "/broker/stytch/login", nil)
	htmlReq.Header.Set("Cookie", setCookieHeader(cookie))
	htmlReq.Header.Set("Accept", "text/html")
	html := serve(handler, htmlReq)
	assert.Equal(t, http.StatusOK, html.Code)
	assertHasCSP(t, html)
	htmlBody := readBody(t, html)
	assert.Contains(t, htmlBody, "csrf")
	assert.NotContains(t, htmlBody, "project-")
}

func TestStartRoutesRejectWrongContentTypeOriginCSRFAndCookie(t *testing.T) {
	artifact := uniqueLabel("http-start-deny")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("start")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("state")))
	cookie := cookieValue(started)
	require.NotEmpty(t, cookie)
	csrf := loginCSRF(t, handler, cookie)

	paths := []string{
		"/broker/stytch/email/start",
		"/broker/stytch/email/verify",
		"/broker/stytch/sso/start",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			form := url.Values{"csrf": {csrf}}
			wrongType := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
			wrongType.Header.Set("Content-Type", "application/json")
			wrongType.Header.Set("Origin", testOrigin)
			wrongType.Header.Set("Cookie", setCookieHeader(cookie))
			rr := serve(handler, wrongType)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assertBrokerSecurityHeaders(t, rr)

			wrongOrigin := postForm(path, form, cookie, "https://evil.example")
			rr = serve(handler, wrongOrigin)
			assert.Equal(t, http.StatusBadRequest, rr.Code)

			wrongCSRF := postForm(path, url.Values{"csrf": {"nope"}}, cookie, testOrigin)
			rr = serve(handler, wrongCSRF)
			assert.Equal(t, http.StatusBadRequest, rr.Code)

			noCookie := postForm(path, form, "", testOrigin)
			rr = serve(handler, noCookie)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func loginCSRF(t *testing.T, handler http.Handler, cookie string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/broker/stytch/login", nil)
	req.Header.Set("Cookie", setCookieHeader(cookie))
	req.Header.Set("Accept", "application/json")
	rr := serve(handler, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(readBody(t, rr)), &payload))
	csrf, _ := payload["csrf"].(string)
	require.NotEmpty(t, csrf)
	return csrf
}

func postForm(path string, form url.Values, cookie, origin string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != "" {
		req.Header.Set("Cookie", setCookieHeader(cookie))
	}
	if csrf := form.Get("csrf"); csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	return req
}

func TestStartRoutesAcceptAllFourScriptedMethods(t *testing.T) {
	cases := []struct {
		path   string
		method brokerprovider.Method
		form   url.Values
	}{
		{"/broker/stytch/email/start", brokerprovider.MethodEmailMagicLink, url.Values{}},
		{"/broker/stytch/email/verify", brokerprovider.MethodEmailOTP, url.Values{}},
		{"/broker/stytch/sso/start", brokerprovider.MethodSSOSAML, url.Values{"type": {"saml"}}},
		{"/broker/stytch/sso/start", brokerprovider.MethodSSOOIDC, url.Values{"type": {"oidc"}}},
	}
	for _, tc := range cases {
		t.Run(string(tc.method), func(t *testing.T) {
			artifact := uniqueLabel("start-" + string(tc.method))
			svc := newBrokerService(t, scripted(t, successFixture(artifact, tc.method)))
			handler, _ := newBrokerAPI(t, svc)
			clientID := uniqueLabel("m")
			redirect, resource, audience := registerHTTPClient(t, clientID)
			started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("state")))
			cookie := cookieValue(started)
			csrf := loginCSRF(t, handler, cookie)
			form := tc.form
			if form == nil {
				form = url.Values{}
			}
			form.Set("csrf", csrf)
			rr := serve(handler, postForm(tc.path, form, cookie, testOrigin))
			assert.Equal(t, http.StatusOK, rr.Code)
			assertBrokerSecurityHeaders(t, rr)
			body := readBody(t, rr)
			assert.NotContains(t, strings.ToLower(body), "jwt")
			assert.NotContains(t, strings.ToLower(body), "session_token")
		})
	}
}

func TestCallbackSuccessRedirectsExactCodeStateIssAndDeletesCookie(t *testing.T) {
	artifact := uniqueLabel("cb-ok")
	state := uniqueLabel("orig-state")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("cb")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, state))
	cookie := cookieValue(started)
	require.NotEmpty(t, cookie)

	req := httptest.NewRequest(http.MethodGet, "/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
	req.Header.Set("Cookie", setCookieHeader(cookie))
	rr := serve(handler, req)
	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assertBrokerSecurityHeaders(t, rr)
	loc := rr.Header().Get("Location")
	parsed, err := url.Parse(loc)
	require.NoError(t, err)
	assert.Equal(t, redirect, parsed.Scheme+"://"+parsed.Host+parsed.Path)
	assert.Equal(t, state, parsed.Query().Get("state"))
	assert.Equal(t, testIssuer, parsed.Query().Get("iss"))
	assert.NotEmpty(t, parsed.Query().Get("code"))
	assert.NotContains(t, loc, artifact)
	assert.NotContains(t, readBody(t, rr), artifact)

	expired := strings.Join(rr.Result().Header.Values("Set-Cookie"), "\n")
	assert.Contains(t, expired, broker.BrokerCookieName+"=")
	assert.True(t, strings.Contains(expired, "Max-Age=0") || strings.Contains(expired, "max-age=0"))
}

func TestCallbackIncompleteMFAStaysLocal(t *testing.T) {
	artifact := uniqueLabel("cb-mfa")
	project, org, member, session := uniqueTuple()
	svc := newBrokerService(t, scripted(t, brokerprovider.Fixture{
		Artifact: artifact, Method: brokerprovider.MethodEmailOTP, Outcome: brokerprovider.OutcomeIncompleteMFA,
		ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("mfa")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("st")))
	cookie := cookieValue(started)
	req := httptest.NewRequest(http.MethodGet, "/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
	req.Header.Set("Cookie", setCookieHeader(cookie))
	rr := serve(handler, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assertBrokerSecurityHeaders(t, rr)
	assertHasCSP(t, rr)
	assert.Empty(t, rr.Header().Get("Location"))
	assert.NotContains(t, readBody(t, rr), artifact)
}

func TestConcurrentCallbackIssuesOneRedirect(t *testing.T) {
	artifact := uniqueLabel("cb-race")
	state := uniqueLabel("race-state")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("race")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, state))
	cookie := cookieValue(started)

	const n = 16
	var redirects atomic.Int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
			req.Header.Set("Cookie", setCookieHeader(cookie))
			rr := serve(handler, req)
			if rr.Code == http.StatusSeeOther {
				redirects.Add(1)
				assert.Contains(t, rr.Header().Get("Location"), "code=")
				assert.Contains(t, rr.Header().Get("Location"), "state="+url.QueryEscape(state))
			} else {
				assert.Equal(t, http.StatusBadRequest, rr.Code)
				assert.Empty(t, rr.Header().Get("Location"))
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), redirects.Load())
}

func TestCallbackProviderTransientIsGeneric503(t *testing.T) {
	artifact := uniqueLabel("cb-503")
	project, org, member, session := uniqueTuple()
	svc := newBrokerService(t, scripted(t, brokerprovider.Fixture{
		Artifact: artifact, Method: brokerprovider.MethodEmailMagicLink, Outcome: brokerprovider.OutcomeUnavailable,
		ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("unavail")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("st")))
	cookie := cookieValue(started)
	req := httptest.NewRequest(http.MethodGet, "/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
	req.Header.Set("Cookie", setCookieHeader(cookie))
	rr := serve(handler, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assertBrokerSecurityHeaders(t, rr)
	assertHasCSP(t, rr)
	body := readBody(t, rr)
	assert.NotContains(t, body, artifact)
	assert.NotContains(t, strings.ToLower(body), "unavailable fixture")
	assert.Empty(t, rr.Header().Get("Location"))
}

func TestCallbackRejectsDuplicateOrMissingArtifact(t *testing.T) {
	artifact := uniqueLabel("cb-bad")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("badcb")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("st")))
	cookie := cookieValue(started)

	cases := []string{
		"/broker/stytch/callback",
		"/broker/stytch/callback?token=" + url.QueryEscape(artifact) + "&token=other",
		"/broker/stytch/callback?token=" + url.QueryEscape(artifact) + "&sso_token=other",
		"/broker/stytch/callback?token=" + strings.Repeat("a", 4097),
	}
	for _, raw := range cases {
		req := httptest.NewRequest(http.MethodGet, raw, nil)
		req.Header.Set("Cookie", setCookieHeader(cookie))
		rr := serve(handler, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, raw)
		assert.NotContains(t, readBody(t, rr), artifact)
	}
}

func TestOpenAPIInventoryForbiddenSurfacesAndDeterminism(t *testing.T) {
	artifact := uniqueLabel("spec")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	humaAPI, _ := api.New(testutil.DB(t), api.Options{
		Broker: svc,
		BrokerHTTP: api.BrokerHTTPOptions{
			AllowedOrigin:      testOrigin,
			InsecureTestCookie: true,
		},
	})

	first, err := json.Marshal(humaAPI.OpenAPI())
	require.NoError(t, err)
	second, err := json.Marshal(humaAPI.OpenAPI())
	require.NoError(t, err)
	assert.Equal(t, first, second)

	var spec map[string]any
	require.NoError(t, json.Unmarshal(first, &spec))
	paths, _ := spec["paths"].(map[string]any)
	require.NotNil(t, paths)
	for _, p := range []string{
		"/oauth/authorize",
		"/broker/stytch/login",
		"/broker/stytch/email/start",
		"/broker/stytch/email/verify",
		"/broker/stytch/sso/start",
		"/broker/stytch/callback",
	} {
		assert.Contains(t, paths, p)
	}
	assert.NotContains(t, paths, "/oauth/token")
	assert.NotContains(t, paths, "/.well-known/jwks.json")
	assert.NotContains(t, paths, "/jwks")
	assert.NotContains(t, paths, "/oauth/jwks")

	raw := string(first)
	assert.NotContains(t, strings.ToLower(raw), "session_jwt")
	assert.NotContains(t, strings.ToLower(raw), "session_token")
	assert.NotContains(t, strings.ToLower(raw), "provider_payload")
	assert.NotContains(t, strings.ToLower(raw), "id_token")
	assert.NotContains(t, raw, "access_token")
	assert.NotContains(t, raw, "refresh_token")

	assertCallbackArtifactsWriteOnly(t, spec)
}

func assertCallbackArtifactsWriteOnly(t *testing.T, spec map[string]any) {
	t.Helper()
	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	var walk func(v any)
	found := false
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if props, ok := n["properties"].(map[string]any); ok {
				for name, prop := range props {
					if name == "token" || name == "sso_token" {
						pm, _ := prop.(map[string]any)
						if pm["writeOnly"] == true {
							found = true
						}
					}
				}
			}
			if n["writeOnly"] == true {
				if _, ok := n["type"]; ok {
					found = found || true
				}
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(spec)
	// Parameters may live under paths rather than schemas.
	if !found && (bytes.Contains(raw, []byte(`"writeOnly":true`)) || bytes.Contains(raw, []byte(`"writeOnly": true`))) {
		found = true
	}
	assert.True(t, found, "callback artifact inputs must be marked writeOnly in OpenAPI")
}

func TestStartRejectsOversizeBody(t *testing.T) {
	artifact := uniqueLabel("big-body")
	svc := newBrokerService(t, scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	clientID := uniqueLabel("big")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	started := doAuthorize(t, handler, authorizeQuery(clientID, redirect, resource, audience, uniqueLabel("st")))
	cookie := cookieValue(started)
	csrf := loginCSRF(t, handler, cookie)
	body := "csrf=" + url.QueryEscape(csrf) + "&pad=" + strings.Repeat("x", 20_000)
	req := httptest.NewRequest(http.MethodPost, "/broker/stytch/email/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Cookie", setCookieHeader(cookie))
	req.Header.Set("X-CSRF-Token", csrf)
	rr := serve(handler, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
