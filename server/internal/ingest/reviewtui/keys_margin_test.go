package reviewtui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

func TestKeyNavSkipSelectAdvance(t *testing.T) {
	t.Parallel()
	longOV := "overview " + strings.Repeat("x", 150)
	rev := &manifest.Review{Entries: []manifest.ReviewEntry{
		{ID: "a", Title: "A", Kind: manifest.KindMovie, Candidates: []manifest.Candidate{{Title: "A1", TMDB: 11, Year: 1999, Overview: longOV}}},
		{ID: "b", Title: "B", Kind: manifest.KindSeries, Candidates: []manifest.Candidate{{Title: "B1", TVDB: 22}}},
		{ID: "c", Title: "C", Kind: manifest.KindMovie},
	}}
	m := newModel(rev, []int{0, 1, 2}, "/tmp/r.yaml")
	m.width, m.height = 80, 30

	item := candidateItem{cand: rev.Entries[0].Candidates[0], kind: manifest.KindMovie, chosen: true}
	assert.Contains(t, item.Title(), "★")
	assert.Contains(t, item.Description(), "…")
	assert.Equal(t, "A1", item.FilterValue())
	item2 := candidateItem{cand: manifest.Candidate{Title: "X"}, chosen: false}
	assert.Contains(t, item2.Title(), "X")
	assert.Contains(t, item2.Description(), "no provider")

	// skip current
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	mm := next.(model)
	assert.Equal(t, 1, mm.skipped)
	assert.Equal(t, 1, mm.pos)

	// prev back
	next, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	mm = next.(model)
	assert.Equal(t, 0, mm.pos)

	// next forward
	next, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm = next.(model)
	assert.Equal(t, 1, mm.pos)

	// select candidate on entry 0
	mm.pos = 0
	mm.list = mm.buildList()
	mm.list.Select(0)
	next, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = next.(model)
	assert.Equal(t, 1, mm.chosen)
	assert.Equal(t, 11, rev.Entries[0].ChosenTMDB)
	assert.Equal(t, 1, mm.pos)
	_ = cmd

	// advance to last and finish via skip
	mm.pos = 2
	mm.list = mm.buildList()
	next, cmd = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	mm = next.(model)
	assert.True(t, mm.done)
	require.NotNil(t, cmd)

	// applyChoice nil entry no-op
	empty := newModel(rev, []int{}, "/tmp/x")
	empty.applyChoice(manifest.Candidate{TMDB: 1})

	// series choice sets TVDB
	m2 := newModel(rev, []int{1}, "/tmp/x")
	m2.applyChoice(manifest.Candidate{TVDB: 99, TMDB: 1})
	assert.Equal(t, 99, rev.Entries[1].ChosenTVDB)

	// empty pending Run returns immediately
	res, err := Run(&manifest.Review{Entries: []manifest.ReviewEntry{
		{ID: "done", ChosenTMDB: 1},
	}}, "/tmp/r")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.Chosen)

	res, err = Run(nil, "/tmp/r")
	require.NoError(t, err)
	require.NotNil(t, res)
}
