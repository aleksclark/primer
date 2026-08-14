package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/config"
)

// P1-E3 / P1-S2: production with STUDIO_DATABASE_URL truly unset must fail at
// config validation — before migrate/listen — with no localhost default and no
// bare DATABASE_URL inheritance.
func TestLoadProductionUnsetDatabaseURL(t *testing.T) {
	// Clear Studio + bare DSN keys that might be ambient in the agent shell.
	for _, k := range []string{
		"STUDIO_DATABASE_URL",
		"DATABASE_URL",
		"TEST_DATABASE_URL",
		"STUDIO_TEST_DATABASE_URL",
	} {
		_ = os.Unsetenv(k)
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	t.Setenv("STUDIO_ENV", "production")
	t.Setenv("STUDIO_HOST", "127.0.0.1")
	t.Setenv("STUDIO_PORT", "18088")
	// Ensure bare DATABASE_URL cannot satisfy Studio if reintroduced.
	t.Setenv("DATABASE_URL", "postgres://lms:x@localhost:5432/primer?sslmode=disable")

	_, err := config.Load()
	require.Error(t, err)
	msg := strings.ToLower(err.Error())
	assert.Contains(t, msg, "database")
	assert.Contains(t, msg, "required")
	assert.NotContains(t, msg, "listening")
	assert.NotContains(t, msg, "migrate:")
}
