// Package identityauth verifies Primer Identity access tokens (ES256,
// typ=at+jwt) at the LMS boundary. It fetches the Identity JWKS endpoint and
// validates signatures and claims without importing the Identity module or any
// Stytch SDK. The LMS uses this package for the "Primer JWT" leg of its
// dual-path authentication guard.
//
// Fail-closed: a nil Verifier, empty configuration, or any verification error
// rejects the request. The LMS never falls through to grant access on error.
package identityauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrDenied is the single public error — callers must not distinguish between
// malformed, expired, wrong-audience, or unknown-key tokens.
var ErrDenied = errors.New("identity token denied")

// MaxClockSkew is the time tolerance for nbf/exp checks.
const MaxClockSkew = 5 * time.Second

// MaxTokenLifetime is the largest accepted exp-iat window.
const MaxTokenLifetime = 15 * time.Minute

// maxJWKSBytes caps how much we read from the JWKS endpoint.
const maxJWKSBytes = 64 * 1024

// jwksCacheTTL is how long fetched keys are trusted before a re-fetch.
const jwksCacheTTL = 5 * time.Minute

// Config holds the identity verification settings. All three fields are
// required for the verifier to be active; if any is empty, New returns nil
// (fail-closed: every JWT presented is rejected).
type Config struct {
	// Issuer is the expected "iss" claim (e.g. "https://identity.primer.local").
	Issuer string
	// Audience is the expected "aud" claim (e.g. "primer-lms").
	Audience string
	// JWKSURL is the Identity service's /.well-known/jwks.json endpoint.
	JWKSURL string
}

// Principal is the verified token result. It never retains the raw token.
type Principal struct {
	Subject   string
	Issuer    string
	Audience  string
	ClientID  string
	Scope     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Verifier validates Primer Identity access tokens. A nil *Verifier always
// denies (fail-closed).
type Verifier struct {
	issuer   string
	audience string
	jwksURL  string
	client   *http.Client

	mu        sync.RWMutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

// New creates a Verifier from config. Returns nil if config is incomplete,
// which makes the JWT path inactive (fail-closed).
func New(cfg Config) *Verifier {
	if cfg.Issuer == "" || cfg.Audience == "" || cfg.JWKSURL == "" {
		return nil
	}
	return &Verifier{
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
		jwksURL:  cfg.JWKSURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		keys: make(map[string]*ecdsa.PublicKey),
	}
}

// NewWithKeys creates a Verifier pre-loaded with keys (for testing). Still
// returns nil if issuer or audience is empty.
func NewWithKeys(issuer, audience string, keys map[string]*ecdsa.PublicKey) *Verifier {
	if issuer == "" || audience == "" {
		return nil
	}
	v := &Verifier{
		issuer:   issuer,
		audience: audience,
		keys:     keys,
		fetchedAt: time.Now(),
	}
	return v
}

// Verify parses, verifies, and returns a Principal. The raw token is not
// retained. Returns ErrDenied for any failure — callers cannot distinguish
// failure modes.
func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if v == nil {
		return Principal{}, ErrDenied
	}
	if ctx == nil || ctx.Err() != nil {
		return Principal{}, ErrDenied
	}

	// Split into 3 base64url segments.
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Principal{}, ErrDenied
	}

	// Total size check.
	if len(raw) > 8*1024 {
		return Principal{}, ErrDenied
	}

	// Decode header.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Principal{}, ErrDenied
	}

	header, err := parseHeader(headerBytes)
	if err != nil {
		return Principal{}, ErrDenied
	}

	// Decode payload.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, ErrDenied
	}

	claims, err := parseClaims(payloadBytes)
	if err != nil {
		return Principal{}, ErrDenied
	}

	// Validate claims before expensive signature check.
	if claims.Iss != v.issuer {
		return Principal{}, ErrDenied
	}
	if claims.Aud != v.audience {
		return Principal{}, ErrDenied
	}
	now := time.Now().UTC()
	if err := validateLifetime(claims, now); err != nil {
		return Principal{}, ErrDenied
	}

	// Decode signature.
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Principal{}, ErrDenied
	}

	// Look up public key.
	pub, err := v.lookupKey(ctx, header.Kid)
	if err != nil {
		return Principal{}, ErrDenied
	}

	// Verify ES256 signature over header.payload.
	signingInput := []byte(parts[0] + "." + parts[1])
	if !verifyES256(pub, signingInput, sigBytes) {
		return Principal{}, ErrDenied
	}

	return Principal{
		Subject:   claims.Sub,
		Issuer:    claims.Iss,
		Audience:  claims.Aud,
		ClientID:  claims.ClientID,
		Scope:     claims.Scope,
		IssuedAt:  time.Unix(claims.Iat, 0).UTC(),
		ExpiresAt: time.Unix(claims.Exp, 0).UTC(),
	}, nil
}

// --- Header parsing ---

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

func parseHeader(raw []byte) (jwtHeader, error) {
	var h jwtHeader
	if err := json.Unmarshal(raw, &h); err != nil {
		return jwtHeader{}, ErrDenied
	}
	if h.Alg != "ES256" {
		return jwtHeader{}, ErrDenied
	}
	if h.Typ != "at+jwt" {
		return jwtHeader{}, ErrDenied
	}
	if h.Kid == "" || len(h.Kid) > 128 {
		return jwtHeader{}, ErrDenied
	}
	return h, nil
}

// --- Claims parsing ---

type jwtClaims struct {
	Iss      string `json:"iss"`
	Sub      string `json:"sub"`
	Aud      string `json:"aud"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
	Nbf      int64  `json:"nbf"`
	Jti      string `json:"jti"`
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
}

func parseClaims(raw []byte) (jwtClaims, error) {
	var c jwtClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return jwtClaims{}, ErrDenied
	}
	if c.Iss == "" || c.Sub == "" || c.Aud == "" || c.Jti == "" {
		return jwtClaims{}, ErrDenied
	}
	if c.Exp == 0 || c.Iat == 0 || c.Nbf == 0 {
		return jwtClaims{}, ErrDenied
	}
	return c, nil
}

func validateLifetime(c jwtClaims, now time.Time) error {
	if c.Nbf != c.Iat || c.Exp <= c.Iat {
		return ErrDenied
	}
	ttl := time.Duration(c.Exp-c.Iat) * time.Second
	if ttl <= 0 || ttl > MaxTokenLifetime {
		return ErrDenied
	}
	iat := time.Unix(c.Iat, 0).UTC()
	exp := time.Unix(c.Exp, 0).UTC()

	// Not-yet-valid?
	if now.Before(iat.Add(-MaxClockSkew)) {
		return ErrDenied
	}
	// Expired?
	if now.After(exp.Add(MaxClockSkew)) {
		return ErrDenied
	}
	return nil
}

// --- ES256 verification ---

func verifyES256(pub *ecdsa.PublicKey, signingInput, sig []byte) bool {
	if pub == nil || len(sig) != 64 {
		return false
	}
	hash := sha256.Sum256(signingInput)
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(pub, hash[:], r, s)
}

// --- JWKS fetching and key lookup ---

func (v *Verifier) lookupKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	// Try cached first.
	v.mu.RLock()
	pub, ok := v.keys[kid]
	stale := time.Since(v.fetchedAt) > jwksCacheTTL
	v.mu.RUnlock()

	if ok && !stale {
		return pub, nil
	}

	// If JWKS URL is not configured (test mode), fail.
	if v.jwksURL == "" {
		if ok {
			return pub, nil // test-injected keys
		}
		return nil, ErrDenied
	}

	// Fetch fresh JWKS.
	if err := v.fetchJWKS(ctx); err != nil {
		// If we had a cached key and fetch failed, use stale.
		if ok {
			return pub, nil
		}
		return nil, ErrDenied
	}

	v.mu.RLock()
	pub, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrDenied
	}
	return pub, nil
}

func (v *Verifier) fetchJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS fetch: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes))
	if err != nil {
		return err
	}

	keys, err := parseJWKSDocument(body)
	if err != nil {
		return err
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

// jwksDocument is the minimal JWKS response shape.
type jwksDocument struct {
	Keys []jwkEntry `json:"keys"`
}

type jwkEntry struct {
	KTY string `json:"kty"`
	CRV string `json:"crv"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func parseJWKSDocument(raw []byte) (map[string]*ecdsa.PublicKey, error) {
	var doc jwksDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Keys) == 0 {
		return nil, errors.New("empty JWKS")
	}
	keys := make(map[string]*ecdsa.PublicKey, len(doc.Keys))
	for _, entry := range doc.Keys {
		if entry.KTY != "EC" || entry.CRV != "P-256" || entry.Alg != "ES256" {
			continue // skip non-ES256 keys
		}
		if entry.Kid == "" {
			continue
		}
		pub, err := decodeEC256Public(entry.X, entry.Y)
		if err != nil {
			continue // skip invalid keys
		}
		keys[entry.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("no valid ES256 keys in JWKS")
	}
	return keys, nil
}

func decodeEC256Public(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil || len(xBytes) != 32 {
		return nil, errors.New("invalid x coordinate")
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil || len(yBytes) != 32 {
		return nil, errors.New("invalid y coordinate")
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, errors.New("point not on P-256")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// IsJWT returns true if the token looks like a compact JWS (3 dot-separated
// base64url segments). Used by the dual-path guard to route tokens.
func IsJWT(token string) bool {
	first := strings.IndexByte(token, '.')
	if first < 1 {
		return false
	}
	second := strings.IndexByte(token[first+1:], '.')
	if second < 1 {
		return false
	}
	rest := token[first+1+second+1:]
	// Must not contain another dot.
	return !strings.ContainsRune(rest, '.') && len(rest) > 0
}
