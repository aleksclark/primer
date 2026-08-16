package stytch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/discovery/intermediatesessions"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/magiclinks"
	magicdiscovery "github.com/stytchauth/stytch-go/v18/stytch/b2b/magiclinks/discovery"
	emaildiscovery "github.com/stytchauth/stytch-go/v18/stytch/b2b/magiclinks/email/discovery"
	otpemail "github.com/stytchauth/stytch-go/v18/stytch/b2b/otp/email"
	emaildiscoveryotp "github.com/stytchauth/stytch-go/v18/stytch/b2b/otp/email/discovery"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/sessions"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/sso"

	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/stytch"
)

const (
	brokerEmail     = "member@school.example"
	brokerToken     = "one-time-callback-artifact-must-not-leak"
	brokerSSOToken  = "one-time-sso-artifact-must-not-leak"
	brokerOTP       = "123456"
	brokerOrg       = "organization-test-example"
	brokerMember    = "member-test-example"
	brokerSession   = "member-session-test-example"
	brokerConn      = "saml-connection-test-example"
	brokerPublicTok = "public-token-test-example"
	brokerRedirect  = "https://identity.example/broker/stytch/callback"
)

func brokerCfg(baseURI string) stytch.BrokerConfig {
	return stytch.BrokerConfig{
		Stytch:               adapterConfig(baseURI),
		DiscoveryRedirectURL: brokerRedirect,
		LoginRedirectURL:     brokerRedirect,
		SignupRedirectURL:    brokerRedirect,
		PublicToken:          brokerPublicTok,
	}
}

func TestPinnedBrokerSDKSurfaces(t *testing.T) {
	t.Parallel()

	magicDiscSend := reflect.TypeOf(emaildiscovery.SendParams{})
	requireField(t, magicDiscSend, "EmailAddress", "email_address")
	requireField(t, magicDiscSend, "DiscoveryRedirectURL", "discovery_redirect_url")

	magicDiscAuth := reflect.TypeOf(magicdiscovery.AuthenticateParams{})
	require.Equal(t, 2, magicDiscAuth.NumField())
	requireField(t, magicDiscAuth, "DiscoveryMagicLinksToken", "discovery_magic_links_token")
	requireField(t, magicDiscAuth, "PkceCodeVerifier", "pkce_code_verifier")

	magicAuth := reflect.TypeOf(magiclinks.AuthenticateParams{})
	requireField(t, magicAuth, "MagicLinksToken", "magic_links_token")
	requireField(t, magicAuth, "SessionDurationMinutes", "session_duration_minutes")

	otpSend := reflect.TypeOf(emaildiscoveryotp.SendParams{})
	requireField(t, otpSend, "EmailAddress", "email_address")

	otpDiscAuth := reflect.TypeOf(emaildiscoveryotp.AuthenticateParams{})
	require.Equal(t, 2, otpDiscAuth.NumField())
	requireField(t, otpDiscAuth, "EmailAddress", "email_address")
	requireField(t, otpDiscAuth, "Code", "code")

	otpAuth := reflect.TypeOf(otpemail.AuthenticateParams{})
	requireField(t, otpAuth, "OrganizationID", "organization_id")
	requireField(t, otpAuth, "EmailAddress", "email_address")
	requireField(t, otpAuth, "Code", "code")
	requireField(t, otpAuth, "SessionDurationMinutes", "session_duration_minutes")

	ssoAuth := reflect.TypeOf(sso.AuthenticateParams{})
	requireField(t, ssoAuth, "SSOToken", "sso_token")
	requireField(t, ssoAuth, "SessionDurationMinutes", "session_duration_minutes")

	exchange := reflect.TypeOf(intermediatesessions.ExchangeParams{})
	requireField(t, exchange, "IntermediateSessionToken", "intermediate_session_token")
	requireField(t, exchange, "OrganizationID", "organization_id")
	requireField(t, exchange, "SessionDurationMinutes", "session_duration_minutes")

	sessAuth := reflect.TypeOf(sessions.AuthenticateParams{})
	requireField(t, sessAuth, "SessionToken", "session_token")
	requireField(t, sessAuth, "SessionDurationMinutes", "session_duration_minutes")

	// v18.1.0 has no server-side SSO Start method; public start is URL-only.
	ssoClient := reflect.TypeOf(sso.AuthenticateParams{})
	_, hasStart := ssoClient.FieldByName("ConnectionID")
	require.False(t, hasStart)
}

func requireField(t *testing.T, typ reflect.Type, name, jsonName string) {
	t.Helper()
	field, ok := typ.FieldByName(name)
	require.True(t, ok, "%s.%s", typ.Name(), name)
	tag := field.Tag.Get("json")
	assert.True(t, strings.HasPrefix(tag, jsonName), "%s.%s json=%q", typ.Name(), name, tag)
}

func TestNewBrokerRejectsNonHTTPSRedirectsAndLiveOverride(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*stytch.BrokerConfig)
	}{
		{"http discovery", func(c *stytch.BrokerConfig) { c.DiscoveryRedirectURL = "http://identity.example/callback" }},
		{"query login", func(c *stytch.BrokerConfig) { c.LoginRedirectURL = brokerRedirect + "?x=1" }},
		{"fragment signup", func(c *stytch.BrokerConfig) { c.SignupRedirectURL = brokerRedirect + "#x" }},
		{"userinfo", func(c *stytch.BrokerConfig) { c.DiscoveryRedirectURL = "https://user:pass@identity.example/callback" }},
		{"empty public token", func(c *stytch.BrokerConfig) { c.PublicToken = "" }},
		{"live base override", func(c *stytch.BrokerConfig) {
			c.Stytch.Env = "live"
			c.Stytch.ProjectID = "project-live-example"
			c.Stytch.BaseURI = "https://stytch.test.example"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := brokerCfg("https://stytch.test.example")
			tc.mut(&cfg)
			got, err := stytch.NewBrokerWithHTTPClient(cfg, nil)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), brokerPublicTok)
			assert.NotContains(t, err.Error(), "user:pass")
		})
	}
}

func TestBrokerSatisfiesProviderAndRejectsUntypedCallback(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(server.Close)

	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	var facade brokerprovider.Provider = p
	var typed brokerprovider.TypedProvider = p
	require.NotNil(t, facade)
	require.NotNil(t, typed)

	_, err = p.CompleteCallback(context.Background(), brokerToken)
	assert.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
	assert.Zero(t, requests)
}

func TestStartLoginDiscoveryMagicLinkPostsExactPathAndPinnedRedirect(t *testing.T) {
	t.Parallel()
	var got methodCapture
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.capture(r)
		writeJSON(w, http.StatusOK, map[string]any{"status_code": 200, "request_id": "req-start"})
	}))
	t.Cleanup(server.Close)

	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	start, err := p.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: brokerprovider.MethodEmailMagicLink, EmailAddress: brokerEmail,
	})
	require.NoError(t, err)
	assert.Equal(t, brokerprovider.MethodEmailMagicLink, start.Method)
	assert.NotEmpty(t, start.Handle)
	assert.Empty(t, start.ContinueURL)
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, "/v1/b2b/magic_links/email/discovery/send", got.path)
	assert.Equal(t, brokerEmail, got.body["email_address"])
	assert.Equal(t, brokerRedirect, got.body["discovery_redirect_url"])
	assert.NotContains(t, got.body, "session_duration_minutes")
	assert.NotContains(t, got.body, "login_redirect_url")
}

func TestStartLoginEmailOTPPostsDiscoverySendWithoutEnumeration(t *testing.T) {
	t.Parallel()
	var got methodCapture
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.capture(r)
		writeJSON(w, http.StatusOK, map[string]any{"status_code": 200})
	}))
	t.Cleanup(server.Close)

	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	_, err = p.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: brokerprovider.MethodEmailOTP, EmailAddress: brokerEmail,
	})
	require.NoError(t, err)
	assert.Equal(t, "/v1/b2b/otps/email/discovery/send", got.path)
	assert.Equal(t, brokerEmail, got.body["email_address"])
	assert.NotContains(t, got.body, "session_duration_minutes")
}

func TestStartLoginSSOConstructsOfficialPublicStartURLWithoutHTTP(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(server.Close)

	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	start, err := p.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: brokerprovider.MethodSSOSAML, ConnectionID: brokerConn,
	})
	require.NoError(t, err)
	assert.Zero(t, requests)
	parsed, err := url.Parse(start.ContinueURL)
	require.NoError(t, err)
	assert.Equal(t, "/v1/public/sso/start", parsed.Path)
	assert.Equal(t, brokerConn, parsed.Query().Get("connection_id"))
	assert.Equal(t, brokerPublicTok, parsed.Query().Get("public_token"))
	assert.Equal(t, brokerRedirect, parsed.Query().Get("login_redirect_url"))
	assert.Equal(t, brokerRedirect, parsed.Query().Get("signup_redirect_url"))
	assert.Empty(t, parsed.Query().Get("session_duration_minutes"))

	oidc, err := p.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: brokerprovider.MethodSSOOIDC, ConnectionID: "oidc-connection-test-example",
	})
	require.NoError(t, err)
	assert.Contains(t, oidc.ContinueURL, "oidc-connection-test-example")
	assert.Zero(t, requests)
}

func TestStartLoginRejectsOversizedAndControlInputsBeforeOutbound(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(server.Close)
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)

	cases := []brokerprovider.StartRequest{
		{Method: brokerprovider.MethodEmailMagicLink, EmailAddress: ""},
		{Method: brokerprovider.MethodEmailMagicLink, EmailAddress: "not-an-email"},
		{Method: brokerprovider.MethodEmailOTP, EmailAddress: "bad\n" + brokerEmail},
		{Method: brokerprovider.MethodEmailOTP, EmailAddress: strings.Repeat("a", 255) + "@x.example"},
		{Method: brokerprovider.MethodEmailMagicLink, EmailAddress: "a@x.example\x00"},
		{Method: "password", EmailAddress: brokerEmail},
		{Method: brokerprovider.MethodSSOSAML, ConnectionID: ""},
		{Method: brokerprovider.MethodSSOOIDC, ConnectionID: "conn\nid"},
	}
	for _, req := range cases {
		_, err := p.StartLogin(context.Background(), req)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), brokerEmail)
		if req.EmailAddress != "" {
			assert.NotContains(t, err.Error(), req.EmailAddress)
		}
		if req.ConnectionID != "" {
			assert.NotContains(t, err.Error(), req.ConnectionID)
		}
	}
	assert.Zero(t, requests)
}

func TestCompleteTypedCallbackDiscoveryMagicLinkAuthenticatesThenRevalidates(t *testing.T) {
	t.Parallel()
	var hits []methodCapture
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cap methodCapture
		cap.capture(r)
		mu.Lock()
		hits = append(hits, cap)
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/b2b/magic_links/discovery/authenticate":
			writeJSON(w, http.StatusOK, map[string]any{
				"status_code":                200,
				"intermediate_session_token": "ist-must-not-leak",
				"email_address":              brokerEmail,
				"discovered_organizations": []map[string]any{{
					"member_authenticated": false,
					"organization":         map[string]any{"organization_id": brokerOrg},
					"membership": map[string]any{
						"type":   "active_member",
						"member": map[string]any{"member_id": brokerMember, "organization_id": brokerOrg, "email_address": brokerEmail},
					},
				}},
			})
		case "/v1/b2b/discovery/intermediate_sessions/exchange":
			writeJSON(w, http.StatusOK, authenticatedMemberBody("sess-token-must-not-leak"))
		case "/v1/b2b/sessions/authenticate":
			writeJSON(w, http.StatusOK, sessionAuthenticateBody())
		case "/v1/b2b/sessions":
			writeJSON(w, http.StatusOK, sessionGetBody())
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	got, err := p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeDiscoveryMagicLink, Artifact: brokerToken,
	})
	require.NoError(t, err)
	assert.Equal(t, brokerprovider.OutcomeAuthenticated, got.Outcome)
	assert.Equal(t, brokerprovider.MethodEmailMagicLink, got.Method)
	assert.Equal(t, "project-test-example", got.ProjectID)
	assert.Equal(t, brokerOrg, got.OrganizationID)
	assert.Equal(t, brokerMember, got.MemberID)
	assert.Equal(t, brokerSession, got.MemberSessionID)
	assert.False(t, got.MemberSessionExpiresAt.IsZero())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, hits, 4)
	assert.Equal(t, "/v1/b2b/magic_links/discovery/authenticate", hits[0].path)
	assert.Equal(t, brokerToken, hits[0].body["discovery_magic_links_token"])
	assert.NotContains(t, hits[0].body, "session_duration_minutes")
	assert.Equal(t, "/v1/b2b/discovery/intermediate_sessions/exchange", hits[1].path)
	assert.Equal(t, "ist-must-not-leak", hits[1].body["intermediate_session_token"])
	assert.Equal(t, brokerOrg, hits[1].body["organization_id"])
	assert.NotContains(t, hits[1].body, "session_duration_minutes")
	assert.Equal(t, "/v1/b2b/sessions/authenticate", hits[2].path)
	assert.Equal(t, "sess-token-must-not-leak", hits[2].body["session_token"])
	assert.NotContains(t, hits[2].body, "session_duration_minutes")
	assert.Equal(t, http.MethodGet, hits[3].method)
	assert.Equal(t, "/v1/b2b/sessions", hits[3].path)
	assert.Equal(t, brokerOrg, hits[3].query.Get("organization_id"))
	assert.Equal(t, brokerMember, hits[3].query.Get("member_id"))
	assert.Empty(t, hits[3].query.Get("cursor"))
	assertNoRawFields(t, got)
}

func TestCompleteTypedCallbackOrgMagicLinkAndSSOOmitDurationAndRevalidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		typ    brokerprovider.ArtifactType
		method brokerprovider.Method
		path   string
		tokenK string
		token  string
	}{
		{"org magic", brokerprovider.ArtifactTypeMagicLink, brokerprovider.MethodEmailMagicLink, "/v1/b2b/magic_links/authenticate", "magic_links_token", brokerToken},
		{"sso", brokerprovider.ArtifactTypeSSOToken, brokerprovider.MethodSSOSAML, "/v1/b2b/sso/authenticate", "sso_token", brokerSSOToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var hits []methodCapture
			var mu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var cap methodCapture
				cap.capture(r)
				mu.Lock()
				hits = append(hits, cap)
				mu.Unlock()
				switch r.URL.Path {
				case tc.path:
					writeJSON(w, http.StatusOK, authenticatedMemberBody("sess-token-must-not-leak"))
				case "/v1/b2b/sessions/authenticate":
					writeJSON(w, http.StatusOK, sessionAuthenticateBody())
				case "/v1/b2b/sessions":
					writeJSON(w, http.StatusOK, sessionGetBody())
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
			require.NoError(t, err)
			got, err := p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
				Type: tc.typ, Artifact: tc.token, OrganizationID: brokerOrg,
			})
			require.NoError(t, err)
			assert.Equal(t, brokerprovider.OutcomeAuthenticated, got.Outcome)
			assert.Equal(t, tc.method, got.Method)
			assert.Equal(t, brokerSession, got.MemberSessionID)
			mu.Lock()
			defer mu.Unlock()
			require.GreaterOrEqual(t, len(hits), 3)
			assert.Equal(t, tc.path, hits[0].path)
			assert.Equal(t, tc.token, hits[0].body[tc.tokenK])
			assert.NotContains(t, hits[0].body, "session_duration_minutes")
			assert.NotContains(t, hits[0].body, "session_jwt")
			assert.NotContains(t, hits[1].body, "session_duration_minutes")
			assertNoRawFields(t, got)
		})
	}
}

func TestCompleteTypedCallbackEmailOTPDiscoveryAndOrg(t *testing.T) {
	t.Parallel()
	t.Run("discovery", func(t *testing.T) {
		t.Parallel()
		var hits []methodCapture
		var mu sync.Mutex
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var cap methodCapture
			cap.capture(r)
			mu.Lock()
			hits = append(hits, cap)
			mu.Unlock()
			switch r.URL.Path {
			case "/v1/b2b/otps/email/discovery/authenticate":
				writeJSON(w, http.StatusOK, map[string]any{
					"status_code":                200,
					"intermediate_session_token": "ist-otp",
					"email_address":              brokerEmail,
					"discovered_organizations": []map[string]any{{
						"organization": map[string]any{"organization_id": brokerOrg},
						"membership": map[string]any{
							"type":   "active_member",
							"member": map[string]any{"member_id": brokerMember, "organization_id": brokerOrg},
						},
					}},
				})
			case "/v1/b2b/discovery/intermediate_sessions/exchange":
				writeJSON(w, http.StatusOK, authenticatedMemberBody("sess-token-must-not-leak"))
			case "/v1/b2b/sessions/authenticate":
				writeJSON(w, http.StatusOK, sessionAuthenticateBody())
			case "/v1/b2b/sessions":
				writeJSON(w, http.StatusOK, sessionGetBody())
			default:
				t.Errorf("unexpected path %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)
		p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
		require.NoError(t, err)
		got, err := p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
			Type: brokerprovider.ArtifactTypeDiscoveryEmailOTP, Artifact: brokerOTP, EmailAddress: brokerEmail,
		})
		require.NoError(t, err)
		assert.Equal(t, brokerprovider.OutcomeAuthenticated, got.Outcome)
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "/v1/b2b/otps/email/discovery/authenticate", hits[0].path)
		assert.Equal(t, brokerEmail, hits[0].body["email_address"])
		assert.Equal(t, brokerOTP, hits[0].body["code"])
		assert.NotContains(t, hits[0].body, "session_duration_minutes")
	})
	t.Run("org", func(t *testing.T) {
		t.Parallel()
		var hits []methodCapture
		var mu sync.Mutex
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var cap methodCapture
			cap.capture(r)
			mu.Lock()
			hits = append(hits, cap)
			mu.Unlock()
			switch r.URL.Path {
			case "/v1/b2b/otps/email/authenticate":
				writeJSON(w, http.StatusOK, authenticatedMemberBody("sess-token-must-not-leak"))
			case "/v1/b2b/sessions/authenticate":
				writeJSON(w, http.StatusOK, sessionAuthenticateBody())
			case "/v1/b2b/sessions":
				writeJSON(w, http.StatusOK, sessionGetBody())
			default:
				t.Errorf("unexpected path %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)
		p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
		require.NoError(t, err)
		got, err := p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
			Type: brokerprovider.ArtifactTypeEmailOTP, Artifact: brokerOTP,
			EmailAddress: brokerEmail, OrganizationID: brokerOrg,
		})
		require.NoError(t, err)
		assert.Equal(t, brokerprovider.MethodEmailOTP, got.Method)
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "/v1/b2b/otps/email/authenticate", hits[0].path)
		assert.Equal(t, brokerOrg, hits[0].body["organization_id"])
		assert.Equal(t, brokerEmail, hits[0].body["email_address"])
		assert.Equal(t, brokerOTP, hits[0].body["code"])
		assert.NotContains(t, hits[0].body, "session_duration_minutes")
	})
}

func TestCompleteTypedCallbackIncompleteMFAIsIdentityOnly(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/b2b/sso/authenticate" {
			t.Errorf("MFA continuation must not authenticate a session: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status_code":                200,
			"member_authenticated":       false,
			"member_id":                  brokerMember,
			"organization_id":            brokerOrg,
			"intermediate_session_token": "ist-mfa-must-not-leak",
			"session_token":              "",
			"session_jwt":                "",
			"member":                     map[string]any{"member_id": brokerMember, "organization_id": brokerOrg, "email_address": brokerEmail},
			"organization":               map[string]any{"organization_id": brokerOrg},
		})
	}))
	t.Cleanup(server.Close)
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	got, err := p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeSSOToken, Artifact: brokerSSOToken,
	})
	require.NoError(t, err)
	assert.Equal(t, brokerprovider.OutcomeIncompleteMFA, got.Outcome)
	assert.False(t, got.Authenticated())
	assert.Empty(t, got.MemberSessionID)
	assert.True(t, got.MemberSessionExpiresAt.IsZero())
	assert.Equal(t, brokerOrg, got.OrganizationID)
	assert.Equal(t, brokerMember, got.MemberID)
	assertNoRawFields(t, got)
}

func TestCompleteTypedCallbackClassifiesDefinitiveAndTransient(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		kind   string
		want   error
	}{
		{"invalid token", http.StatusBadRequest, "unable_to_auth_magic_link", brokerprovider.ErrDefinitiveDenial},
		{"expired", http.StatusUnauthorized, "unable_to_auth_sso_token", brokerprovider.ErrDefinitiveDenial},
		{"unknown otp", http.StatusBadRequest, "otp_code_not_found", brokerprovider.ErrDefinitiveDenial},
		{"rate", http.StatusTooManyRequests, "too_many_requests", brokerprovider.ErrProviderUnavailable},
		{"server", http.StatusBadGateway, "internal_server_error", brokerprovider.ErrProviderUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tc.status, map[string]any{
					"status_code": tc.status, "error_type": tc.kind,
					"error_message": "rejected " + brokerToken + " for " + brokerEmail,
				})
			}))
			t.Cleanup(server.Close)
			p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
			require.NoError(t, err)
			_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
				Type: brokerprovider.ArtifactTypeMagicLink, Artifact: brokerToken, OrganizationID: brokerOrg,
			})
			require.ErrorIs(t, err, tc.want)
			assert.NotContains(t, err.Error(), brokerToken)
			assert.NotContains(t, err.Error(), brokerEmail)
			assert.NotContains(t, err.Error(), tc.kind)
		})
	}
}

func TestCompleteTypedCallbackRejectsUnknownTypeAndOversizedArtifact(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(server.Close)
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)

	_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: "password", Artifact: brokerToken,
	})
	assert.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)

	_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeMagicLink, Artifact: strings.Repeat("x", 4097),
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "xxxx")

	_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeEmailOTP, Artifact: brokerOTP, EmailAddress: "bad\nemail",
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "bad")
	assert.Zero(t, requests)
}

func TestBrokerDoesNotFollowRedirectsAndCapsResponse(t *testing.T) {
	t.Parallel()
	t.Run("no redirect follow", func(t *testing.T) {
		t.Parallel()
		var hops int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops++
			if r.URL.Path == "/v1/b2b/magic_links/email/discovery/send" {
				http.Redirect(w, r, "/redirected", http.StatusFound)
				return
			}
			t.Errorf("followed redirect to %s", r.URL.Path)
		}))
		t.Cleanup(server.Close)
		p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
		require.NoError(t, err)
		_, err = p.StartLogin(context.Background(), brokerprovider.StartRequest{
			Method: brokerprovider.MethodEmailMagicLink, EmailAddress: brokerEmail,
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, brokerprovider.ErrProviderUnavailable)
		assert.Equal(t, 1, hops)
	})
	t.Run("1MiB cap", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status_code":200`))
			_, _ = w.Write(bytes.Repeat([]byte(" "), 1<<20))
			_, _ = w.Write([]byte(`}`))
		}))
		t.Cleanup(server.Close)
		p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
		require.NoError(t, err)
		_, err = p.StartLogin(context.Background(), brokerprovider.StartRequest{
			Method: brokerprovider.MethodEmailMagicLink, EmailAddress: brokerEmail,
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, brokerprovider.ErrProviderUnavailable)
		assert.NotContains(t, err.Error(), brokerEmail)
	})
}

func TestBrokerResultAndErrorsExposeNoProviderSecrets(t *testing.T) {
	t.Parallel()
	forbidden := []string{"token", "jwt", "secret", "password", "payload", "email", "credential", "cookie", "assertion"}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(brokerprovider.CallbackResult{}),
		reflect.TypeOf(brokerprovider.StartResult{}),
		reflect.TypeOf(stytch.BrokerConfig{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.ToLower(field.Name)
			tag := strings.ToLower(string(field.Tag))
			for _, bad := range forbidden {
				if typ == reflect.TypeOf(stytch.BrokerConfig{}) && (bad == "token" || bad == "secret") {
					// Config may name PublicToken / embed Stytch credentials; they must stay unexported-to-result.
					continue
				}
				assert.NotContains(t, name, bad, "%s field %s", typ.Name(), field.Name)
				assert.NotContains(t, tag, bad, "%s tag %s", typ.Name(), field.Name)
			}
		}
	}
}

func TestCompleteTypedCallbackHonorsCancellationAndMissingMembership(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/b2b/magic_links/discovery/authenticate" {
			writeJSON(w, http.StatusOK, map[string]any{
				"status_code":                200,
				"intermediate_session_token": "ist-empty",
				"discovered_organizations":   []map[string]any{},
			})
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	}))
	t.Cleanup(server.Close)
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.StartLogin(ctx, brokerprovider.StartRequest{
		Method: brokerprovider.MethodEmailMagicLink, EmailAddress: brokerEmail,
	})
	assert.ErrorIs(t, err, brokerprovider.ErrProviderUnavailable)

	_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeDiscoveryMagicLink, Artifact: brokerToken,
	})
	assert.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
}

func TestStartLoginSSOUsesOrganizationWhenConnectionMissing(t *testing.T) {
	t.Parallel()
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg("https://test.stytch.example"), nil)
	require.NoError(t, err)
	start, err := p.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: brokerprovider.MethodSSOOIDC, OrganizationID: brokerOrg,
	})
	require.NoError(t, err)
	parsed, err := url.Parse(start.ContinueURL)
	require.NoError(t, err)
	assert.Equal(t, brokerOrg, parsed.Query().Get("organization_id"))
	assert.Empty(t, parsed.Query().Get("connection_id"))
}

func TestNewBrokerConstructsEnabledAdapter(t *testing.T) {
	t.Parallel()
	p, err := stytch.NewBroker(brokerCfg(""))
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestSSOStartUsesLiveBaseURIWithoutOverride(t *testing.T) {
	t.Parallel()
	cfg := brokerCfg("")
	cfg.Stytch.Env = "live"
	cfg.Stytch.ProjectID = "project-live-example"
	cfg.Stytch.BaseURI = ""
	p, err := stytch.NewBrokerWithHTTPClient(cfg, nil)
	require.NoError(t, err)
	start, err := p.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: brokerprovider.MethodSSOSAML, ConnectionID: brokerConn,
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(start.ContinueURL, "https://api.stytch.com/v1/public/sso/start"))
}

func TestProveSessionMapsDefinitiveAndTransientSessionFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		body   map[string]any
		want   error
	}{
		{"definitive", http.StatusBadRequest, map[string]any{"status_code": 400, "error_type": "session_not_found", "error_message": "gone"}, brokerprovider.ErrDefinitiveDenial},
		{"transient", http.StatusBadGateway, map[string]any{"status_code": 502, "error_type": "internal_server_error"}, brokerprovider.ErrProviderUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/b2b/sso/authenticate":
					writeJSON(w, http.StatusOK, authenticatedMemberBody("sess-token-must-not-leak"))
				case "/v1/b2b/sessions/authenticate":
					writeJSON(w, tc.status, tc.body)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
			require.NoError(t, err)
			_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
				Type: brokerprovider.ArtifactTypeSSOToken, Artifact: brokerSSOToken,
			})
			require.ErrorIs(t, err, tc.want)
			assert.NotContains(t, err.Error(), "sess-token")
		})
	}
}

func TestCompleteTypedCallbackRejectsNonNumericOTPAndInvalidUTF8(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(server.Close)
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)
	_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeEmailOTP, Artifact: "12ab56",
		EmailAddress: brokerEmail, OrganizationID: brokerOrg,
	})
	require.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
	_, err = p.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeDiscoveryEmailOTP, Artifact: "12",
		EmailAddress: brokerEmail,
	})
	require.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
	assert.Zero(t, requests)
}

func TestBrokerHighCountStartRaceReturnsToZeroInflight(t *testing.T) {
	t.Parallel()
	var inflight atomic.Int64
	var max atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cur := inflight.Add(1)
		for {
			prev := max.Load()
			if cur <= prev || max.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer inflight.Add(-1)
		writeJSON(w, http.StatusOK, map[string]any{"status_code": 200})
	}))
	t.Cleanup(server.Close)
	p, err := stytch.NewBrokerWithHTTPClient(brokerCfg(server.URL), server.Client())
	require.NoError(t, err)

	const workers = 128
	var wg sync.WaitGroup
	start := make(chan struct{})
	var failures atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, callErr := p.StartLogin(context.Background(), brokerprovider.StartRequest{
				Method: brokerprovider.MethodEmailOTP, EmailAddress: brokerEmail,
			})
			if callErr != nil {
				failures.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	assert.Zero(t, failures.Load())
	assert.Zero(t, inflight.Load())
	assert.Positive(t, max.Load())
}

type methodCapture struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
	raw    []byte
}

func (c *methodCapture) capture(r *http.Request) {
	c.method = r.Method
	c.path = r.URL.Path
	c.query = r.URL.Query()
	c.raw, _ = io.ReadAll(r.Body)
	if len(c.raw) > 0 {
		_ = json.Unmarshal(c.raw, &c.body)
	}
	if c.body == nil {
		c.body = map[string]any{}
	}
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func authenticatedMemberBody(sessionToken string) map[string]any {
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	return map[string]any{
		"status_code":          200,
		"member_authenticated": true,
		"member_id":            brokerMember,
		"organization_id":      brokerOrg,
		"session_token":        sessionToken,
		"session_jwt":          "eyJhbGciOiJub25lIn0.session-jwt-must-not-leak",
		"member": map[string]any{
			"member_id": brokerMember, "organization_id": brokerOrg,
		},
		"organization":               map[string]any{"organization_id": brokerOrg},
		"member_session":             map[string]any{"member_session_id": brokerSession, "organization_id": brokerOrg, "member_id": brokerMember, "expires_at": exp, "roles": []string{"stytch_admin"}},
		"intermediate_session_token": "",
	}
}

func sessionAuthenticateBody() map[string]any {
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	return map[string]any{
		"request_id": "req-auth",
		"member_session": map[string]any{
			"member_session_id": brokerSession,
			"organization_id":   brokerOrg,
			"member_id":         brokerMember,
			"expires_at":        exp,
			"roles":             []string{"stytch_admin"},
		},
	}
}

func sessionGetBody() map[string]any {
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	return map[string]any{
		"member_sessions": []map[string]any{{
			"member_session_id": brokerSession,
			"organization_id":   brokerOrg,
			"member_id":         brokerMember,
			"expires_at":        exp,
		}},
	}
}

func assertNoRawFields(t *testing.T, got brokerprovider.CallbackResult) {
	t.Helper()
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	blob := strings.ToLower(string(encoded))
	for _, leak := range []string{
		strings.ToLower(brokerToken), strings.ToLower(brokerSSOToken), strings.ToLower(brokerEmail),
		"session-jwt", "sess-token", "ist-", "session_token", "session_jwt",
		"intermediate_session", "stytch_admin", "eyj", "email_address",
	} {
		assert.NotContains(t, blob, leak)
	}
	assert.LessOrEqual(t, utf8.RuneCountInString(got.MemberSessionID), 255)
}

var _ = errors.New
