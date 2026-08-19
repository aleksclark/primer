package authn

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxTokenBytes     = 8 * 1024
	maxHeaderBytes    = 512
	maxPayloadBytes   = 4096
	maxSignatureBytes = 256
	maxJWKSBytes      = 64 * 1024
	maxClientIDBytes  = 128
	maxLifetime       = 15 * time.Minute
	maxClockSkew      = 5 * time.Second
	jwksTTL           = 30 * time.Second
	jwksRotationGrace = 15 * time.Minute
)

var headerAllowed = map[string]struct{}{
	"alg": {}, "typ": {}, "kid": {},
}

var claimsAllowed = map[string]struct{}{
	"iss": {}, "sub": {}, "aud": {}, "exp": {}, "iat": {}, "nbf": {},
	"jti": {}, "client_id": {}, "scope": {},
}

// ── JWKS cache ────────────────────────────────────────────────────────────────

type jwksCache struct {
	url  string
	http HTTPDoer
	mu   sync.Mutex
	keys map[string]cachedJWK
	exp  time.Time
}

type cachedJWK struct {
	pub        *ecdsa.PublicKey
	staleUntil time.Time
}

func newJWKSCache(rawURL string, doer HTTPDoer) *jwksCache {
	if doer == nil {
		doer = &stdHTTP{}
	}
	return &jwksCache{url: rawURL, http: doer, keys: map[string]cachedJWK{}}
}

type stdHTTP struct{ c *http.Client }

func (s *stdHTTP) Do(req *http.Request) (*http.Response, error) {
	c := s.c
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	return c.Do(req)
}

func (c *jwksCache) lookup(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid == "" {
		return nil, ErrUnauthorized
	}
	c.mu.Lock()
	if e, ok := c.keys[kid]; ok && e.pub != nil && time.Now().Before(c.exp) {
		pub := e.pub
		c.mu.Unlock()
		return pub, nil
	}
	c.mu.Unlock()

	if err := c.refresh(ctx); err != nil {
		// On refresh failure still try stale keys (bounded by rotation grace).
		c.mu.Lock()
		defer c.mu.Unlock()
		if e, ok := c.keys[kid]; ok && e.pub != nil &&
			(e.staleUntil.IsZero() || time.Now().Before(e.staleUntil)) {
			return e.pub, nil
		}
		return nil, ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.keys[kid]; ok && e.pub != nil &&
		(e.staleUntil.IsZero() || time.Now().Before(e.staleUntil)) {
		return e.pub, nil
	}
	return nil, ErrUnauthorized
}

func (c *jwksCache) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return ErrUnauthorized
	}
	resp, err := c.http.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return ErrUnauthorized
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrUnauthorized
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxJWKSBytes {
		return ErrUnauthorized
	}
	var doc struct {
		Keys []struct {
			KTY string `json:"kty"`
			CRV string `json:"crv"`
			Use string `json:"use"`
			Alg string `json:"alg"`
			Kid string `json:"kid"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Keys) == 0 {
		return ErrUnauthorized
	}
	next := make(map[string]*ecdsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.KTY != "EC" || k.CRV != "P-256" || k.Alg != "ES256" || k.Use != "sig" || k.Kid == "" {
			continue
		}
		pub, err := parseECPublic(k.X, k.Y)
		if err != nil {
			continue
		}
		if _, dup := next[k.Kid]; dup {
			return ErrUnauthorized
		}
		next[k.Kid] = pub
	}
	if len(next) == 0 {
		return ErrUnauthorized
	}
	now := time.Now()
	c.mu.Lock()
	merged := make(map[string]cachedJWK, len(next)+len(c.keys))
	for kid, pub := range next {
		merged[kid] = cachedJWK{pub: pub}
	}
	for kid, old := range c.keys {
		if _, current := next[kid]; current {
			continue
		}
		if old.staleUntil.IsZero() {
			old.staleUntil = now.Add(jwksRotationGrace)
		}
		if now.Before(old.staleUntil) {
			merged[kid] = old
		}
	}
	c.keys = merged
	c.exp = now.Add(jwksTTL)
	c.mu.Unlock()
	return nil
}

func parseECPublic(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	xb, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil || len(xb) != 32 {
		return nil, ErrUnauthorized
	}
	yb, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil || len(yb) != 32 {
		return nil, ErrUnauthorized
	}
	x, y := new(big.Int).SetBytes(xb), new(big.Int).SetBytes(yb)
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, ErrUnauthorized
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

// ── JWT verification ──────────────────────────────────────────────────────────

func (v *Validator) verify(ctx context.Context, raw string) (Principal, error) {
	if len(raw) == 0 || len(raw) > maxTokenBytes || strings.Count(raw, ".") != 2 {
		return Principal{}, ErrUnauthorized
	}
	parts := strings.SplitN(raw, ".", 3)
	headerRaw, err := decodeSeg(parts[0], maxHeaderBytes)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	payloadRaw, err := decodeSeg(parts[1], maxPayloadBytes)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	sigRaw, err := decodeSeg(parts[2], maxSignatureBytes)
	if err != nil || len(sigRaw) != 64 {
		return Principal{}, ErrUnauthorized
	}
	kid, err := parseHeader(headerRaw)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	pub, err := v.keys.lookup(ctx, kid)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sigRaw[:32]), new(big.Int).SetBytes(sigRaw[32:])
	if !ecdsa.Verify(pub, sum[:], r, s) {
		return Principal{}, ErrUnauthorized
	}
	claims, err := parseClaims(payloadRaw)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	if claims.iss != v.issuer || claims.aud != AudiencePrimerAgents {
		return Principal{}, ErrUnauthorized
	}
	if err := validateLifetime(claims, v.now().UTC()); err != nil {
		return Principal{}, ErrUnauthorized
	}
	kind, subjectRef, err := classifySubject(claims.sub)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		SubjectRef: subjectRef,
		Kind:       kind,
		ClientID:   claims.clientID,
		Scopes:     claims.scopes,
		Audience:   claims.aud,
		Issuer:     claims.iss,
	}, nil
}

// ── Parsing helpers ───────────────────────────────────────────────────────────

func parseHeader(raw []byte) (string, error) {
	obj, err := decodeStrictObject(raw, headerAllowed, maxHeaderBytes)
	if err != nil || len(obj) != 3 {
		return "", ErrUnauthorized
	}
	if asString(obj["alg"]) != "ES256" || asString(obj["typ"]) != "at+jwt" {
		return "", ErrUnauthorized
	}
	kid := asString(obj["kid"])
	if kid == "" || len(kid) > 128 || !utf8.ValidString(kid) {
		return "", ErrUnauthorized
	}
	return kid, nil
}

type parsedClaims struct {
	iss      string
	sub      string
	aud      string
	exp      int64
	iat      int64
	nbf      int64
	jti      string
	clientID string
	scopes   []string
}

func parseClaims(raw []byte) (parsedClaims, error) {
	obj, err := decodeStrictObject(raw, claimsAllowed, maxPayloadBytes)
	if err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	required := []string{"iss", "sub", "aud", "exp", "iat", "nbf", "jti", "client_id", "scope"}
	if len(obj) != len(required) {
		return parsedClaims{}, ErrUnauthorized
	}
	for _, k := range required {
		if _, ok := obj[k]; !ok {
			return parsedClaims{}, ErrUnauthorized
		}
	}
	c := parsedClaims{
		iss:      asString(obj["iss"]),
		sub:      asString(obj["sub"]),
		aud:      asString(obj["aud"]),
		jti:      asString(obj["jti"]),
		clientID: asString(obj["client_id"]),
	}
	var ok bool
	if c.exp, ok = asInt(obj["exp"]); !ok {
		return parsedClaims{}, ErrUnauthorized
	}
	if c.iat, ok = asInt(obj["iat"]); !ok {
		return parsedClaims{}, ErrUnauthorized
	}
	if c.nbf, ok = asInt(obj["nbf"]); !ok {
		return parsedClaims{}, ErrUnauthorized
	}
	if err := requireUUID(c.jti); err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	if err := requirePublicClientID(c.clientID); err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	scopes, err := splitScopes(asString(obj["scope"]))
	if err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	c.scopes = scopes
	if c.iss == "" || c.aud == "" || c.sub == "" {
		return parsedClaims{}, ErrUnauthorized
	}
	return c, nil
}

func validateLifetime(c parsedClaims, now time.Time) error {
	if c.nbf != c.iat || c.exp <= c.iat {
		return ErrUnauthorized
	}
	ttl := c.exp - c.iat
	if ttl <= 0 || ttl > int64(maxLifetime/time.Second) {
		return ErrUnauthorized
	}
	iat := time.Unix(c.iat, 0).UTC()
	exp := time.Unix(c.exp, 0).UTC()
	if now.Before(iat.Add(-maxClockSkew)) || now.After(exp.Add(maxClockSkew)) {
		return ErrUnauthorized
	}
	return nil
}

const (
	humanPrefix   = "identity:"
	servicePrefix = "identity:svc:"
)

func classifySubject(sub string) (Kind, string, error) {
	if sub == "" || !utf8.ValidString(sub) {
		return "", "", ErrUnauthorized
	}
	lower := strings.ToLower(sub)
	switch {
	case strings.HasPrefix(lower, servicePrefix):
		svcID := sub[len(servicePrefix):]
		if err := validateServiceID(svcID); err != nil {
			return "", "", ErrUnauthorized
		}
		return KindService, servicePrefix + svcID, nil
	case strings.HasPrefix(lower, humanPrefix):
		raw := sub[len(humanPrefix):]
		if strings.Contains(raw, ":") {
			return "", "", ErrUnauthorized
		}
		u, err := uuid.Parse(raw)
		if err != nil || u == uuid.Nil {
			return "", "", ErrUnauthorized
		}
		return KindHuman, humanPrefix + u.String(), nil
	default:
		return "", "", ErrUnauthorized
	}
}

func validateServiceID(id string) error {
	if id == "" || strings.TrimSpace(id) != id || len(id) > 128 {
		return ErrUnauthorized
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return ErrUnauthorized
	}
	return nil
}

func requireUUID(s string) error {
	u, err := uuid.Parse(s)
	if err != nil || u == uuid.Nil || u.String() != s {
		return ErrUnauthorized
	}
	return nil
}

func requirePublicClientID(v string) error {
	if v == "" || !utf8.ValidString(v) || len(v) > maxClientIDBytes || strings.TrimSpace(v) != v {
		return ErrUnauthorized
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return ErrUnauthorized
		}
	}
	// Must not be a bare UUID (internal OAuth-client IDs are opaque strings).
	if _, err := uuid.Parse(v); err == nil {
		return ErrUnauthorized
	}
	return nil
}

func splitScopes(raw string) ([]string, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 || len(parts) > 32 {
		return nil, ErrUnauthorized
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || len(p) > 64 {
			return nil, ErrUnauthorized
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

func decodeSeg(s string, max int) ([]byte, error) {
	if s == "" || len(s) > max*2 {
		return nil, ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) == 0 || len(raw) > max {
		return nil, ErrUnauthorized
	}
	return raw, nil
}

func decodeStrictObject(raw []byte, allowed map[string]struct{}, max int) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > max || !utf8.Valid(raw) {
		return nil, ErrUnauthorized
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, ErrUnauthorized
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, ErrUnauthorized
	}
	obj := make(map[string]any, len(allowed))
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, ErrUnauthorized
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, ErrUnauthorized
		}
		if _, ok := allowed[key]; !ok {
			return nil, ErrUnauthorized
		}
		if _, dup := obj[key]; dup {
			return nil, ErrUnauthorized
		}
		var val any
		if err := dec.Decode(&val); err != nil {
			return nil, ErrUnauthorized
		}
		obj[key] = val
	}
	end, err := dec.Token()
	if err != nil {
		return nil, ErrUnauthorized
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return nil, ErrUnauthorized
	}
	var trailing any
	if dec.Decode(&trailing) != io.EOF {
		return nil, ErrUnauthorized
	}
	return obj, nil
}

func asString(v any) string { s, _ := v.(string); return s }

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case float64:
		return int64(n), float64(int64(n)) == n
	}
	return 0, false
}
