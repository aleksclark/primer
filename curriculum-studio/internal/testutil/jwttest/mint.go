// Package jwttest mints protocol-compatible Primer ES256 access tokens
// and JWKS documents for Studio tests. It lives outside the Studio
// process path: production Studio code must not import this package
// to issue tokens.
package jwttest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Audience is the frozen Studio JWT audience.
const Audience = "curriculum-studio"

// ClientID is a registered public client_id (not an internal UUID).
const ClientID = "studio-bff"

// Issuer is the default test Identity issuer.
const Issuer = "https://identity.example.test"

// Keypair is an ES256 signing key plus its public JWK.
type Keypair struct {
	Private *ecdsa.PrivateKey
	Kid     string
	JWK     map[string]string
}

// GenerateKey returns a P-256 signing key with a random kid.
func GenerateKey(t testing.TB) *Keypair {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	kid := uuid.NewString()
	return &Keypair{
		Private: priv,
		Kid:     kid,
		JWK: map[string]string{
			"kty": "EC",
			"crv": "P-256",
			"use": "sig",
			"alg": "ES256",
			"kid": kid,
			"x":   b64Int(priv.PublicKey.X),
			"y":   b64Int(priv.PublicKey.Y),
		},
	}
}

// JWKSDocument returns a JWKS object containing the given keys.
func JWKSDocument(keys ...*Keypair) map[string]any {
	out := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.JWK)
	}
	return map[string]any{"keys": out}
}

// Claims is the IB2-compatible access-token claim set.
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
	Extra     map[string]any
}

// ValidHumanClaims returns a 5-minute human token claim set.
func ValidHumanClaims(now time.Time, sub string) Claims {
	if sub == "" {
		sub = uuid.NewString()
	}
	return Claims{
		Issuer:    Issuer,
		Subject:   sub,
		Audience:  Audience,
		ClientID:  ClientID,
		Scope:     "openid",
		IssuedAt:  now.UTC().Truncate(time.Second),
		NotBefore: now.UTC().Truncate(time.Second),
		ExpiresAt: now.UTC().Truncate(time.Second).Add(5 * time.Minute),
		JTI:       uuid.NewString(),
	}
}

// ValidServiceClaims returns a machine-principal claim set.
func ValidServiceClaims(now time.Time, serviceID, scope string) Claims {
	c := ValidHumanClaims(now, "identity:svc:"+serviceID)
	c.Scope = scope
	return c
}

// Mint signs claims with the keypair as a compact ES256 JWT (typ=at+jwt).
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
			continue
		}
		payload[k] = v
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
	h64 := b64(hb)
	p64 := b64(pb)
	input := h64 + "." + p64
	sum := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key.Private, sum[:])
	require.NoError(t, err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + b64(sig)
}

func b64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func b64Int(n *big.Int) string {
	buf := make([]byte, 32)
	n.FillBytes(buf)
	return b64(buf)
}

// MustParseKid is a tiny helper for tests that need the kid string.
func MustParseKid(t testing.TB, key *Keypair) string {
	t.Helper()
	if key == nil || key.Kid == "" {
		t.Fatal("missing kid")
	}
	return key.Kid
}

// HumanSubjectRef builds identity:<uuid> for a human account sub.
func HumanSubjectRef(sub string) string {
	return fmt.Sprintf("identity:%s", sub)
}
