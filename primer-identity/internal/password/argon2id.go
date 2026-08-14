// Package password implements modern password KDFs for Identity credentials.
//
// Default algorithm: Argon2id encoded as a PHC string
// (https://github.com/P-H-C/phc-string-format/blob/master/phc-sf-spec.md):
//
//	$argon2id$v=19$m=<memory_kib>,t=<time>,p=<parallelism>$<b64salt>$<b64hash>
//
// Bounded parameters (OWASP-aligned production defaults; strict online ceilings):
//
//	memory      = 64 MiB (65536 KiB) default; online max 128 MiB
//	iterations  = 3 default; online max 4
//	parallelism = 4 default; online max 8
//	salt length = 16 bytes (crypto/rand); online 8–64
//	key length  = 32 bytes default; online 16–64
//
// Plaintext password hard limit: MaxPasswordBytes (1024). Empty and oversize
// inputs are rejected before any Argon2 evaluation.
//
// Verify is constant-time on the derived key via subtle.ConstantTimeCompare.
// Malformed or resource-unbounded PHC strings fail closed (false, error)
// before Argon2 allocation. Plaintext is never logged.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// AlgorithmArgon2id is the algorithm label stored on credentials_password.algorithm.
const AlgorithmArgon2id = "argon2id"

// MaxPasswordBytes is the hard upper bound on plaintext password size.
// Rejected before KDF so giant inputs cannot burn CPU/memory via Argon2.
// 1024 bytes is well above legitimate passphrases and password-manager secrets
// while remaining a fixed, defensible DoS ceiling.
const MaxPasswordBytes = 1024

// Default Argon2id parameters (documented, bounded).
const (
	DefaultTime    uint32 = 3
	DefaultMemory  uint32 = 64 * 1024 // KiB = 64 MiB
	DefaultThreads uint8  = 4
	DefaultKeyLen  uint32 = 32
	DefaultSaltLen        = 16
)

// Online parameter ceilings / floors shared by HashWithParams and Verify.
// These intentionally reject 1 GiB / 256 MiB, t=10, p=16 style costs.
const (
	minTime    uint32 = 1
	maxTime    uint32 = 4
	minMemory  uint32 = 8 * 1024   // 8 MiB
	maxMemory  uint32 = 128 * 1024 // 128 MiB
	minThreads uint8  = 1
	maxThreads uint8  = 8
	minKeyLen  uint32 = 16
	maxKeyLen  uint32 = 64
	minSaltLen        = 8
	maxSaltLen        = 64

	// Base64 (raw) length ceilings: allow a few chars past max decoded sizes so
	// oversize decoded salt/key are rejected by post-decode bounds (not only by
	// character count). Still far below multi-KiB DoS payloads.
	maxB64SaltChars = 96
	maxB64KeyChars  = 96
)

// Params holds Argon2id cost parameters.
type Params struct {
	Time    uint32
	Memory  uint32 // KiB
	Threads uint8
	KeyLen  uint32
}

// DefaultParams returns the Identity v1 Argon2id parameters.
func DefaultParams() Params {
	return Params{
		Time:    DefaultTime,
		Memory:  DefaultMemory,
		Threads: DefaultThreads,
		KeyLen:  DefaultKeyLen,
	}
}

var (
	// ErrMalformedHash is returned when a stored hash is not a valid PHC argon2id string
	// or carries parameters outside the online resource bounds.
	ErrMalformedHash = errors.New("malformed password hash")

	// ErrInvalidPassword is returned when plaintext is empty or exceeds MaxPasswordBytes.
	ErrInvalidPassword = errors.New("invalid password")
)

// Hash returns a PHC-encoded Argon2id hash of plaintext using DefaultParams
// and a fresh random salt.
func Hash(plaintext string) (phc string, err error) {
	return HashWithParams(plaintext, DefaultParams())
}

// HashWithParams hashes plaintext with the given params and a random salt.
// Rejects empty/oversize plaintext and out-of-bound params before KDF.
func HashWithParams(plaintext string, p Params) (string, error) {
	if err := validatePassword(plaintext); err != nil {
		return "", err
	}
	if err := validateParams(p); err != nil {
		return "", err
	}
	salt := make([]byte, DefaultSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	sum := argon2.IDKey([]byte(plaintext), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads,
		b64.EncodeToString(salt),
		b64.EncodeToString(sum),
	), nil
}

// Verify reports whether plaintext matches a PHC argon2id hash.
// Empty/oversize plaintext returns (false, ErrInvalidPassword) without KDF.
// Malformed or resource-unbounded hashes return (false, ErrMalformedHash) —
// fail closed, before Argon2 allocation. Comparison of derived keys uses
// constant-time equality.
func Verify(phc, plaintext string) (bool, error) {
	if err := validatePassword(plaintext); err != nil {
		return false, err
	}
	p, salt, want, err := decodePHC(phc)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plaintext), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return true, nil
	}
	return false, nil
}

// DummyVerify burns roughly one DefaultParams Argon2id evaluation so callers
// can equalize latency on missing accounts / missing credentials (non-enumerating).
func DummyVerify() {
	salt := make([]byte, DefaultSaltLen)
	_, _ = rand.Read(salt)
	_ = argon2.IDKey([]byte("identity-dummy-password"), salt, DefaultTime, DefaultMemory, DefaultThreads, DefaultKeyLen)
}

func validatePassword(plaintext string) error {
	if plaintext == "" {
		return fmt.Errorf("%w: empty", ErrInvalidPassword)
	}
	if len(plaintext) > MaxPasswordBytes {
		return fmt.Errorf("%w: exceeds %d bytes", ErrInvalidPassword, MaxPasswordBytes)
	}
	return nil
}

func validateParams(p Params) error {
	if p.Time < minTime || p.Time > maxTime {
		return fmt.Errorf("argon2id time out of bounds")
	}
	if p.Memory < minMemory || p.Memory > maxMemory {
		return fmt.Errorf("argon2id memory out of bounds")
	}
	if p.Threads < minThreads || p.Threads > maxThreads {
		return fmt.Errorf("argon2id threads out of bounds")
	}
	if p.KeyLen < minKeyLen || p.KeyLen > maxKeyLen {
		return fmt.Errorf("argon2id key length out of bounds")
	}
	return nil
}

func decodePHC(phc string) (Params, []byte, []byte, error) {
	// Canonical form only:
	// $argon2id$v=19$m=<mem>,t=<time>,p=<par>$<b64salt>$<b64hash>
	// Exact field count/order; no trailing segments or param garbage.
	if phc == "" || !strings.HasPrefix(phc, "$argon2id$") {
		return Params{}, nil, nil, ErrMalformedHash
	}
	parts := strings.Split(phc, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	if len(parts) != 6 {
		return Params{}, nil, nil, ErrMalformedHash
	}
	if parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrMalformedHash
	}
	if parts[2] != "v=19" || argon2.Version != 19 {
		return Params{}, nil, nil, ErrMalformedHash
	}

	mem, timeCost, threads, err := parseExactMTP(parts[3])
	if err != nil {
		return Params{}, nil, nil, ErrMalformedHash
	}

	saltB64 := parts[4]
	hashB64 := parts[5]
	if saltB64 == "" || hashB64 == "" {
		return Params{}, nil, nil, ErrMalformedHash
	}
	if len(saltB64) > maxB64SaltChars || len(hashB64) > maxB64KeyChars {
		return Params{}, nil, nil, ErrMalformedHash
	}
	if !isRawStdBase64(saltB64) || !isRawStdBase64(hashB64) {
		return Params{}, nil, nil, ErrMalformedHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(saltB64)
	if err != nil || len(salt) < minSaltLen || len(salt) > maxSaltLen {
		return Params{}, nil, nil, ErrMalformedHash
	}
	sum, err := b64.DecodeString(hashB64)
	if err != nil || len(sum) < int(minKeyLen) || len(sum) > int(maxKeyLen) {
		return Params{}, nil, nil, ErrMalformedHash
	}

	p := Params{
		Memory:  mem,
		Time:    timeCost,
		Threads: threads,
		KeyLen:  uint32(len(sum)),
	}
	// Validate online bounds *after* full decode so decoded KeyLen cannot bypass limits.
	if err := validateParams(p); err != nil {
		return Params{}, nil, nil, ErrMalformedHash
	}
	return p, salt, sum, nil
}

// parseExactMTP requires the exact canonical order m=<u>,t=<u>,p=<u> with no
// trailing characters, extra params, or reordered fields.
func parseExactMTP(s string) (memory uint32, timeCost uint32, threads uint8, err error) {
	const prefixM = "m="
	if !strings.HasPrefix(s, prefixM) {
		return 0, 0, 0, ErrMalformedHash
	}
	rest := s[len(prefixM):]
	memStr, rest, ok := splitExact(rest, ",t=")
	if !ok {
		return 0, 0, 0, ErrMalformedHash
	}
	timeStr, parStr, ok := splitExact(rest, ",p=")
	if !ok {
		return 0, 0, 0, ErrMalformedHash
	}
	if memStr == "" || timeStr == "" || parStr == "" {
		return 0, 0, 0, ErrMalformedHash
	}
	if !isAllDigits(memStr) || !isAllDigits(timeStr) || !isAllDigits(parStr) {
		return 0, 0, 0, ErrMalformedHash
	}
	mem64, err := strconv.ParseUint(memStr, 10, 32)
	if err != nil {
		return 0, 0, 0, ErrMalformedHash
	}
	time64, err := strconv.ParseUint(timeStr, 10, 32)
	if err != nil {
		return 0, 0, 0, ErrMalformedHash
	}
	par64, err := strconv.ParseUint(parStr, 10, 8)
	if err != nil {
		return 0, 0, 0, ErrMalformedHash
	}
	return uint32(mem64), uint32(time64), uint8(par64), nil
}

func splitExact(s, sep string) (head, tail string, ok bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+len(sep):], true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isRawStdBase64(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '+' || c == '/':
		default:
			return false
		}
	}
	return true
}
