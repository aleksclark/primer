package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

const (
	hostileBareSecret    = "hostile-bare-secret-must-not-be-used"
	hostileBareIssuer    = "https://hostile-bare.example"
	hostileBareProjectID = "project-live-hostile-bare"
	identityDSN          = "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable"
)

var prefixedIdentityKeys = []string{
	"IDENTITY_DATABASE_URL",
	"IDENTITY_ENV",
	"IDENTITY_ISSUER",
	"IDENTITY_HOST",
	"IDENTITY_PORT",
	"IDENTITY_LOG_LEVEL",
	"IDENTITY_SHUTDOWN_TIMEOUT",
	"IDENTITY_HTTP_READ_HEADER_TIMEOUT",
	"IDENTITY_HTTP_MAX_BODY_BYTES",
	"IDENTITY_STYTCH_ENABLED",
	"IDENTITY_STYTCH_PROJECT_ID",
	"IDENTITY_STYTCH_SECRET",
	"IDENTITY_STYTCH_ENV",
	"IDENTITY_STYTCH_BASE_URI",
	"IDENTITY_STYTCH_REQUEST_TIMEOUT",
	"IDENTITY_STYTCH_POSITIVE_CACHE_TTL",
	"IDENTITY_STYTCH_NEGATIVE_CACHE_TTL",
	"IDENTITY_STYTCH_POSITIVE_CACHE_CAPACITY",
	"IDENTITY_STYTCH_NEGATIVE_CACHE_CAPACITY",
	"IDENTITY_STYTCH_PUBLIC_TOKEN",
	"IDENTITY_BROKER_ALLOWED_ORIGIN",
	"IDENTITY_BROKER_DISCOVERY_REDIRECT_URL",
	"IDENTITY_BROKER_LOGIN_REDIRECT_URL",
	"IDENTITY_BROKER_SIGNUP_REDIRECT_URL",
	"IDENTITY_INSECURE_BROKER_COOKIE",
	"IDENTITY_STATE_SEAL_KEYS",
	"IDENTITY_STATE_SEAL_ACTIVE_VERSION",
	"IDENTITY_STATE_HASH_PEPPERS",
	"IDENTITY_STATE_HASH_ACTIVE_VERSION",
	"IDENTITY_BROKER_COOKIE_PEPPERS",
	"IDENTITY_BROKER_COOKIE_ACTIVE_VERSION",
	"IDENTITY_AUTHORIZATION_CODE_PEPPERS",
	"IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION",
	"IDENTITY_PROVIDER_PROOF_CACHE_TTL",
}

var hostileBareEnv = map[string]string{
	"SECRET":                     hostileBareSecret,
	"PROJECT_ID":                 hostileBareProjectID,
	"ISSUER":                     hostileBareIssuer,
	"ENABLED":                    "true",
	"ENV":                        "test",
	"BASE_URI":                   "https://hostile-bare.stytch.example",
	"HOST":                       "10.255.255.1",
	"PORT":                       "1",
	"LOG_LEVEL":                  "error",
	"DATABASE_URL":               "postgres://foreign:***@localhost:5432/primer_identity?sslmode=disable",
	"REQUEST_TIMEOUT":            "4s",
	"POSITIVE_CACHE_TTL":         "16s",
	"NEGATIVE_CACHE_TTL":         "6s",
	"POSITIVE_CACHE_CAPACITY":    "10001",
	"NEGATIVE_CACHE_CAPACITY":    "2001",
	"SHUTDOWN_TIMEOUT":           "1s",
	"HTTP_READ_HEADER_TIMEOUT":   "1s",
	"HTTP_MAX_BODY_BYTES":        "1",
	"STYTCH_SECRET":              hostileBareSecret,
	"STYTCH_PROJECT_ID":          hostileBareProjectID,
	"STYTCH_ENABLED":             "true",
	"STYTCH_ENV":                 "live",
	"STYTCH_BASE_URI":            "https://hostile-stytch.example",
	"STYTCH_REQUEST_TIMEOUT":     "4s",
	"STYTCH_POSITIVE_CACHE_TTL":  "16s",
	"STATE_SEAL_KEYS":            "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"STATE_SEAL_ACTIVE_VERSION":  "1",
	"STATE_HASH_PEPPERS":         "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"BROKER_COOKIE_PEPPERS":      "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"AUTHORIZATION_CODE_PEPPERS": "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"PROVIDER_PROOF_CACHE_TTL":   "16s",
}

func clearPrefixedIdentityEnv(t *testing.T) {
	t.Helper()
	for _, key := range prefixedIdentityKeys {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func plantHostileBareEnv(t *testing.T) {
	t.Helper()
	for key, value := range hostileBareEnv {
		t.Setenv(key, value)
	}
}

func assertNoHostileLeak(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, hostileBareSecret)
	assert.NotContains(t, strings.ToLower(msg), "password=")
}

func TestLoadIgnoresHostileBareVariablesInDevelopment(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

	cfg, err := config.Load()
	require.NoError(t, err, "development defaults must still load when only IDENTITY_* required values are set")

	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8090, cfg.Port)
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "http://localhost:8090", cfg.Issuer)
	assert.NotEqual(t, hostileBareIssuer, cfg.Issuer)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 10*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, int64(1<<20), cfg.HTTPMaxBodyBytes)
	assert.False(t, cfg.Stytch.Enabled)
	assert.Empty(t, cfg.Stytch.ProjectID)
	assert.Empty(t, cfg.Stytch.Secret)
	assert.Equal(t, "test", cfg.Stytch.Env)
	assert.Empty(t, cfg.Stytch.BaseURI)
	assert.Equal(t, 3*time.Second, cfg.Stytch.RequestTimeout)
	assert.Equal(t, 15*time.Second, cfg.Stytch.PositiveCacheTTL)
	assert.Equal(t, 5*time.Second, cfg.Stytch.NegativeCacheTTL)
	assert.Equal(t, 10000, cfg.Stytch.PositiveCacheCapacity)
	assert.Equal(t, 2000, cfg.Stytch.NegativeCacheCapacity)
	assert.Empty(t, cfg.StateSealKeys)
	assert.Zero(t, cfg.StateSealActiveVersion)
	assert.Equal(t, 15*time.Second, cfg.ProviderProofCacheTTL)
}

func TestLoadIgnoresHostileBareVariablesInTest(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_ENV", "test")
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_ISSUER", "https://id.example.test")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "test", cfg.Env)
	assert.Equal(t, "https://id.example.test", cfg.Issuer)
	assert.False(t, cfg.Stytch.Enabled)
	assert.Empty(t, cfg.Stytch.Secret)
	assert.Empty(t, cfg.Stytch.ProjectID)
}

func TestLoadProductionFailsWhenPrefixedIssuerMissingDespiteBareIssuer(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_ENV", "live")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-live-example")
	t.Setenv("IDENTITY_STYTCH_SECRET", "prefixed-secret-value")
	require.NoError(t, os.Unsetenv("IDENTITY_ISSUER"))

	cfg, err := config.Load()
	assertNoHostileLeak(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, strings.ToLower(err.Error()), "issuer")
}

func TestLoadProductionFailsWhenPrefixedSecretMissingDespiteBareSecret(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_ENV", "live")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-live-example")
	t.Setenv("IDENTITY_BROKER_ALLOWED_ORIGIN", "https://id.example")
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", "public-token-live-example")
	require.NoError(t, os.Unsetenv("IDENTITY_STYTCH_SECRET"))

	cfg, err := config.Load()
	assertNoHostileLeak(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, strings.ToLower(err.Error()), "credential")
}

func TestLoadProductionFailsWhenPrefixedProjectIDMissingDespiteBareProjectID(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_ENV", "live")
	t.Setenv("IDENTITY_STYTCH_SECRET", "prefixed-secret-value")
	t.Setenv("IDENTITY_BROKER_ALLOWED_ORIGIN", "https://id.example")
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", "public-token-live-example")
	require.NoError(t, os.Unsetenv("IDENTITY_STYTCH_PROJECT_ID"))

	cfg, err := config.Load()
	assertNoHostileLeak(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, strings.ToLower(err.Error()), "credential")
}

func TestLoadConsumesOnlyPrefixedIdentityAndStytchNames(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_ENV", "test")
	t.Setenv("IDENTITY_HOST", "127.0.0.1")
	t.Setenv("IDENTITY_PORT", "9090")
	t.Setenv("IDENTITY_LOG_LEVEL", "debug")
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_ISSUER", "https://id.example.test")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("IDENTITY_HTTP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("IDENTITY_HTTP_MAX_BODY_BYTES", "4096")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-test-example")
	t.Setenv("IDENTITY_STYTCH_SECRET", "prefixed-secret-value")
	t.Setenv("IDENTITY_STYTCH_ENV", "test")
	t.Setenv("IDENTITY_STYTCH_BASE_URI", "https://stytch.test.example/path")
	t.Setenv("IDENTITY_STYTCH_POSITIVE_CACHE_TTL", "10s")
	t.Setenv("IDENTITY_STYTCH_NEGATIVE_CACHE_TTL", "4s")
	t.Setenv("IDENTITY_STYTCH_POSITIVE_CACHE_CAPACITY", "50")
	t.Setenv("IDENTITY_STYTCH_NEGATIVE_CACHE_CAPACITY", "25")
	t.Setenv("IDENTITY_BROKER_ALLOWED_ORIGIN", "https://id.example.test")
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", "https://id.example.test/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", "https://id.example.test/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", "https://id.example.test/broker/stytch/callback")
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", "public-token-test")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", cfg.Host)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "test", cfg.Env)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "https://id.example.test", cfg.Issuer)
	assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 2*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, int64(4096), cfg.HTTPMaxBodyBytes)
	assert.True(t, cfg.Stytch.Enabled)
	assert.Equal(t, "project-test-example", cfg.Stytch.ProjectID)
	assert.Equal(t, "prefixed-secret-value", cfg.Stytch.Secret)
	assert.Equal(t, "test", cfg.Stytch.Env)
	assert.Equal(t, "https://stytch.test.example/path", cfg.Stytch.BaseURI)
	assert.Equal(t, 3*time.Second, cfg.Stytch.RequestTimeout)
	assert.Equal(t, 10*time.Second, cfg.Stytch.PositiveCacheTTL)
	assert.Equal(t, 4*time.Second, cfg.Stytch.NegativeCacheTTL)
	assert.Equal(t, 50, cfg.Stytch.PositiveCacheCapacity)
	assert.Equal(t, 25, cfg.Stytch.NegativeCacheCapacity)
}
