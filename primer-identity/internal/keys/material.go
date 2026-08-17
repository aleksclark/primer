// Package keys generates, seals, and unseals Identity ES256 signing material.
// Private key bytes never appear in JSON, errors, or formatted output.
package keys

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const (
	EnvelopeVersion1      byte = 1
	MaxSealedPrivateBytes      = domain.MaxSealedPrivate
	nonceSize                  = 12
	aadVersion                 = "v1"
)

var (
	// ErrUnsealFailed is returned for any integrity, AAD, or key mismatch.
	ErrUnsealFailed = errors.New("unseal failed")
	// ErrUnknownEnvelope is returned when the sealed blob version is unsupported.
	ErrUnknownEnvelope = errors.New("unknown sealed envelope version")
	// ErrInvalidMaterial is returned when generation or sealing input is invalid.
	ErrInvalidMaterial = errors.New("invalid signing key material")
	// ErrMaterialDestroyed is returned when destroyed material is used.
	ErrMaterialDestroyed = errors.New("signing key material has been destroyed")
)

// Material is generated key custody state. The private key stays behind this
// type and the Sealed envelope; it is never serialized or exposed.
type Material struct {
	mu      *sync.RWMutex
	public  domain.PublicJWK
	private *privateKeyMaterial
}

type privateKeyMaterial struct{ key *ecdsa.PrivateKey }

// Format keeps the scalar out of formatting even if a Material value is
// copied and formatted instead of using Material's String method.
func (privateKeyMaterial) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[private key redacted]")
}

var _ crypto.Signer = (*Material)(nil)
var _ io.Closer = (*Material)(nil)

// Generate creates a fresh P-256 ES256 key with a UUID kid (>=128 bits).
func Generate() (*Material, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate es256 key: %w", err)
	}
	kid := uuid.NewString()
	pub, err := publicJWKFromECDSA(kid, &priv.PublicKey)
	if err != nil {
		zeroPrivate(priv)
		return nil, err
	}
	return &Material{mu: &sync.RWMutex{}, public: pub, private: &privateKeyMaterial{key: priv}}, nil
}

func publicJWKFromECDSA(kid string, pub *ecdsa.PublicKey) (domain.PublicJWK, error) {
	if pub == nil || pub.Curve != elliptic.P256() || pub.X == nil || pub.Y == nil || !pub.Curve.IsOnCurve(pub.X, pub.Y) {
		return domain.PublicJWK{}, fmt.Errorf("%w: public key must be P-256", ErrInvalidMaterial)
	}
	jwk := domain.PublicJWK{
		KTY: "EC",
		CRV: "P-256",
		Use: "sig",
		Alg: domain.SigningAlgES256,
		Kid: kid,
		X:   encodeCoord(pub.X.Bytes()),
		Y:   encodeCoord(pub.Y.Bytes()),
	}
	if _, err := domain.ParsePublicJWK(mustMarshal(jwk)); err != nil {
		return domain.PublicJWK{}, err
	}
	return jwk, nil
}

func encodeCoord(raw []byte) string {
	out := make([]byte, 32)
	if len(raw) > 32 {
		raw = raw[len(raw)-32:]
	}
	copy(out[32-len(raw):], raw)
	return base64.RawURLEncoding.EncodeToString(out)
}

func mustMarshal(j domain.PublicJWK) []byte {
	raw, err := j.MarshalJSON()
	if err != nil {
		return nil
	}
	return raw
}

// PublicJWK returns a copy of the safe public JWK. It never returns private
// scalar material and rejects use after destruction.
func (m *Material) PublicJWK() (domain.PublicJWK, error) {
	if m == nil || m.mu == nil {
		return domain.PublicJWK{}, fmt.Errorf("%w: nil material", ErrInvalidMaterial)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.private == nil || m.private.key == nil {
		return domain.PublicJWK{}, ErrMaterialDestroyed
	}
	return m.public, nil
}

// Public returns a fresh ECDSA public key copy for crypto.Signer consumers.
// Mutating the returned key cannot mutate custody state.
func (m *Material) Public() crypto.PublicKey {
	if m == nil || m.mu == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.private == nil || m.private.key == nil {
		return nil
	}
	return &ecdsa.PublicKey{
		Curve: m.private.key.PublicKey.Curve,
		X:     new(big.Int).Set(m.private.key.PublicKey.X),
		Y:     new(big.Int).Set(m.private.key.PublicKey.Y),
	}
}

// Sign implements crypto.Signer without exposing the private key pointer.
func (m *Material) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if m == nil || m.mu == nil {
		return nil, fmt.Errorf("%w: nil material", ErrInvalidMaterial)
	}
	if opts == nil || opts.HashFunc() != crypto.SHA256 {
		return nil, fmt.Errorf("%w: SHA-256 signer options are required", ErrInvalidMaterial)
	}
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("%w: SHA-256 digest must be %d bytes", ErrInvalidMaterial, sha256.Size)
	}
	if random == nil {
		return nil, fmt.Errorf("%w: random source is required", ErrInvalidMaterial)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.private == nil || m.private.key == nil {
		return nil, ErrMaterialDestroyed
	}
	return ecdsa.SignASN1(random, m.private.key, digest)
}

// Destroy zeros the private scalar and closes the material. It is safe to call
// concurrently with Sign and is idempotent.
func (m *Material) Destroy() error {
	if m == nil || m.mu == nil {
		return fmt.Errorf("%w: nil material", ErrInvalidMaterial)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.private == nil || m.private.key == nil {
		return nil
	}
	zeroPrivate(m.private.key)
	m.private.key = nil
	return nil
}

// Close implements io.Closer as an alias for Destroy.
func (m *Material) Close() error { return m.Destroy() }

func (m *Material) String() string {
	if m == nil || m.mu == nil {
		return "material <nil>"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.private == nil || m.private.key == nil {
		return "material{destroyed}"
	}
	return fmt.Sprintf("material{kid=%s alg=%s}", m.public.Kid, m.public.Alg)
}

func (m *Material) GoString() string { return m.String() }

// Format protects both pointer and copied-value formatting. Material copies
// share the same mutex/private holder, so the public-only String path remains
// race-safe and destruction is visible through every copy.
func (m Material) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, (&m).String())
}

// MarshalJSON refuses serialization rather than risking accidental custody
// representation. Call PublicJWK when a public representation is required.
func (Material) MarshalJSON() ([]byte, error) {
	return nil, errors.New("signing key material JSON serialization refused; use PublicJWK")
}

// sealSnapshot serializes only while holding the read lock, preventing a
// concurrent Destroy from changing the key during private-key encoding.
func (m *Material) sealSnapshot() (domain.PublicJWK, []byte, error) {
	if m == nil || m.mu == nil {
		return domain.PublicJWK{}, nil, fmt.Errorf("%w: missing material", ErrInvalidMaterial)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.private == nil || m.private.key == nil {
		return domain.PublicJWK{}, nil, ErrMaterialDestroyed
	}
	if m.private.key.Curve != elliptic.P256() {
		return domain.PublicJWK{}, nil, fmt.Errorf("%w: private key must be P-256", ErrInvalidMaterial)
	}
	expected, err := publicJWKFromECDSA(m.public.Kid, &m.private.key.PublicKey)
	if err != nil {
		return domain.PublicJWK{}, nil, err
	}
	if expected != m.public {
		return domain.PublicJWK{}, nil, fmt.Errorf("%w: public and private components do not match", ErrInvalidMaterial)
	}
	der, err := x509.MarshalECPrivateKey(m.private.key)
	if err != nil {
		return domain.PublicJWK{}, nil, fmt.Errorf("marshal private key: %w", err)
	}
	return m.public, der, nil
}

// Seal marshals the private key with the standard library and wraps it in a
// versioned AES-256-GCM envelope bound to version+kid+alg+public digest.
func Seal(mat *Material, sealKey [32]byte) ([]byte, error) {
	public, der, err := mat.sealSnapshot()
	if err != nil {
		return nil, err
	}
	defer zeroBytes(der)

	block, err := aes.NewCipher(sealKey[:])
	if err != nil {
		return nil, fmt.Errorf("seal cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("seal aead: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("seal nonce: %w", err)
	}
	aad, err := bindAAD(public)
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, der, aad)
	out := make([]byte, 1+len(nonce)+len(ciphertext))
	out[0] = EnvelopeVersion1
	copy(out[1:], nonce)
	copy(out[1+len(nonce):], ciphertext)
	if len(out) > MaxSealedPrivateBytes {
		return nil, fmt.Errorf("%w: sealed blob exceeds bound", ErrInvalidMaterial)
	}
	return out, nil
}

// Unseal verifies the versioned envelope against the supplied public JWK and
// returns private material only after AAD and component checks succeed.
func Unseal(sealed []byte, public domain.PublicJWK, sealKey [32]byte) (*Material, error) {
	if len(sealed) == 0 || len(sealed) > MaxSealedPrivateBytes {
		return nil, ErrUnsealFailed
	}
	if sealed[0] != EnvelopeVersion1 {
		return nil, ErrUnknownEnvelope
	}
	if len(sealed) < 1+nonceSize+16 {
		return nil, ErrUnsealFailed
	}
	if _, err := public.ECDSAPublic(); err != nil {
		return nil, ErrUnsealFailed
	}
	nonce := sealed[1 : 1+nonceSize]
	ciphertext := sealed[1+nonceSize:]
	aad, err := bindAAD(public)
	if err != nil {
		return nil, ErrUnsealFailed
	}
	block, err := aes.NewCipher(sealKey[:])
	if err != nil {
		return nil, ErrUnsealFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrUnsealFailed
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrUnsealFailed
	}
	defer zeroBytes(plain)
	priv, err := x509.ParseECPrivateKey(plain)
	if err != nil || priv == nil || priv.Curve != elliptic.P256() {
		if priv != nil {
			zeroPrivate(priv)
		}
		return nil, ErrUnsealFailed
	}
	expected, err := publicJWKFromECDSA(public.Kid, &priv.PublicKey)
	if err != nil || expected != public {
		zeroPrivate(priv)
		return nil, ErrUnsealFailed
	}
	return &Material{mu: &sync.RWMutex{}, public: public, private: &privateKeyMaterial{key: priv}}, nil
}

func bindAAD(public domain.PublicJWK) ([]byte, error) {
	raw, err := public.MarshalJSON()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	// version|kid|alg|sha256(public-jwk)
	out := make([]byte, 0, len(aadVersion)+1+len(public.Kid)+1+len(public.Alg)+1+len(sum))
	out = append(out, aadVersion...)
	out = append(out, '|')
	out = append(out, public.Kid...)
	out = append(out, '|')
	out = append(out, public.Alg...)
	out = append(out, '|')
	out = append(out, sum[:]...)
	return out, nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func zeroPrivate(priv *ecdsa.PrivateKey) {
	if priv == nil || priv.D == nil {
		return
	}
	words := priv.D.Bits()
	for i := range words {
		words[i] = 0
	}
	priv.D.SetInt64(0)
}

// ConstantTimeKidEqual compares kids without leaking length-adjacent secrets.
func ConstantTimeKidEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// PublicSetETag returns deterministic ETag material for an authenticated
// public JWK set. It is HTTP-free helper output: a quoted weak-safe hash of
// canonical public documents only.
func PublicSetETag(pubs []domain.PublicJWK) (string, error) {
	h := sha256.New()
	for _, pub := range pubs {
		raw, err := pub.MarshalJSON()
		if err != nil {
			return "", err
		}
		_, _ = h.Write(raw)
		_, _ = h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return `W/"` + base64.RawURLEncoding.EncodeToString(sum) + `"`, nil
}
