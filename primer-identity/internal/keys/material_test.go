package keys_test

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
)

func testSealKey(t *testing.T) [32]byte {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("PLANT_SEAL_SECRET_VALUE_AAAA!!!!"))
	return key
}

func TestGenerateES256MaterialUsesP256AndUUIDKid(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	require.NotNil(t, mat)
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	assert.Equal(t, domain.SigningAlgES256, pub.Alg)
	assert.Equal(t, "EC", pub.KTY)
	assert.Equal(t, "P-256", pub.CRV)
	assert.Equal(t, "sig", pub.Use)
	_, err = uuid.Parse(pub.Kid)
	require.NoError(t, err)
	thumb, err := pub.Thumbprint()
	require.NoError(t, err)
	assert.NotEmpty(t, thumb)

	key, ok := mat.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, elliptic.P256(), key.Curve)
	assert.NotNil(t, key.X)
	assert.NotNil(t, key.Y)
}

func TestPublicJWKOmitsPrivateDAndUnknownFields(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	raw, err := json.Marshal(pub)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"d"`)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"use": "sig",
		"alg": "ES256",
		"kid": pub.Kid,
		"x":   pub.X,
		"y":   pub.Y,
	}, decoded)
	decodedX, err := base64.RawURLEncoding.DecodeString(pub.X)
	require.NoError(t, err)
	decodedY, err := base64.RawURLEncoding.DecodeString(pub.Y)
	require.NoError(t, err)
	assert.Len(t, decodedX, 32)
	assert.Len(t, decodedY, 32)
}

func TestMaterialSignsThroughCryptoSignerAndPublicIsACopy(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })

	message := sha256.Sum256([]byte("identity signing test"))
	sig, err := mat.Sign(rand.Reader, message[:], crypto.SHA256)
	require.NoError(t, err)
	pub, ok := mat.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	require.True(t, ecdsa.VerifyASN1(pub, message[:], sig))

	originalX := new(big.Int).Set(pub.X)
	pub.X.SetInt64(1)
	fresh, ok := mat.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, originalX, fresh.X)
}

func TestMaterialRequiresSHA256OptionsAndDigest(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })

	message := sha256.Sum256([]byte("strict signer profile"))
	for name, tc := range map[string]struct {
		opts   crypto.SignerOpts
		digest []byte
	}{
		"nil options":  {opts: nil, digest: message[:]},
		"wrong hash":   {opts: crypto.SHA512, digest: message[:]},
		"short digest": {opts: crypto.SHA256, digest: message[:31]},
		"long digest":  {opts: crypto.SHA256, digest: append(message[:], 0)},
	} {
		t.Run(name, func(t *testing.T) {
			sig, signErr := mat.Sign(rand.Reader, tc.digest, tc.opts)
			require.Error(t, signErr)
			require.ErrorIs(t, signErr, keys.ErrInvalidMaterial)
			assert.Nil(t, sig)
		})
	}
}

func TestSealUnsealRoundTrip(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	sealed, err := keys.Seal(mat, testSealKey(t))
	require.NoError(t, err)
	assert.NotEmpty(t, sealed)
	assert.GreaterOrEqual(t, len(sealed), 1+12+16)
	assert.EqualValues(t, keys.EnvelopeVersion1, sealed[0])

	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	got, err := keys.Unseal(sealed, pub, testSealKey(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = got.Destroy() })
	gotPub, err := got.PublicJWK()
	require.NoError(t, err)
	assert.Equal(t, pub, gotPub)

	message := sha256.Sum256([]byte("round trip"))
	sig, err := got.Sign(rand.Reader, message[:], crypto.SHA256)
	require.NoError(t, err)
	publicKey, ok := got.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.True(t, ecdsa.VerifyASN1(publicKey, message[:], sig))
}

func TestUnsealRejectsTamperWrongKeyAndAAD(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	sealed, err := keys.Seal(mat, testSealKey(t))
	require.NoError(t, err)

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0x01
	_, err = keys.Unseal(tampered, pub, testSealKey(t))
	require.Error(t, err)
	assert.True(t, errors.Is(err, keys.ErrUnsealFailed))

	wrong := testSealKey(t)
	wrong[0] ^= 0xff
	_, err = keys.Unseal(sealed, pub, wrong)
	require.Error(t, err)
	assert.True(t, errors.Is(err, keys.ErrUnsealFailed))

	other, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Destroy() })
	otherPub, err := other.PublicJWK()
	require.NoError(t, err)
	_, err = keys.Unseal(sealed, otherPub, testSealKey(t))
	require.Error(t, err)
	assert.True(t, errors.Is(err, keys.ErrUnsealFailed))

	nonceFlip := append([]byte(nil), sealed...)
	nonceFlip[2] ^= 0x02
	_, err = keys.Unseal(nonceFlip, pub, testSealKey(t))
	require.Error(t, err)

	unknown := append([]byte(nil), sealed...)
	unknown[0] = 99
	_, err = keys.Unseal(unknown, pub, testSealKey(t))
	require.Error(t, err)
	assert.True(t, errors.Is(err, keys.ErrUnknownEnvelope))
}

func TestUnsealRejectsMalformedAndOversizedBlobs(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	_, err = keys.Unseal(nil, pub, testSealKey(t))
	require.Error(t, err)
	_, err = keys.Unseal(bytes.Repeat([]byte{1}, keys.MaxSealedPrivateBytes+1), pub, testSealKey(t))
	require.Error(t, err)
}

func TestMaterialRejectsUseAfterDestroyAndNeverFormatsPrivateMaterial(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	sealed, err := keys.Seal(mat, testSealKey(t))
	require.NoError(t, err)
	sealedText := base64.RawStdEncoding.EncodeToString(sealed)

	for _, formatted := range []string{
		fmt.Sprintf("%v", mat), fmt.Sprintf("%+v", mat), fmt.Sprintf("%#v", mat),
		fmt.Sprintf("%v", *mat), fmt.Sprintf("%+v", *mat), fmt.Sprintf("%#v", *mat),
	} {
		assert.LessOrEqual(t, len(formatted), 256)
		assert.NotContains(t, formatted, sealedText)
		assert.NotContains(t, formatted, `"d"`)
		assert.Contains(t, formatted, pub.Kid)
	}
	for _, value := range []any{mat, *mat} {
		jsonRaw, jsonErr := json.Marshal(value)
		require.Error(t, jsonErr)
		assert.Empty(t, jsonRaw)
		assert.NotContains(t, jsonErr.Error(), sealedText)
		assert.NotContains(t, jsonErr.Error(), `"d"`)
	}

	require.NoError(t, mat.Destroy())
	assert.NoError(t, mat.Close())
	_, err = mat.PublicJWK()
	assert.ErrorIs(t, err, keys.ErrMaterialDestroyed)
	assert.Nil(t, mat.Public())
	_, err = mat.Sign(rand.Reader, make([]byte, 32), crypto.SHA256)
	assert.ErrorIs(t, err, keys.ErrMaterialDestroyed)
	_, err = keys.Seal(mat, testSealKey(t))
	assert.ErrorIs(t, err, keys.ErrMaterialDestroyed)
}

func TestPublicJWKCopyCannotMutateMaterial(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	first, err := mat.PublicJWK()
	require.NoError(t, err)
	first.X = strings.Repeat("A", len(first.X))
	second, err := mat.PublicJWK()
	require.NoError(t, err)
	assert.NotEqual(t, first.X, second.X)
}

func TestMaterialSignAndDestroyIsRaceSafe(t *testing.T) {
	mat, err := keys.Generate()
	require.NoError(t, err)
	message := sha256.Sum256([]byte("race-safe signing"))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = mat.Sign(rand.Reader, message[:], crypto.SHA256)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mat.Destroy()
	}()
	wg.Wait()
	assert.Nil(t, mat.Public())
}

func TestPublicSetETagIsStableAndOmitsPrivateMaterial(t *testing.T) {
	t.Parallel()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	etag, err := keys.PublicSetETag([]domain.PublicJWK{pub})
	require.NoError(t, err)
	again, err := keys.PublicSetETag([]domain.PublicJWK{pub})
	require.NoError(t, err)
	assert.Equal(t, etag, again)
	assert.NotContains(t, etag, `"d"`)
	assert.NotContains(t, strings.ToLower(etag), "private")
}
