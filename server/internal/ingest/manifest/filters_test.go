package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

func TestParseYAMLVideosAndFilterPointers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	yaml := `
items:
  - id: yt-chan
    title: "Example Channel"
    kind: youtube_channel
    url: https://www.youtube.com/@Example
    class: educational
    filters:
      playlists: [Curriculum, Science]
      min_duration_seconds: 120
      exclude_shorts: false
      exclude_live: false
    max_episodes: 50
    exclude_episodes: [S01E07, dQw4w9WgXcQ]
    videos:
      - id: dQw4w9WgXcQ
        title: "Override title"
        class: entertainment
        subject_tags: [music]
        standard_codes: [TN.ART.1]
        exclude: true
      - id: abcdefghijk
        title: "Keep me"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))

	m, err := manifest.Load(path)
	require.NoError(t, err)
	it := m.ByID("yt-chan")
	require.NotNil(t, it)

	assert.Equal(t, []string{"Curriculum", "Science"}, it.Filters.Playlists)
	assert.Equal(t, 120, it.Filters.MinDurationSeconds)
	require.NotNil(t, it.Filters.ExcludeShorts)
	assert.False(t, *it.Filters.ExcludeShorts)
	require.NotNil(t, it.Filters.ExcludeLive)
	assert.False(t, *it.Filters.ExcludeLive)
	assert.Equal(t, 50, it.MaxEpisodes)

	require.Len(t, it.Videos, 2)
	assert.Equal(t, "dQw4w9WgXcQ", it.Videos[0].ID)
	assert.Equal(t, "Override title", it.Videos[0].Title)
	assert.Equal(t, manifest.ClassEntertainment, it.Videos[0].Class)
	assert.Equal(t, []string{"music"}, it.Videos[0].SubjectTags)
	assert.Equal(t, []string{"TN.ART.1"}, it.Videos[0].StandardCodes)
	assert.True(t, it.Videos[0].Exclude)
	assert.Equal(t, "abcdefghijk", it.Videos[1].ID)
	assert.False(t, it.Videos[1].Exclude)
}

func TestValidateRejectsDuplicateMissingBadVideoOverride(t *testing.T) {
	t.Parallel()

	base := func(videos []manifest.VideoOverride) *manifest.Manifest {
		return &manifest.Manifest{Items: []manifest.Item{{
			ID: "yt", Title: "YT", Kind: manifest.KindYouTubeChannel,
			URL: "https://www.youtube.com/@x", Class: manifest.ClassMixed,
			Videos: videos,
		}}}
	}

	// missing video id
	err := base([]manifest.VideoOverride{{Title: "no-id"}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")

	// duplicate video ids
	err = base([]manifest.VideoOverride{
		{ID: "dQw4w9WgXcQ"},
		{ID: "dQw4w9WgXcQ"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")

	// bad override class
	err = base([]manifest.VideoOverride{
		{ID: "dQw4w9WgXcQ", Class: "nope"},
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "class")

	// empty class is ok
	require.NoError(t, base([]manifest.VideoOverride{
		{ID: "dQw4w9WgXcQ"},
	}).Validate())

	// valid classes ok
	for _, c := range []string{
		manifest.ClassEducational, manifest.ClassEntertainment, manifest.ClassMixed,
	} {
		require.NoError(t, base([]manifest.VideoOverride{
			{ID: "dQw4w9WgXcQ", Class: c},
		}).Validate())
	}
}

func TestExcludedMatchesYouTubeIDAndPaddedEpisode(t *testing.T) {
	t.Parallel()
	it := manifest.Item{
		ExcludeEpisodes: []string{"S01E07", "dQw4w9WgXcQ"},
	}

	// episode keys still work (case-insensitive)
	assert.True(t, it.Excluded("s01e07"))
	assert.True(t, it.Excluded("S01E07"))

	// padded episode form S01E007 matches S01E07
	assert.True(t, it.Excluded("S01E007"))
	assert.True(t, it.Excluded("s01e007"))

	// youtube id match is case-sensitive as stored
	assert.True(t, it.Excluded("dQw4w9WgXcQ"))
	assert.False(t, it.Excluded("DQw4w9WgXcQ"))

	assert.False(t, it.Excluded("S01E01"))
	assert.False(t, it.Excluded("otherid1234"))
}

func TestExcludedVideoAndOverrideFor(t *testing.T) {
	t.Parallel()
	it := manifest.Item{
		ExcludeEpisodes: []string{"S01E07", "aaaaaaaaaaa"},
		Videos: []manifest.VideoOverride{
			{ID: "bbbbbbbbbbb", Exclude: true, Title: "skip"},
			{ID: "ccccccccccc", Title: "keep", Class: manifest.ClassEducational},
		},
	}

	// exclude_episodes by youtube id
	assert.True(t, manifest.ExcludedVideo(it, "aaaaaaaaaaa", ""))
	// exclude_episodes by episode key
	assert.True(t, manifest.ExcludedVideo(it, "", "S01E07"))
	assert.True(t, manifest.ExcludedVideo(it, "zzzzzzzzzzz", "S01E007"))
	// videos[] exclude flag
	assert.True(t, manifest.ExcludedVideo(it, "bbbbbbbbbbb", ""))
	// not excluded
	assert.False(t, manifest.ExcludedVideo(it, "ccccccccccc", "S01E01"))
	assert.False(t, manifest.ExcludedVideo(it, "ddddddddddd", ""))

	ov := manifest.OverrideFor(it, "ccccccccccc")
	require.NotNil(t, ov)
	assert.Equal(t, "keep", ov.Title)
	assert.Equal(t, manifest.ClassEducational, ov.Class)

	assert.Nil(t, manifest.OverrideFor(it, "missingxxxx"))
	ovEx := manifest.OverrideFor(it, "bbbbbbbbbbb")
	require.NotNil(t, ovEx)
	assert.True(t, ovEx.Exclude)
}

func TestDefaultFilterEffectiveValues(t *testing.T) {
	t.Parallel()

	// unset → defaults
	var zero manifest.Filters
	assert.Equal(t, manifest.DefaultMinDurationSeconds, manifest.EffectiveMinDuration(zero))
	assert.Equal(t, 60, manifest.DefaultMinDurationSeconds)
	assert.True(t, manifest.EffectiveExcludeShorts(zero))
	assert.True(t, manifest.EffectiveExcludeLive(zero))

	// explicit values
	min := 120
	f := manifest.Filters{MinDurationSeconds: min}
	assert.Equal(t, 120, manifest.EffectiveMinDuration(f))

	// pointer false → opt out
	fFalse := false
	fTrue := true
	assert.False(t, manifest.EffectiveExcludeShorts(manifest.Filters{ExcludeShorts: &fFalse}))
	assert.False(t, manifest.EffectiveExcludeLive(manifest.Filters{ExcludeLive: &fFalse}))
	assert.True(t, manifest.EffectiveExcludeShorts(manifest.Filters{ExcludeShorts: &fTrue}))
	assert.True(t, manifest.EffectiveExcludeLive(manifest.Filters{ExcludeLive: &fTrue}))
}

func TestPlaylistFilterHelpers(t *testing.T) {
	t.Parallel()

	// empty playlists on youtube_channel → /videos only (no playlist filter)
	empty := manifest.Item{
		Kind: manifest.KindYouTubeChannel,
		URL:  "https://www.youtube.com/@x",
	}
	assert.False(t, manifest.UsesPlaylistFilter(empty))
	assert.Empty(t, manifest.PlaylistNames(empty))

	// non-empty playlists set UsesPlaylistFilter
	withPL := manifest.Item{
		Kind: manifest.KindYouTubeChannel,
		URL:  "https://www.youtube.com/@x",
		Filters: manifest.Filters{
			Playlists: []string{"Curriculum", "Science"},
		},
	}
	assert.True(t, manifest.UsesPlaylistFilter(withPL))
	assert.Equal(t, []string{"Curriculum", "Science"}, manifest.PlaylistNames(withPL))

	// empty string playlist name fails closed
	bad := manifest.Item{
		Kind: manifest.KindYouTubeChannel,
		URL:  "https://www.youtube.com/@x",
		Filters: manifest.Filters{
			Playlists: []string{"Curriculum", "", "Science"},
		},
	}
	err := manifest.ValidatePlaylists(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "playlist")

	require.NoError(t, manifest.ValidatePlaylists(withPL))
	require.NoError(t, manifest.ValidatePlaylists(empty))
}

func TestMaxEpisodesImportHelpers(t *testing.T) {
	t.Parallel()

	// MaxEpisodes applies to Video+Episode import (YouTube / series), not Folder.
	yt := manifest.Item{Kind: manifest.KindYouTubeChannel, MaxEpisodes: 25}
	assert.True(t, yt.AppliesToYouTubeImport())
	assert.True(t, yt.CapsImports())
	assert.Equal(t, 25, yt.ImportLimit())

	series := manifest.Item{Kind: manifest.KindSeries, MaxEpisodes: 10}
	assert.False(t, series.AppliesToYouTubeImport())
	assert.True(t, series.CapsImports())
	assert.Equal(t, 10, series.ImportLimit())

	uncapped := manifest.Item{Kind: manifest.KindYouTubePlaylist, MaxEpisodes: 0}
	assert.True(t, uncapped.AppliesToYouTubeImport())
	assert.False(t, uncapped.CapsImports())
	assert.Equal(t, 0, uncapped.ImportLimit())

	movie := manifest.Item{Kind: manifest.KindMovie, MaxEpisodes: 5}
	assert.False(t, movie.AppliesToYouTubeImport())
}

func TestSampleYAMLStillLoads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(sampleYAML(t)), 0o644))
	m, err := manifest.Load(path)
	require.NoError(t, err)
	require.Len(t, m.Items, 4)
	assert.NotNil(t, m.ByID("paul-sellers"))
	assert.False(t, manifest.UsesPlaylistFilter(*m.ByID("paul-sellers")))
}

func TestValidatePlaylistsIntegratedInValidate(t *testing.T) {
	t.Parallel()
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "yt", Title: "YT", Kind: manifest.KindYouTubeChannel,
		URL: "https://www.youtube.com/@x", Class: manifest.ClassMixed,
		Filters: manifest.Filters{Playlists: []string{"ok", "  "}},
	}}}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "playlist")
}
