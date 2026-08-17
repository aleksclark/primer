// Package stateseal implements the exact state-seal-v1 OAuth state envelope.
// Plaintext is never formatted or returned in errors; callers must Zero it once
// a terminal outcome has been constructed.
package stateseal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	envelopeVersion  byte = 0x01
	minStateBytes         = 1
	maxStateBytes         = 1024
	nonceBytes            = 12
	tagBytes              = 16
	minEnvelopeBytes      = 1 + nonceBytes + tagBytes + minStateBytes
	maxEnvelopeBytes      = 1 + nonceBytes + tagBytes + maxStateBytes
	maxRedirectBytes      = 2048
	maxResourceBytes      = 2048
	maxAudienceBytes      = 128
	maxAADBytes           = 4287
)

// ErrInvalid is intentionally non-oracular and contains no plaintext or AAD.
var ErrInvalid = errors.New("invalid sealed oauth state")

// Bindings are the exact immutable OAuth registration and transaction binding.
type Bindings struct {
	TransactionID uuid.UUID
	OAuthClientID uuid.UUID
	RedirectURI   string
	ResourceURI   string
	Audience      string
}

// Config carries one active key and any still-valid decrypt-only keys. Keys are
// copied, and only ActiveKeyVersion is selected for encryption.
type Config struct {
	ActiveKeyVersion int
	Keys             map[int][]byte
	NonceSource      io.Reader
}

// Sealer has no exposed key material.
type Sealer struct {
	active int
	keys   map[int][]byte
	nonce  io.Reader
}

// New validates and defensively copies exact AES-256 keys.
func New(cfg Config) (*Sealer, error) {
	if cfg.ActiveKeyVersion <= 0 || len(cfg.Keys) == 0 {
		return nil, ErrInvalid
	}
	keys := make(map[int][]byte, len(cfg.Keys))
	for version, key := range cfg.Keys {
		if version <= 0 || len(key) != 32 {
			return nil, ErrInvalid
		}
		keys[version] = append([]byte(nil), key...)
	}
	if _, ok := keys[cfg.ActiveKeyVersion]; !ok {
		return nil, ErrInvalid
	}
	if cfg.NonceSource == nil {
		cfg.NonceSource = rand.Reader
	}
	return &Sealer{active: cfg.ActiveKeyVersion, keys: keys, nonce: cfg.NonceSource}, nil
}

// AAD returns the frozen, byte-framed state-seal-v1 additional authenticated
// data. UUIDs use their RFC 4122 network-order 16-byte representation.
func AAD(b Bindings) ([]byte, error) {
	if !validText(b.RedirectURI, maxRedirectBytes) || !validText(b.ResourceURI, maxResourceBytes) || !validText(b.Audience, maxAudienceBytes) {
		return nil, ErrInvalid
	}
	// The documented constant has 18 bytes. Do not derive it from strings that
	// can accidentally introduce encoding changes.
	aad := make([]byte, 0, maxAADBytes)
	aad = append(aad, []byte("primer.oauth.state")...)
	aad = append(aad, envelopeVersion)
	aad = append(aad, b.TransactionID[:]...)
	aad = append(aad, b.OAuthClientID[:]...)
	for _, value := range []string{b.RedirectURI, b.ResourceURI, b.Audience} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(value)))
		aad = append(aad, length[:]...)
		aad = append(aad, value...)
	}
	if len(aad) > maxAADBytes {
		return nil, ErrInvalid
	}
	return aad, nil
}

func validText(value string, max int) bool {
	if len(value) == 0 || len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Seal returns state-seal-v1 bytes and the selected active key version.
func (s *Sealer) Seal(state []byte, bindings Bindings) ([]byte, int, error) {
	if s == nil || len(state) < minStateBytes || len(state) > maxStateBytes {
		return nil, 0, ErrInvalid
	}
	aad, err := AAD(bindings)
	if err != nil {
		return nil, 0, ErrInvalid
	}
	key, ok := s.keys[s.active]
	if !ok || len(key) != 32 {
		return nil, 0, ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, 0, ErrInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || gcm.NonceSize() != nonceBytes || gcm.Overhead() != tagBytes {
		return nil, 0, ErrInvalid
	}
	nonce := make([]byte, nonceBytes)
	if _, err = io.ReadFull(s.nonce, nonce); err != nil {
		return nil, 0, ErrInvalid
	}
	sealed := gcm.Seal(nil, nonce, state, aad)
	envelope := make([]byte, 0, 1+len(nonce)+len(sealed))
	envelope = append(envelope, envelopeVersion)
	envelope = append(envelope, nonce...)
	envelope = append(envelope, sealed...)
	return envelope, s.active, nil
}

// Open uses only the recorded positive key version; it never guesses or falls
// back. All malformed or authenticated failures share ErrInvalid.
func (s *Sealer) Open(envelope []byte, keyVersion int, bindings Bindings) ([]byte, error) {
	if s == nil || keyVersion <= 0 || len(envelope) < minEnvelopeBytes || len(envelope) > maxEnvelopeBytes || envelope[0] != envelopeVersion {
		return nil, ErrInvalid
	}
	aad, err := AAD(bindings)
	if err != nil {
		return nil, ErrInvalid
	}
	key, ok := s.keys[keyVersion]
	if !ok || len(key) != 32 {
		return nil, ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || gcm.NonceSize() != nonceBytes || gcm.Overhead() != tagBytes {
		return nil, ErrInvalid
	}
	plain, err := gcm.Open(nil, envelope[1:1+nonceBytes], envelope[1+nonceBytes:], aad)
	if err != nil || len(plain) < minStateBytes || len(plain) > maxStateBytes {
		Zero(plain)
		return nil, ErrInvalid
	}
	return plain, nil
}

// Zero explicitly wipes a sensitive plaintext buffer. KeepAlive prevents the
// compiler from proving the buffer dead before the overwrite loop.
func Zero(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
	runtime.KeepAlive(buf)
}
