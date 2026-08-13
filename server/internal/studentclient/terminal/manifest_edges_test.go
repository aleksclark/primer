package terminal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestCaptureManifestEmptyRootAndTree(t *testing.T) {
	t.Parallel()
	m, err := terminal.CaptureManifest("")
	require.NoError(t, err)
	assert.Empty(t, m.Entries)
	assert.Equal(t, contracts.WorkspaceManifestSchemaVersion, m.SchemaVersion)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "home", "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "home", "a.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "home", "docs", "b.txt"), []byte("world"), 0o600))
	// Symlink entry is recorded as symlink type.
	require.NoError(t, os.Symlink("a.txt", filepath.Join(root, "home", "link")))

	m, err = terminal.CaptureManifest(root)
	require.NoError(t, err)
	require.NotEmpty(t, m.Entries)
	assert.NotEmpty(t, m.Digest)
	assert.False(t, m.Truncated)

	byPath := map[string]contracts.WorkspaceManifestEntry{}
	for _, e := range m.Entries {
		byPath[e.Path] = e
	}
	require.Contains(t, byPath, "home")
	assert.Equal(t, contracts.PathTypeDirectory, byPath["home"].Type)
	require.Contains(t, byPath, "home/a.txt")
	assert.Equal(t, contracts.PathTypeFile, byPath["home/a.txt"].Type)
	assert.NotEmpty(t, byPath["home/a.txt"].SHA256)
	assert.Equal(t, int64(5), byPath["home/a.txt"].Size)
	require.Contains(t, byPath, "home/link")
	assert.Equal(t, contracts.PathTypeSymlink, byPath["home/link"].Type)
}

func TestWriteSetDiffChangesAndCap(t *testing.T) {
	t.Parallel()
	before := contracts.WorkspaceManifest{Entries: []contracts.WorkspaceManifestEntry{
		{Path: "a", Type: contracts.PathTypeFile, Mode: "0644", SHA256: "aa", Size: 1},
		{Path: "b", Type: contracts.PathTypeFile, Mode: "0644", SHA256: "bb", Size: 1},
		{Path: "gone", Type: contracts.PathTypeFile, Mode: "0644", SHA256: "gg", Size: 1},
	}}
	after := contracts.WorkspaceManifest{Entries: []contracts.WorkspaceManifestEntry{
		{Path: "a", Type: contracts.PathTypeFile, Mode: "0644", SHA256: "aa", Size: 1}, // unchanged
		{Path: "b", Type: contracts.PathTypeFile, Mode: "0755", SHA256: "bb", Size: 1}, // mode change
		{Path: "c", Type: contracts.PathTypeFile, Mode: "0644", SHA256: "cc", Size: 2}, // new
	}}
	diff := terminal.WriteSetDiff(before, after)
	assert.ElementsMatch(t, []string{"b", "c", "gone"}, diff)

	// Cap at 100 paths.
	var manyBefore, manyAfter []contracts.WorkspaceManifestEntry
	for i := 0; i < 120; i++ {
		p := filepath.Join("f", string(rune('a'+(i%26)))+filepath.Base(t.Name())+string(rune(i)))
		// Use numeric path to avoid weirdness.
		p = "p" + string(rune('0'+i%10)) + "-" + string(rune('a'+i%26)) + "-" + string(rune(i%50+65))
		// simpler:
		p = "file-" + itoa(i)
		manyBefore = append(manyBefore, contracts.WorkspaceManifestEntry{Path: p, Type: contracts.PathTypeFile, Mode: "0644", SHA256: "x", Size: 1})
		manyAfter = append(manyAfter, contracts.WorkspaceManifestEntry{Path: p + "-new", Type: contracts.PathTypeFile, Mode: "0644", SHA256: "y", Size: 1})
	}
	diff = terminal.WriteSetDiff(
		contracts.WorkspaceManifest{Entries: manyBefore},
		contracts.WorkspaceManifest{Entries: manyAfter},
	)
	assert.LessOrEqual(t, len(diff), 100)
	assert.Equal(t, 100, len(diff))

	// Empty path not added; identical manifests → empty.
	assert.Empty(t, terminal.WriteSetDiff(before, before))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
