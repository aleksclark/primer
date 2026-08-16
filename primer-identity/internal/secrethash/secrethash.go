// Package secrethash stores no plaintext secrets. A hash is HMAC-SHA-256 keyed
// by the exact 32-byte pepper selected by version over the unambiguous message:
// uint16be(pepperVersion) || uint32be(len(context)) || context ||
// uint32be(len(secret)) || secret. It always returns exactly 32 bytes.
package secrethash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid secret hash configuration")

const (
	maxContextBytes = 128
	maxSecretBytes  = 4096
)

type Peppers map[int][]byte

func Hash(peppers Peppers, version int, context string, secret []byte) ([]byte, error) {
	if version <= 0 || context == "" || len(secret) == 0 || len(secret) > maxSecretBytes || len(context) > maxContextBytes || !utf8.ValidString(context) {
		return nil, ErrInvalid
	}
	for _, r := range context {
		if r < 0x20 || r == 0x7f {
			return nil, ErrInvalid
		}
	}
	pepper, ok := peppers[version]
	if !ok || len(pepper) != 32 {
		return nil, ErrInvalid
	}
	copied := append([]byte(nil), pepper...)
	message := make([]byte, 0, 2+4+len(context)+4+len(secret))
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], uint16(version))
	message = append(message, v[:]...)
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(context)))
	message = append(message, l[:]...)
	message = append(message, context...)
	binary.BigEndian.PutUint32(l[:], uint32(len(secret)))
	message = append(message, l[:]...)
	message = append(message, secret...)
	mac := hmac.New(sha256.New, copied)
	_, _ = mac.Write(message)
	for i := range copied {
		copied[i] = 0
	}
	return mac.Sum(nil), nil
}

func Equal(left, right []byte) bool {
	return len(left) == sha256.Size && len(right) == sha256.Size && hmac.Equal(left, right)
}
