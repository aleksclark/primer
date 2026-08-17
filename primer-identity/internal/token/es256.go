package token

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"math/big"
)

var p256HalfN = func() *big.Int {
	n := new(big.Int).Set(elliptic.P256().Params().N)
	return n.Rsh(n, 1)
}()

func canonicalizeES256Signature(der []byte) ([]byte, error) {
	var parsed struct {
		R *big.Int
		S *big.Int
	}
	rest, err := asn1.Unmarshal(der, &parsed)
	if err != nil || len(rest) != 0 || parsed.R == nil || parsed.S == nil {
		return nil, denyUnavailable()
	}
	n := elliptic.P256().Params().N
	if parsed.S.Cmp(p256HalfN) > 0 {
		parsed.S = new(big.Int).Sub(n, parsed.S)
	}
	return encodeRawRS(parsed.R, parsed.S)
}

func encodeRawRS(r, s *big.Int) ([]byte, error) {
	n := elliptic.P256().Params().N
	if r == nil || s == nil || r.Sign() <= 0 || s.Sign() <= 0 || r.Cmp(n) >= 0 || s.Cmp(n) >= 0 {
		return nil, denyUnavailable()
	}
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	if len(rBytes) > 32 || len(sBytes) > 32 {
		return nil, denyUnavailable()
	}
	out := make([]byte, jwsSignatureRawLen)
	copy(out[32-len(rBytes):32], rBytes)
	copy(out[64-len(sBytes):], sBytes)
	return out, nil
}

func parseRawRS(sig []byte) (r, s *big.Int, err error) {
	if len(sig) != jwsSignatureRawLen {
		return nil, nil, denyInvalid()
	}
	r = new(big.Int).SetBytes(sig[:32])
	s = new(big.Int).SetBytes(sig[32:])
	n := elliptic.P256().Params().N
	if r.Sign() <= 0 || s.Sign() <= 0 || r.Cmp(n) >= 0 || s.Cmp(n) >= 0 {
		return nil, nil, denyInvalid()
	}
	if s.Cmp(p256HalfN) > 0 {
		return nil, nil, denyInvalid()
	}
	return r, s, nil
}

func verifyES256(pub *ecdsa.PublicKey, signingInput, rawSig []byte) error {
	if pub == nil || pub.Curve != elliptic.P256() || pub.X == nil || pub.Y == nil || !pub.Curve.IsOnCurve(pub.X, pub.Y) {
		return denyInvalid()
	}
	r, s, err := parseRawRS(rawSig)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(signingInput)
	if !ecdsa.Verify(pub, sum[:], r, s) {
		return denyInvalid()
	}
	return nil
}

func publicMatchesJWK(pub any, jwkPub *ecdsa.PublicKey) error {
	got, ok := pub.(*ecdsa.PublicKey)
	if !ok || got == nil || jwkPub == nil {
		return denyUnavailable()
	}
	if got.Curve != elliptic.P256() || jwkPub.Curve != elliptic.P256() {
		return denyUnavailable()
	}
	if got.X == nil || got.Y == nil || jwkPub.X == nil || jwkPub.Y == nil {
		return denyUnavailable()
	}
	if got.X.Cmp(jwkPub.X) != 0 || got.Y.Cmp(jwkPub.Y) != 0 {
		return denyUnavailable()
	}
	return nil
}
