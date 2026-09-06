package parentauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.clark.team/aleksclark/authstack/auth"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestJWKSRejectsMalformedEmptyPrivateAndRedirectedResponses(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: key, KeyID: "private"}}})
	if err != nil {
		t.Fatal(err)
	}
	missingID, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey}}})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"malformed": "invalid", "empty": `{"keys":[]}`, "private": string(private), "missing kid": string(missingID), "oversized": strings.Repeat("x", (1<<20)+1)} {
		t.Run(name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer s.Close()
			if _, err := New(context.Background(), "https://clerk.example", s.URL); err == nil {
				t.Fatal("invalid JWKS accepted")
			}
		})
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://other.example/jwks", 302) }))
	defer s.Close()
	if _, err := New(context.Background(), "https://clerk.example", s.URL); err == nil {
		t.Fatal("JWKS redirect accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(ctx, "https://clerk.example", s.URL); err == nil {
		t.Fatal("canceled load accepted")
	}
}

func TestPublishedVerifierKeyRotationAndBoundedOutage(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	next, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	active := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "one", Algorithm: "RS256", Use: "sig"}
	outage := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if outage {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{active}})
	}))
	defer server.Close()
	verifier, err := New(context.Background(), "https://clerk.example", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	policy := auth.AuthenticationPolicy{AcceptedCredentials: []auth.CredentialKind{auth.CredentialSession}, AuthorizedParties: []string{"https://api.primerlms.com"}}
	token := func(key *rsa.PrivateKey, kid string) auth.Credential {
		signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, nil)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := jwt.Signed(signer).Claims(map[string]any{"iss": "https://clerk.example", "sub": "user", "sid": "session", "azp": "https://api.primerlms.com", "exp": time.Now().Add(time.Minute).Unix()}).Serialize()
		if e != nil {
			t.Fatal(e)
		}
		return auth.Credential{Token: raw}
	}
	if p, e := verifier.Authenticate(context.Background(), token(key, "one"), policy); e != nil || p.SessionID != "session" {
		t.Fatal("valid published verifier token rejected")
	}
	active = jose.JSONWebKey{Key: &next.PublicKey, KeyID: "two", Algorithm: "RS256", Use: "sig"}
	verifier.attempted = time.Now().Add(-time.Minute)
	if _, e := verifier.Authenticate(context.Background(), token(next, "two"), policy); e != nil {
		t.Fatal("unknown signing key did not refresh")
	}
	outage = true
	verifier.loaded = time.Now().Add(-10 * time.Minute)
	verifier.attempted = time.Now().Add(-time.Minute)
	if _, e := verifier.Authenticate(context.Background(), token(next, "two"), policy); e != nil {
		t.Fatal("bounded cached key rejected")
	}
	verifier.loaded = time.Now().Add(-2 * time.Hour)
	if _, e := verifier.Authenticate(context.Background(), token(next, "two"), policy); e == nil {
		t.Fatal("stale keys accepted during outage")
	}
	if _, e := New(context.Background(), "https://clerk.example", server.URL); e == nil {
		t.Fatal("startup accepted unavailable JWKS")
	}
}
