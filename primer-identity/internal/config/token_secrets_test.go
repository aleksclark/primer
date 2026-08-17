package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

func tokenEnv(t *testing.T) {
	t.Helper()
	brokerEnv(t)
	t.Setenv("IDENTITY_CLIENT_SECRET_PEPPERS", "1:"+key(0x51))
	t.Setenv("IDENTITY_CLIENT_SECRET_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_REFRESH_TOKEN_PEPPERS", "1:"+key(0x61))
	t.Setenv("IDENTITY_REFRESH_TOKEN_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_CLIENT_ASSERTION_PEPPERS", "1:"+key(0x71))
	t.Setenv("IDENTITY_CLIENT_ASSERTION_ACTIVE_VERSION", "1")
}

func TestTokenSecretsComposeCopySafeAndFailClosed(t *testing.T) {
	baseEnv(t)
	tokenEnv(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	secrets, err := cfg.TokenSecrets()
	require.NoError(t, err)
	assert.Equal(t, 1, secrets.ClientSecretActiveVersion)
	assert.Equal(t, 1, secrets.RefreshTokenActiveVersion)
	assert.Equal(t, 1, secrets.AssertionActiveVersion)
	assert.Len(t, secrets.ClientSecretPeppers[1], 32)
	assert.Len(t, secrets.RefreshTokenPeppers[1], 32)
	assert.Len(t, secrets.AssertionPeppers[1], 32)
	assert.Equal(t, 1, secrets.AuthorizationCodeActiveVersion)
	assert.Len(t, secrets.AuthorizationCodePeppers[1], 32)

	secrets.ClientSecretPeppers[1][0] = 0xAA
	again, err := cfg.TokenSecrets()
	require.NoError(t, err)
	assert.NotEqual(t, byte(0xAA), again.ClientSecretPeppers[1][0])
}

func TestTokenSecretsFailClosedWhenIncomplete(t *testing.T) {
	baseEnv(t)
	tokenEnv(t)
	t.Setenv("IDENTITY_REFRESH_TOKEN_PEPPERS", "")
	cfg, err := config.Load()
	require.NoError(t, err)
	_, err = cfg.TokenSecrets()
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
}

func TestTokenSecretsIgnoreBareEnvironmentNames(t *testing.T) {
	baseEnv(t)
	brokerEnv(t)
	t.Setenv("CLIENT_SECRET_PEPPERS", "1:"+key(0x51))
	t.Setenv("CLIENT_SECRET_ACTIVE_VERSION", "1")
	t.Setenv("REFRESH_TOKEN_PEPPERS", "1:"+key(0x61))
	t.Setenv("REFRESH_TOKEN_ACTIVE_VERSION", "1")
	t.Setenv("CLIENT_ASSERTION_PEPPERS", "1:"+key(0x71))
	t.Setenv("CLIENT_ASSERTION_ACTIVE_VERSION", "1")
	cfg, err := config.Load()
	require.NoError(t, err)
	_, err = cfg.TokenSecrets()
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
}

func TestTokenSecretsAcceptRotatedVersions(t *testing.T) {
	baseEnv(t)
	tokenEnv(t)
	t.Setenv("IDENTITY_REFRESH_TOKEN_PEPPERS", "1:"+key(0x61)+",2:"+key(0x62))
	t.Setenv("IDENTITY_REFRESH_TOKEN_ACTIVE_VERSION", "2")
	cfg, err := config.Load()
	require.NoError(t, err)
	secrets, err := cfg.TokenSecrets()
	require.NoError(t, err)
	assert.Equal(t, 2, secrets.RefreshTokenActiveVersion)
	require.Len(t, secrets.RefreshTokenPeppers, 2)
	assert.Len(t, secrets.RefreshTokenPeppers[1], 32)
	assert.Len(t, secrets.RefreshTokenPeppers[2], 32)
	assert.NotEqual(t, secrets.RefreshTokenPeppers[1][0], secrets.RefreshTokenPeppers[2][0])
}

func TestTokenSecretsRejectMalformedMaterialWithoutEchoingIt(t *testing.T) {
	const hostile = "notbase64-hostile-token-secret"
	baseEnv(t)
	tokenEnv(t)
	t.Setenv("IDENTITY_CLIENT_SECRET_PEPPERS", "1:"+hostile)
	cfg, err := config.Load()
	require.NoError(t, err)
	_, err = cfg.TokenSecrets()
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
	assert.NotContains(t, err.Error(), hostile)
}

func TestTokenSecretsRejectActiveVersionWithoutMaterial(t *testing.T) {
	baseEnv(t)
	tokenEnv(t)
	t.Setenv("IDENTITY_CLIENT_ASSERTION_ACTIVE_VERSION", "9")
	cfg, err := config.Load()
	require.NoError(t, err)
	_, err = cfg.TokenSecrets()
	assert.ErrorIs(t, err, config.ErrBrokerSecretsUnavailable)
}
