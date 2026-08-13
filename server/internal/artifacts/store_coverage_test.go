package artifacts_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestMaterializeBundleCopiesRegularTree(t *testing.T) {
	t.Parallel()
	bundle := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(bundle, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "nested", "a.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "b.txt"), []byte("beta"), 0o600))

	dest := t.TempDir()
	require.NoError(t, artifacts.MaterializeBundle(bundle, dest))

	a, err := os.ReadFile(filepath.Join(dest, "nested", "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "alpha", string(a))
	b, err := os.ReadFile(filepath.Join(dest, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "beta", string(b))

	// Empty roots rejected.
	require.Error(t, artifacts.MaterializeBundle("", dest))
	require.Error(t, artifacts.MaterializeBundle(bundle, ""))
}

func TestMaterializeBundleRejectsFIFO(t *testing.T) {
	t.Parallel()
	bundle := t.TempDir()
	fifo := filepath.Join(bundle, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	dest := t.TempDir()
	err := artifacts.MaterializeBundle(bundle, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "special file")
}

func TestSafeFilenameAndRequireSafeIDEdges(t *testing.T) {
	t.Parallel()
	store, err := artifacts.NewStore(t.TempDir())
	require.NoError(t, err)

	// SafeFilename edges
	n, err := artifacts.SafeFilename("  report.pdf  ")
	require.NoError(t, err)
	assert.Equal(t, "report.pdf", n)
	_, err = artifacts.SafeFilename(".")
	require.Error(t, err)
	_, err = artifacts.SafeFilename("..")
	require.Error(t, err)
	_, err = artifacts.SafeFilename("a\\b")
	// Base strips dir; backslash may remain as name component on unix after Base
	// depending on path — ensure either success with sanitized base or error.
	if err == nil {
		// On Unix filepath.Base leaves backslash; function replaces \\ with _
		got, e2 := artifacts.SafeFilename(`dir\file.txt`)
		require.NoError(t, e2)
		assert.NotContains(t, got, `\`)
	}

	// requireSafeID exercised via SessionArtifactPath / BundleRoot / NewStore.
	_, err = artifacts.NewStore("")
	require.Error(t, err)

	_, err = store.SessionArtifactPath("../escape", "art")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal")
	_, err = store.SessionArtifactPath("sess", "art/../x")
	require.Error(t, err)
	_, err = store.SessionArtifactPath("", "art")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
	_, err = store.SessionArtifactPath("sess", "")
	require.Error(t, err)

	_, err = store.BundleRoot("stu/../x", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.Error(t, err)
	_, err = store.BundleRoot("student-1", "not-hex")
	require.Error(t, err)

	// Happy path IDs.
	p, err := store.SessionArtifactPath("sess-ok", "art-ok")
	require.NoError(t, err)
	assert.Contains(t, p, "sessions")

	// MaterializeFixturesSafe rejects unsupported type and bad mode.
	dest := t.TempDir()
	err = artifacts.MaterializeFixturesSafe(dest, []contracts.FixtureEntry{
		{Path: "x", Type: "symlink"},
	})
	require.Error(t, err)
	err = artifacts.MaterializeFixturesSafe(dest, []contracts.FixtureEntry{
		{Path: "y.txt", Type: contracts.FixtureFile, Content: "z", Mode: "not-a-mode"},
	})
	require.Error(t, err)

	// Directory with mode.
	err = artifacts.MaterializeFixturesSafe(dest, []contracts.FixtureEntry{
		{Path: "dir", Type: contracts.FixtureDirectory, Mode: "0755"},
		{Path: "dir/f.txt", Type: contracts.FixtureFile, Content: "ok", Mode: "0644"},
	})
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dest, "dir", "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ok", string(data))
}

func TestWriteBundleEntryAndOpenObject(t *testing.T) {
	t.Parallel()
	store, err := artifacts.NewStore(t.TempDir())
	require.NoError(t, err)

	root, err := store.BundleRoot("stu1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(root, 0o750))
	require.NoError(t, store.WriteBundleEntry(root, "nested/file.txt", []byte("bundle"), 0))
	b, err := os.ReadFile(filepath.Join(root, "nested", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bundle", string(b))

	// Traversal rejected.
	require.Error(t, store.WriteBundleEntry(root, "../escape", []byte("x"), 0o644))
}
