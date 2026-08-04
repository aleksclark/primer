package reviewtui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

func TestPendingSkipsAlreadyChosen(t *testing.T) {
	t.Parallel()
	r := &manifest.Review{Entries: []manifest.ReviewEntry{
		{ID: "a", Title: "A", Kind: manifest.KindMovie},
		{ID: "b", Title: "B", Kind: manifest.KindMovie, ChosenTMDB: 1},
		{ID: "c", Title: "C", Kind: manifest.KindSeries, ChosenTVDB: 2},
		{ID: "d", Title: "D", Kind: manifest.KindSeries},
	}}
	idxs := pending(r)
	require.Equal(t, []int{0, 3}, idxs)
}

func TestApplyChoiceMovieAndSeries(t *testing.T) {
	t.Parallel()
	r := &manifest.Review{Entries: []manifest.ReviewEntry{
		{
			ID: "matrix", Title: "The Matrix", Kind: manifest.KindMovie,
			Candidates: []manifest.Candidate{
				{Title: "The Matrix", Year: 1999, TMDB: 603},
				{Title: "The Matrix Reloaded", Year: 2003, TMDB: 604},
			},
		},
		{
			ID: "lp", Title: "Living Planet", Kind: manifest.KindSeries,
			Candidates: []manifest.Candidate{
				{Title: "The Living Planet", Year: 1984, TVDB: 79165, TMDB: 99},
			},
		},
	}}
	m := newModel(r, pending(r), "review.yaml")
	require.Equal(t, "matrix", m.entry().ID)

	m.applyChoice(r.Entries[0].Candidates[0])
	assert.Equal(t, 603, r.Entries[0].ChosenTMDB)
	assert.Equal(t, 0, r.Entries[0].ChosenTVDB)

	m.pos = 1
	m.applyChoice(r.Entries[1].Candidates[0])
	assert.Equal(t, 79165, r.Entries[1].ChosenTVDB)
	assert.Equal(t, 99, r.Entries[1].ChosenTMDB)
}

func TestCandidateItemRendering(t *testing.T) {
	t.Parallel()
	c := candidateItem{
		cand: manifest.Candidate{
			Title: "The Matrix", Year: 1999, TMDB: 603,
			Overview: "A computer hacker learns about the true nature of reality and his role in the war against its controllers.",
		},
		chosen: true,
	}
	assert.Contains(t, c.Title(), "★")
	assert.Contains(t, c.Title(), "1999")
	assert.Contains(t, c.Description(), "tmdb=603")
	assert.Contains(t, c.FilterValue(), "Matrix")
}

func TestRunEmptyQueue(t *testing.T) {
	t.Parallel()
	res, err := Run(&manifest.Review{}, "x.yaml")
	require.NoError(t, err)
	assert.Equal(t, 0, res.Chosen)
	assert.False(t, res.QuitEarly)
}

func sampleReview() *manifest.Review {
	return &manifest.Review{Entries: []manifest.ReviewEntry{
		{
			ID: "matrix", Title: "The Matrix", Year: 1999, Kind: manifest.KindMovie,
			Reason: "2 hits",
			Candidates: []manifest.Candidate{
				{Title: "The Matrix", Year: 1999, TMDB: 603, Overview: "Wake up."},
				{Title: "The Matrix Reloaded", Year: 2003, TMDB: 604},
			},
		},
		{
			ID: "ghost", Title: "Ghost Show", Year: 1901, Kind: manifest.KindMovie,
			Reason: "no hits",
			// No candidates — enter/skip path.
		},
		{
			ID: "lp", Title: "Living Planet", Year: 1984, Kind: manifest.KindSeries,
			Candidates: []manifest.Candidate{
				{Title: "The Living Planet", Year: 1984, TVDB: 79165},
			},
		},
	}}
}

func TestModelSelectSkipQuit(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	m := newModel(r, pending(r), "review.yaml")
	require.Len(t, m.queue, 3)

	// Resize.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)
	assert.Equal(t, 100, m.width)

	// View should render the first entry.
	view := m.View()
	assert.Contains(t, view, "The Matrix")
	assert.Contains(t, view, "1 / 3")
	assert.Contains(t, view, "content-ingest review")

	// Select first candidate.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	assert.Equal(t, 603, r.Entries[0].ChosenTMDB)
	assert.Equal(t, 1, m.chosen)
	assert.Equal(t, 1, m.pos, "advanced to next entry")
	assert.Nil(t, cmd)

	// Skip the no-candidate entry.
	view = m.View()
	assert.Contains(t, view, "No candidates")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(model)
	assert.Equal(t, 1, m.skipped)
	assert.Equal(t, 2, m.pos)

	// Prev / next navigation.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(model)
	assert.Equal(t, 1, m.pos)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(model)
	assert.Equal(t, 2, m.pos)

	// Choose the series candidate and finish.
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	assert.Equal(t, 79165, r.Entries[2].ChosenTVDB)
	assert.Equal(t, 2, m.chosen)
	assert.True(t, m.done)
	require.NotNil(t, cmd) // tea.Quit
}

func TestModelQuitEarly(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	m := newModel(r, pending(r), "")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = updated.(model)
	assert.True(t, m.quitEarly)
	assert.True(t, m.done)
	require.NotNil(t, cmd)
}

func TestApplyChoiceDefaultKind(t *testing.T) {
	t.Parallel()
	r := &manifest.Review{Entries: []manifest.ReviewEntry{{
		ID: "x", Title: "X", Kind: manifest.KindManual,
	}}}
	m := newModel(r, []int{0}, "")
	m.applyChoice(manifest.Candidate{TMDB: 1, TVDB: 2})
	assert.Equal(t, 1, r.Entries[0].ChosenTMDB)
	assert.Equal(t, 2, r.Entries[0].ChosenTVDB)
}

func TestPendingNil(t *testing.T) {
	t.Parallel()
	assert.Empty(t, pending(nil))
}
