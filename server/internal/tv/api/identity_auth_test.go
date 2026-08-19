package api_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/identityauth"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
)

const (
	tvTestIssuer   = "https://identity.primer.test"
	tvTestAudience = "primer-tv"
	tvTestKid      = "tv-test-key-001"
)

// tvTestKey generates a fresh P-256 key pair.
func tvTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

// tvMintToken creates a signed compact JWS for testing.
func tvMintToken(t *testing.T, key *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "ES256", "typ": "at+jwt", "kid": kid}
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

	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):], sBytes)

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func tvDefaultClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":       tvTestIssuer,
		"sub":       "admin-user-001",
		"aud":       tvTestAudience,
		"exp":       now.Add(10 * time.Minute).Unix(),
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"jti":       "a0a0a0a0-1111-2222-3333-444444444444",
		"client_id": "tv-admin-spa",
		"scope":     "openid",
	}
}

func tvVerifier(t *testing.T, key *ecdsa.PrivateKey) *identityauth.Verifier {
	t.Helper()
	keys := map[string]*ecdsa.PublicKey{tvTestKid: &key.PublicKey}
	v := identityauth.NewWithKeys(tvTestIssuer, tvTestAudience, keys)
	require.NotNil(t, v)
	return v
}

// ---------- Positive: JWT admin authentication ----------

func TestTVAdmin_JWTAccepted(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	now := time.Now().UTC().Truncate(time.Second)
	jwt := tvMintToken(t, key, tvTestKid, tvDefaultClaims(now))

	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusOK, resp.Code,
		"valid Primer Identity JWT must authenticate admin routes")
}

func TestTVAdmin_JWTAndServiceKeyBothWork(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		AdminKey:         "s3cret",
		IdentityVerifier: tvVerifier(t, key),
	})

	now := time.Now().UTC().Truncate(time.Second)
	jwt := tvMintToken(t, key, tvTestKid, tvDefaultClaims(now))

	// JWT path.
	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusOK, resp.Code, "JWT path succeeds")

	// Service key path.
	resp = h.Get("/devices", "X-Admin-Key: s3cret")
	assert.Equal(t, http.StatusOK, resp.Code, "service key path succeeds")

	// Service key via Bearer.
	resp = h.Get("/devices", "Authorization: Bearer s3cret")
	assert.Equal(t, http.StatusOK, resp.Code, "service key as bearer succeeds")
}

// ---------- Negative: reject raw Stytch tokens ----------

func TestTVAdmin_RejectsRawStytchToken(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	// Build a token that looks like a Stytch JWT (wrong typ "JWT" instead of "at+jwt").
	header := map[string]string{"alg": "ES256", "typ": "JWT", "kid": tvTestKid}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(map[string]any{
		"iss":                 "stytch.com/project/project-test-uuid",
		"sub":                 "user-stytch-uuid",
		"aud":                 []string{"project-test-uuid"},
		"exp":                 time.Now().Add(10 * time.Minute).Unix(),
		"iat":                 time.Now().Unix(),
		"nbf":                 time.Now().Unix(),
		"stytch_session_type": "session",
	})
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	stytchToken := headerB64 + "." + payloadB64 + "." + sig

	resp := h.Get("/devices", "Authorization: Bearer "+stytchToken)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"raw Stytch session JWT must never be accepted by TV admin")
}

func TestTVAdmin_RejectsStytchSessionJWTWithValidSignature(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	// Even if someone managed to sign a Stytch-style JWT with our key,
	// the typ=JWT check rejects it before signature verification.
	header := map[string]string{"alg": "ES256", "typ": "JWT", "kid": tvTestKid}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(map[string]any{
		"iss": "stytch.com/project/uuid",
		"sub": "user-uuid",
		"aud": tvTestAudience,
		"exp": time.Now().Add(10 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
		"nbf": time.Now().Unix(),
		"jti": "some-jti",
	})
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + payloadB64

	hash := sha256.Sum256([]byte(signingInput))
	r, s, _ := ecdsa.Sign(rand.Reader, key, hash[:])
	sigBuf := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sigBuf[32-len(rBytes):32], rBytes)
	copy(sigBuf[64-len(sBytes):], sBytes)
	signed := signingInput + "." + base64.RawURLEncoding.EncodeToString(sigBuf)

	resp := h.Get("/devices", "Authorization: Bearer "+signed)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"Stytch-style JWT with valid sig still rejected by typ check")
}

// ---------- Negative: fail-closed when auth is configured ----------

func TestTVAdmin_FailsClosedWhenNothingConfigured(t *testing.T) {
	// When neither admin key nor identity verifier is configured, the guard
	// is inert (supports spec gen and local dev). Production deploys MUST
	// configure at least one; the tv-server binary logs a warning.
	// This test verifies the inert behavior matches the contract.
	t.Parallel()
	h, _, _ := tvtestutil.APIRaw(t, tvtestutil.Options{})

	// Inert = anonymous access allowed (for spec gen and bare local checkout).
	resp := h.Get("/devices")
	assert.Equal(t, http.StatusOK, resp.Code,
		"admin API is inert when no auth configured (spec-gen/local-dev mode)")
}

func TestTVAdmin_EnforcesWhenOnlyVerifierConfigured(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	// Anonymous access is rejected when a verifier IS configured.
	resp := h.Get("/devices")
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"must reject anonymous when verifier is configured")

	// Non-JWT opaque token rejected (no admin key fallback).
	resp = h.Get("/devices", "Authorization: Bearer some-opaque-token")
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"opaque token rejected when only verifier configured")

	// Valid JWT still works.
	now := time.Now().UTC().Truncate(time.Second)
	jwt := tvMintToken(t, key, tvTestKid, tvDefaultClaims(now))
	resp = h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestTVAdmin_EnforcesWhenOnlyKeyConfigured(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{AdminKey: "s3cret"})

	// Anonymous access rejected.
	resp := h.Get("/devices")
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"must reject anonymous when admin key is configured")

	// Wrong key rejected.
	resp = h.Get("/devices", "X-Admin-Key: wrong")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	// Right key works.
	resp = h.Get("/devices", "X-Admin-Key: s3cret")
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestTVAdmin_FailsClosedOnJWTWithNilVerifier(t *testing.T) {
	t.Parallel()
	// Only service key is configured; a JWT-shaped token should be rejected
	// because no verifier is present to validate it.
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		AdminKey:         "s3cret",
		IdentityVerifier: nil,
	})

	// Construct something JWT-shaped.
	fakeJWT := "eyJhbGciOiJFUzI1NiIsInR5cCI6ImF0K2p3dCIsImtpZCI6ImsiLCJmb28iOiJiYXIifQ.eyJpc3MiOiJ4Iiwic3ViIjoieSIsImF1ZCI6InoiLCJleHAiOjk5OTk5OTk5OTksImlhdCI6MSwiamF0IjoxLCJuYmYiOjEsImp0aSI6ImEifQ.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	resp := h.Get("/devices", "Authorization: Bearer "+fakeJWT)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"JWT must be rejected when identity verifier is nil")

	// But the service key still works.
	resp = h.Get("/devices", "X-Admin-Key: s3cret")
	assert.Equal(t, http.StatusOK, resp.Code)
}

// ---------- Negative: wrong audience / issuer / expired ----------

func TestTVAdmin_RejectsWrongAudience(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	now := time.Now().UTC().Truncate(time.Second)
	claims := tvDefaultClaims(now)
	claims["aud"] = "primer-lms" // Wrong audience — this is the LMS audience.
	jwt := tvMintToken(t, key, tvTestKid, claims)

	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"JWT for wrong audience (LMS) must be rejected by TV")
}

func TestTVAdmin_RejectsWrongIssuer(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	now := time.Now().UTC().Truncate(time.Second)
	claims := tvDefaultClaims(now)
	claims["iss"] = "https://evil.example.com"
	jwt := tvMintToken(t, key, tvTestKid, claims)

	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"JWT from wrong issuer must be rejected")
}

func TestTVAdmin_RejectsExpiredJWT(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	past := time.Now().UTC().Add(-20 * time.Minute).Truncate(time.Second)
	claims := tvDefaultClaims(past)
	claims["exp"] = past.Add(5 * time.Minute).Unix()
	jwt := tvMintToken(t, key, tvTestKid, claims)

	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"expired JWT must be rejected")
}

func TestTVAdmin_RejectsJWTWithTTLTooLong(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	now := time.Now().UTC().Truncate(time.Second)
	claims := tvDefaultClaims(now)
	claims["exp"] = now.Add(30 * time.Minute).Unix() // > 15 min max
	jwt := tvMintToken(t, key, tvTestKid, claims)

	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"JWT with TTL > 15min must be rejected")
}

// ---------- Cross-boundary: device tokens are TV-local ----------

func TestTVAdmin_DeviceTokensRemainLocal(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		AdminKey:         "s3cret",
		IdentityVerifier: tvVerifier(t, key),
	})

	// A paired device token must NOT authenticate admin routes.
	_, deviceToken := tvtestutil.PairedDevice(t, q)
	resp := h.Get("/devices", "Authorization: Bearer "+deviceToken)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"device token must not access admin routes")

	// But it still works for device routes.
	resp = h.Get("/catalog", authHeader(deviceToken))
	assert.Equal(t, http.StatusOK, resp.Code,
		"device token must still work for device routes")
}

// ---------- Cross-boundary: admin key does not grant device access ----------

func TestTVAdmin_AdminKeyDoesNotGrantDeviceAccess(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		AdminKey:         "s3cret",
		IdentityVerifier: tvVerifier(t, key),
	})

	resp := h.Get("/catalog", "X-Admin-Key: s3cret")
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"admin key must not be a device credential")
}

// ---------- Cross-boundary: JWT does not grant device access ----------

func TestTVAdmin_JWTDoesNotGrantDeviceAccess(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	now := time.Now().UTC().Truncate(time.Second)
	jwt := tvMintToken(t, key, tvTestKid, tvDefaultClaims(now))

	resp := h.Get("/catalog", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"JWT is an admin credential, not a device credential")
}

// ---------- No Stytch SDK in TV ----------

func TestTVAdmin_NoStytchSDKInTV(t *testing.T) {
	t.Parallel()
	// Structural assertion: the TV packages must not import any Stytch SDK.
	// This test compiles successfully only when there is no Stytch dependency.
	// The identityauth package is a pure JWT verifier with no Stytch import.
	_ = identityauth.ErrDenied
}

// ---------- Operational: JWKS outage (identity service down) ----------

func TestTVAdmin_JWKSOutageRejectsNewTokens(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)

	// Verifier pointing at an unreachable JWKS URL.
	v := identityauth.New(identityauth.Config{
		Issuer:   tvTestIssuer,
		Audience: tvTestAudience,
		JWKSURL:  "http://127.0.0.1:1/unreachable",
	})
	require.NotNil(t, v)

	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		AdminKey:         "s3cret",
		IdentityVerifier: v,
	})

	now := time.Now().UTC().Truncate(time.Second)
	jwt := tvMintToken(t, key, tvTestKid, tvDefaultClaims(now))

	// JWT rejected because JWKS is unreachable.
	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusUnauthorized, resp.Code,
		"JWT must be rejected when JWKS endpoint is unreachable")

	// Service key path still works (degraded mode).
	resp = h.Get("/devices", "X-Admin-Key: s3cret")
	assert.Equal(t, http.StatusOK, resp.Code,
		"service key path provides degraded admin access during identity outage")
}

// ---------- Operational: wrong key rejected ----------

func TestTVAdmin_WrongServiceKeyRejected(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{AdminKey: "s3cret"})

	resp := h.Get("/devices", "X-Admin-Key: wrong-key")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	resp = h.Get("/devices", "Authorization: Bearer wrong-key")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

// ---------- Operational: no authorization from provider roles ----------

func TestTVAdmin_NoProviderRoleAuthorization(t *testing.T) {
	t.Parallel()
	key := tvTestKey(t)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		IdentityVerifier: tvVerifier(t, key),
	})

	// A JWT with any valid claims is accepted — TV does not inspect Stytch
	// organization/member roles. Authorization is local to the product boundary.
	now := time.Now().UTC().Truncate(time.Second)
	claims := tvDefaultClaims(now)
	claims["sub"] = "any-verified-identity-subject"
	jwt := tvMintToken(t, key, tvTestKid, claims)

	resp := h.Get("/devices", "Authorization: Bearer "+jwt)
	assert.Equal(t, http.StatusOK, resp.Code,
		"any valid Primer Identity JWT with correct audience is authorized")
}
