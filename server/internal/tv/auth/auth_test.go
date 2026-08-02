package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/auth"
)

func TestNewPairingCode(t *testing.T) {
	t.Parallel()

	const ambiguous = "OIL01258BZS"
	seen := make(map[string]bool, 200)
	for range 200 {
		code, err := auth.NewPairingCode()
		require.NoError(t, err)
		assert.Len(t, code, auth.PairingCodeLength)
		for _, r := range code {
			assert.NotContains(t, ambiguous, string(r), "code %q uses an ambiguous character", code)
		}
		seen[code] = true
	}
	assert.Greater(t, len(seen), 150, "codes should not repeat often")
}

func TestNewToken(t *testing.T) {
	t.Parallel()

	token, hash, err := auth.NewToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.NotContains(t, hash, token, "the hash must not embed the plaintext")
	assert.Equal(t, auth.HashToken(token), hash)
	assert.Len(t, hash, 64, "sha-256 hex digest")

	other, otherHash, err := auth.NewToken()
	require.NoError(t, err)
	assert.NotEqual(t, token, other)
	assert.NotEqual(t, hash, otherHash)
}

func TestHashTokenIsStable(t *testing.T) {
	t.Parallel()
	assert.Equal(t, auth.HashToken("abc"), auth.HashToken("abc"))
	assert.NotEqual(t, auth.HashToken("abc"), auth.HashToken("abd"))
	assert.Equal(t, strings.ToLower(auth.HashToken("abc")), auth.HashToken("abc"), "hex digests are lowercase")
}
