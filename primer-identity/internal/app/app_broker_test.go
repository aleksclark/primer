package app_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/app"
	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

const (
	appBrokerOrigin = "http://127.0.0.1"
	appBrokerIssuer = "http://id.test"
	pkceChallenge   = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
)

func encodedKey(b byte) string {
	raw := bytes.Repeat([]byte{b}, 32)
	return "1:" + base64.RawURLEncoding.EncodeToString(raw)
}

func brokerTestConfig(t *testing.T, env string) *config.Config {
	t.Helper()
	return &config.Config{
		DatabaseURL:                    testutil.DatabaseURL(t),
		Host:                           "127.0.0.1",
		Port:                           0,
		Env:                            env,
		LogLevel:                       "info",
		Issuer:                         appBrokerIssuer,
		ShutdownTimeout:                3 * time.Second,
		HTTPReadHeaderTimeout:          3 * time.Second,
		HTTPMaxBodyBytes:               1 << 20,
		StateSealKeys:                  encodedKey(0x11),
		StateSealActiveVersion:         1,
		StateHashPeppers:               encodedKey(0x22),
		StateHashActiveVersion:         1,
		BrokerCookiePeppers:            encodedKey(0x33),
		BrokerCookieActiveVersion:      1,
		AuthorizationCodePeppers:       encodedKey(0x44),
		AuthorizationCodeActiveVersion: 1,
		BrokerAllowedOrigin:            appBrokerOrigin,
		BrokerDiscoveryRedirectURL:     "http://127.0.0.1/broker/stytch/callback",
		BrokerLoginRedirectURL:         "http://127.0.0.1/broker/stytch/callback",
		BrokerSignupRedirectURL:        "http://127.0.0.1/broker/stytch/callback",
		StytchPublicToken:              "public-token-test",
	}
}

func startApp(t *testing.T, cfg *config.Config, opts app.Options) (baseURL string, logs *bytes.Buffer, shutdown chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	baseURL = "http://" + ln.Addr().String()
	logs = &bytes.Buffer{}
	shutdown = make(chan struct{})
	opts.Config = cfg
	opts.Stdout = logs
	opts.Listener = ln
	opts.SkipMigrate = true
	opts.ShutdownSignal = shutdown
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(context.Background(), opts) }()
	t.Cleanup(func() {
		select {
		case <-shutdown:
		default:
			close(shutdown)
		}
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Run exit")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, app.WaitReady(ctx, baseURL))
	return baseURL, logs, shutdown
}

func noFollow() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func scriptedOK(t *testing.T, artifact string) *brokerprovider.ScriptedProvider {
	t.Helper()
	suffix := uuid.NewString()[:8]
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{{
			Artifact: artifact, Method: brokerprovider.MethodEmailMagicLink,
			Outcome:   brokerprovider.OutcomeAuthenticated,
			ProjectID: "project-" + suffix, OrganizationID: "organization-" + suffix,
			MemberID: "member-" + suffix, MemberSessionID: "member-session-" + suffix,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}},
	})
	require.NoError(t, err)
	return p
}

func registerAppClient(t *testing.T, clientID string) (redirect, resource, audience string) {
	t.Helper()
	pool := testutil.DB(t)
	client, err := repo.CreateOAuthClient(context.Background(), pool, domain.OAuthClient{
		ClientID: clientID, Name: "app broker client", ClientType: "public",
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

func TestRunDisabledBrokerKeepsHealthReadyAndOmitsBrokerRoutes(t *testing.T) {
	cfg := brokerTestConfig(t, "test")
	cfg.BrokerAllowedOrigin = ""
	cfg.BrokerDiscoveryRedirectURL = ""
	cfg.BrokerLoginRedirectURL = ""
	cfg.BrokerSignupRedirectURL = ""
	cfg.StytchPublicToken = ""
	baseURL, _, _ := startApp(t, cfg, app.Options{})

	client := noFollow()
	health, err := client.Get(baseURL + "/healthz")
	require.NoError(t, err)
	_ = health.Body.Close()
	assert.Equal(t, http.StatusOK, health.StatusCode)

	ready, err := client.Get(baseURL + "/readyz")
	require.NoError(t, err)
	_ = ready.Body.Close()
	assert.Equal(t, http.StatusOK, ready.StatusCode)

	authz, err := client.Get(baseURL + "/oauth/authorize")
	require.NoError(t, err)
	_ = authz.Body.Close()
	assert.Equal(t, http.StatusNotFound, authz.StatusCode)

	login, err := client.Get(baseURL + "/broker/stytch/login")
	require.NoError(t, err)
	_ = login.Body.Close()
	assert.Equal(t, http.StatusNotFound, login.StatusCode)

	token, err := client.Get(baseURL + "/oauth/token")
	require.NoError(t, err)
	_ = token.Body.Close()
	assert.Equal(t, http.StatusNotFound, token.StatusCode)
}

func TestRunRejectsProductionTestSeamsBeforeListen(t *testing.T) {
	provider := scriptedOK(t, "prod-seam")
	cases := []struct {
		name string
		opts app.Options
	}{
		{"provider", app.Options{Provider: provider}},
		{"enable", app.Options{EnableBrokerForTest: true}},
		{"insecure cookie", app.Options{InsecureBrokerCookieForTest: true}},
		{"proof key source", app.Options{ProofKeySource: func([]byte) (int, error) { return 32, nil }}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := brokerTestConfig(t, "production")
			cfg.Stytch = config.StytchConfig{
				Enabled: true, ProjectID: "project-live-example", Secret: "secret-must-not-leak",
				Env: "live", RequestTimeout: 3 * time.Second,
				PositiveCacheTTL: 15 * time.Second, NegativeCacheTTL: 5 * time.Second,
				PositiveCacheCapacity: 10000, NegativeCacheCapacity: 2000,
			}
			cfg.BrokerAllowedOrigin = "https://id.example"
			cfg.BrokerDiscoveryRedirectURL = "https://id.example/broker/stytch/callback"
			cfg.BrokerLoginRedirectURL = "https://id.example/broker/stytch/callback"
			cfg.BrokerSignupRedirectURL = "https://id.example/broker/stytch/callback"
			cfg.Issuer = "https://id.example"
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			addr := ln.Addr().String()
			require.NoError(t, ln.Close())

			tc.opts.Config = cfg
			tc.opts.SkipMigrate = true
			err = app.Run(context.Background(), tc.opts)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret-must-not-leak")
			assert.NotContains(t, strings.ToLower(err.Error()), "listening")

			conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
			if dialErr == nil {
				_ = conn.Close()
				t.Fatal("process listened despite rejected production test seam")
			}
		})
	}
}

func TestRunFailsBeforeListenWhenBrokerSecretsMissing(t *testing.T) {
	cfg := brokerTestConfig(t, "test")
	cfg.StateSealKeys = ""
	provider := scriptedOK(t, "missing-secrets")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	err = app.Run(context.Background(), app.Options{
		Config: cfg, SkipMigrate: true, EnableBrokerForTest: true, Provider: provider,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
	conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("process listened despite missing broker secrets")
	}
}

func TestRunFailsBeforeListenWhenBrokerOriginMalformed(t *testing.T) {
	cfg := brokerTestConfig(t, "test")
	cfg.BrokerAllowedOrigin = "https://user:pass@evil.example"
	err := app.Run(context.Background(), app.Options{
		Config: cfg, SkipMigrate: true, EnableBrokerForTest: true, Provider: scriptedOK(t, "bad-origin"),
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "user:pass")
	assert.Contains(t, strings.ToLower(err.Error()), "origin")
}

func TestRunInjectedBrokerComposesAuthorizeLoginCallback(t *testing.T) {
	artifact := "app-e01-" + uuid.NewString()
	state := "state-" + uuid.NewString()
	cfg := brokerTestConfig(t, "test")
	baseURL, logs, _ := startApp(t, cfg, app.Options{
		Provider:                    scriptedOK(t, artifact),
		EnableBrokerForTest:         true,
		InsecureBrokerCookieForTest: true,
	})
	clientID := "app-" + uuid.NewString()
	redirect, resource, audience := registerAppClient(t, clientID)
	client := noFollow()

	q := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect},
		"resource": {resource}, "audience": {audience}, "scope": {"openid"},
		"state": {state}, "code_challenge": {pkceChallenge}, "code_challenge_method": {"S256"},
	}
	authz, err := client.Get(baseURL + "/oauth/authorize?" + q.Encode())
	require.NoError(t, err)
	body, _ := io.ReadAll(authz.Body)
	_ = authz.Body.Close()
	require.Equal(t, http.StatusSeeOther, authz.StatusCode)
	assert.Equal(t, "/broker/stytch/login", authz.Header.Get("Location"))
	cookie := brokerCookie(authz)
	require.NotEmpty(t, cookie)

	loginReq, err := http.NewRequest(http.MethodGet, baseURL+"/broker/stytch/login", nil)
	require.NoError(t, err)
	loginReq.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	loginReq.Header.Set("Accept", "application/json")
	login, err := client.Do(loginReq)
	require.NoError(t, err)
	loginBody, _ := io.ReadAll(login.Body)
	_ = login.Body.Close()
	require.Equal(t, http.StatusOK, login.StatusCode)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(loginBody, &payload))
	csrf, _ := payload["csrf"].(string)
	require.NotEmpty(t, csrf)

	form := url.Values{"csrf": {csrf}, "email": {"member@school.example"}}
	startReq, err := http.NewRequest(http.MethodPost, baseURL+"/broker/stytch/email/start", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	startReq.Header.Set("Origin", appBrokerOrigin)
	startReq.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	startReq.Header.Set("X-CSRF-Token", csrf)
	started, err := client.Do(startReq)
	require.NoError(t, err)
	_ = started.Body.Close()
	assert.Equal(t, http.StatusOK, started.StatusCode)

	cbReq, err := http.NewRequest(http.MethodGet, baseURL+"/broker/stytch/callback?token="+url.QueryEscape(artifact)+"&type=discovery_magic_link", nil)
	require.NoError(t, err)
	cbReq.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	cb, err := client.Do(cbReq)
	require.NoError(t, err)
	cbBody, _ := io.ReadAll(cb.Body)
	_ = cb.Body.Close()
	require.Equal(t, http.StatusSeeOther, cb.StatusCode)
	loc, err := url.Parse(cb.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, redirect, loc.Scheme+"://"+loc.Host+loc.Path)
	assert.Equal(t, state, loc.Query().Get("state"))
	assert.Equal(t, appBrokerIssuer, loc.Query().Get("iss"))
	assert.NotEmpty(t, loc.Query().Get("code"))
	assert.NotContains(t, string(cbBody), artifact)
	assert.Contains(t, strings.ToLower(strings.Join(cb.Header.Values("Set-Cookie"), "\n")), "max-age=0")

	token, err := client.Get(baseURL + "/oauth/token")
	require.NoError(t, err)
	_ = token.Body.Close()
	assert.Equal(t, http.StatusNotFound, token.StatusCode)

	combined := logs.String() + string(body) + string(loginBody) + string(cbBody)
	assert.NotContains(t, strings.ToLower(combined), "session_jwt")
	assert.NotContains(t, strings.ToLower(combined), "session_token")
	assert.NotContains(t, combined, artifact)
}

func TestRunFailsBeforeListenWhenOfficialProofCacheCSPRNGFails(t *testing.T) {
	cfg := brokerTestConfig(t, "test")
	cfg.Stytch = config.StytchConfig{
		Enabled: true, ProjectID: "project-test-example", Secret: "secret-must-not-leak",
		Env: "test", RequestTimeout: 3 * time.Second,
		PositiveCacheTTL: 15 * time.Second, NegativeCacheTTL: 5 * time.Second,
		PositiveCacheCapacity: 10000, NegativeCacheCapacity: 2000,
	}
	cfg.ProviderProofCacheTTL = 15 * time.Second
	cfg.ProviderProofCacheCapacity = 256
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	err = app.Run(context.Background(), app.Options{
		Config: cfg, SkipMigrate: true,
		ProofKeySource: func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF },
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-must-not-leak")
	assert.NotContains(t, strings.ToLower(err.Error()), "listening")
	conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("process listened despite official proof-cache CSPRNG failure")
	}
}

func TestRunInjectedProviderDoesNotComposeOfficialProofCache(t *testing.T) {
	var reads atomic.Int64
	cfg := brokerTestConfig(t, "test")
	baseURL, logs, _ := startApp(t, cfg, app.Options{
		Provider:                    scriptedOK(t, "scripted-no-proof-cache"),
		EnableBrokerForTest:         true,
		InsecureBrokerCookieForTest: true,
		ProofKeySource: func(b []byte) (int, error) {
			reads.Add(1)
			return copy(b, bytes.Repeat([]byte{7}, len(b))), nil
		},
	})
	assert.Zero(t, reads.Load(), "scripted injected provider must not mint an official proof-cache key")
	assert.NotContains(t, strings.ToLower(logs.String()), "proof cache")
	assert.NotContains(t, strings.ToLower(logs.String()), "hmac")
	client := noFollow()
	ready, err := client.Get(baseURL + "/readyz")
	require.NoError(t, err)
	_ = ready.Body.Close()
	assert.Equal(t, http.StatusOK, ready.StatusCode)
}

func brokerCookie(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == broker.BrokerCookieName {
			return c.Value
		}
	}
	for _, h := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(h, broker.BrokerCookieName+"=") {
			return strings.TrimPrefix(strings.SplitN(h, ";", 2)[0], broker.BrokerCookieName+"=")
		}
	}
	return ""
}
