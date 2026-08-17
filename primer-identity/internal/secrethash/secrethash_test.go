package secrethash_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/secrethash"
)

func TestHashSeparatesVersionAndContext(t *testing.T) {
	peppers := secrethash.Peppers{1: bytes.Repeat([]byte{0x11}, 32), 2: bytes.Repeat([]byte{0x22}, 32)}
	state, err := secrethash.Hash(peppers, 1, "oauth-state", []byte("same-secret"))
	require.NoError(t, err)
	cookie, err := secrethash.Hash(peppers, 1, "broker-cookie", []byte("same-secret"))
	require.NoError(t, err)
	code, err := secrethash.Hash(peppers, 2, "oauth-state", []byte("same-secret"))
	require.NoError(t, err)
	require.Len(t, state, 32)
	require.NotEqual(t, state, cookie)
	require.NotEqual(t, state, code)
	require.True(t, secrethash.Equal(state, append([]byte(nil), state...)))
	require.False(t, secrethash.Equal(state, cookie))
}

func TestHashGoldenAndSafeErrors(t *testing.T) {
	secret := []byte("never-format-this-secret")
	hash, err := secrethash.Hash(secrethash.Peppers{9: bytes.Repeat([]byte{0xab}, 32)}, 9, "authorization-code", secret)
	require.NoError(t, err)
	const want = "cd9b409a3ef9e2ee65b4424300fecd31a5681562afc3f5c53a4b45c5de977eef"
	require.Equal(t, want, hex.EncodeToString(hash))
	_, err = secrethash.Hash(secrethash.Peppers{}, 9, "authorization-code", secret)
	require.ErrorIs(t, err, secrethash.ErrInvalid)
	require.NotContains(t, strings.ToLower(err.Error()), "never-format-this-secret")
}

func TestHashRejectsZeroAndShortPepper(t *testing.T) {
	for _, tc := range []struct {
		version int
		peppers secrethash.Peppers
	}{{0, secrethash.Peppers{1: bytes.Repeat([]byte{1}, 32)}}, {1, secrethash.Peppers{1: []byte("short")}}, {1, secrethash.Peppers{}}} {
		_, err := secrethash.Hash(tc.peppers, tc.version, "oauth-state", []byte("x"))
		require.ErrorIs(t, err, secrethash.ErrInvalid)
	}
}

func TestHashCopiesPepperAndFramesContextWithoutConcatCollision(t *testing.T) {
	pepper := bytes.Repeat([]byte{0x5a}, 32)
	want, err := secrethash.Hash(secrethash.Peppers{1: bytes.Repeat([]byte{0x5a}, 32)}, 1, "oauth-state", []byte("abc"))
	require.NoError(t, err)
	got, err := secrethash.Hash(secrethash.Peppers{1: pepper}, 1, "oauth-state", []byte("abc"))
	require.NoError(t, err)
	pepper[0] ^= 0xff
	require.Equal(t, want, got)

	left, err := secrethash.Hash(secrethash.Peppers{1: bytes.Repeat([]byte{0x5a}, 32)}, 1, "ab", []byte("c"))
	require.NoError(t, err)
	right, err := secrethash.Hash(secrethash.Peppers{1: bytes.Repeat([]byte{0x5a}, 32)}, 1, "a", []byte("bc"))
	require.NoError(t, err)
	require.NotEqual(t, left, right)
}

func TestHashRejectsEmptyOverlongAndInvalidContextAndSecret(t *testing.T) {
	peppers := secrethash.Peppers{1: bytes.Repeat([]byte{0x11}, 32)}
	for _, tc := range []struct {
		context string
		secret  []byte
	}{
		{"", []byte("x")},
		{"oauth-state", nil},
		{"oauth-state", []byte{}},
		{strings.Repeat("c", 129), []byte("x")},
		{"oauth-state", bytes.Repeat([]byte{1}, 4097)},
		{"oauth-state\x00", []byte("x")},
		{string([]byte{0xff}), []byte("x")},
	} {
		_, err := secrethash.Hash(peppers, 1, tc.context, tc.secret)
		require.ErrorIs(t, err, secrethash.ErrInvalid)
		if err != nil {
			require.NotContains(t, strings.ToLower(err.Error()), "oauth-state")
		}
	}
	ok, err := secrethash.Hash(peppers, 1, strings.Repeat("c", 128), bytes.Repeat([]byte{1}, 4096))
	require.NoError(t, err)
	require.Len(t, ok, 32)
	require.False(t, secrethash.Equal(ok, ok[:31]))
	require.False(t, secrethash.Equal(nil, nil))
}
