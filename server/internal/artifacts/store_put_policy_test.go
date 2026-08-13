package artifacts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestPutObjectIdempotentAndMismatch(t *testing.T) {
	t.Parallel()
	store, err := artifacts.NewStore(t.TempDir())
	require.NoError(t, err)

	data := []byte("hello-artifact")
	dig := shaHex(data)

	rel, n, err := store.PutObject(dig, bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), n)
	assert.Contains(t, rel, dig[:2])

	// Idempotent reuse.
	rel2, n2, err := store.PutObject(dig, bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, rel, rel2)
	assert.Equal(t, n, n2)

	// Wrong expected size on existing object.
	_, _, err = store.PutObject(dig, bytes.NewReader(data), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exists with size")

	// Digest mismatch on new object.
	_, _, err = store.PutObject(strings.Repeat("0", 64), bytes.NewReader(data), int64(len(data)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest mismatch")

	// Size mismatch (too short).
	other := []byte("xy")
	otherDig := shaHex(other)
	_, _, err = store.PutObject(otherDig, bytes.NewReader(other), 99)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size mismatch")

	// Bad digest rejected.
	_, _, err = store.PutObject("not-hex", bytes.NewReader(data), int64(len(data)))
	require.Error(t, err)

	// Link session artifact + open.
	_, err = store.LinkSessionArtifact("sess-1", "art-1", dig)
	require.NoError(t, err)
	// Second link is idempotent.
	relLink, err := store.LinkSessionArtifact("sess-1", "art-1", dig)
	require.NoError(t, err)
	assert.Contains(t, relLink, "sessions")

	// Missing object.
	_, err = store.LinkSessionArtifact("sess-1", "art-2", strings.Repeat("ab", 32))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object missing")

	f, err := store.OpenObject(dig)
	require.NoError(t, err)
	defer f.Close()
	got, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, data, got)

	_, err = store.OpenObject("zz")
	require.Error(t, err)
}

func TestMaterializeBundleRejectsSymlinkPutPolicy(t *testing.T) {
	t.Parallel()
	bundle := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "ok.txt"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink("ok.txt", filepath.Join(bundle, "link")))
	err := artifacts.MaterializeBundle(bundle, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestPolicyCheckEdges(t *testing.T) {
	t.Parallel()
	require.Error(t, artifacts.PolicyCheck(nil, "text/plain", 1, 0, 0))
	require.Error(t, artifacts.PolicyCheck(&contracts.ArtifactPolicy{Enabled: false}, "text/plain", 1, 0, 0))

	pol := &contracts.ArtifactPolicy{
		Enabled: true, MaxBytesEach: 10, MaxBytesTotal: 20, MaxFiles: 2,
		AllowedTypes: []string{"text/*", "application/pdf"},
	}
	require.NoError(t, artifacts.PolicyCheck(pol, "text/plain", 5, 0, 0))
	require.NoError(t, artifacts.PolicyCheck(pol, "application/pdf", 5, 1, 5))

	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", -1, 0, 0))
	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", 11, 0, 0))
	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", 1, 2, 0))  // file quota
	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", 6, 0, 15)) // byte quota
	require.Error(t, artifacts.PolicyCheck(pol, "image/png", 1, 0, 0))

	// Defaults when limits zero + wildcard.
	pol2 := &contracts.ArtifactPolicy{Enabled: true, AllowedTypes: []string{"*/*"}}
	require.NoError(t, artifacts.PolicyCheck(pol2, "anything/x", 1, 0, 0))

	// Empty allowed types → any media ok.
	pol3 := &contracts.ArtifactPolicy{Enabled: true}
	require.NoError(t, artifacts.PolicyCheck(pol3, "image/png", 1, 0, 0))
}

func TestSafeFilenameNullAndEmpty(t *testing.T) {
	t.Parallel()
	_, err := artifacts.SafeFilename("")
	require.Error(t, err)
	_, err = artifacts.SafeFilename("   ")
	require.Error(t, err)
	_, err = artifacts.SafeFilename("a\x00b")
	require.Error(t, err)

	n, err := artifacts.SafeFilename("/tmp/../report.pdf")
	require.NoError(t, err)
	assert.Equal(t, "report.pdf", n)
}
