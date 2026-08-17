package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

func TestProductionRequiresTokenPeppersBeforeMigrateListen(t *testing.T) {
	productionKeyEnv(t)
	encoded, _ := canonicalSealSecret(t)
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)
	require.NoError(t, os.Unsetenv("IDENTITY_CLIENT_SECRET_PEPPERS"))
	require.NoError(t, os.Unsetenv("IDENTITY_REFRESH_TOKEN_PEPPERS"))
	require.NoError(t, os.Unsetenv("IDENTITY_CLIENT_ASSERTION_PEPPERS"))

	_, err := config.Load()
	require.Error(t, err, "production must fail closed before migrate/listen without token peppers")
	assert.Contains(t, strings.ToLower(err.Error()), "token")
	assert.NotContains(t, err.Error(), encoded)
}

func TestProductionLoadSucceedsWithKeyAndTokenAuthorityMaterial(t *testing.T) {
	productionKeyEnv(t)
	encoded, raw := canonicalSealSecret(t)
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)
	tokenEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.TokenAuthorityEnabled())
	assert.Equal(t, raw, cfg.Key.SealKey())

	secrets, err := cfg.TokenSecrets()
	require.NoError(t, err)
	assert.Equal(t, 1, secrets.RefreshTokenActiveVersion)
	assert.Len(t, secrets.RefreshTokenPeppers[1], 32)
}

func TestDevelopmentHealthOnlyLeavesTokenAuthorityDisabled(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	require.NoError(t, os.Unsetenv("IDENTITY_KEY_ENABLED"))
	require.NoError(t, os.Unsetenv("IDENTITY_CLIENT_SECRET_PEPPERS"))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.False(t, cfg.TokenAuthorityEnabled())
	assert.False(t, cfg.Key.Enabled)
}

func TestDevelopmentTokenAuthorityRequiresKeyAndTokenPeppers(t *testing.T) {
	encoded, _ := canonicalSealSecret(t)
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	t.Setenv("IDENTITY_KEY_ENABLED", "true")
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "token")
	assert.NotContains(t, err.Error(), encoded)

	tokenEnv(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.TokenAuthorityEnabled())
}

func TestTokenAuthorityIgnoresBarePepperAndKeyNames(t *testing.T) {
	const hostile = "hostile-bare-token-pepper"
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	t.Setenv("IDENTITY_KEY_ENABLED", "true")
	t.Setenv("KEY_SEAL_SECRET", hostile)
	t.Setenv("CLIENT_SECRET_PEPPERS", "1:"+key(0x51))
	t.Setenv("REFRESH_TOKEN_PEPPERS", "1:"+key(0x61))
	t.Setenv("CLIENT_ASSERTION_PEPPERS", "1:"+key(0x71))

	_, err := config.Load()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), hostile)
}
