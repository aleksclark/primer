package bff

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const pkceUnreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// GeneratePKCE returns a 43..128 character RFC 7636 verifier and its S256
// unpadded base64url challenge.
func GeneratePKCE() (verifier, challenge string, err error) {
	// 32 random bytes → 43 unpadded base64url characters (exact S256 input).
	raw, err := randomURLToken(32)
	if err != nil {
		return "", "", fmt.Errorf("bff pkce: %w", err)
	}
	if len(raw) < minVerifierLen {
		return "", "", fmt.Errorf("bff pkce: verifier too short")
	}
	if len(raw) > maxVerifierLen {
		raw = raw[:maxVerifierLen]
	}
	for i := 0; i < len(raw); i++ {
		if !isPKCEUnreserved(raw[i]) {
			return "", "", fmt.Errorf("bff pkce: verifier is not RFC 7636 unreserved")
		}
	}
	sum := sha256.Sum256([]byte(raw))
	return raw, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func isPKCEUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '-', c == '.', c == '_', c == '~':
		return true
	default:
		return false
	}
}

func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
