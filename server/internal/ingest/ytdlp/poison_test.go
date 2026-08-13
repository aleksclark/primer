package ytdlp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestRemoveRootShowNFO_OnlyRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rootNFO := filepath.Join(dir, "tvshow.nfo")
	require.NoError(t, os.WriteFile(rootNFO, []byte("<tvshow>Primer Plano</tvshow>"), 0o644))

	showDir := filepath.Join(dir, "Shows", "essential-craftsman")
	require.NoError(t, os.MkdirAll(showDir, 0o755))
	showNFO := filepath.Join(showDir, "tvshow.nfo")
	require.NoError(t, os.WriteFile(showNFO, []byte("<tvshow>EC</tvshow>"), 0o644))

	err := ytdlp.RemoveRootShowNFO(dir)
	require.NoError(t, err)

	_, err = os.Stat(rootNFO)
	assert.True(t, os.IsNotExist(err), "root poison tvshow.nfo deleted")
	data, err := os.ReadFile(showNFO)
	require.NoError(t, err)
	assert.Contains(t, string(data), "EC")
}

func TestRemoveRootShowNFO_MissingOK(t *testing.T) {
	t.Parallel()
	err := ytdlp.RemoveRootShowNFO(t.TempDir())
	require.NoError(t, err)
}
