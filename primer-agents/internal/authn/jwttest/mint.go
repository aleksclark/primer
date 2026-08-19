// Package jwttest mints protocol-compatible Primer ES256 access tokens and
// JWKS documents for primer-agents tests. Production primer-agents code must
// never import this package.
package jwttest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Audience is the frozen agents JWT audience.
const Audience = "primer-agents"

// DefaultClientID is a registered public client_id used in test fixtures.
const DefaultClientID = "agents-bff"

// DefaultIssuer is the default test Identity issuer (loopback httptest server
// overrides this at the URL level; the Issuer claim must match).
const DefaultIssuer = "https://identity.example.test"

// AllScopes is the union of all agents scopes; tests that don't need
// restriction use this.
const AllScopes = "agents:runs:write agents:runs:read agents:runs:cancel agents:sessions:write agents:sessions:read agents:jobs:write agents:jobs:read agents:student:session"

// Keypair is an ES256 signing key with its public JWK.
type Keypair struct {
	Private *ecdsa.PrivateKey
	Kid     string
	JWK     map[string]string
}

// GenerateKey returns a fresh P-256 signing key.
func GenerateKey(t testing.TB) *Keypair {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	kid := uuid.NewString()
	return &Keypair{
		Private: priv,
		Kid:     kid,
		JWK: map[string]string{
			"kty": "EC", "crv": "P-256", "use": "sig", "alg": "ES256",
			"kid": kid,
			"x":   b64Int(priv.PublicKey.X),
			"y":   b64Int(priv.PublicKey.Y),
		},
	}
}

// JWKSDoc returns a JWKS object containing the given keys.
func JWKSDoc(keys ...*Keypair) map[string]any {
	list := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		list = append(list, k.JWK)
	}
	return map[string]any{"keys": list}
}

// Claims carries the full claim set for a test token.
type Claims struct {
	Issuer    string
	Subject   string
	Audience  string
	ClientID  string
	Scope     string
	IssuedAt  time.Time
	NotBefore time.Time
	ExpiresAt time.Time
	JTI       string
	// Extra allows per-test claim overrides; nil value deletes the key.
	Extra map[string]any
}

// ValidHumanClaims returns a 5-minute human-principal claim set.
func ValidHumanClaims(now time.Time, sub string) Claims {
	if sub == "" {
		sub = "identity:" + uuid.NewString()
	}
	t := now.UTC().Truncate(time.Second)
	return Claims{
		Issuer:    DefaultIssuer,
		Subject:   sub,
		Audience:  Audience,
		ClientID:  DefaultClientID,
		Scope:     AllScopes,
		IssuedAt:  t,
		NotBefore: t,
		ExpiresAt: t.Add(5 * time.Minute),
		JTI:       uuid.NewString(),
	}
}

// ValidServiceClaims returns a 5-minute service-principal claim set.
func ValidServiceClaims(now time.Time, serviceID, scope string) Claims {
	c := ValidHumanClaims(now, "identity:svc:"+serviceID)
	c.Scope = scope
	return c
}

// Mint signs c with key and returns a compact ES256 JWT (typ=at+jwt).
func Mint(t testing.TB, key *Keypair, c Claims) string {
	t.Helper()
	header := map[string]string{"alg": "ES256", "typ": "at+jwt", "kid": key.Kid}
	payload := map[string]any{
		"iss":       c.Issuer,
		"sub":       c.Subject,
		"aud":       c.Audience,
		"exp":       c.ExpiresAt.UTC().Unix(),
		"iat":       c.IssuedAt.UTC().Unix(),
		"nbf":       c.NotBefore.UTC().Unix(),
		"jti":       c.JTI,
		"client_id": c.ClientID,
		"scope":     c.Scope,
	}
	for k, v := range c.Extra {
		if v == nil {
			delete(payload, k)
		} else {
			payload[k] = v
		}
	}
	return Sign(t, key, header, payload)
}

// Sign encodes and ES256-signs arbitrary header/payload maps.
func Sign(t testing.TB, key *Keypair, header map[string]string, payload map[string]any) string {
	t.Helper()
	hb, err := json.Marshal(header)
	require.NoError(t, err)
	pb, err := json.Marshal(payload)
	require.NoError(t, err)
	h64, p64 := b64(hb), b64(pb)
	sum := sha256.Sum256([]byte(h64 + "." + p64))
	r, s, err := ecdsa.Sign(rand.Reader, key.Private, sum[:])
	require.NoError(t, err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return h64 + "." + p64 + "." + b64(sig)
}

func b64(raw []byte) string       { return base64.RawURLEncoding.EncodeToString(raw) }
func b64Int(n *big.Int) string    { buf := make([]byte, 32); n.FillBytes(buf); return b64(buf) }
