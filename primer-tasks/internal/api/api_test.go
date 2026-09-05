package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSealReportsRandomSourceFailure(t *testing.T) {
	original := cryptoRandRead
	cryptoRandRead = func([]byte) (int, error) { return 0, fmt.Errorf("entropy unavailable") }
	t.Cleanup(func() { cryptoRandRead = original })
	s := NewWithAuth(nil, "test", AuthConfig{SessionSecret: []byte("state-secret")})
	if _, err := s.seal("verifier"); err == nil {
		t.Fatal("expected random source failure")
	}
}

func hmacSHA256(key, value []byte) []byte {
	// The production helper uses crypto/hmac; this small test helper keeps the
	// fixture readable while producing the exact same HMAC-SHA256 bytes.
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(value)
	return m.Sum(nil)
}
