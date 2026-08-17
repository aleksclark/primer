package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
)

func validPublicJWKJSON(kid string) []byte {
	return []byte(`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`)
}

func TestParsePublicJWKAcceptsCanonicalDocument(t *testing.T) {
	t.Parallel()
	kid := "11111111-2222-3333-4444-555555555555"
	got, err := domain.ParsePublicJWK(validPublicJWKJSON(kid))
	require.NoError(t, err)
	assert.Equal(t, "EC", got.KTY)
	assert.Equal(t, "P-256", got.CRV)
	assert.Equal(t, "sig", got.Use)
	assert.Equal(t, domain.SigningAlgES256, got.Alg)
	assert.Equal(t, kid, got.Kid)
	assert.Equal(t, "MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4", got.X)
	assert.Equal(t, "4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM", got.Y)
}

// RFC 7515 Appendix A.3 / RFC 7518 Appendix C known public P-256 vector.
// Parse/ECDSAPublic must accept the published coordinates and reject the
// published private "d" so no plaintext/JWK private leakage is possible.
func TestParsePublicJWKAcceptsRFC7515AppendixA3KnownVector(t *testing.T) {
	t.Parallel()
	kid := "11111111-2222-3333-4444-555555555555"
	raw := []byte(`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0"}`)
	got, err := domain.ParsePublicJWK(raw)
	require.NoError(t, err)
	pub, err := got.ECDSAPublic()
	require.NoError(t, err)
	require.NotNil(t, pub.X)
	require.NotNil(t, pub.Y)

	_, err = domain.ParsePublicJWK([]byte(`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0","d":"jpsQnnGQmL-YBIffH1136cspYG6-0iY7X1fCE9-E9LI"}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))
	assert.NotContains(t, err.Error(), "jpsQnnGQmL-YBIffH1136cspYG6-0iY7X1fCE9-E9LI")
	assert.NotContains(t, err.Error(), `"d"`)
}

func TestPublicJWKMarshalIsCanonicalAndOmitsPrivateD(t *testing.T) {
	t.Parallel()
	kid := "11111111-2222-3333-4444-555555555555"
	jwk, err := domain.ParsePublicJWK(validPublicJWKJSON(kid))
	require.NoError(t, err)
	raw, err := json.Marshal(jwk)
	require.NoError(t, err)
	assert.Equal(t, string(validPublicJWKJSON(kid)), string(raw))
	assert.NotContains(t, string(raw), `"d"`)
	assert.NotContains(t, string(raw), "key_ops")
}

func TestParsePublicJWKRejectsPrivateAndUnknownFields(t *testing.T) {
	t.Parallel()
	kid := "11111111-2222-3333-4444-555555555555"
	cases := []string{
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM","d":"870MB6gfuTJ4HtUnUvYMyJpr5eUZNP4Bk43bVdj3eAE"}`,
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM","key_ops":["sign"]}`,
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM","extra":true}`,
		`{"kty":"RSA","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
		`{"kty":"EC","crv":"P-384","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
		`{"kty":"EC","crv":"P-256","use":"enc","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"RS256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"AA","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
		`{"kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","y":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`,
		`{"kty":"EC","kty":"EC","crv":"P-256","use":"sig","alg":"ES256","kid":"` + kid + `","x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4","y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"}`,
	}
	for _, raw := range cases {
		_, err := domain.ParsePublicJWK([]byte(raw))
		require.Error(t, err, raw)
		assert.True(t, errors.Is(err, domain.ErrInvalid), raw)
		assert.NotContains(t, err.Error(), `"d":`)
		assert.NotContains(t, err.Error(), "870MB6gfuTJ4HtUnUvYMyJpr5eUZNP4Bk43bVdj3eAE")
	}
}

func TestParsePublicJWKRejectsOversizedDocument(t *testing.T) {
	t.Parallel()
	raw := []byte(strings.Repeat(" ", domain.MaxPublicJWKBytes+1))
	_, err := domain.ParsePublicJWK(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))
}

func TestSigningKeyJSONIsPublicOnlyAndFormatsWithoutPrivateMaterial(t *testing.T) {
	t.Parallel()
	kid := "11111111-2222-3333-4444-555555555555"
	jwk, err := domain.ParsePublicJWK(validPublicJWKJSON(kid))
	require.NoError(t, err)
	key := domain.SigningKey{
		Kid:        kid,
		Alg:        domain.SigningAlgES256,
		KeyVersion: 1,
		PublicJWK:  jwk,
		Status:     domain.SigningKeyStatusActive,
	}
	raw, err := json.Marshal(key)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"d"`)
	assert.NotContains(t, string(raw), "sealed")
	assert.NotContains(t, strings.ToLower(string(raw)), "private")
	assert.Contains(t, fmt.Sprint(key), kid)
	assert.NotContains(t, fmt.Sprintf("%#v", key), "870MB6gfuTJ4HtUnUvYMyJpr5eUZNP4Bk43bVdj3eAE")
}

func TestValidateSigningKeyStatusAndKid(t *testing.T) {
	t.Parallel()
	require.NoError(t, domain.ValidateSigningKid("11111111-2222-3333-4444-555555555555"))
	require.Error(t, domain.ValidateSigningKid(""))
	require.Error(t, domain.ValidateSigningKid(strings.Repeat("k", domain.MaxKidLen+1)))
	require.Error(t, domain.ValidateSigningKid("bad\tkid"))
	require.NoError(t, domain.ValidateSigningKeyStatus(domain.SigningKeyStatusActive))
	require.NoError(t, domain.ValidateSigningKeyStatus(domain.SigningKeyStatusNext))
	require.NoError(t, domain.ValidateSigningKeyStatus(domain.SigningKeyStatusRetired))
	require.NoError(t, domain.ValidateSigningKeyStatus(domain.SigningKeyStatusDestroyed))
	require.Error(t, domain.ValidateSigningKeyStatus("pending"))
}

func TestPublicJWKThumbprintIsStableAndPublicOnly(t *testing.T) {
	t.Parallel()
	kid := "11111111-2222-3333-4444-555555555555"
	jwk, err := domain.ParsePublicJWK(validPublicJWKJSON(kid))
	require.NoError(t, err)
	thumb, err := jwk.Thumbprint()
	require.NoError(t, err)
	assert.NotEmpty(t, thumb)
	again, err := jwk.Thumbprint()
	require.NoError(t, err)
	assert.Equal(t, thumb, again)
	assert.NotContains(t, thumb, `"d"`)
	assert.NotContains(t, thumb, kid)
}
