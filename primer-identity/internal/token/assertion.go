package token

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
)

var assertionHeaderAllowed = map[string]struct{}{
	"alg": {},
	"typ": {},
	"kid": {},
}

var assertionClaimsAllowed = map[string]struct{}{
	"iss": {}, "sub": {}, "aud": {}, "exp": {}, "iat": {}, "nbf": {}, "jti": {},
}

// ParseClientAssertion validates a library-level private_key_jwt assertion.
// Durable replay remains a later repository concern.
func ParseClientAssertion(raw string, in AssertionInput) (ClientAssertion, error) {
	if err := requireClientID(in.ClientID); err != nil {
		return ClientAssertion{}, err
	}
	if err := boundedUTF8("aud", in.Audience, 2048); err != nil {
		return ClientAssertion{}, err
	}
	if len(in.Registered) == 0 || len(in.Registered) > maxPublishedKeys {
		return ClientAssertion{}, denyInvalid()
	}
	maxLife := in.MaxLifetime
	if maxLife <= 0 || maxLife > domain.MaxAssertionTTL {
		maxLife = domain.MaxAssertionTTL
	}
	headerSeg, payloadSeg, sigSeg, err := splitCompact(raw)
	if err != nil {
		return ClientAssertion{}, err
	}
	headerRaw, err := decodeRawURL(headerSeg, maxHeaderBytes)
	if err != nil {
		return ClientAssertion{}, err
	}
	payloadRaw, err := decodeRawURL(payloadSeg, maxPayloadBytes)
	if err != nil {
		return ClientAssertion{}, err
	}
	sigRaw, err := decodeRawURL(sigSeg, maxSignatureBytes)
	if err != nil {
		return ClientAssertion{}, err
	}
	header, err := parseAssertionHeader(headerRaw)
	if err != nil {
		return ClientAssertion{}, err
	}
	var pub *ecdsa.PublicKey
	for _, jwk := range in.Registered {
		if jwk.Kid != header.kid {
			continue
		}
		got, err := acceptPublicJWK(header.kid, jwk)
		if err != nil {
			return ClientAssertion{}, denyInvalid()
		}
		ecdsaPub, ok := got.key.(*ecdsa.PublicKey)
		if !ok {
			return ClientAssertion{}, denyInvalid()
		}
		pub = ecdsaPub
		break
	}
	if pub == nil {
		return ClientAssertion{}, denyInvalid()
	}
	if err := verifyES256(pub, []byte(headerSeg+"."+payloadSeg), sigRaw); err != nil {
		return ClientAssertion{}, err
	}
	claims, err := parseAssertionClaims(payloadRaw)
	if err != nil {
		return ClientAssertion{}, err
	}
	if claims.iss != in.ClientID || claims.sub != in.ClientID || claims.aud != in.Audience {
		return ClientAssertion{}, denyInvalid()
	}
	if claims.exp <= claims.iat || claims.exp-claims.iat > int64(maxLife/time.Second) {
		return ClientAssertion{}, denyInvalid()
	}
	now := in.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	iat := time.Unix(claims.iat, 0).UTC()
	exp := time.Unix(claims.exp, 0).UTC()
	if now.Before(iat.Add(-AssertionClockSkew)) || now.After(exp.Add(AssertionClockSkew)) {
		return ClientAssertion{}, denyInvalid()
	}
	if claims.hasNBF {
		nbf := time.Unix(claims.nbf, 0).UTC()
		if now.Before(nbf.Add(-AssertionClockSkew)) || claims.nbf < claims.iat-int64(AssertionClockSkew/time.Second) || claims.nbf > claims.exp {
			return ClientAssertion{}, denyInvalid()
		}
	}
	return ClientAssertion{
		ClientID:  claims.iss,
		Subject:   claims.sub,
		Audience:  claims.aud,
		Kid:       header.kid,
		JTI:       claims.jti,
		IssuedAt:  iat,
		ExpiresAt: exp,
	}, nil
}

func parseAssertionHeader(raw []byte) (parsedHeader, error) {
	obj, err := decodeStrictJSONObject(raw, assertionHeaderAllowed, maxHeaderBytes)
	if err != nil {
		return parsedHeader{}, err
	}
	if _, hasJKU := obj["jku"]; hasJKU {
		return parsedHeader{}, denyInvalid()
	}
	alg, err := decodeJSONString(obj["alg"])
	if err != nil || alg != Algorithm {
		return parsedHeader{}, denyInvalid()
	}
	if typRaw, ok := obj["typ"]; ok {
		typ, err := decodeJSONString(typRaw)
		if err != nil || typ != assertionTyp {
			return parsedHeader{}, denyInvalid()
		}
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

type parsedAssertion struct {
	iss, sub, aud, jti string
	iat, exp, nbf      int64
	hasNBF             bool
}

func parseAssertionClaims(raw []byte) (parsedAssertion, error) {
	obj, err := decodeStrictJSONObject(raw, assertionClaimsAllowed, maxPayloadBytes)
	if err != nil {
		return parsedAssertion{}, err
	}
	for _, key := range []string{"iss", "sub", "aud", "exp", "iat", "jti"} {
		if _, ok := obj[key]; !ok {
			return parsedAssertion{}, denyInvalid()
		}
	}
	out := parsedAssertion{}
	if out.iss, err = decodeJSONString(obj["iss"]); err != nil {
		return parsedAssertion{}, err
	}
	if out.sub, err = decodeJSONString(obj["sub"]); err != nil {
		return parsedAssertion{}, err
	}
	if out.aud, err = decodeJSONString(obj["aud"]); err != nil {
		return parsedAssertion{}, err
	}
	if out.jti, err = decodeJSONString(obj["jti"]); err != nil {
		return parsedAssertion{}, err
	}
	if out.iat, err = decodeJSONInt(obj["iat"]); err != nil {
		return parsedAssertion{}, err
	}
	if out.exp, err = decodeJSONInt(obj["exp"]); err != nil {
		return parsedAssertion{}, err
	}
	if nbfRaw, ok := obj["nbf"]; ok {
		out.hasNBF = true
		if out.nbf, err = decodeJSONInt(nbfRaw); err != nil {
			return parsedAssertion{}, err
		}
	}
	if err := requireClientID(out.iss); err != nil || out.iss != out.sub {
		return parsedAssertion{}, denyInvalid()
	}
	if err := boundedUTF8("jti", out.jti, maxAssertionJTIBytes); err != nil {
		return parsedAssertion{}, err
	}
	return out, nil
}

// GenerateRefreshSecret returns a 256-bit opaque refresh secret. No redemption.
func GenerateRefreshSecret() ([]byte, error) {
	out := make([]byte, refreshSecretBytes)
	if _, err := rand.Read(out); err != nil {
		return nil, denyUnavailable()
	}
	return out, nil
}

// HashRefreshSecret is a thin helper that SHA-256s an opaque secret. Callers
// that persist refresh material should prefer the repo secrethash package.
func HashRefreshSecret(secret []byte) ([32]byte, error) {
	if len(secret) < refreshSecretBytes {
		return [32]byte{}, denyInvalid()
	}
	return sha256.Sum256(secret), nil
}
