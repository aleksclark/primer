package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/config"
)

// TestLoadProductionUnsetDatabaseURL verifies that production with
// PRIMER_AGENTS_DATABASE_URL truly unset fails at config validation —
// before migrate/listen — with no localhost default and no bare DATABASE_URL
// inheritance.
func TestLoadProductionUnsetDatabaseURL(t *testing.T) {
	for _, k := range []string{
		"PRIMER_AGENTS_DATABASE_URL",
		"DATABASE_URL",
		"TEST_DATABASE_URL",
		"PRIMER_AGENTS_TEST_DATABASE_URL",
		"STUDIO_DATABASE_URL",
		"IDENTITY_DATABASE_URL",
		"TV_DATABASE_URL",
	} {
		require.NoError(t, os.Unsetenv(k))
	}
	t.Setenv("PRIMER_AGENTS_ENV", "production")
	// Poison bare DATABASE_URL to confirm it can never satisfy agents config.
	t.Setenv("DATABASE_URL", "postgres://lms:x@localhost:5432/primer?sslmode=disable")

	_, err := config.Load()
	require.Error(t, err)
	msg := strings.ToLower(err.Error())
	assert.Contains(t, msg, "database")
	assert.Contains(t, msg, "required")
	assert.NotContains(t, msg, "listening")
	assert.NotContains(t, msg, "migrate:")
}
