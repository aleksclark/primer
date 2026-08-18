// Package fingerprint provides deterministic hashes for persisted JSON inputs.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// CanonicalJSON parses and re-encodes JSON. encoding/json emits object keys in
// deterministic order, so semantically equivalent object snapshots hash alike.
func CanonicalJSON(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return json.Marshal(value)
}

// Hash returns the lowercase SHA-256 hex digest of canonical JSON.
func Hash(raw []byte) (string, error) {
	canonical, err := CanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
