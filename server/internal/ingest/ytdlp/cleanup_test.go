package ytdlp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

// Fixture matches live EC leftovers: orphan jpg, .f137.mp4.part, S01E000 without [id].
func TestCleanupShowDir_ECFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "essential-craftsman"
	season := filepath.Join(dir, "Shows", slug, "Season 01")
	require.NoError(t, os.MkdirAll(season, 0o755))

	// Keep: complete mkv with [id]
	keepMKV := filepath.Join(season, "essential-craftsman - S01E001 - Good [abcdefghijk].mkv")
	require.NoError(t, os.WriteFile(keepMKV, []byte("mkv"), 0o644))
	keepJPG := filepath.Join(season, "essential-craftsman - S01E001 - Good [abcdefghijk].jpg")
	require.NoError(t, os.WriteFile(keepJPG, []byte("jpg"), 0o644))

	// Delete: EC-style orphans
	orphanJPG := filepath.Join(season, "essential-craftsman - S01E000 - Fire as a Tool.jpg")
	require.NoError(t, os.WriteFile(orphanJPG, []byte("jpg"), 0o644))
	partFrag := filepath.Join(season, "essential-craftsman - S01E000 - Fire as a Tool.f137.mp4.part")
	require.NoError(t, os.WriteFile(partFrag, []byte("part"), 0o644))
	s01e000 := filepath.Join(season, "essential-craftsman - S01E000 - Fire as a Tool.mkv")
	// no id — not complete identity; if present without [id], cleanup removes thumbs without id
	// Spec: S01E000 thumbs without [id]. Also *.part, *.ytdl, *.part-Frag*, orphan *.f[0-9]*.mp4/.webm
	require.NoError(t, os.WriteFile(filepath.Join(season, "x.part"), []byte("p"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(season, "x.ytdl"), []byte("y"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(season, "vid.part-Frag1"), []byte("f"), 0o644))
	// orphan format stream without matching mkv
	require.NoError(t, os.WriteFile(filepath.Join(season, "clip.f137.mp4"), []byte("f"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(season, "clip.f251.webm"), []byte("f"), 0o644))

	// S01E000 thumb without [id]
	_ = s01e000 // not creating full mkv without id as "complete"; only thumbs as EC

	n, err := ytdlp.CleanupShowDir(dir, slug)
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	_, err = os.Stat(keepMKV)
	require.NoError(t, err, "complete [id].mkv must be kept")
	_, err = os.Stat(keepJPG)
	require.NoError(t, err, "matching jpg with [id] kept")

	_, err = os.Stat(orphanJPG)
	assert.True(t, os.IsNotExist(err), "S01E000 jpg without [id] deleted")
	_, err = os.Stat(partFrag)
	assert.True(t, os.IsNotExist(err), ".f137.mp4.part deleted")
	_, err = os.Stat(filepath.Join(season, "x.part"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(season, "x.ytdl"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(season, "clip.f137.mp4"))
	assert.True(t, os.IsNotExist(err), "orphan f###.mp4 deleted")
	_, err = os.Stat(filepath.Join(season, "clip.f251.webm"))
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupShowDir_MissingShowOK(t *testing.T) {
	t.Parallel()
	n, err := ytdlp.CleanupShowDir(t.TempDir(), "no-such")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}
