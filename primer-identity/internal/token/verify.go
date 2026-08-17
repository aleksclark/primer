package token

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
)

var headerAllowed = map[string]struct{}{
	"alg": {},
	"typ": {},
	"kid": {},
}

var claimsAllowed = map[string]struct{}{
	"iss": {}, "sub": {}, "aud": {}, "exp": {}, "iat": {}, "nbf": {},
	"jti": {}, "client_id": {}, "scope": {},
}

// Verifier validates compact Primer access tokens against one expected audience.
type Verifier struct {
	keys     PublicKeySource
	issuer   string
	audience string
	clock    Clock
	clients  ClientLookup
	refresh  *refreshCoordinator
}

// NewVerifier constructs a fail-closed verifier for one issuer and audience.
func NewVerifier(keys PublicKeySource, issuer, audience string, clock Clock, clients ClientLookup) (*Verifier, error) {
	if keys == nil {
		return nil, denyUnavailable()
	}
	normalized, err := normalizeConfiguredIssuer(issuer)
	if err != nil {
		return nil, err
	}
	if err := requireAudience(audience); err != nil {
		return nil, err
	}
	return &Verifier{
		keys:     keys,
		issuer:   normalized,
		audience: audience,
		clock:    resolveClock(clock),
		clients:  clients,
		refresh:  newRefreshCoordinator(),
	}, nil
}

// Verify parses, verifies, and returns a copied Principal. The raw token is discarded.
func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if v == nil || v.keys == nil {
		return Principal{}, denyUnavailable()
	}
	if ctx == nil || ctx.Err() != nil {
		return Principal{}, denyUnavailable()
	}
	headerSeg, payloadSeg, sigSeg, err := splitCompact(raw)
	if err != nil {
		return Principal{}, err
	}
	headerRaw, err := decodeRawURL(headerSeg, maxHeaderBytes)
	if err != nil {
		return Principal{}, err
	}
	payloadRaw, err := decodeRawURL(payloadSeg, maxPayloadBytes)
	if err != nil {
		return Principal{}, err
	}
	sigRaw, err := decodeRawURL(sigSeg, maxSignatureBytes)
	if err != nil {
		return Principal{}, err
	}
	header, err := parseHeader(headerRaw)
	if err != nil {
		return Principal{}, err
	}
	if _, _, err := parseRawRS(sigRaw); err != nil {
		return Principal{}, err
	}
	pub, err := v.lookupKey(ctx, header.kid)
	if err != nil {
		return Principal{}, err
	}
	if ctx.Err() != nil {
		return Principal{}, denyUnavailable()
	}
	signingInput := headerSeg + "." + payloadSeg
	if err := verifyES256(pub, []byte(signingInput), sigRaw); err != nil {
		return Principal{}, err
	}
	claims, err := parseClaims(payloadRaw)
	if err != nil {
		return Principal{}, err
	}
	if claims.iss != v.issuer || claims.aud != v.audience {
		return Principal{}, denyInvalid()
	}
	now := v.clock.Now().UTC()
	if err := validateLifetime(claims, now); err != nil {
		return Principal{}, err
	}
	if err := v.bindClient(ctx, claims); err != nil {
		return Principal{}, err
	}
	if ctx.Err() != nil {
		return Principal{}, denyUnavailable()
	}
	return Principal{
		Kind:      claims.kind,
		Issuer:    claims.iss,
		Subject:   claims.sub,
		Audience:  claims.aud,
		IssuedAt:  time.Unix(claims.iat, 0).UTC(),
		NotBefore: time.Unix(claims.nbf, 0).UTC(),
		ExpiresAt: time.Unix(claims.exp, 0).UTC(),
		JTI:       claims.jti,
		ClientID:  claims.clientID,
		Scope:     claims.scope,
	}, nil
}

func (v *Verifier) bindClient(ctx context.Context, claims parsedClaims) error {
	if v.clients == nil {
		if claims.kind != KindHuman {
			return denyInvalid()
		}
		return nil
	}
	reg, err := v.clients(ctx, claims.clientID)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return denyUnavailable()
		}
		return denyInvalid()
	}
	if reg.ClientID != claims.clientID || reg.Audience != v.audience {
		return denyInvalid()
	}
	if reg.SubjectClass != "" && reg.SubjectClass != claims.kind {
		return denyInvalid()
	}
	if reg.Scope != "" {
		canon, err := canonicalScope(reg.Scope)
		if err != nil || canon != claims.scope {
			return denyInvalid()
		}
	}
	return nil
}

type parsedHeader struct {
	kid string
}

func parseHeader(raw []byte) (parsedHeader, error) {
	obj, err := decodeStrictJSONObject(raw, headerAllowed, maxHeaderBytes)
	if err != nil {
		return parsedHeader{}, err
	}
	if len(obj) != 3 {
		return parsedHeader{}, denyInvalid()
	}
	alg, err := decodeJSONString(obj["alg"])
	if err != nil || alg != Algorithm {
		return parsedHeader{}, denyInvalid()
	}
	typ, err := decodeJSONString(obj["typ"])
	if err != nil || typ != HeaderType {
		return parsedHeader{}, denyInvalid()
	}
	kid, err := decodeJSONString(obj["kid"])
	if err != nil {
		return parsedHeader{}, denyInvalid()
	}
	if err := boundedUTF8("kid", kid, 128); err != nil {
		return parsedHeader{}, err
	}
	return parsedHeader{kid: kid}, nil
}

type parsedClaims struct {
	kind     Kind
	iss      string
	sub      string
	aud      string
	exp      int64
	iat      int64
	nbf      int64
	jti      string
	clientID string
	scope    string
}

func parseClaims(raw []byte) (parsedClaims, error) {
	obj, err := decodeStrictJSONObject(raw, claimsAllowed, maxPayloadBytes)
	if err != nil {
		return parsedClaims{}, err
	}
	required := []string{"iss", "sub", "aud", "exp", "iat", "nbf", "jti", "client_id", "scope"}
	if len(obj) != len(required) {
		return parsedClaims{}, denyInvalid()
	}
	for _, key := range required {
		if _, ok := obj[key]; !ok {
			return parsedClaims{}, denyInvalid()
		}
	}
	out := parsedClaims{}
	if out.iss, err = decodeJSONString(obj["iss"]); err != nil {
		return parsedClaims{}, err
	}
	if out.sub, err = decodeJSONString(obj["sub"]); err != nil {
		return parsedClaims{}, err
	}
	if out.aud, err = decodeJSONString(obj["aud"]); err != nil {
		return parsedClaims{}, err
	}
	if out.jti, err = decodeJSONString(obj["jti"]); err != nil {
		return parsedClaims{}, err
	}
	if out.clientID, err = decodeJSONString(obj["client_id"]); err != nil {
		return parsedClaims{}, err
	}
	if out.scope, err = decodeJSONString(obj["scope"]); err != nil {
		return parsedClaims{}, err
	}
	if out.exp, err = decodeJSONInt(obj["exp"]); err != nil {
		return parsedClaims{}, err
	}
	if out.iat, err = decodeJSONInt(obj["iat"]); err != nil {
		return parsedClaims{}, err
	}
	if out.nbf, err = decodeJSONInt(obj["nbf"]); err != nil {
		return parsedClaims{}, err
	}
	kind, err := classifySubject(out.sub)
	if err != nil {
		return parsedClaims{}, err
	}
	out.kind = kind
	if err := requireUUID(out.jti); err != nil {
		return parsedClaims{}, err
	}
	if err := requireAudience(out.aud); err != nil {
		return parsedClaims{}, err
	}
	if err := requireClientID(out.clientID); err != nil {
		return parsedClaims{}, err
	}
	canonScope, err := canonicalScope(out.scope)
	if err != nil || canonScope != out.scope {
		return parsedClaims{}, denyInvalid()
	}
	if out.iss == "" {
		return parsedClaims{}, denyInvalid()
	}
	return out, nil
}

func validateLifetime(c parsedClaims, now time.Time) error {
	if c.nbf != c.iat || c.exp <= c.iat {
		return denyInvalid()
	}
	ttl := c.exp - c.iat
	if ttl <= 0 || ttl > int64(MaxLifetime/time.Second) {
		return denyInvalid()
	}
	if c.iat < numericDateMinUnix || c.exp > numericDateMaxUnix {
		return denyInvalid()
	}
	iat := time.Unix(c.iat, 0).UTC()
	exp := time.Unix(c.exp, 0).UTC()
	nbf := time.Unix(c.nbf, 0).UTC()
	now = now.UTC()
	if !nbf.Equal(iat) || !exp.Equal(iat.Add(time.Duration(ttl)*time.Second)) {
		return denyInvalid()
	}
	if now.Before(iat.Add(-MaxClockSkew)) {
		return denyInvalid()
	}
	if now.After(exp.Add(MaxClockSkew)) {
		return denyInvalid()
	}
	if exp.Sub(now) > MaxLifetime+MaxClockSkew {
		return denyInvalid()
	}
	return nil
}

func (v *Verifier) lookupKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, denyUnavailable()
	}
	jwk, ok, err := v.keys.Lookup(ctx, kid)
	if ctx.Err() != nil {
		return nil, denyUnavailable()
	}
	if err != nil {
		return nil, denyUnavailable()
	}
	if ok {
		return acceptTrustedECDSA(kid, jwk)
	}
	refreshErr := v.refresh.maybeRefresh(ctx, v.clock, kid, v.keys)
	if ctx.Err() != nil {
		return nil, denyUnavailable()
	}
	if refreshErr != nil {
		if !errors.Is(refreshErr, ErrInvalid) {
			return nil, refreshErr
		}
		return v.recheckPublished(ctx, kid, refreshErr)
	}
	return v.recheckPublished(ctx, kid, nil)
}

func (v *Verifier) recheckPublished(ctx context.Context, kid string, refreshErr error) (*ecdsa.PublicKey, error) {
	jwk, ok, err := v.keys.Lookup(ctx, kid)
	if ctx.Err() != nil {
		return nil, denyUnavailable()
	}
	if err != nil {
		return nil, denyUnavailable()
	}
	if !ok {
		if refreshErr != nil {
			return nil, refreshErr
		}
		if v.refresh != nil {
			v.refresh.rememberNegative(kid, v.clock.Now())
		}
		return nil, denyInvalid()
	}
	return acceptTrustedECDSA(kid, jwk)
}

func acceptTrustedECDSA(wantKid string, jwk domain.PublicJWK) (*ecdsa.PublicKey, error) {
	pub, err := acceptTrustedPublicJWK(wantKid, jwk)
	if err != nil {
		return nil, err
	}
	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, denyUnavailable()
	}
	return ecdsaPub, nil
}
