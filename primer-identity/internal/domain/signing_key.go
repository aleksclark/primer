package domain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	SigningAlgES256 = "ES256"

	SigningKeyStatusActive    = "active"
	SigningKeyStatusNext      = "next"
	SigningKeyStatusRetired   = "retired"
	SigningKeyStatusDestroyed = "destroyed"

	MaxKidLen          = 128
	MaxPublicJWKBytes  = 2048
	MaxSealedPrivate   = 4096
	publicCoordB64Len  = 43
	publicCoordByteLen = 32
)

// PublicJWK is the safe, public-only ES256 JWK published on JWKS.
type PublicJWK struct {
	KTY string `json:"kty"`
	CRV string `json:"crv"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// SigningKey is the persisted public metadata for a signing key. Private
// material never lives on this type.
type SigningKey struct {
	ID          uuid.UUID  `json:"id"`
	Kid         string     `json:"kid"`
	Alg         string     `json:"alg"`
	KeyVersion  int        `json:"key_version"`
	PublicJWK   PublicJWK  `json:"public_jwk"`
	Status      string     `json:"status"`
	NotBefore   time.Time  `json:"not_before"`
	NotAfter    *time.Time `json:"not_after,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	RetiredAt   *time.Time `json:"retired_at,omitempty"`
	DestroyedAt *time.Time `json:"destroyed_at,omitempty"`
}

func (k SigningKey) String() string {
	return fmt.Sprintf("signing-key kid=%s status=%s alg=%s", k.Kid, k.Status, k.Alg)
}

func (k SigningKey) GoString() string { return k.String() }

// MarshalJSON emits only the canonical public JWK field set, in a stable order.
func (j PublicJWK) MarshalJSON() ([]byte, error) {
	if err := j.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Grow(192)
	buf.WriteString(`{"kty":`)
	writeJSONString(&buf, j.KTY)
	buf.WriteString(`,"crv":`)
	writeJSONString(&buf, j.CRV)
	buf.WriteString(`,"use":`)
	writeJSONString(&buf, j.Use)
	buf.WriteString(`,"alg":`)
	writeJSONString(&buf, j.Alg)
	buf.WriteString(`,"kid":`)
	writeJSONString(&buf, j.Kid)
	buf.WriteString(`,"x":`)
	writeJSONString(&buf, j.X)
	buf.WriteString(`,"y":`)
	writeJSONString(&buf, j.Y)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func writeJSONString(buf *bytes.Buffer, s string) {
	encoded, err := json.Marshal(s)
	if err != nil {
		buf.WriteString(`""`)
		return
	}
	buf.Write(encoded)
}

// ParsePublicJWK accepts only a canonical bounded public ES256 JWK.
func ParsePublicJWK(raw []byte) (PublicJWK, error) {
	if len(raw) == 0 || len(raw) > MaxPublicJWKBytes {
		return PublicJWK{}, invalidf("public_jwk", "document size is invalid")
	}
	obj, err := decodeStrictObject(raw)
	if err != nil {
		return PublicJWK{}, err
	}
	jwk := PublicJWK{
		KTY: obj["kty"],
		CRV: obj["crv"],
		Use: obj["use"],
		Alg: obj["alg"],
		Kid: obj["kid"],
		X:   obj["x"],
		Y:   obj["y"],
	}
	if err := jwk.validate(); err != nil {
		return PublicJWK{}, err
	}
	return jwk, nil
}

func decodeStrictObject(raw []byte) (map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, invalidf("public_jwk", "must be a JSON object")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, invalidf("public_jwk", "must be a JSON object")
	}
	allowed := map[string]struct{}{
		"kty": {}, "crv": {}, "use": {}, "alg": {}, "kid": {}, "x": {}, "y": {},
	}
	out := make(map[string]string, 7)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, invalidf("public_jwk", "malformed JSON")
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, invalidf("public_jwk", "malformed JSON")
		}
		if _, known := allowed[key]; !known {
			return nil, invalidf("public_jwk", "contains forbidden or unknown field")
		}
		if _, dup := out[key]; dup {
			return nil, invalidf("public_jwk", "contains duplicate field")
		}
		valTok, err := dec.Token()
		if err != nil {
			return nil, invalidf("public_jwk", "malformed JSON")
		}
		val, ok := valTok.(string)
		if !ok {
			return nil, invalidf("public_jwk", "field %s must be a string", key)
		}
		out[key] = val
	}
	end, err := dec.Token()
	if err != nil {
		return nil, invalidf("public_jwk", "malformed JSON")
	}
	if endDelim, ok := end.(json.Delim); !ok || endDelim != '}' {
		return nil, invalidf("public_jwk", "malformed JSON")
	}
	if dec.More() {
		return nil, invalidf("public_jwk", "trailing JSON")
	}
	return out, nil
}

func (j PublicJWK) validate() error {
	if j.KTY != "EC" || j.CRV != "P-256" || j.Use != "sig" || j.Alg != SigningAlgES256 {
		return invalidf("public_jwk", "must be a public ES256 P-256 signing key")
	}
	if err := ValidateSigningKid(j.Kid); err != nil {
		return err
	}
	if err := validateCoord("x", j.X); err != nil {
		return err
	}
	if err := validateCoord("y", j.Y); err != nil {
		return err
	}
	if _, err := j.ECDSAPublic(); err != nil {
		return err
	}
	return nil
}

func validateCoord(field, value string) error {
	if len(value) != publicCoordB64Len {
		return invalidf("public_jwk", "%s must be a 32-byte base64url coordinate", field)
	}
	if strings.ContainsAny(value, "+/=") {
		return invalidf("public_jwk", "%s must be canonical base64url without padding", field)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != publicCoordByteLen || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return invalidf("public_jwk", "%s must be a 32-byte base64url coordinate", field)
	}
	return nil
}

// ECDSAPublic reconstructs the P-256 public key and rejects invalid points.
func (j PublicJWK) ECDSAPublic() (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil || len(xBytes) != publicCoordByteLen {
		return nil, invalidf("public_jwk", "x is not a valid coordinate")
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil || len(yBytes) != publicCoordByteLen {
		return nil, invalidf("public_jwk", "y is not a valid coordinate")
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, invalidf("public_jwk", "coordinates are not on P-256")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// Thumbprint returns the RFC 7638 SHA-256 thumbprint of the public JWK.
// It is computed from the canonical public members only.
func (j PublicJWK) Thumbprint() (string, error) {
	if err := j.validate(); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	buf.WriteString(`{"crv":`)
	writeJSONString(&buf, j.CRV)
	buf.WriteString(`,"kty":`)
	writeJSONString(&buf, j.KTY)
	buf.WriteString(`,"x":`)
	writeJSONString(&buf, j.X)
	buf.WriteString(`,"y":`)
	writeJSONString(&buf, j.Y)
	buf.WriteByte('}')
	sum := sha256.Sum256(buf.Bytes())
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// ValidateSigningKid requires a nonempty bounded UTF-8 identifier.
func ValidateSigningKid(kid string) error {
	if kid == "" {
		return invalidf("kid", "must not be empty")
	}
	if !utf8.ValidString(kid) {
		return invalidf("kid", "must be valid UTF-8")
	}
	if utf8.RuneCountInString(kid) > MaxKidLen {
		return invalidf("kid", "exceeds max length %d", MaxKidLen)
	}
	for _, r := range kid {
		if r < 0x20 || r == 0x7f {
			return invalidf("kid", "must not contain control characters")
		}
	}
	return nil
}

// ValidateSigningKeyStatus accepts only the persisted lifecycle states.
func ValidateSigningKeyStatus(status string) error {
	switch status {
	case SigningKeyStatusActive, SigningKeyStatusNext, SigningKeyStatusRetired, SigningKeyStatusDestroyed:
		return nil
	case "":
		return invalidf("status", "must not be empty")
	default:
		return invalidf("status", "unsupported signing key status %q", status)
	}
}
