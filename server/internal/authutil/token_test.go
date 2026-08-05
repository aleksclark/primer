package authutil_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/authutil"
)

func TestNewPairingCodeUsesSafeAlphabet(t *testing.T) {
	t.Parallel()
	const alphabet = "ACDEFGHJKMNPQRTUVWXY34679"
	for i := 0; i < 20; i++ {
		code, err := authutil.NewPairingCode()
		require.NoError(t, err)
		require.Len(t, code, authutil.PairingCodeLength)
		for _, r := range code {
			assert.Contains(t, alphabet, string(r))
		}
		// Easy-to-confuse characters must never appear.
		assert.NotContains(t, code, "0")
		assert.NotContains(t, code, "O")
		assert.NotContains(t, code, "1")
		assert.NotContains(t, code, "I")
		assert.NotContains(t, code, "L")
	}
}

func TestNewTokenAndEqualHash(t *testing.T) {
	t.Parallel()
	tok, hash, err := authutil.NewToken()
	require.NoError(t, err)
	require.NotEmpty(t, tok)
	require.NotEmpty(t, hash)
	assert.NotEqual(t, tok, hash)
	assert.Equal(t, authutil.HashToken(tok), hash)
	assert.True(t, authutil.EqualHash(tok, hash))
	assert.False(t, authutil.EqualHash(tok+"x", hash))
	assert.False(t, authutil.EqualHash(tok, ""))
	assert.False(t, authutil.EqualHash("", hash))
	// Different token must not collide.
	tok2, hash2, err := authutil.NewToken()
	require.NoError(t, err)
	assert.NotEqual(t, tok, tok2)
	assert.NotEqual(t, hash, hash2)
	assert.False(t, authutil.EqualHash(tok2, hash))
}

func TestHashTokenIsStableAndHex(t *testing.T) {
	t.Parallel()
	h1 := authutil.HashToken("fixed-token-value")
	h2 := authutil.HashToken("fixed-token-value")
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 64)
	for _, r := range h1 {
		assert.True(t, (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'), "unexpected hex char %q in %s", r, h1)
	}
	assert.True(t, strings.HasPrefix(h1, authutil.HashToken("fixed-token-value")[:8]))
}
