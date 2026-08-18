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
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
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
	// jwksRotationGrace keeps a previously published key usable while a
	// still-live token crosses an Identity key rotation. It is bounded and is
	// never used when a JWKS refresh itself fails.
	jwksRotationGrace = 15 * time.Minute
)

var headerAllowed = map[string]struct{}{
	"alg": {}, "typ": {}, "kid": {},
}

var claimsAllowed = map[string]struct{}{
	"iss": {}, "sub": {}, "aud": {}, "exp": {}, "iat": {}, "nbf": {},
	"jti": {}, "client_id": {}, "scope": {},
}

type stdHTTP struct {
	client *http.Client
}

func (s stdHTTP) Do(req *http.Request) (*http.Response, error) {
	c := s.client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	return c.Do(req)
}

type jwksCache struct {
	url  string
	http HTTPDoer
	mu   sync.Mutex
	keys map[string]cachedJWK
	exp  time.Time
}

type publicJWK struct {
	Kid string
	X   string
	Y   string
	pub *ecdsa.PublicKey
}

type cachedJWK struct {
	publicJWK
	// staleUntil is set only for a key omitted by a later successful JWKS
	// response. A zero value means the key is in the current published set.
	staleUntil time.Time
}

func newJWKSCache(rawURL string, doer HTTPDoer) *jwksCache {
	if doer == nil {
		doer = stdHTTP{}
	}
	return &jwksCache{url: rawURL, http: doer, keys: map[string]cachedJWK{}}
}

func (c *jwksCache) lookup(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid == "" {
		return nil, ErrUnauthorized
	}
	c.mu.Lock()
	if pub, ok := c.keys[kid]; ok && time.Now().Before(c.exp) && pub.pub != nil {
		out := pub.pub
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if pub, ok := c.keys[kid]; ok && pub.pub != nil &&
		(pub.staleUntil.IsZero() || time.Now().Before(pub.staleUntil)) {
		return pub.pub, nil
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
	next := make(map[string]publicJWK, len(doc.Keys))
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
		next[k.Kid] = publicJWK{Kid: k.Kid, X: k.X, Y: k.Y, pub: pub}
	}
	if len(next) == 0 {
		return ErrUnauthorized
	}
	now := time.Now()
	// Keep omitted keys only for the bounded rotation grace period. This
	// accepts already-issued tokens during dual-key rotation without allowing
	// an unbounded stale-key set or bypassing failed JWKS fetches.
	c.mu.Lock()
	merged := make(map[string]cachedJWK, len(next)+len(c.keys))
	for kid, pub := range next {
		merged[kid] = cachedJWK{publicJWK: pub}
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
	x := new(big.Int).SetBytes(xb)
	y := new(big.Int).SetBytes(yb)
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, ErrUnauthorized
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func (v *Validator) verify(ctx context.Context, raw string) (AuthContext, error) {
	if len(raw) == 0 || len(raw) > maxTokenBytes || strings.Count(raw, ".") != 2 {
		return AuthContext{}, ErrUnauthorized
	}
	parts := strings.Split(raw, ".")
	headerRaw, err := decodeSeg(parts[0], maxHeaderBytes)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	payloadRaw, err := decodeSeg(parts[1], maxPayloadBytes)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	sigRaw, err := decodeSeg(parts[2], maxSignatureBytes)
	if err != nil || len(sigRaw) != 64 {
		return AuthContext{}, ErrUnauthorized
	}
	header, err := parseHeader(headerRaw)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	pub, err := v.keys.lookup(ctx, header.kid)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sigRaw[:32])
	s := new(big.Int).SetBytes(sigRaw[32:])
	if !ecdsa.Verify(pub, sum[:], r, s) {
		return AuthContext{}, ErrUnauthorized
	}
	claims, err := parseClaims(payloadRaw)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	if claims.iss != v.issuer || claims.aud != v.audience {
		return AuthContext{}, ErrUnauthorized
	}
	if err := validateLifetime(claims, v.now().UTC()); err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	kind, subjectRef, err := classifySubject(claims.sub)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	return AuthContext{
		SubjectRef: subjectRef,
		Kind:       kind,
		Scopes:     claims.scopes,
		ClientID:   claims.clientID,
		Audience:   claims.aud,
		Issuer:     claims.iss,
	}, nil
}

type parsedHeader struct {
	kid string
}

func parseHeader(raw []byte) (parsedHeader, error) {
	obj, err := decodeStrictObject(raw, headerAllowed, maxHeaderBytes)
	if err != nil || len(obj) != 3 {
		return parsedHeader{}, ErrUnauthorized
	}
	if asString(obj["alg"]) != "ES256" || asString(obj["typ"]) != "at+jwt" {
		return parsedHeader{}, ErrUnauthorized
	}
	kid := asString(obj["kid"])
	if kid == "" || len(kid) > 128 || !utf8.ValidString(kid) {
		return parsedHeader{}, ErrUnauthorized
	}
	return parsedHeader{kid: kid}, nil
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
	out := parsedClaims{
		iss:      asString(obj["iss"]),
		sub:      asString(obj["sub"]),
		aud:      asString(obj["aud"]),
		jti:      asString(obj["jti"]),
		clientID: asString(obj["client_id"]),
	}
	var ok bool
	if out.exp, ok = asInt(obj["exp"]); !ok {
		return parsedClaims{}, ErrUnauthorized
	}
	if out.iat, ok = asInt(obj["iat"]); !ok {
		return parsedClaims{}, ErrUnauthorized
	}
	if out.nbf, ok = asInt(obj["nbf"]); !ok {
		return parsedClaims{}, ErrUnauthorized
	}
	if err := requireUUID(out.jti); err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	if err := requirePublicClientID(out.clientID); err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	scopes, err := splitScopes(asString(obj["scope"]))
	if err != nil {
		return parsedClaims{}, ErrUnauthorized
	}
	out.scopes = scopes
	if out.iss == "" || out.aud == "" || out.sub == "" {
		return parsedClaims{}, ErrUnauthorized
	}
	return out, nil
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

func classifySubject(sub string) (Kind, string, error) {
	if requireUUID(sub) == nil {
		return KindHuman, domain.HumanSubjectRef(mustUUID(sub)), nil
	}
	if strings.HasPrefix(sub, domain.ServiceSubjectPrefix) {
		canon, err := domain.CanonicalizeSubjectRef(sub)
		if err != nil {
			return "", "", ErrUnauthorized
		}
		return KindService, canon.String(), nil
	}
	return "", "", ErrUnauthorized
}

func mustUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

func requireUUID(value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil || parsed.String() != value {
		return ErrUnauthorized
	}
	return nil
}

func requirePublicClientID(value string) error {
	if value == "" || !utf8.ValidString(value) || len(value) > maxClientIDBytes {
		return ErrUnauthorized
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return ErrUnauthorized
		}
	}
	if _, err := uuid.Parse(value); err == nil {
		return ErrUnauthorized
	}
	if strings.TrimSpace(value) != value {
		return ErrUnauthorized
	}
	return nil
}

func splitScopes(raw string) ([]string, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 || len(parts) > 16 {
		return nil, ErrUnauthorized
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		if p == "" || len(p) > 64 {
			return nil, ErrUnauthorized
		}
		if _, ok := seen[p]; ok {
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
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
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
		if _, duplicate := obj[key]; duplicate {
			return nil, ErrUnauthorized
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, ErrUnauthorized
		}
		obj[key] = value
	}
	end, err := dec.Token()
	if err != nil {
		return nil, ErrUnauthorized
	}
	if delim, ok := end.(json.Delim); !ok || delim != '}' {
		return nil, ErrUnauthorized
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, ErrUnauthorized
	}
	return obj, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case float64:
		return int64(n), float64(int64(n)) == n
	default:
		return 0, false
	}
}
