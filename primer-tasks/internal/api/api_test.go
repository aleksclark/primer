package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPKCEChallengeIsRFC7636S256(t *testing.T) {
	got := pkceChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got != want {
		t.Fatalf("PKCE challenge = %q, want %q", got, want)
	}
	if len(got) != 43 || strings.ContainsAny(got, "+/=") {
		t.Fatalf("invalid base64url PKCE challenge %q", got)
	}
}

func TestStateSealRoundTripAndTamperRejection(t *testing.T) {
	s := NewWithAuth(nil, "test", AuthConfig{SessionSecret: []byte("a durable state secret")})
	sealed, err := s.seal("verifier")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := s.open(sealed)
	if err != nil || plain != "verifier" {
		t.Fatalf("round trip: %q %v", plain, err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = s.open(sealed); err == nil {
		t.Fatal("tampered state was accepted")
	}
}

func TestRoutesExposeCallbackAndGeneratedContract(t *testing.T) {
	r := New(nil, "test").Routes()
	for _, path := range []string{"/health", "/auth/callback", "/openapi.yaml"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if path == "/health" && rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		if path == "/auth/callback" && rec.Code != http.StatusBadRequest {
			t.Fatalf("callback route missing: %d", rec.Code)
		}
	}
	paths := New(nil, "test").humaAPI().OpenAPI().Paths
	for _, path := range []string{"/auth/callback", "/students/{id}/pairing", "/device/pair"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("contract omitted %s", path)
		}
	}
}

func TestVerifyIDTokenChecksIssuerAudienceExpiryAndSignature(t *testing.T) {
	s := NewWithAuth(nil, "test", AuthConfig{IssuerURL: "https://issuer.test", PublicIssuerURL: "https://issuer.test", ClientID: "tasks", SessionSecret: []byte("state-secret"), IssuerSecret: []byte("secret")})
	makeToken := func(claims map[string]any) string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
		payload, _ := json.Marshal(claims)
		part := base64.RawURLEncoding.EncodeToString(payload)
		mac := hmacSHA256([]byte("secret"), []byte(header+"."+part))
		return header + "." + part + "." + base64.RawURLEncoding.EncodeToString(mac)
	}
	valid := map[string]any{"iss": "https://issuer.test", "sub": "parent-a", "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix()}
	if got, err := s.verifyIDToken(makeToken(valid)); err != nil || got.Subject != "parent-a" {
		t.Fatalf("valid token rejected: %v", err)
	}
	arrayAudience := make(map[string]any, len(valid))
	for k, v := range valid {
		arrayAudience[k] = v
	}
	arrayAudience["aud"] = []any{"other", "tasks"}
	if _, err := s.verifyIDToken(makeToken(arrayAudience)); err != nil {
		t.Fatalf("array audience rejected: %v", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"bad issuer":      func(c map[string]any) { c["iss"] = "https://other.example" },
		"bad audience":    func(c map[string]any) { c["aud"] = "other" },
		"expired":         func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"missing subject": func(c map[string]any) { delete(c, "sub") },
	} {
		t.Run(name, func(t *testing.T) {
			claims := make(map[string]any, len(valid))
			for k, v := range valid {
				claims[k] = v
			}
			edit(claims)
			if _, err := s.verifyIDToken(makeToken(claims)); err == nil {
				t.Fatal("invalid claims accepted")
			}
		})
	}
	if _, err := s.verifyIDToken("malformed"); err == nil {
		t.Fatal("malformed token accepted")
	}
	parts := strings.Split(makeToken(valid), ".")
	parts[2] = "bad"
	if _, err := s.verifyIDToken(strings.Join(parts, ".")); err == nil {
		t.Fatal("bad signature accepted")
	}
}

func TestOpenRejectsInvalidCiphertextAndLoginRequiresIdentity(t *testing.T) {
	s := NewWithAuth(nil, "test", AuthConfig{SessionSecret: []byte("state-secret")})
	if _, err := s.open(nil); err == nil {
		t.Fatal("empty sealed state accepted")
	}
	if _, err := s.open([]byte("too short")); err == nil {
		t.Fatal("short sealed state accepted")
	}
	unconfigured := NewWithAuth(nil, "test", AuthConfig{IssuerURL: "", ClientID: ""})
	rec := httptest.NewRecorder()
	unconfigured.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured login = %d, want 503", rec.Code)
	}
}

func failCryptoRand(t *testing.T) {
	t.Helper()
	original := cryptoRandRead
	cryptoRandRead = func([]byte) (int, error) { return 0, fmt.Errorf("entropy unavailable") }
	t.Cleanup(func() { cryptoRandRead = original })
}

func TestSealReportsRandomSourceFailure(t *testing.T) {
	failCryptoRand(t)
	s := NewWithAuth(nil, "test", AuthConfig{SessionSecret: []byte("state-secret")})
	if _, err := s.seal("verifier"); err == nil {
		t.Fatal("expected random source failure")
	}
}

func TestRandomStringAndPairingCodeFailClosedOnEntropyLoss(t *testing.T) {
	failCryptoRand(t)
	if got, err := randomString(32); err == nil || !errors.Is(err, errEntropyUnavailable) || got != "" {
		t.Fatalf("randomString = %q, %v", got, err)
	}
	if got, err := pairingCode(); err == nil || !errors.Is(err, errEntropyUnavailable) || got != "" {
		t.Fatalf("pairingCode = %q, %v", got, err)
	}
}

func TestRandomStringFailsClosedOnShortRead(t *testing.T) {
	original := cryptoRandRead
	cryptoRandRead = func(dst []byte) (int, error) {
		if len(dst) == 0 {
			return 0, nil
		}
		dst[0] = 1
		return 1, nil
	}
	t.Cleanup(func() { cryptoRandRead = original })
	if got, err := randomString(32); err == nil || !errors.Is(err, errEntropyUnavailable) || got != "" {
		t.Fatalf("short randomString = %q, %v", got, err)
	}
}

func TestLoginFailsClosedWhenEntropyUnavailable(t *testing.T) {
	failCryptoRand(t)
	s := NewWithAuth(nil, "test", AuthConfig{Mode: "test", IssuerURL: "http://issuer.test", ClientID: "tasks", SessionSecret: []byte("state-secret")})
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("login without entropy = %d, want 503", rec.Code)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "cookie") {
		t.Fatalf("entropy failure leaked session material: %s", rec.Body.String())
	}
}

func TestPairingClientKeyIgnoresForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/student/pair", nil)
	req.RemoteAddr = "192.0.2.9:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.8, 10.0.0.1")
	if got := pairingClientKey(req); got != "192.0.2.9" {
		t.Fatalf("client key = %q, want RemoteAddr host", got)
	}
	req.RemoteAddr = "2001:db8::1"
	if got := pairingClientKey(req); got != "2001:db8::1" {
		t.Fatalf("unsplit client key = %q", got)
	}
	req.RemoteAddr = ""
	if got := pairingClientKey(req); got != "unknown" {
		t.Fatalf("empty client key = %q", got)
	}
}

func TestRequestOriginPrefersForwardedProto(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	req.Host = "127.0.0.1:39077"
	if got := requestOrigin(req, req.Host); got != "http://127.0.0.1:39077" {
		t.Fatalf("loopback origin = %q", got)
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := requestOrigin(req, "web.example.test"); got != "https://web.example.test" {
		t.Fatalf("forwarded origin = %q", got)
	}
}

func TestEnvOrFallsBack(t *testing.T) {
	t.Setenv("TASKS_HOST_STACK_TEST_KEY", "")
	if got := envOr("TASKS_HOST_STACK_TEST_KEY", "fallback"); got != "fallback" {
		t.Fatalf("envOr empty = %q", got)
	}
	t.Setenv("TASKS_HOST_STACK_TEST_KEY", "set")
	if got := envOr("TASKS_HOST_STACK_TEST_KEY", "fallback"); got != "set" {
		t.Fatalf("envOr set = %q", got)
	}
}

func TestOIDCModeRejectsHS256EvenWithValidHMAC(t *testing.T) {
	s := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: "https://identity.example", PublicIssuerURL: "https://identity.example", ClientID: "tasks", IssuerSecret: []byte("secret")})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"iss": "https://identity.example", "sub": "parent-a", "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix()})
	part := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmacSHA256([]byte("secret"), []byte(header+"."+part))
	token := header + "." + part + "." + base64.RawURLEncoding.EncodeToString(mac)
	if _, err := s.verifyIDToken(token); err == nil {
		t.Fatal("oidc mode accepted HS256")
	}
}

func TestOIDCVerifierUsesDiscoveryJWKSAndRejectsBadAlgKidAndSignature(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kid := "kid-1"
	x := base64.RawURLEncoding.EncodeToString(priv.X.FillBytes(make([]byte, 32)))
	y := base64.RawURLEncoding.EncodeToString(priv.Y.FillBytes(make([]byte, 32)))
	makeToken := func(alg, tokenKid, iss string, claims map[string]any, signer *ecdsa.PrivateKey) string {
		header, _ := json.Marshal(map[string]string{"alg": alg, "kid": tokenKid, "typ": "JWT"})
		h := base64.RawURLEncoding.EncodeToString(header)
		payload, _ := json.Marshal(claims)
		p := base64.RawURLEncoding.EncodeToString(payload)
		unsigned := h + "." + p
		sum := sha256.Sum256([]byte(unsigned))
		r, ss, err := ecdsa.Sign(rand.Reader, signer, sum[:])
		if err != nil {
			t.Fatal(err)
		}
		sig := append(r.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
		return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
	}
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer.URL, "jwks_uri": issuer.URL + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "kid": kid, "x": x, "y": y,
			}}})
		case "/oauth/token":
			claims := map[string]any{"iss": issuer.URL, "sub": "parent-a", "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix()}
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": makeToken("ES256", kid, issuer.URL, claims, priv)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()
	s := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: issuer.URL, PublicIssuerURL: issuer.URL, ClientID: "tasks"})
	valid := map[string]any{"iss": issuer.URL, "sub": "parent-a", "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix()}
	if got, err := s.verifyIDToken(makeToken("ES256", kid, issuer.URL, valid, priv)); err != nil || got.Subject != "parent-a" {
		t.Fatalf("valid OIDC token rejected: %v", err)
	}
	if _, err := s.verifyIDToken(makeToken("HS256", kid, issuer.URL, valid, priv)); err == nil {
		t.Fatal("HS256 accepted in oidc mode")
	}
	if _, err := s.verifyIDToken(makeToken("ES256", "missing", issuer.URL, valid, priv)); err == nil {
		t.Fatal("unknown kid accepted")
	}
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.verifyIDToken(makeToken("ES256", kid, issuer.URL, valid, other)); err == nil {
		t.Fatal("foreign signature accepted")
	}
	if got, err := s.exchange(context.Background(), "code", "verifier", "https://tasks.example/auth/callback", "tasks"); err != nil || got.Subject != "parent-a" {
		t.Fatalf("oidc exchange rejected: %v", err)
	}
	badHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer badHTTP.Close()
	bad := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: badHTTP.URL, PublicIssuerURL: issuer.URL, ClientID: "tasks"})
	if _, err := bad.exchange(context.Background(), "code", "verifier", "https://tasks.example/auth/callback", "tasks"); err == nil {
		t.Fatal("failed token endpoint accepted")
	}
	if _, err := bad.fetchOIDCKeys(context.Background()); err == nil {
		t.Fatal("failed discovery accepted")
	}
	var jwksOnly *httptest.Server
	jwksOnly = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": jwksOnly.URL, "jwks_uri": jwksOnly.URL + "/jwks"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "alg": "RS256", "kid": "rsa", "n": "x", "e": "AQAB",
			}}})
		}
	}))
	defer jwksOnly.Close()
	rsaOnly := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: jwksOnly.URL, PublicIssuerURL: jwksOnly.URL, ClientID: "tasks"})
	if _, err := rsaOnly.fetchOIDCKeys(context.Background()); err == nil {
		t.Fatal("RSA-only JWKS accepted")
	}
	missingURI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": "https://identity.example"})
	}))
	defer missingURI.Close()
	missing := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: missingURI.URL, PublicIssuerURL: "https://identity.example", ClientID: "tasks"})
	if _, err := missing.fetchOIDCKeys(context.Background()); err == nil {
		t.Fatal("discovery without jwks_uri accepted")
	}
	mismatch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": "https://other.example", "jwks_uri": "https://other.example/jwks"})
	}))
	defer mismatch.Close()
	wrongIss := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: mismatch.URL, PublicIssuerURL: "https://identity.example", ClientID: "tasks"})
	if _, err := wrongIss.fetchOIDCKeys(context.Background()); err == nil {
		t.Fatal("discovery issuer mismatch accepted")
	}
	emptyToken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "x"})
	}))
	defer emptyToken.Close()
	noID := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: emptyToken.URL, PublicIssuerURL: emptyToken.URL, ClientID: "tasks"})
	if _, err := noID.exchange(context.Background(), "code", "verifier", "https://tasks.example/auth/callback", "tasks"); err == nil {
		t.Fatal("token response without id_token accepted")
	}
}

func TestOIDCJWKSCacheHitExpiryUnknownKidAndTimeout(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kid := "kid-cache"
	x := base64.RawURLEncoding.EncodeToString(priv.X.FillBytes(make([]byte, 32)))
	y := base64.RawURLEncoding.EncodeToString(priv.Y.FillBytes(make([]byte, 32)))
	var fetches atomic.Int32
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer.URL, "jwks_uri": issuer.URL + "/jwks"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "kid": kid, "x": x, "y": y,
			}}})
		}
	}))
	defer issuer.Close()
	s := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: issuer.URL, PublicIssuerURL: issuer.URL, ClientID: "tasks"})
	makeToken := func() string {
		header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
		h := base64.RawURLEncoding.EncodeToString(header)
		payload, _ := json.Marshal(map[string]any{"iss": issuer.URL, "sub": "parent-a", "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix()})
		p := base64.RawURLEncoding.EncodeToString(payload)
		unsigned := h + "." + p
		sum := sha256.Sum256([]byte(unsigned))
		r, ss, err := ecdsa.Sign(rand.Reader, priv, sum[:])
		if err != nil {
			t.Fatal(err)
		}
		sig := append(r.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
		return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
	}
	token := makeToken()
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.verifyIDToken(token)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent first-use: %v", err)
		}
	}
	first := fetches.Load()
	if first == 0 {
		t.Fatal("expected discovery/JWKS fetch")
	}
	if _, err := s.verifyIDToken(token); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != first {
		t.Fatalf("cache hit refetched JWKS: %d then %d", first, fetches.Load())
	}
	badKidHeader, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": "missing", "typ": "JWT"})
	parts := strings.Split(token, ".")
	parts[0] = base64.RawURLEncoding.EncodeToString(badKidHeader)
	if _, err := s.verifyIDToken(strings.Join(parts, ".")); err == nil {
		t.Fatal("unknown kid accepted from cache")
	}
	s.jwks.expires = time.Now().Add(-time.Second)
	if _, err := s.verifyIDToken(token); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() <= first {
		t.Fatal("expired cache did not refetch")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": "https://identity.example"})
	}))
	defer slow.Close()
	timeoutClient := &http.Client{Timeout: 20 * time.Millisecond}
	cancelServer := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: slow.URL, PublicIssuerURL: slow.URL, ClientID: "tasks"})
	cancelServer.httpClient = timeoutClient
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := cancelServer.fetchOIDCKeys(ctx); err == nil {
		t.Fatal("expected timeout/cancellation")
	}
}

func TestOpenAPIHelpersEmitContract(t *testing.T) {
	yaml := OpenAPI()
	if !strings.Contains(yaml, "Primer Tasks") || !strings.Contains(yaml, "/student/pair") {
		t.Fatal("OpenAPI YAML missing pairing contract")
	}
	js := OpenAPIJSON()
	if !strings.Contains(js, "student-pair") {
		t.Fatal("OpenAPI JSON missing student-pair")
	}
}

func TestConstructorsInitializeJWKSCacheAndHTTPClient(t *testing.T) {
	s := New(nil, "test")
	if s.jwks == nil || s.httpClient == nil {
		t.Fatal("New left JWKS cache or HTTP client unset")
	}
	custom := NewWithAuth(nil, "production", AuthConfig{Mode: "oidc", IssuerURL: "https://identity.example", ClientID: "tasks"})
	if custom.jwks == nil || custom.httpClient == nil || custom.Auth.Mode != "oidc" {
		t.Fatal("NewWithAuth left verifier state unset")
	}
}

func TestDecodeRejectsInvalidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/student/pair", strings.NewReader("{not json"))
	var in struct {
		Code string `json:"code"`
	}
	if decode(rec, req, &in) {
		t.Fatal("invalid JSON accepted")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("decode status = %d", rec.Code)
	}
}

func TestHTTPDoUsesSharedClientWhenUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	s := NewWithAuth(nil, "test", AuthConfig{Mode: "test"})
	s.httpClient = nil
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.httpDo(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("httpDo = %d", res.StatusCode)
	}
}

func TestPairingCodeHas128Bits(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 32; i++ {
		code, err := pairingCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 32 {
			t.Fatalf("pairingCode length = %d", len(code))
		}
		for _, r := range code {
			if (r < '0' || r > '9') && (r < 'A' || r > 'F') {
				t.Fatalf("pairingCode is not uppercase hex: %q", code)
			}
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("pairingCode collision %q", code)
		}
		seen[code] = struct{}{}
	}
}

func hmacSHA256(key, value []byte) []byte {
	// The production helper uses crypto/hmac; this small test helper keeps the
	// fixture readable while producing the exact same HMAC-SHA256 bytes.
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(value)
	return m.Sum(nil)
}
