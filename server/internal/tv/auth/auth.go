// Package auth issues and verifies device credentials. Pairing codes are
// short and human-transcribable because a parent reads them off the admin UI
// and types them into the app; device tokens are long random secrets stored
// only as a SHA-256 hash so a database disclosure cannot be replayed.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// pairingAlphabet omits characters that are easy to confuse when read aloud
// or typed on a D-pad keyboard (0/O, 1/I/L, 2/Z, 5/S, 8/B).
const pairingAlphabet = "ACDEFGHJKMNPQRTUVWXY34679"

// PairingCodeLength is the number of characters in a pairing code.
const PairingCodeLength = 6

// tokenBytes is the entropy behind a device token.
const tokenBytes = 32

// NewPairingCode returns a fresh random pairing code.
func NewPairingCode() (string, error) {
	buf := make([]byte, PairingCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate pairing code: %w", err)
	}
	out := make([]byte, PairingCodeLength)
	for i, b := range buf {
		out[i] = pairingAlphabet[int(b)%len(pairingAlphabet)]
	}
	return string(out), nil
}

// NewToken returns a fresh device token and its hash. The plaintext is
// returned to the caller exactly once, at pairing time.
func NewToken() (token, hash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate device token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}

// HashToken returns the stored representation of a device token. A plain
// SHA-256 is sufficient because tokens are full-entropy random values, not
// user-chosen passwords, so there is nothing to brute-force.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
