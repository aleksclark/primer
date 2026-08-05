package artifacts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestPutObjectIdempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := artifacts.NewStore(root)
	require.NoError(t, err)

	data := []byte("hello portfolio")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	rel1, n1, err := store.PutObject(digest, bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), n1)
	assert.NotEmpty(t, rel1)

	rel2, n2, err := store.PutObject(digest, bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, rel1, rel2)
	assert.Equal(t, n1, n2)

	// Wrong size against existing object is rejected; wrong body with matching
	// digest key is ignored because the object is content-addressed.
	_, _, err = store.PutObject(digest, bytes.NewReader([]byte("wrong")), int64(len(data))+1)
	require.Error(t, err)
}

func TestPutObjectDigestMismatch(t *testing.T) {
	t.Parallel()
	store, err := artifacts.NewStore(t.TempDir())
	require.NoError(t, err)
	data := []byte("abc")
	_, _, err = store.PutObject(hex.EncodeToString(make([]byte, 32)), bytes.NewReader(data), int64(len(data)))
	require.Error(t, err)
}

func TestMaterializeBundleRejectsSymlink(t *testing.T) {
	t.Parallel()
	bundle := t.TempDir()
	// regular file
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "ok.txt"), []byte("x"), 0o644))
	// symlink escape attempt
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(bundle, "evil")))

	dest := t.TempDir()
	err := artifacts.MaterializeBundle(bundle, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestMaterializeBundleRejectsTraversalEntry(t *testing.T) {
	t.Parallel()
	// Craft a bundle directory name that would be unsafe if not checked via SafeRelPath.
	// Walk only sees real children; inject via MaterializeFixturesSafe path checks.
	err := artifacts.MaterializeFixturesSafe(t.TempDir(), []contracts.FixtureEntry{
		{Path: "../escape.txt", Type: contracts.FixtureFile, Content: "nope"},
	})
	require.Error(t, err)
}

func TestMaterializeFixturesSafeAndAbsoluteReject(t *testing.T) {
	t.Parallel()
	dest := t.TempDir()
	err := artifacts.MaterializeFixturesSafe(dest, []contracts.FixtureEntry{
		{Path: "notes/readme.txt", Type: contracts.FixtureFile, Content: "hi", Mode: "0644"},
		{Path: "notes", Type: contracts.FixtureDirectory, Mode: "0755"},
	})
	// order: directory may come after file — still ok because MkdirAll on parent
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dest, "notes", "readme.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hi", string(b))

	err = artifacts.MaterializeFixturesSafe(dest, []contracts.FixtureEntry{
		{Path: "/abs", Type: contracts.FixtureFile, Content: "x"},
	})
	require.Error(t, err)
}

func TestPolicyCheck(t *testing.T) {
	t.Parallel()
	pol := &contracts.ArtifactPolicy{
		Enabled:       true,
		MaxFiles:      2,
		MaxBytesEach:  10,
		MaxBytesTotal: 15,
		AllowedTypes:  []string{"text/plain", "image/*"},
	}
	require.NoError(t, artifacts.PolicyCheck(pol, "text/plain", 5, 0, 0))
	require.NoError(t, artifacts.PolicyCheck(pol, "image/png", 5, 0, 0))
	require.Error(t, artifacts.PolicyCheck(pol, "application/zip", 5, 0, 0))
	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", 11, 0, 0))
	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", 5, 2, 0))
	require.Error(t, artifacts.PolicyCheck(pol, "text/plain", 5, 0, 12))
	require.Error(t, artifacts.PolicyCheck(nil, "text/plain", 1, 0, 0))
}

func TestSafeFilename(t *testing.T) {
	t.Parallel()
	n, err := artifacts.SafeFilename("../../etc/passwd")
	require.NoError(t, err)
	assert.Equal(t, "passwd", n)
	_, err = artifacts.SafeFilename("")
	require.Error(t, err)
}

func TestLinkSessionArtifact(t *testing.T) {
	t.Parallel()
	store, err := artifacts.NewStore(t.TempDir())
	require.NoError(t, err)
	data := []byte("payload")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	_, _, err = store.PutObject(digest, bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	rel, err := store.LinkSessionArtifact("sess-1", "art-1", digest)
	require.NoError(t, err)
	assert.Contains(t, rel, "sessions")
	// idempotent
	_, err = store.LinkSessionArtifact("sess-1", "art-1", digest)
	require.NoError(t, err)
}
