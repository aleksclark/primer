package identityauth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/identityauth"
)

const (
	testIssuer   = "https://identity.primer.test"
	testAudience = "primer-lms"
	testKid      = "test-key-001"
	testClientID = "lms-spa"
	testScope    = "openid profile"
	testSubject  = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
)

// testKey generates a fresh P-256 key pair for testing.
func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

// mintTestToken creates a signed compact JWS for testing.
func mintTestToken(t *testing.T, key *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]string{
		"alg": "ES256",
		"typ": "at+jwt",
		"kid": kid,
	}
	headerJSON, err := json.Marshal(header)
	require.NoError(t, err)
	claimsJSON, err := json.Marshal(claims)
	require.NoError(t, err)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + payloadB64

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	require.NoError(t, err)

	// Fixed-width 32 bytes each (IEEE P1363 / JWS)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	sig := make([]byte, 64)
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):], sBytes)

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64
}

func defaultClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":       testIssuer,
		"sub":       testSubject,
		"aud":       testAudience,
		"exp":       now.Add(10 * time.Minute).Unix(),
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"jti":       "c0ffee00-1234-5678-9abc-def012345678",
		"client_id": testClientID,
		"scope":     testScope,
	}
}

func keysMap(t *testing.T, key *ecdsa.PrivateKey, kid string) map[string]*ecdsa.PublicKey {
	t.Helper()
	return map[string]*ecdsa.PublicKey{kid: &key.PublicKey}
}

func TestNewReturnsNilOnEmptyConfig(t *testing.T) {
	t.Parallel()
	assert.Nil(t, identityauth.New(identityauth.Config{}))
	assert.Nil(t, identityauth.New(identityauth.Config{Issuer: "x"}))
	assert.Nil(t, identityauth.New(identityauth.Config{Issuer: "x", Audience: "y"}))
	assert.Nil(t, identityauth.New(identityauth.Config{JWKSURL: "http://x"}))
}

func TestNilVerifierFailsClosed(t *testing.T) {
	t.Parallel()
	var v *identityauth.Verifier
	_, err := v.Verify(context.Background(), "anything")
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyValidToken(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))
	require.NotNil(t, v)

	now := time.Now().UTC().Truncate(time.Second)
	token := mintTestToken(t, key, testKid, defaultClaims(now))

	p, err := v.Verify(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, testSubject, p.Subject)
	assert.Equal(t, testIssuer, p.Issuer)
	assert.Equal(t, testAudience, p.Audience)
	assert.Equal(t, testClientID, p.ClientID)
	assert.Equal(t, testScope, p.Scope)
}

func TestVerifyRejectsWrongIssuer(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	claims := defaultClaims(now)
	claims["iss"] = "https://evil.example.com"
	token := mintTestToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsWrongAudience(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	claims := defaultClaims(now)
	claims["aud"] = "some-other-service"
	token := mintTestToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	past := time.Now().UTC().Add(-20 * time.Minute).Truncate(time.Second)
	claims := defaultClaims(past)
	claims["exp"] = past.Add(5 * time.Minute).Unix()
	token := mintTestToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsFutureToken(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	future := time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
	claims := defaultClaims(future)
	token := mintTestToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsTTLTooLong(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	claims := defaultClaims(now)
	claims["exp"] = now.Add(30 * time.Minute).Unix() // > 15 min max
	token := mintTestToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsNbfNotEqualIat(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	claims := defaultClaims(now)
	claims["nbf"] = now.Add(-1 * time.Minute).Unix()
	token := mintTestToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsUnknownKid(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	token := mintTestToken(t, key, "unknown-kid", defaultClaims(now))

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsWrongSignature(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	otherKey := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	// Signed with otherKey but verifier has key's public key.
	token := mintTestToken(t, otherKey, testKid, defaultClaims(now))

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsNonES256Algorithm(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	header := map[string]string{"alg": "RS256", "typ": "at+jwt", "kid": testKid}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(defaultClaims(time.Now().UTC().Truncate(time.Second)))

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	token := headerB64 + "." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64))

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsWrongTypHeader(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	header := map[string]string{"alg": "ES256", "typ": "JWT", "kid": testKid}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(defaultClaims(time.Now().UTC().Truncate(time.Second)))

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	token := headerB64 + "." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64))

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsGarbage(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	for _, tc := range []string{
		"",
		"not-a-token",
		"a.b",
		"a.b.c.d",
		"....",
		"abc",
	} {
		_, err := v.Verify(context.Background(), tc)
		assert.ErrorIs(t, err, identityauth.ErrDenied, "input: %q", tc)
	}
}

func TestVerifyRejectsCancelledContext(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	token := mintTestToken(t, key, testKid, defaultClaims(now))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := v.Verify(ctx, token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyRejectsRawStytchToken(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	// A typical Stytch session JWT uses typ "JWT", not "at+jwt", and has different claims.
	// Even if someone tried to pass one, the typ check rejects it.
	stytchLike := map[string]string{"alg": "ES256", "typ": "JWT", "kid": testKid}
	headerJSON, _ := json.Marshal(stytchLike)
	claimsJSON, _ := json.Marshal(map[string]any{
		"iss":                 "stytch.com/project/project-test-uuid",
		"sub":                 "user-test-uuid",
		"aud":                 []string{"project-test-uuid"},
		"exp":                 time.Now().Add(10 * time.Minute).Unix(),
		"iat":                 time.Now().Unix(),
		"nbf":                 time.Now().Unix(),
		"stytch_session_type": "session",
	})
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	fakeToken := headerB64 + "." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64))

	_, err := v.Verify(context.Background(), fakeToken)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestVerifyFetchesJWKS(t *testing.T) {
	t.Parallel()
	key := testKey(t)

	// Serve JWKS from a test server.
	jwks := serveJWKS(t, key, testKid)
	defer jwks.Close()

	v := identityauth.New(identityauth.Config{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
	})
	require.NotNil(t, v)

	now := time.Now().UTC().Truncate(time.Second)
	token := mintTestToken(t, key, testKid, defaultClaims(now))

	p, err := v.Verify(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, testSubject, p.Subject)
}

func TestVerifyFailsWhenJWKSUnavailable(t *testing.T) {
	t.Parallel()
	key := testKey(t)

	// Point at an unreachable URL.
	v := identityauth.New(identityauth.Config{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  "http://127.0.0.1:1/unreachable",
	})
	require.NotNil(t, v)

	now := time.Now().UTC().Truncate(time.Second)
	token := mintTestToken(t, key, testKid, defaultClaims(now))

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

func TestIsJWT(t *testing.T) {
	t.Parallel()
	assert.True(t, identityauth.IsJWT("a.b.c"))
	assert.True(t, identityauth.IsJWT("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.MEUCIQDxyz"))
	assert.False(t, identityauth.IsJWT(""))
	assert.False(t, identityauth.IsJWT("opaque-token-no-dots"))
	assert.False(t, identityauth.IsJWT("a.b"))
	assert.False(t, identityauth.IsJWT("a.b.c.d"))
	assert.False(t, identityauth.IsJWT(".b.c"))
	assert.False(t, identityauth.IsJWT("a..c"))
	assert.False(t, identityauth.IsJWT("a.b."))
}

func TestVerifyRejectsMissingClaims(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	for _, remove := range []string{"iss", "sub", "aud", "jti"} {
		claims := defaultClaims(now)
		delete(claims, remove)
		token := mintTestToken(t, key, testKid, claims)
		_, err := v.Verify(context.Background(), token)
		assert.ErrorIs(t, err, identityauth.ErrDenied, "missing %s should be rejected", remove)
	}
}

// --- Helpers ---

func serveJWKS(t *testing.T, key *ecdsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	xBytes := key.PublicKey.X.Bytes()
	yBytes := key.PublicKey.Y.Bytes()
	// Pad to 32 bytes.
	xPad := make([]byte, 32)
	yPad := make([]byte, 32)
	copy(xPad[32-len(xBytes):], xBytes)
	copy(yPad[32-len(yBytes):], yBytes)

	jwksDoc := map[string]any{
		"keys": []map[string]string{
			{
				"kty": "EC",
				"crv": "P-256",
				"use": "sig",
				"alg": "ES256",
				"kid": kid,
				"x":   base64.RawURLEncoding.EncodeToString(xPad),
				"y":   base64.RawURLEncoding.EncodeToString(yPad),
			},
		},
	}
	jwksJSON, err := json.Marshal(jwksDoc)
	require.NoError(t, err)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(jwksJSON))
	}))
}

func TestVerifyServiceSubject(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	v := identityauth.NewWithKeys(testIssuer, testAudience, keysMap(t, key, testKid))

	now := time.Now().UTC().Truncate(time.Second)
	claims := defaultClaims(now)
	claims["sub"] = "identity:svc:primer-lms"
	token := mintTestToken(t, key, testKid, claims)

	p, err := v.Verify(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "identity:svc:primer-lms", p.Subject)
}

// Verify the constant values are not accidentally broken.
func TestConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5*time.Second, identityauth.MaxClockSkew)
	assert.Equal(t, 15*time.Minute, identityauth.MaxTokenLifetime)
}

// Ensure JWKS document parsing rejects malformed documents.
func TestVerifyRejectsEmptyJWKS(t *testing.T) {
	t.Parallel()
	key := testKey(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"keys":[]}`)
	}))
	defer srv.Close()

	v := identityauth.New(identityauth.Config{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  srv.URL,
	})
	require.NotNil(t, v)

	now := time.Now().UTC().Truncate(time.Second)
	token := mintTestToken(t, key, testKid, defaultClaims(now))

	_, err := v.Verify(context.Background(), token)
	assert.ErrorIs(t, err, identityauth.ErrDenied)
}

// Suppress unused import warning — big is only used in mintTestToken via ecdsa.Sign.
var _ = new(big.Int)
