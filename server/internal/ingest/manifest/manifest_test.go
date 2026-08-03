package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

func sampleYAML(t *testing.T) string {
	t.Helper()
	return `
items:
  - id: living-planet
    title: "The Living Planet"
    year: 1984
    kind: series
    provider: {tvdb: 79165}
    class: educational
    subject_tags: [science, life-science]
    standard_codes: [TN.SCI.6.LS2.1]
    priority: 1
    exclude_episodes: [S01E07]
  - id: paul-sellers
    title: "Paul Sellers"
    kind: youtube_channel
    url: https://www.youtube.com/@PaulSellersWoodwork
    class: mixed
    subject_tags: [practical, woodworking]
  - id: bernstein-ypc
    title: "Leonard Bernstein Young People's Concerts"
    kind: manual
    class: educational
    subject_tags: [music, arts]
  - id: matrix
    title: "The Matrix"
    year: 1999
    kind: movie
    class: entertainment
    priority: 5
`
}

func TestLoadValidateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(sampleYAML(t)), 0o644))

	m, err := manifest.Load(path)
	require.NoError(t, err)
	require.Len(t, m.Items, 4)

	lp := m.ByID("living-planet")
	require.NotNil(t, lp)
	assert.Equal(t, 79165, lp.Provider.TVDB)
	assert.True(t, lp.Excluded("s01e07"))
	assert.False(t, lp.Excluded("S01E01"))
	assert.False(t, lp.NeedsResolve())

	matrix := m.ByID("matrix")
	require.NotNil(t, matrix)
	assert.True(t, matrix.NeedsResolve())

	require.NoError(t, m.SetProvider("matrix", manifest.Provider{TMDB: 603}))
	assert.False(t, m.ByID("matrix").NeedsResolve())

	out := filepath.Join(dir, "out.yaml")
	require.NoError(t, manifest.Save(out, m))
	again, err := manifest.Load(out)
	require.NoError(t, err)
	assert.Equal(t, 603, again.ByID("matrix").Provider.TMDB)
}

func TestValidateRejectsBad(t *testing.T) {
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "x", Title: "X", Kind: "nope", Class: manifest.ClassMixed,
	}}}
	assert.Error(t, m.Validate())

	m = &manifest.Manifest{Items: []manifest.Item{
		{ID: "a", Title: "A", Kind: manifest.KindManual, Class: manifest.ClassMixed},
		{ID: "a", Title: "B", Kind: manifest.KindManual, Class: manifest.ClassMixed},
	}}
	assert.Error(t, m.Validate())

	m = &manifest.Manifest{Items: []manifest.Item{{
		ID: "yt", Title: "YT", Kind: manifest.KindYouTubeChannel, Class: manifest.ClassMixed,
	}}}
	assert.Error(t, m.Validate())
}

func TestSortedByPriority(t *testing.T) {
	m := &manifest.Manifest{Items: []manifest.Item{
		{ID: "c", Title: "C", Kind: manifest.KindManual, Class: manifest.ClassMixed, Priority: 0},
		{ID: "a", Title: "A", Kind: manifest.KindManual, Class: manifest.ClassMixed, Priority: 2},
		{ID: "b", Title: "B", Kind: manifest.KindManual, Class: manifest.ClassMixed, Priority: 1},
	}}
	ordered := m.SortedByPriority()
	assert.Equal(t, []string{"b", "a", "c"}, []string{ordered[0].ID, ordered[1].ID, ordered[2].ID})
}

func TestReviewRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yaml")

	r, err := manifest.LoadReview(path)
	require.NoError(t, err)
	assert.Empty(t, r.Entries)

	r.Upsert(manifest.ReviewEntry{
		ID: "matrix", Title: "The Matrix", Year: 1999, Kind: manifest.KindMovie,
		Reason: "multiple hits",
		Candidates: []manifest.Candidate{
			{Title: "The Matrix", Year: 1999, TMDB: 603},
			{Title: "The Matrix Reloaded", Year: 2003, TMDB: 604},
		},
	})
	require.NoError(t, manifest.SaveReview(path, r))

	again, err := manifest.LoadReview(path)
	require.NoError(t, err)
	require.Len(t, again.Entries, 1)
	assert.Equal(t, 603, again.Entries[0].Candidates[0].TMDB)

	// Preserve human choice across upsert.
	again.Entries[0].ChosenTMDB = 603
	again.Upsert(manifest.ReviewEntry{
		ID: "matrix", Title: "The Matrix", Kind: manifest.KindMovie, Reason: "still multi",
	})
	assert.Equal(t, 603, again.ByID("matrix").ChosenTMDB)

	again.Remove("matrix")
	assert.Empty(t, again.Entries)
}
