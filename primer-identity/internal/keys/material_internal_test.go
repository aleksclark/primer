package keys

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZeroPrivateOverwritesEveryBackingWord(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	words := priv.D.Bits()
	require.NotEmpty(t, words)

	zeroPrivate(priv)

	for i, word := range words {
		require.Zero(t, word, "private scalar backing word %d was not zeroed", i)
	}
}
