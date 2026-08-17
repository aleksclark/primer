package app_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/identity/internal/app"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/token"
)

func canonicalSeal(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func tokenAuthorityConfig(t *testing.T, env, databaseURL string) *config.Config {
	t.Helper()
	cfg := brokerTestConfig(t, env)
	cfg.DatabaseURL = databaseURL
	cfg.Issuer = "https://id.example.test"
	cfg.Key.Enabled = true
	cfg.Key.AutoBootstrap = true
	cfg.Key.SetSealSecretForTest(canonicalSeal(t))
	cfg.ClientSecretPeppers = encodedKey(0x51)
	cfg.ClientSecretActiveVersion = 1
	cfg.RefreshTokenPeppers = encodedKey(0x61)
	cfg.RefreshTokenActiveVersion = 1
	cfg.ClientAssertionPeppers = encodedKey(0x71)
	cfg.ClientAssertionActiveVersion = 1
	require.NoError(t, cfg.Validate())
	return cfg
}

func dedicatedIdentityURL(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("primer_identity_test"),
		tcpostgres.WithUsername("primer"),
		tcpostgres.WithPassword("primer"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, url))
	return url
}

func connectURL(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := db.Connect(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestRunTokenAuthorityBootstrapsOneActiveKeyAndServesJWKS(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	baseURL, _, _ := startApp(t, cfg, app.Options{})

	client := noFollow()
	ready, err := client.Get(baseURL + "/readyz")
	require.NoError(t, err)
	_ = ready.Body.Close()
	assert.Equal(t, http.StatusOK, ready.StatusCode)

	jwksResp, err := client.Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	body, err := io.ReadAll(jwksResp.Body)
	_ = jwksResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, jwksResp.StatusCode)
	assert.LessOrEqual(t, len(body), 64*1024)
	assert.Contains(t, jwksResp.Header.Get("Content-Type"), "jwk-set+json")
	assert.Equal(t, "public,max-age=300", jwksResp.Header.Get("Cache-Control"))
	assert.NotEmpty(t, jwksResp.Header.Get("ETag"))

	parsed, err := token.ParseJWKS(body)
	require.NoError(t, err)
	require.NotEmpty(t, parsed)
	assert.Equal(t, "EC", parsed[0].KTY)
	assert.Equal(t, "P-256", parsed[0].CRV)
	assert.Equal(t, "sig", parsed[0].Use)
	assert.Equal(t, "ES256", parsed[0].Alg)
	assert.NotContains(t, string(body), `"d"`)
	assert.NotContains(t, strings.ToLower(string(body)), "sealed")

	n, err := repo.CountSigningKeysByStatus(context.Background(), connectURL(t, url), domain.SigningKeyStatusActive)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestRunJWKSCanVerifyCurrentTokenPackageOutput(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	baseURL, _, _ := startApp(t, cfg, app.Options{})

	keySvc := keys.NewService(connectURL(t, url), cfg.Key, "test")
	t.Cleanup(func() { _ = keySvc.Close() })
	minter, err := token.NewMinter(keyServiceSigner{svc: keySvc}, cfg.Issuer, nil)
	require.NoError(t, err)
	subject := uuid.NewString()
	issued, err := minter.IssueHuman(context.Background(), token.HumanInput{
		Subject:  subject,
		Audience: "studio",
		ClientID: "studio-bff",
		Scope:    "openid",
		TTL:      15 * time.Minute,
	}, func(context.Context, token.IssuedToken) error { return nil })
	require.NoError(t, err)

	resp, err := noFollow().Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	pubs, err := token.ParseJWKS(body)
	require.NoError(t, err)
	keyset, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) { return pubs, nil })
	require.NoError(t, err)
	require.NoError(t, keyset.Refresh(context.Background()))
	verifier, err := token.NewVerifier(keyset, cfg.Issuer, "studio", nil, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		return token.ClientRegistration{ClientID: clientID, Audience: "studio", SubjectClass: token.KindHuman, Scope: "openid"}, nil
	})
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	assert.Equal(t, subject, got.Subject)
	assert.Equal(t, "studio-bff", got.ClientID)
}

func TestRunConcurrentTokenAuthorityBootstrapProducesOneActive(t *testing.T) {
	url := dedicatedIdentityURL(t)
	first := tokenAuthorityConfig(t, "test", url)
	second := tokenAuthorityConfig(t, "test", url)
	second.Key = first.Key

	type started struct {
		base string
		err  error
	}
	startedCh := make(chan started, 2)
	var wg sync.WaitGroup
	for _, cfg := range []*config.Config{first, second} {
		wg.Add(1)
		go func(cfg *config.Config) {
			defer wg.Done()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				startedCh <- started{err: err}
				return
			}
			base := "http://" + ln.Addr().String()
			shutdown := make(chan struct{})
			errCh := make(chan error, 1)
			go func() {
				errCh <- app.Run(context.Background(), app.Options{
					Config: cfg, Listener: ln, SkipMigrate: true, ShutdownSignal: shutdown,
				})
			}()
			t.Cleanup(func() {
				select {
				case <-shutdown:
				default:
					close(shutdown)
				}
				select {
				case runErr := <-errCh:
					if runErr != nil {
						t.Errorf("run: %v", runErr)
					}
				case <-time.After(5 * time.Second):
					t.Error("timeout waiting for Run exit")
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			startedCh <- started{base: base, err: app.WaitReady(ctx, base)}
		}(cfg)
	}
	wg.Wait()
	close(startedCh)
	var bases []string
	for got := range startedCh {
		require.NoError(t, got.err)
		bases = append(bases, got.base)
	}
	require.Len(t, bases, 2)

	n, err := repo.CountSigningKeysByStatus(context.Background(), connectURL(t, url), domain.SigningKeyStatusActive)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var firstKids []string
	for _, base := range bases {
		resp, err := noFollow().Get(base + "/readyz")
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		jwks, err := noFollow().Get(base + "/.well-known/jwks.json")
		require.NoError(t, err)
		body, err := io.ReadAll(jwks.Body)
		_ = jwks.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, jwks.StatusCode)
		parsed, err := token.ParseJWKS(body)
		require.NoError(t, err)
		require.Len(t, parsed, 1)
		firstKids = append(firstKids, parsed[0].Kid)
	}
	assert.Equal(t, firstKids[0], firstKids[1])
}

func TestRunTokenAuthorityRestartReusesActiveKey(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	firstURL, _, shutdown := startApp(t, cfg, app.Options{})
	first, err := noFollow().Get(firstURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	firstBody, err := io.ReadAll(first.Body)
	_ = first.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, first.StatusCode)
	close(shutdown)

	restart := tokenAuthorityConfig(t, "test", url)
	restart.Key = cfg.Key
	secondURL, _, _ := startApp(t, restart, app.Options{})
	second, err := noFollow().Get(secondURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	secondBody, err := io.ReadAll(second.Body)
	_ = second.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, second.StatusCode)
	assert.Equal(t, firstBody, secondBody)

	n, err := repo.CountSigningKeysByStatus(context.Background(), connectURL(t, url), domain.SigningKeyStatusActive)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestRunDisabledTokenAuthorityKeepsHealthReadyAndOmitsJWKS(t *testing.T) {
	cfg := brokerTestConfig(t, "test")
	cfg.BrokerAllowedOrigin = ""
	cfg.BrokerDiscoveryRedirectURL = ""
	cfg.BrokerLoginRedirectURL = ""
	cfg.BrokerSignupRedirectURL = ""
	cfg.StytchPublicToken = ""
	baseURL, _, _ := startApp(t, cfg, app.Options{})

	client := noFollow()
	ready, err := client.Get(baseURL + "/readyz")
	require.NoError(t, err)
	_ = ready.Body.Close()
	assert.Equal(t, http.StatusOK, ready.StatusCode)

	jwks, err := client.Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	_ = jwks.Body.Close()
	assert.Equal(t, http.StatusNotFound, jwks.StatusCode)
}

func TestRunRejectsProductionInjectedSignerSeamsBeforeListen(t *testing.T) {
	cfg := tokenAuthorityConfig(t, "test", "postgres://identity:***@127.0.0.1:5432/primer_identity?sslmode=disable")
	cfg.Env = "production"
	cfg.Key.AutoBootstrap = false
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
	require.NoError(t, cfg.Validate())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	err = app.Run(context.Background(), app.Options{
		Config: cfg, SkipMigrate: true, Signer: stubSigner{}, JWKS: stubJWKS{},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-must-not-leak")
	conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("process listened despite rejected production signer seam")
	}
}

func TestRunSignerAwareReadinessFailsGenericallyWithoutUsableKey(t *testing.T) {
	cfg := tokenAuthorityConfig(t, "test", dedicatedIdentityURL(t))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	baseURL := "http://" + ln.Addr().String()
	shutdown := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background(), app.Options{
			Config: cfg, Listener: ln, SkipMigrate: true, ShutdownSignal: shutdown,
			JWKS: failingJWKS{},
		})
	}()
	t.Cleanup(func() {
		select {
		case <-shutdown:
		default:
			close(shutdown)
		}
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = app.WaitReady(ctx, baseURL)

	resp, err := noFollow().Get(baseURL + "/readyz")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NotContains(t, strings.ToLower(string(body)), "corrupt")
	assert.NotContains(t, strings.ToLower(string(body)), "sql")
	assert.NotContains(t, strings.ToLower(string(body)), "signing")
	assert.NotContains(t, string(body), "postgres://")
}

func TestRunReadinessAndJWKSFailGenericallyOnCorruptSigner(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	baseURL, _, _ := startApp(t, cfg, app.Options{})

	pool := connectURL(t, url)
	var kid string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT kid FROM signing_keys WHERE status='active'`).Scan(&kid))
	var sealed []byte
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT sealed_private_key FROM signing_keys WHERE kid=$1`, kid).Scan(&sealed))
	require.Greater(t, len(sealed), 8)
	sealed[7] ^= 0xff
	_, err := pool.Exec(context.Background(), `UPDATE signing_keys SET sealed_private_key=$2 WHERE kid=$1`, kid, sealed)
	require.NoError(t, err)

	client := noFollow()
	ready, err := client.Get(baseURL + "/readyz")
	require.NoError(t, err)
	readyBody, err := io.ReadAll(ready.Body)
	_ = ready.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, ready.StatusCode)
	assert.NotContains(t, strings.ToLower(string(readyBody)), "corrupt")
	assert.NotContains(t, string(readyBody), kid)

	jwks, err := client.Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	jwksBody, err := io.ReadAll(jwks.Body)
	_ = jwks.Body.Close()
	require.NoError(t, err)
	assert.NotEqual(t, http.StatusOK, jwks.StatusCode)
	assert.NotContains(t, strings.ToLower(string(jwksBody)), "corrupt")
	assert.NotContains(t, string(jwksBody), kid)
	assert.NotContains(t, strings.ToLower(string(jwksBody)), "sql")
}

func TestRunNeverCallsCreateInitialActiveDirectly(t *testing.T) {
	src, err := os.ReadFile("app.go")
	require.NoError(t, err)
	assert.NotContains(t, string(src), "CreateInitialActive")
	assert.Contains(t, string(src), "Bootstrap")
	assert.Contains(t, string(src), ".Ready(")
}

func TestRunProductionTokenAuthorityWithEmptyKeysFailsClosedBeforeListenAndMigrate(t *testing.T) {
	url := dedicatedUnmigratedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	cfg.Env = "production"
	cfg.Key.AutoBootstrap = false
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
	cfg.StateSealKeys = encodedKey(0x11)
	cfg.StateSealActiveVersion = 1
	cfg.StateHashPeppers = encodedKey(0x22)
	cfg.StateHashActiveVersion = 1
	cfg.BrokerCookiePeppers = encodedKey(0x33)
	cfg.BrokerCookieActiveVersion = 1
	cfg.AuthorizationCodePeppers = encodedKey(0x44)
	cfg.AuthorizationCodeActiveVersion = 1
	cfg.StytchPublicToken = "public-token-live-example"
	require.NoError(t, cfg.Validate())
	assert.False(t, cfg.Key.AutoBootstrap)
	assert.True(t, cfg.TokenAuthorityEnabled())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	shutdown := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background(), app.Options{Config: cfg, ShutdownSignal: shutdown})
	}()
	select {
	case err = <-errCh:
		require.Error(t, err)
	case <-time.After(8 * time.Second):
		close(shutdown)
		t.Fatal("Run did not fail closed before listen/migration")
	}
	assert.NotContains(t, err.Error(), "secret-must-not-leak")
	assert.NotContains(t, strings.ToLower(err.Error()), "sql")

	conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("process listened despite empty production signing_keys")
	}

	pool, err := db.Connect(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	var relation *string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT to_regclass('public.signing_keys')::text`).Scan(&relation))
	assert.Nil(t, relation)
}

func TestRunTokenAuthorityWithoutAutoBootstrapRequiresPreseededKey(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	cfg.Key.AutoBootstrap = false
	assert.False(t, cfg.Key.AutoBootstrap)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	shutdown := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background(), app.Options{
			Config: cfg, SkipMigrate: true, ShutdownSignal: shutdown,
		})
	}()
	select {
	case err = <-errCh:
		require.Error(t, err)
	case <-time.After(8 * time.Second):
		close(shutdown)
		t.Fatal("Run did not fail closed without a usable active key")
	}

	conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("process listened without a usable active key")
	}

	n, countErr := repo.CountSigningKeysByStatus(context.Background(), connectURL(t, url), domain.SigningKeyStatusActive)
	require.NoError(t, countErr)
	assert.Zero(t, n)
}

func TestRunTokenAuthorityAcceptsExplicitlyPreseededActiveKey(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	cfg.Key.AutoBootstrap = false
	pool := connectURL(t, url)
	keySvc := keys.NewService(pool, cfg.Key, "test")
	t.Cleanup(func() { _ = keySvc.Close() })
	created, err := keySvc.CreateInitialActive(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, created.Kid)

	baseURL, _, _ := startApp(t, cfg, app.Options{})
	ready, err := noFollow().Get(baseURL + "/readyz")
	require.NoError(t, err)
	_ = ready.Body.Close()
	assert.Equal(t, http.StatusOK, ready.StatusCode)

	jwks, err := noFollow().Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	body, err := io.ReadAll(jwks.Body)
	_ = jwks.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, jwks.StatusCode)
	parsed, err := token.ParseJWKS(body)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, created.Kid, parsed[0].Kid)
}

func dedicatedUnmigratedIdentityURL(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("primer_identity_test"),
		tcpostgres.WithUsername("primer"),
		tcpostgres.WithPassword("primer"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

func TestJWKSEtagConditionalAndDeterministicOrder(t *testing.T) {
	cfg := tokenAuthorityConfig(t, "test", dedicatedIdentityURL(t))
	baseURL, _, _ := startApp(t, cfg, app.Options{})
	client := noFollow()

	first, err := client.Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	firstBody, err := io.ReadAll(first.Body)
	_ = first.Body.Close()
	require.NoError(t, err)
	etag := first.Header.Get("ETag")
	require.NotEmpty(t, etag)

	req, err := http.NewRequest(http.MethodGet, baseURL+"/.well-known/jwks.json", nil)
	require.NoError(t, err)
	req.Header.Set("If-None-Match", etag)
	second, err := client.Do(req)
	require.NoError(t, err)
	secondBody, err := io.ReadAll(second.Body)
	_ = second.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotModified, second.StatusCode)
	assert.Empty(t, secondBody)

	third, err := client.Get(baseURL + "/.well-known/jwks.json")
	require.NoError(t, err)
	thirdBody, err := io.ReadAll(third.Body)
	_ = third.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, firstBody, thirdBody)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(firstBody, &doc))
	_, hasPrivate := doc["d"]
	assert.False(t, hasPrivate)
}

func TestAuthorizationServerMetadataExactPathAndFields(t *testing.T) {
	url := dedicatedIdentityURL(t)
	cfg := tokenAuthorityConfig(t, "test", url)
	cfg.Issuer = "https://id.example.test/issuer/path"
	require.NoError(t, cfg.Validate())
	baseURL, _, _ := startApp(t, cfg, app.Options{})

	client := noFollow()
	resp, err := client.Get(baseURL + "/.well-known/oauth-authorization-server/issuer/path")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.LessOrEqual(t, len(body), 32*1024)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	assert.Equal(t, "public,max-age=300", resp.Header.Get("Cache-Control"))

	var meta map[string]any
	require.NoError(t, json.Unmarshal(body, &meta))
	assert.Equal(t, "https://id.example.test/issuer/path", meta["issuer"])
	assert.Equal(t, "https://id.example.test/oauth/authorize", meta["authorization_endpoint"])
	assert.Equal(t, "https://id.example.test/oauth/token", meta["token_endpoint"])
	assert.Equal(t, "https://id.example.test/oauth/revoke", meta["revocation_endpoint"])
	assert.Contains(t, string(body), "/oauth/revoke")
	assert.Equal(t, "https://id.example.test/.well-known/jwks.json", meta["jwks_uri"])
	assert.Equal(t, []any{"code"}, meta["response_types_supported"])
	assert.Equal(t, []any{"query"}, meta["response_modes_supported"])
	assert.Equal(t, []any{"authorization_code"}, meta["grant_types_supported"])
	assert.Equal(t, []any{"S256"}, meta["code_challenge_methods_supported"])
	assert.Equal(t, []any{"none", "client_secret_basic", "private_key_jwt"}, meta["token_endpoint_auth_methods_supported"])
	assert.Equal(t, []any{"ES256"}, meta["token_endpoint_auth_signing_alg_values_supported"])
	assert.Equal(t, []any{"none", "client_secret_basic", "private_key_jwt"}, meta["revocation_endpoint_auth_methods_supported"])
	assert.Equal(t, []any{"ES256"}, meta["revocation_endpoint_auth_signing_alg_values_supported"])
	assert.Equal(t, true, meta["authorization_response_iss_parameter_supported"])
	assert.NotContains(t, meta, "registration_endpoint")
	assert.NotContains(t, meta, "introspection_endpoint")
	assert.NotContains(t, meta, "device_authorization_endpoint")
	assert.NotContains(t, meta, "grant_types_supported_password")
	raw := string(body)
	assert.NotContains(t, raw, "password")
	assert.NotContains(t, raw, "implicit")
	assert.NotContains(t, raw, "urn:ietf:params:oauth:grant-type:device_code")

	root, err := client.Get(baseURL + "/.well-known/oauth-authorization-server")
	require.NoError(t, err)
	_ = root.Body.Close()
	assert.Equal(t, http.StatusNotFound, root.StatusCode)
}

type keyServiceSigner struct{ svc *keys.Service }

func (s keyServiceSigner) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	return s.svc.ActiveSigner(ctx)
}

type stubSigner struct{}

func (stubSigner) Current(context.Context) (token.Signer, *domain.SigningKey, error) {
	return nil, nil, token.ErrUnavailable
}

type stubJWKS struct{}

func (stubJWKS) PublicJWKS(context.Context) ([]domain.PublicJWK, error) {
	return nil, token.ErrUnavailable
}
func (stubJWKS) PublicSetETag(context.Context) (string, error) { return "", token.ErrUnavailable }

type failingJWKS struct{}

func (failingJWKS) PublicJWKS(context.Context) ([]domain.PublicJWK, error) {
	return nil, token.ErrUnavailable
}
func (failingJWKS) PublicSetETag(context.Context) (string, error) { return "", token.ErrUnavailable }
