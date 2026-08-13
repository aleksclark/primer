package ytdlp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestWriteShowNFO_Golden(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := ytdlp.WriteShowNFO(dir, "essential-craftsman", "Essential Craftsman", "UCchannelidxx")
	require.NoError(t, err)
	wantPath := filepath.Join(dir, "Shows", "essential-craftsman", "tvshow.nfo")
	assert.Equal(t, wantPath, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(data)
	assert.True(t, strings.HasPrefix(s, `<?xml version="1.0" encoding="utf-8" standalone="yes"?>`))
	assert.Contains(t, s, "<tvshow>")
	assert.Contains(t, s, "<title>Essential Craftsman</title>")
	assert.Contains(t, s, "<originaltitle>Essential Craftsman</originaltitle>")
	assert.Contains(t, s, "<sorttitle>Essential Craftsman</sorttitle>")
	assert.Contains(t, s, "Slug essential-craftsman.")
	assert.Contains(t, s, `<uniqueid type="primer-slug" default="true">essential-craftsman</uniqueid>`)
	assert.Contains(t, s, `<uniqueid type="youtube-channel">UCchannelidxx</uniqueid>`)
	assert.Contains(t, s, "<premiered></premiered>")
	assert.Contains(t, s, "<status>Continuing</status>")
	assert.Contains(t, s, "</tvshow>")
}

func TestWriteEpisodeNFO_Golden(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "Shows", "essential-craftsman", "Season 01",
		"essential-craftsman - S01E007 - Fire as a Tool [dQw4w9wgxcQ]")
	require.NoError(t, os.MkdirAll(filepath.Dir(base), 0o755))

	meta := ytdlp.EpisodeNFO{
		Title:      "Fire as a Tool",
		ShowTitle:  "Essential Craftsman",
		Season:     1,
		Episode:    7,
		Plot:       "A long description about fire." + strings.Repeat("x", 50),
		UploadDate: "20180312",
		RuntimeSec: 18*60 + 30, // 18 minutes
		YouTubeID:  "dQw4w9wgxcQ",
		Slug:       "essential-craftsman",
	}
	path, err := ytdlp.WriteEpisodeNFO(base+".mkv", meta)
	require.NoError(t, err)
	assert.Equal(t, base+".nfo", path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(data)
	assert.True(t, strings.HasPrefix(s, `<?xml version="1.0" encoding="utf-8" standalone="yes"?>`))
	assert.Contains(t, s, "<episodedetails>")
	assert.Contains(t, s, "<title>Fire as a Tool</title>")
	assert.Contains(t, s, "<showtitle>Essential Craftsman</showtitle>")
	assert.Contains(t, s, "<season>1</season>")
	assert.Contains(t, s, "<episode>7</episode>")
	assert.Contains(t, s, "<premiered>2018-03-12</premiered>")
	assert.Contains(t, s, "<aired>2018-03-12</aired>")
	assert.Contains(t, s, "<runtime>18</runtime>")
	assert.Contains(t, s, `<uniqueid type="youtube" default="true">dQw4w9wgxcQ</uniqueid>`)
	assert.Contains(t, s, `<uniqueid type="primer-slug">essential-craftsman</uniqueid>`)
	assert.Contains(t, s, "A long description about fire.")
}

func TestWriteEpisodeNFO_TruncatesPlot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "ep")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	long := strings.Repeat("a", 5000)
	path, err := ytdlp.WriteEpisodeNFO(base+".mkv", ytdlp.EpisodeNFO{
		Title: "T", ShowTitle: "S", Season: 1, Episode: 1,
		Plot: long, UploadDate: "20200101", RuntimeSec: 60,
		YouTubeID: "abcdefghijk", Slug: "s",
	})
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// plot content capped at 4000 runes/bytes of description
	assert.NotContains(t, string(data), strings.Repeat("a", 4001))
	assert.Contains(t, string(data), strings.Repeat("a", 4000))
}

func TestFormatUploadDate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "2018-03-12", ytdlp.FormatUploadDate("20180312"))
	assert.Equal(t, "", ytdlp.FormatUploadDate(""))
	assert.Equal(t, "", ytdlp.FormatUploadDate("bad"))
}
