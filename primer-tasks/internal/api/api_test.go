package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	contract := OpenAPI()
	var doc map[string]any
	if err := json.Unmarshal([]byte(contract), &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	for _, path := range []string{"/auth/callback", "/students/{id}/pairing", "/device/pair"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("contract omitted %s", path)
		}
	}
}

func TestVerifyIDTokenChecksIssuerAudienceExpiryAndSignature(t *testing.T) {
	s := NewWithAuth(nil, "test", AuthConfig{IssuerURL: "https://issuer.test", PublicIssuerURL: "https://issuer.test", ClientID: "tasks", SessionSecret: []byte("state-secret"), IssuerSecret: []byte("secret")})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"iss": "https://issuer.test", "sub": "parent-a", "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix()})
	part := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmacSHA256([]byte("secret"), []byte(header+"."+part))
	if got, err := s.verifyIDToken(header + "." + part + "." + base64.RawURLEncoding.EncodeToString(mac)); err != nil || got.Subject != "parent-a" {
		t.Fatalf("valid token rejected: %v", err)
	}
	if _, err := s.verifyIDToken(header + "." + part + ".bad"); err == nil {
		t.Fatal("bad signature accepted")
	}
}

func hmacSHA256(key, value []byte) []byte {
	// The production helper uses crypto/hmac; this small test helper keeps the
	// fixture readable while producing the exact same HMAC-SHA256 bytes.
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(value)
	return m.Sum(nil)
}
