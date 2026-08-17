package config_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

func key(b byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = b
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func brokerEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_STATE_SEAL_KEYS", "1:"+key(0x11)+",2:"+key(0x12))
	t.Setenv("IDENTITY_STATE_SEAL_ACTIVE_VERSION", "2")
	t.Setenv("IDENTITY_STATE_HASH_PEPPERS", "1:"+key(0x21))
	t.Setenv("IDENTITY_STATE_HASH_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_BROKER_COOKIE_PEPPERS", "1:"+key(0x31))
	t.Setenv("IDENTITY_BROKER_COOKIE_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_PEPPERS", "1:"+key(0x41))
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION", "1")
}

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@127.0.0.1:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("IDENTITY_ENV", "test")
}

// Broker secret material must be composable outside production so the
// credential-free IB1 boundary can run in development and test.
func TestBrokerSecretsComposeInNonProductionEnv(t *testing.T) {
	baseEnv(t)
	brokerEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	secrets, err := cfg.BrokerSecrets()
	require.NoError(t, err)

	assert.Equal(t, 2, secrets.StateSealActiveVersion)
	assert.Len(t, secrets.StateSealKeys, 2)
	assert.Len(t, secrets.StateSealKeys[2], 32)
	assert.Equal(t, 1, secrets.StateHashActiveVersion)
	assert.Len(t, secrets.StateHashPeppers[1], 32)
	assert.Equal(t, 1, secrets.BrokerCookieActiveVersion)
	assert.Len(t, secrets.BrokerCookiePeppers[1], 32)
	assert.Equal(t, 1, secrets.AuthorizationCodeActiveVersion)
	assert.Len(t, secrets.AuthorizationCodePeppers[1], 32)
}

// Composition must fail closed (before listen) when any required set is absent.
func TestBrokerSecretsFailClosedWhenIncomplete(t *testing.T) {
	cases := []string{
		"IDENTITY_STATE_SEAL_KEYS",
		"IDENTITY_STATE_HASH_PEPPERS",
		"IDENTITY_BROKER_COOKIE_PEPPERS",
		"IDENTITY_AUTHORIZATION_CODE_PEPPERS",
	}
	for _, missing := range cases {
		t.Run(missing, func(t *testing.T) {
			baseEnv(t)
			brokerEnv(t)
			t.Setenv(missing, "")

			cfg, err := config.Load()
			require.NoError(t, err)

			_, err = cfg.BrokerSecrets()
			require.Error(t, err)
			assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
		})
	}
}

// An active version with no matching key must fail closed rather than silently
// falling back to another version.
func TestBrokerSecretsRejectActiveVersionWithoutMaterial(t *testing.T) {
	baseEnv(t)
	brokerEnv(t)
	t.Setenv("IDENTITY_STATE_SEAL_ACTIVE_VERSION", "9")

	cfg, err := config.Load()
	require.NoError(t, err)
	_, err = cfg.BrokerSecrets()
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
}

// Returned material must be a defensive copy so a caller cannot mutate the
// configured key set.
func TestBrokerSecretsAreCopySafe(t *testing.T) {
	baseEnv(t)
	brokerEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	first, err := cfg.BrokerSecrets()
	require.NoError(t, err)
	for i := range first.StateSealKeys[2] {
		first.StateSealKeys[2][i] = 0xAA
	}
	first.StateHashPeppers[1] = nil
	delete(first.AuthorizationCodePeppers, 1)

	second, err := cfg.BrokerSecrets()
	require.NoError(t, err)
	assert.NotEqual(t, byte(0xAA), second.StateSealKeys[2][0], "key material must not be aliased")
	require.NotNil(t, second.StateHashPeppers[1])
	assert.Len(t, second.StateHashPeppers[1], 32)
	require.Contains(t, second.AuthorizationCodePeppers, 1)
}

// Bare, unprefixed environment names must never satisfy Identity broker
// secrets — envconfig Alt fallback must stay disabled.
func TestBrokerSecretsIgnoreBareEnvironmentNames(t *testing.T) {
	baseEnv(t)
	t.Setenv("STATE_SEAL_KEYS", "1:"+key(0x51))
	t.Setenv("STATE_SEAL_ACTIVE_VERSION", "1")
	t.Setenv("STATE_HASH_PEPPERS", "1:"+key(0x52))
	t.Setenv("STATE_HASH_ACTIVE_VERSION", "1")
	t.Setenv("BROKER_COOKIE_PEPPERS", "1:"+key(0x53))
	t.Setenv("BROKER_COOKIE_ACTIVE_VERSION", "1")
	t.Setenv("AUTHORIZATION_CODE_PEPPERS", "1:"+key(0x54))
	t.Setenv("AUTHORIZATION_CODE_ACTIVE_VERSION", "1")

	cfg, err := config.Load()
	require.NoError(t, err)

	_, err = cfg.BrokerSecrets()
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable,
		"unprefixed env names must not provide Identity broker secrets")
}

// Malformed encodings must fail closed with a non-oracular error that never
// echoes the supplied material.
func TestBrokerSecretsRejectMalformedMaterialWithoutEchoingIt(t *testing.T) {
	const hostile = "notbase64-hostile-secret-value"
	baseEnv(t)
	brokerEnv(t)
	t.Setenv("IDENTITY_STATE_HASH_PEPPERS", "1:"+hostile)

	cfg, err := config.Load()
	require.NoError(t, err)

	_, err = cfg.BrokerSecrets()
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
	assert.NotContains(t, err.Error(), hostile)
}

// A short (non 32-byte) key must be rejected: AES-256/HMAC bounds are exact.
func TestBrokerSecretsRejectWrongLengthMaterial(t *testing.T) {
	baseEnv(t)
	brokerEnv(t)
	t.Setenv("IDENTITY_BROKER_COOKIE_PEPPERS", "1:"+base64.RawURLEncoding.EncodeToString([]byte("too-short")))

	cfg, err := config.Load()
	require.NoError(t, err)
	_, err = cfg.BrokerSecrets()
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
}

// The error text must never contain configured secret material.
func TestBrokerSecretsErrorsNeverContainSecretMaterial(t *testing.T) {
	sealKey := key(0x11)
	baseEnv(t)
	brokerEnv(t)
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_PEPPERS", "")

	cfg, err := config.Load()
	require.NoError(t, err)

	_, err = cfg.BrokerSecrets()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sealKey)
	assert.NotContains(t, err.Error(), strings.TrimSuffix(sealKey, "="))
}
