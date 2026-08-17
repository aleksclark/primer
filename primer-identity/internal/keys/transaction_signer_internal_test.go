package keys

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestTransactionSigner(t *testing.T) *TransactionSigner {
	t.Helper()
	mat, err := Generate()
	require.NoError(t, err)
	pub, err := mat.PublicJWK()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	return &TransactionSigner{
		state:  &transactionSignerState{mat: mat},
		public: pub,
	}
}

func TestTransactionSignerCopyCloseRevokesOriginal(t *testing.T) {
	signer := newTestTransactionSigner(t)
	copyOfSigner := *signer

	require.NoError(t, copyOfSigner.Close())
	digest := sha256.Sum256([]byte("copied transaction signer"))
	_, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	require.ErrorIs(t, err, ErrSignerRevoked)
	_, err = copyOfSigner.PublicJWK()
	require.ErrorIs(t, err, ErrSignerRevoked)
}

func TestTransactionSignerConcurrentSignAndCloseIsRaceSafe(t *testing.T) {
	signer := newTestTransactionSigner(t)
	digest := sha256.Sum256([]byte("concurrent transaction signer"))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = signer.Sign(rand.Reader, digest[:], crypto.SHA256)
				_ = signer.Public()
				_, _ = signer.PublicJWK()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = signer.Close()
	}()
	wg.Wait()

	_, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	require.ErrorIs(t, err, ErrSignerRevoked)
}
