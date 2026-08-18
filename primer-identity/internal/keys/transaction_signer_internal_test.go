package keys

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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

func TestTransactionSignerCopyAndPointerFormattingIsKidOnlyDuringClose(t *testing.T) {
	signer := newTestTransactionSigner(t)
	copyOfSigner := *signer
	privateHex := fmt.Sprintf("%x", signer.state.mat.private.key.D.Bytes())
	want := "transaction-signer kid=" + signer.public.Kid

	for _, value := range []any{copyOfSigner, &copyOfSigner, *signer, signer} {
		for _, format := range []string{"%v", "%+v", "%#v", "%d", "%x", "%q", "%s"} {
			got := fmt.Sprintf(format, value)
			require.Equal(t, want, got, "format %s for %T", format, value)
			require.NotContains(t, got, privateHex)
		}
		blob, err := json.Marshal(value)
		require.Error(t, err, "JSON must refuse for %T", value)
		require.Nil(t, blob)
		require.NotContains(t, err.Error(), privateHex)
	}

	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	digest := sha256.Sum256([]byte("transaction signer formatting"))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				logger.Info("signer", slog.Any("value", copyOfSigner))
				_, _ = signer.Sign(rand.Reader, digest[:], crypto.SHA256)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = signer.Close()
	}()
	wg.Wait()

	got := output.String()
	require.NotEmpty(t, got)
	require.NotContains(t, got, privateHex)
	require.NotContains(t, strings.ToLower(got), "private key")
	require.NotContains(t, got, "sealed")
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
