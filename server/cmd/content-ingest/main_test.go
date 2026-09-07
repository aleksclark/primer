package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgsPlanApplySkipAcquire(t *testing.T) {
	t.Parallel()

	got, err := parseArgs([]string{"apply"})
	require.NoError(t, err)
	assert.Equal(t, "apply", got.Name)
	assert.False(t, got.SkipAcquire)

	got, err = parseArgs([]string{"apply", "--skip-acquire"})
	require.NoError(t, err)
	assert.Equal(t, "apply", got.Name)
	assert.True(t, got.SkipAcquire)

	got, err = parseArgs([]string{"plan", "--skip-acquire"})
	require.NoError(t, err)
	assert.Equal(t, "plan", got.Name)
	assert.True(t, got.SkipAcquire)
}

func TestParseArgsRejectsUnknownFlags(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{"apply", "--only", "matrix"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown flag "--only"`)

	_, err = parseArgs([]string{"apply", "--skip-acquire", "--fresh"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown flag "--fresh"`)

	_, err = parseArgs([]string{"review", "--skip-acquire"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")

	_, err = parseArgs([]string{"nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")

	_, err = parseArgs(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestParseArgsHelp(t *testing.T) {
	t.Parallel()
	for _, in := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		got, err := parseArgs(in)
		require.NoError(t, err)
		assert.Equal(t, "help", got.Name)
	}
	assert.Contains(t, usageText, "--skip-acquire")
	assert.Contains(t, usageText, "INGEST_JELLYFIN_COLLECTION_NAME")
	assert.Contains(t, usageText, "empty disables")
}
