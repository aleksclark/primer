package token

import (
	"bytes"
	"encoding/json"

	"github.com/aleksclark/primer/identity/internal/domain"
)

var jwksAllowed = map[string]struct{}{"keys": {}}

// ParseJWKS accepts a bounded public JWKS document of unique ES256 keys.
func ParseJWKS(raw []byte) ([]domain.PublicJWK, error) {
	if len(raw) == 0 || len(raw) > maxJWKSBytes {
		return nil, denyInvalid()
	}
	obj, err := decodeStrictJSONObject(raw, jwksAllowed, maxJWKSBytes)
	if err != nil {
		return nil, err
	}
	keysRaw, ok := obj["keys"]
	if !ok || len(obj) != 1 {
		return nil, denyInvalid()
	}
	if err := prevalidateJSON(keysRaw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(keysRaw))
	tok, err := dec.Token()
	if err != nil {
		return nil, denyInvalid()
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, denyInvalid()
	}
	var items []json.RawMessage
	for dec.More() {
		var item json.RawMessage
		if err := dec.Decode(&item); err != nil {
			return nil, denyInvalid()
		}
		items = append(items, item)
	}
	end, err := dec.Token()
	if err != nil {
		return nil, denyInvalid()
	}
	if endDelim, ok := end.(json.Delim); !ok || endDelim != ']' {
		return nil, denyInvalid()
	}
	if err := requireExactEOF(dec, keysRaw); err != nil {
		return nil, err
	}
	if len(items) == 0 || len(items) > maxPublishedKeys {
		return nil, denyInvalid()
	}
	out := make([]domain.PublicJWK, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		jwk, err := domain.ParsePublicJWK(item)
		if err != nil {
			return nil, denyInvalid()
		}
		if _, dup := seen[jwk.Kid]; dup {
			return nil, denyInvalid()
		}
		seen[jwk.Kid] = struct{}{}
		out = append(out, jwk)
	}
	return out, nil
}
