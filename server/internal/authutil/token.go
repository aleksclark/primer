// Package authutil issues opaque tokens and pairing codes for parent sessions
// and student devices. Secrets are stored only as SHA-256 hashes.
package authutil

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// pairingAlphabet omits characters that are easy to confuse when read aloud.
const pairingAlphabet = "ACDEFGHJKMNPQRTUVWXY34679"

// PairingCodeLength is the number of characters in a pairing code.
const PairingCodeLength = 6

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

// NewToken returns a fresh opaque token and its hash. The plaintext is returned
// to the caller exactly once.
func NewToken() (token, hash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}

// HashToken returns the stored representation of a high-entropy token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// EqualHash compares a presented token against a stored hash in constant time.
func EqualHash(presented, storedHash string) bool {
	if storedHash == "" {
		return false
	}
	got := HashToken(presented)
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}
