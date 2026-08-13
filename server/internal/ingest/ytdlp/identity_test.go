package ytdlp_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestPathMatches_Boundary(t *testing.T) {
	t.Parallel()
	slug := "paul-sellers"
	assert.True(t, ytdlp.PathMatches("/media/tv/Primer/Shows/paul-sellers/Season 01/x.mkv", slug))
	assert.True(t, ytdlp.PathMatches("/media/tv/Primer/Shows/paul-sellers", slug))
	assert.True(t, ytdlp.PathMatches("Shows/paul-sellers/Season 01/x.mkv", slug))
	assert.False(t, ytdlp.PathMatches("/media/tv/Primer/Shows/paul-sellers-extra/Season 01/x.mkv", slug))
	assert.False(t, ytdlp.PathMatches("/media/tv/Primer/Shows/other/Season 01/x.mkv", slug))
	assert.False(t, ytdlp.PathMatches("/media/tv/Primer/Shows/paul-sellers-backup", slug))
}

func TestParseYouTubeID(t *testing.T) {
	t.Parallel()
	id, ok := ytdlp.ParseYouTubeID("essential-craftsman - S01E007 - Fire as a Tool [dQw4w9wgxcQ].mkv")
	assert.True(t, ok)
	assert.Equal(t, "dQw4w9wgxcQ", id)

	id, ok = ytdlp.ParseYouTubeID("show - title [abcdefghijk].info.json")
	assert.True(t, ok)
	assert.Equal(t, "abcdefghijk", id)

	_, ok = ytdlp.ParseYouTubeID("no-brackets.mkv")
	assert.False(t, ok)

	_, ok = ytdlp.ParseYouTubeID("short [abc].mkv")
	assert.False(t, ok)

	_, ok = ytdlp.ParseYouTubeID("long [abcdefghijkl].mkv") // 12 chars
	assert.False(t, ok)

	_, ok = ytdlp.ParseYouTubeID("bad [abcd!fghijk].mkv")
	assert.False(t, ok)
}

func TestStagingTemplate(t *testing.T) {
	t.Parallel()
	got := ytdlp.StagingTemplate("/media/tv/Primer", "essential-craftsman")
	want := filepath.Join(
		"/media/tv/Primer",
		"Shows",
		"essential-craftsman",
		"Season 01",
		"_staging",
		`essential-craftsman - %(title).80B [%(id)s].%(ext)s`,
	)
	assert.Equal(t, want, got)
}

func TestFinalBasename(t *testing.T) {
	t.Parallel()
	got := ytdlp.FinalBasename("essential-craftsman", 1, 7, "Fire as a Tool", "dQw4w9wgxcQ")
	assert.Equal(t, "essential-craftsman - S01E007 - Fire as a Tool [dQw4w9wgxcQ]", got)
}

func TestPerShowArchivePath(t *testing.T) {
	t.Parallel()
	got := ytdlp.PerShowArchivePath("/media/tv/Primer", "paul-sellers")
	assert.Equal(t, filepath.Join("/media/tv/Primer", "Shows", "paul-sellers", ".ytdlp-archive.txt"), got)
}

func TestPathPrefixStillWorks(t *testing.T) {
	t.Parallel()
	require.Equal(t, filepath.Join("Shows", "x"), ytdlp.PathPrefix("x"))
}
