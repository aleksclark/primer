package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

// clearIdentityEnv removes Identity keys and bare DATABASE_URL so Load cannot
// silently inherit ambient LMS/host DSNs or package defaults.
func clearIdentityEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"IDENTITY_DATABASE_URL",
		"IDENTITY_ENV",
		"IDENTITY_ISSUER",
		"IDENTITY_HOST",
		"IDENTITY_PORT",
		"IDENTITY_LOG_LEVEL",
		"IDENTITY_SHUTDOWN_TIMEOUT",
		"IDENTITY_HTTP_READ_HEADER_TIMEOUT",
		"IDENTITY_HTTP_MAX_BODY_BYTES",
		// envconfig Alt fallback must not pick these up either.
		"DATABASE_URL",
		"ISSUER",
		"HOST",
		"PORT",
		"ENV",
		"LOG_LEVEL",
	}
	for _, k := range keys {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
}

// RED: IDENTITY_DATABASE_URL truly unset must fail in production — no localhost
// default and no bare DATABASE_URL inheritance.
func TestLoadFailFastWhenDatabaseURLUnset(t *testing.T) {
	clearIdentityEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	require.NoError(t, os.Unsetenv("IDENTITY_DATABASE_URL"))
	require.NoError(t, os.Unsetenv("DATABASE_URL"))

	cfg, err := config.Load()
	if err == nil {
		t.Fatalf("expected error when DATABASE_URL unset; got DSN=%q", cfg.DatabaseURL)
	}
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

// RED: same fail-fast in development — defaults must not migrate/listen on
// ambient or built-in localhost DSN.
func TestLoadFailFastWhenDatabaseURLUnsetInDevelopment(t *testing.T) {
	clearIdentityEnv(t)
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	require.NoError(t, os.Unsetenv("IDENTITY_DATABASE_URL"))
	require.NoError(t, os.Unsetenv("DATABASE_URL"))

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

// RED: bare DATABASE_URL must not satisfy Identity config when the prefixed
// IDENTITY_DATABASE_URL is missing.
func TestLoadIgnoresBareDatabaseURL(t *testing.T) {
	clearIdentityEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("DATABASE_URL", "postgres://foreign:secret@localhost:5432/primer_identity?sslmode=disable")
	require.NoError(t, os.Unsetenv("IDENTITY_DATABASE_URL"))

	_, err := config.Load()
	require.Error(t, err, "bare DATABASE_URL must not be accepted as Identity DSN")
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}
