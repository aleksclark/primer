package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
	e2eOrigin = "http://127.0.0.1"
	e2eIssuer = "http://id.test"
	e2ePKCE   = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	e2eSecret = "stytch-secret-must-not-leak"
	e2ePubTok = "public-token-e2e"
)

type processServer struct {
	baseURL  string
	logs     *bytes.Buffer
	shutdown chan struct{}
	errCh    chan error
}

func encodedSecret(b byte) string {
	return "1:" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))
}

func brokerProcessConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		DatabaseURL:                    testutil.DatabaseURL(t),
		Host:                           "127.0.0.1",
		Port:                           0,
		Env:                            "test",
		LogLevel:                       "info",
		Issuer:                         e2eIssuer,
		ShutdownTimeout:                5 * time.Second,
		HTTPReadHeaderTimeout:          5 * time.Second,
		HTTPMaxBodyBytes:               1 << 20,
		StateSealKeys:                  encodedSecret(0x11),
		StateSealActiveVersion:         1,
		StateHashPeppers:               encodedSecret(0x22),
		StateHashActiveVersion:         1,
		BrokerCookiePeppers:            encodedSecret(0x33),
		BrokerCookieActiveVersion:      1,
		AuthorizationCodePeppers:       encodedSecret(0x44),
		AuthorizationCodeActiveVersion: 1,
		BrokerAllowedOrigin:            e2eOrigin,
		BrokerDiscoveryRedirectURL:     "http://127.0.0.1/broker/stytch/callback",
		BrokerLoginRedirectURL:         "http://127.0.0.1/broker/stytch/callback",
		BrokerSignupRedirectURL:        "http://127.0.0.1/broker/stytch/callback",
		StytchPublicToken:              e2ePubTok,
	}
}

func startBrokerProcess(t *testing.T, cfg *config.Config, provider brokerprovider.Provider) *processServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &processServer{
		baseURL:  "http://" + ln.Addr().String(),
		logs:     &bytes.Buffer{},
		shutdown: make(chan struct{}),
		errCh:    make(chan error, 1),
	}
	go func() {
		srv.errCh <- app.Run(context.Background(), app.Options{
			Config:                      cfg,
			Stdout:                      srv.logs,
			Listener:                    ln,
			SkipMigrate:                 true,
			ShutdownSignal:              srv.shutdown,
			Provider:                    provider,
			EnableBrokerForTest:         true,
			InsecureBrokerCookieForTest: true,
		})
	}()
	t.Cleanup(func() {
		select {
		case <-srv.shutdown:
		default:
			close(srv.shutdown)
		}
		require.NoError(t, srv.wait())
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, app.WaitReady(ctx, srv.baseURL))
	return srv
}

func (s *processServer) wait() error {
	if s.errCh == nil {
		return nil
	}
	select {
	case err := <-s.errCh:
		s.errCh = nil
		return err
	case <-time.After(8 * time.Second):
		return fmt.Errorf("broker process did not shut down")
	}
}

func noFollowClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type processStopper struct {
	cmd  *exec.Cmd
	once sync.Once
}

func (s *processStopper) stop() {
	s.once.Do(func() {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			_ = s.cmd.Process.Kill()
			<-done
		}
	})
}

func uniqueLabel(prefix string) string { return prefix + "-" + uuid.NewString() }

func uniqueTuple() (project, org, member, session string) {
	suffix := uuid.NewString()[:8]
	return "project-" + suffix, "organization-" + suffix, "member-" + suffix, "member-session-" + suffix
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

func registerProcessClient(t *testing.T, clientID string) (redirect, resource, audience string) {
	t.Helper()
	pool := testutil.DB(t)
	client, err := repo.CreateOAuthClient(context.Background(), pool, domain.OAuthClient{
		ClientID: clientID, Name: "e2e broker client", ClientType: "public",
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

func cookieFrom(resp *http.Response) string {
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

func authorize(t *testing.T, client *http.Client, baseURL, clientID, redirect, resource, audience, state string) *http.Response {
	t.Helper()
	q := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect},
		"resource": {resource}, "audience": {audience}, "scope": {"openid"},
		"state": {state}, "code_challenge": {e2ePKCE}, "code_challenge_method": {"S256"},
	}
	resp, err := client.Get(baseURL + "/oauth/authorize?" + q.Encode())
	require.NoError(t, err)
	return resp
}

func loginCSRFProcess(t *testing.T, client *http.Client, baseURL, cookie string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/broker/stytch/login", nil)
	require.NoError(t, err)
	req.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	csrf, _ := payload["csrf"].(string)
	require.NotEmpty(t, csrf)
	return csrf
}

func assertNoProviderLeak(t *testing.T, parts ...string) {
	t.Helper()
	combined := strings.ToLower(strings.Join(parts, "\n"))
	for _, banned := range []string{"session_jwt", "session_token", "provider_payload", e2eSecret} {
		assert.NotContains(t, combined, banned)
	}
}

func TestProcessE01AuthorizeLoginStartCallbackExactRedirectAndCookieDeletion(t *testing.T) {
	artifact := uniqueLabel("e01")
	state := uniqueLabel("state")
	srv := startBrokerProcess(t, brokerProcessConfig(t), scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	clientID := uniqueLabel("e01")
	redirect, resource, audience := registerProcessClient(t, clientID)
	client := noFollowClient()

	authz := authorize(t, client, srv.baseURL, clientID, redirect, resource, audience, state)
	defer authz.Body.Close()
	require.Equal(t, http.StatusSeeOther, authz.StatusCode)
	assert.Equal(t, "/broker/stytch/login", authz.Header.Get("Location"))
	cookie := cookieFrom(authz)
	require.NotEmpty(t, cookie)

	csrf := loginCSRFProcess(t, client, srv.baseURL, cookie)
	form := url.Values{"csrf": {csrf}}
	startReq, err := http.NewRequest(http.MethodPost, srv.baseURL+"/broker/stytch/email/start", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	startReq.Header.Set("Origin", e2eOrigin)
	startReq.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	startReq.Header.Set("X-CSRF-Token", csrf)
	started, err := client.Do(startReq)
	require.NoError(t, err)
	startBody, _ := io.ReadAll(started.Body)
	_ = started.Body.Close()
	require.Equal(t, http.StatusOK, started.StatusCode)

	cbReq, err := http.NewRequest(http.MethodGet, srv.baseURL+"/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
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
	assert.Equal(t, e2eIssuer, loc.Query().Get("iss"))
	assert.NotEmpty(t, loc.Query().Get("code"))
	expired := strings.ToLower(strings.Join(cb.Header.Values("Set-Cookie"), "\n"))
	assert.Contains(t, expired, "max-age=0")

	token, err := client.Get(srv.baseURL + "/oauth/token")
	require.NoError(t, err)
	_ = token.Body.Close()
	assert.Equal(t, http.StatusNotFound, token.StatusCode)
	assertNoProviderLeak(t, srv.logs.String(), string(startBody), string(cbBody), loc.String())
}

func TestProcessE05FourMethodsAndIncompleteMFA(t *testing.T) {
	methods := []struct {
		path   string
		method brokerprovider.Method
		form   url.Values
	}{
		{"/broker/stytch/email/start", brokerprovider.MethodEmailMagicLink, url.Values{}},
		{"/broker/stytch/email/verify", brokerprovider.MethodEmailOTP, url.Values{}},
		{"/broker/stytch/sso/start", brokerprovider.MethodSSOSAML, url.Values{"type": {"saml"}}},
		{"/broker/stytch/sso/start", brokerprovider.MethodSSOOIDC, url.Values{"type": {"oidc"}}},
	}
	fixtures := make([]brokerprovider.Fixture, 0, len(methods)+1)
	for _, tc := range methods {
		fixtures = append(fixtures, successFixture(uniqueLabel(string(tc.method)), tc.method))
	}
	mfaArtifact := uniqueLabel("mfa")
	project, org, member, session := uniqueTuple()
	fixtures = append(fixtures, brokerprovider.Fixture{
		Artifact: mfaArtifact, Method: brokerprovider.MethodEmailOTP, Outcome: brokerprovider.OutcomeIncompleteMFA,
		ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
	})
	srv := startBrokerProcess(t, brokerProcessConfig(t), scripted(t, fixtures...))
	client := noFollowClient()

	for _, tc := range methods {
		t.Run(string(tc.method), func(t *testing.T) {
			clientID := uniqueLabel("m")
			redirect, resource, audience := registerProcessClient(t, clientID)
			authz := authorize(t, client, srv.baseURL, clientID, redirect, resource, audience, uniqueLabel("st"))
			cookie := cookieFrom(authz)
			_ = authz.Body.Close()
			require.NotEmpty(t, cookie)
			csrf := loginCSRFProcess(t, client, srv.baseURL, cookie)
			form := tc.form
			if form == nil {
				form = url.Values{}
			}
			form.Set("csrf", csrf)
			req, err := http.NewRequest(http.MethodPost, srv.baseURL+tc.path, strings.NewReader(form.Encode()))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", e2eOrigin)
			req.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
			req.Header.Set("X-CSRF-Token", csrf)
			resp, err := client.Do(req)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assertNoProviderLeak(t, string(body))
		})
	}

	clientID := uniqueLabel("mfa")
	redirect, resource, audience := registerProcessClient(t, clientID)
	authz := authorize(t, client, srv.baseURL, clientID, redirect, resource, audience, uniqueLabel("mfa-st"))
	cookie := cookieFrom(authz)
	_ = authz.Body.Close()
	cbReq, err := http.NewRequest(http.MethodGet, srv.baseURL+"/broker/stytch/callback?token="+url.QueryEscape(mfaArtifact), nil)
	require.NoError(t, err)
	cbReq.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
	cb, err := client.Do(cbReq)
	require.NoError(t, err)
	cbBody, _ := io.ReadAll(cb.Body)
	_ = cb.Body.Close()
	assert.Equal(t, http.StatusBadRequest, cb.StatusCode)
	assert.Empty(t, cb.Header.Get("Location"))
	assert.NotContains(t, string(cbBody), mfaArtifact)
}

func TestProcessE06CrossOrgDistinctAccountsAndNoMemberships(t *testing.T) {
	project := uniqueLabel("project")
	sharedMember := uniqueLabel("member")
	artifactA, artifactB := uniqueLabel("org-a"), uniqueLabel("org-b")
	srv := startBrokerProcess(t, brokerProcessConfig(t), scripted(t,
		brokerprovider.Fixture{
			Artifact: artifactA, Method: brokerprovider.MethodEmailOTP, Outcome: brokerprovider.OutcomeAuthenticated,
			ProjectID: project, OrganizationID: uniqueLabel("orga"), MemberID: sharedMember,
			MemberSessionID: uniqueLabel("sess-a"), ExpiresAt: time.Now().UTC().Add(time.Hour),
		},
		brokerprovider.Fixture{
			Artifact: artifactB, Method: brokerprovider.MethodEmailOTP, Outcome: brokerprovider.OutcomeAuthenticated,
			ProjectID: project, OrganizationID: uniqueLabel("orgb"), MemberID: sharedMember,
			MemberSessionID: uniqueLabel("sess-b"), ExpiresAt: time.Now().UTC().Add(time.Hour),
		},
	))
	client := noFollowClient()
	clientID := uniqueLabel("xorg")
	redirect, resource, audience := registerProcessClient(t, clientID)

	complete := func(artifact, state string) string {
		authz := authorize(t, client, srv.baseURL, clientID, redirect, resource, audience, state)
		cookie := cookieFrom(authz)
		_ = authz.Body.Close()
		req, err := http.NewRequest(http.MethodGet, srv.baseURL+"/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
		require.NoError(t, err)
		req.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
		cb, err := client.Do(req)
		require.NoError(t, err)
		_ = cb.Body.Close()
		require.Equal(t, http.StatusSeeOther, cb.StatusCode)
		loc, err := url.Parse(cb.Header.Get("Location"))
		require.NoError(t, err)
		return loc.Query().Get("code")
	}
	codeA := complete(artifactA, uniqueLabel("st-a"))
	codeB := complete(artifactB, uniqueLabel("st-b"))
	require.NotEmpty(t, codeA)
	require.NotEmpty(t, codeB)
	assert.NotEqual(t, codeA, codeB)

	pool := testutil.DB(t)
	var accounts int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM accounts a
		 JOIN stytch_mappings m ON m.account_id = a.id
		 WHERE m.member_id = $1`, sharedMember).Scan(&accounts))
	assert.Equal(t, 2, accounts)

	var memberships int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE '%membership%'`).Scan(&memberships))
	assert.Zero(t, memberships)
}

func TestProcessE08ConcurrentCallbackOneRedirectAndCode(t *testing.T) {
	artifact := uniqueLabel("e08")
	state := uniqueLabel("race")
	srv := startBrokerProcess(t, brokerProcessConfig(t), scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	clientID := uniqueLabel("race")
	redirect, resource, audience := registerProcessClient(t, clientID)
	client := noFollowClient()
	authz := authorize(t, client, srv.baseURL, clientID, redirect, resource, audience, state)
	cookie := cookieFrom(authz)
	_ = authz.Body.Close()

	const n = 16
	var redirects, codes atomic.Int64
	seen := make(map[string]struct{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, srv.baseURL+"/broker/stytch/callback?token="+url.QueryEscape(artifact), nil)
			if err != nil {
				return
			}
			req.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusSeeOther {
				redirects.Add(1)
				loc, err := url.Parse(resp.Header.Get("Location"))
				if err != nil {
					return
				}
				code := loc.Query().Get("code")
				if code != "" {
					codes.Add(1)
					mu.Lock()
					seen[code] = struct{}{}
					mu.Unlock()
				}
			} else {
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.Empty(t, resp.Header.Get("Location"))
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), redirects.Load())
	assert.Equal(t, int64(1), codes.Load())
	assert.Len(t, seen, 1)
}

func TestProcessFailBeforeListenMissingSecretsOriginRedirectAndPublicToken(t *testing.T) {
	bin := buildIdentityServer(t)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())

	base := []string{
		"IDENTITY_ENV=test",
		"IDENTITY_HOST=127.0.0.1",
		fmt.Sprintf("IDENTITY_PORT=%d", port),
		"IDENTITY_DATABASE_URL=" + testutil.DatabaseURL(t),
		"IDENTITY_ISSUER=http://id.test",
		"IDENTITY_STYTCH_ENABLED=true",
		"IDENTITY_STYTCH_ENV=test",
		"IDENTITY_STYTCH_PROJECT_ID=project-test-example",
		"IDENTITY_STYTCH_SECRET=" + e2eSecret,
		"IDENTITY_STATE_SEAL_KEYS=" + encodedSecret(0x11),
		"IDENTITY_STATE_SEAL_ACTIVE_VERSION=1",
		"IDENTITY_STATE_HASH_PEPPERS=" + encodedSecret(0x22),
		"IDENTITY_STATE_HASH_ACTIVE_VERSION=1",
		"IDENTITY_BROKER_COOKIE_PEPPERS=" + encodedSecret(0x33),
		"IDENTITY_BROKER_COOKIE_ACTIVE_VERSION=1",
		"IDENTITY_AUTHORIZATION_CODE_PEPPERS=" + encodedSecret(0x44),
		"IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION=1",
		"IDENTITY_BROKER_ALLOWED_ORIGIN=http://127.0.0.1",
		"IDENTITY_BROKER_DISCOVERY_REDIRECT_URL=http://127.0.0.1/broker/stytch/callback",
		"IDENTITY_BROKER_LOGIN_REDIRECT_URL=http://127.0.0.1/broker/stytch/callback",
		"IDENTITY_BROKER_SIGNUP_REDIRECT_URL=http://127.0.0.1/broker/stytch/callback",
		"IDENTITY_STYTCH_PUBLIC_TOKEN=" + e2ePubTok,
	}
	cases := []struct {
		name string
		drop string
		set  string
		want string
	}{
		{"missing origin", "IDENTITY_BROKER_ALLOWED_ORIGIN", "", "origin"},
		{"malformed redirect", "IDENTITY_BROKER_LOGIN_REDIRECT_URL", "IDENTITY_BROKER_LOGIN_REDIRECT_URL=https://user:pass@evil.example/cb", "redirect"},
		{"missing public token", "IDENTITY_STYTCH_PUBLIC_TOKEN", "", "public token"},
		{"missing secrets", "IDENTITY_STATE_SEAL_KEYS", "", "broker secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := scrubEnv(append([]string{}, os.Environ()...), "IDENTITY_BROKER_ALLOWED_ORIGIN", "IDENTITY_STYTCH_PUBLIC_TOKEN", "IDENTITY_STATE_SEAL_KEYS", "IDENTITY_BROKER_LOGIN_REDIRECT_URL")
			for _, kv := range base {
				key, _, _ := strings.Cut(kv, "=")
				if key == tc.drop {
					continue
				}
				env = append(env, kv)
			}
			if tc.set != "" {
				env = append(env, tc.set)
			}
			cmd := exec.Command(bin)
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			require.Error(t, err, string(out))
			combined := string(out)
			assert.NotContains(t, combined, "listening")
			assert.NotContains(t, combined, e2eSecret)
			assert.NotContains(t, combined, "user:pass")
			assert.Contains(t, strings.ToLower(combined), tc.want)
			conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 150*time.Millisecond)
			if dialErr == nil {
				_ = conn.Close()
				t.Fatalf("listened despite %s: %s", tc.name, combined)
			}
		})
	}
}

func TestProcessProductionTestSeamsRejectedByBinaryAbsence(t *testing.T) {
	bin := buildIdentityServer(t)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())
	cmd := exec.Command(bin)
	cmd.Env = append(scrubEnv(os.Environ(), "IDENTITY_ENV", "IDENTITY_STYTCH_ENABLED"),
		"IDENTITY_ENV=production",
		"IDENTITY_HOST=127.0.0.1",
		fmt.Sprintf("IDENTITY_PORT=%d", port),
		"IDENTITY_DATABASE_URL="+testutil.DatabaseURL(t),
		"IDENTITY_ISSUER=https://id.example",
		"IDENTITY_STYTCH_ENABLED=true",
		"IDENTITY_STYTCH_ENV=live",
		"IDENTITY_STYTCH_PROJECT_ID=project-live-example",
		"IDENTITY_STYTCH_SECRET="+e2eSecret,
		"IDENTITY_STATE_SEAL_KEYS="+encodedSecret(0x11),
		"IDENTITY_STATE_SEAL_ACTIVE_VERSION=1",
		"IDENTITY_STATE_HASH_PEPPERS="+encodedSecret(0x22),
		"IDENTITY_STATE_HASH_ACTIVE_VERSION=1",
		"IDENTITY_BROKER_COOKIE_PEPPERS="+encodedSecret(0x33),
		"IDENTITY_BROKER_COOKIE_ACTIVE_VERSION=1",
		"IDENTITY_AUTHORIZATION_CODE_PEPPERS="+encodedSecret(0x44),
		"IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION=1",
		"IDENTITY_BROKER_ALLOWED_ORIGIN=https://id.example",
		"IDENTITY_BROKER_DISCOVERY_REDIRECT_URL=https://id.example/broker/stytch/callback",
		"IDENTITY_BROKER_LOGIN_REDIRECT_URL=https://id.example/broker/stytch/callback",
		"IDENTITY_BROKER_SIGNUP_REDIRECT_URL=https://id.example/broker/stytch/callback",
		"IDENTITY_STYTCH_PUBLIC_TOKEN="+e2ePubTok,
		"IDENTITY_INSECURE_BROKER_COOKIE=true",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, string(out))
	combined := string(out)
	assert.NotContains(t, combined, "listening")
	assert.NotContains(t, combined, e2eSecret)
	assert.Contains(t, strings.ToLower(combined), "insecure")
	conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatalf("production insecure cookie listened: %s", combined)
	}
}

func TestProcessBrokerGracefulShutdownLeavesNoListener(t *testing.T) {
	artifact := uniqueLabel("shutdown")
	srv := startBrokerProcess(t, brokerProcessConfig(t), scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink)))
	host := strings.TrimPrefix(srv.baseURL, "http://")
	select {
	case <-srv.shutdown:
	default:
		close(srv.shutdown)
	}
	require.NoError(t, srv.wait())
	conn, err := net.DialTimeout("tcp", host, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("listener still accepted after shutdown")
	}
}

func TestBinaryBrokerDisabledHealthOnlyCheckout(t *testing.T) {
	url := testutil.DatabaseURL(t)
	bin := buildIdentityServer(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"IDENTITY_ENV=test",
		"IDENTITY_HOST=127.0.0.1",
		fmt.Sprintf("IDENTITY_PORT=%d", port),
		"IDENTITY_DATABASE_URL="+url,
		"IDENTITY_ISSUER=http://127.0.0.1:"+fmt.Sprint(port),
		"IDENTITY_SHUTDOWN_TIMEOUT=5s",
		"IDENTITY_LOG_LEVEL=info",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	require.NoError(t, cmd.Start())
	stopper := &processStopper{cmd: cmd}
	t.Cleanup(stopper.stop)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, app.WaitReady(ctx, baseURL))
	client := noFollowClient()
	health, err := client.Get(baseURL + "/healthz")
	require.NoError(t, err)
	_ = health.Body.Close()
	assert.Equal(t, http.StatusOK, health.StatusCode)
	authz, err := client.Get(baseURL + "/oauth/authorize")
	require.NoError(t, err)
	_ = authz.Body.Close()
	assert.Equal(t, http.StatusNotFound, authz.StatusCode)
	stopper.stop()
	assert.NotContains(t, buf.String(), e2eSecret)
}
