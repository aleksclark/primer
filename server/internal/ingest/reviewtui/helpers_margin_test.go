package reviewtui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

func TestMaxAndEntryAndInit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5, max(3, 5))
	assert.Equal(t, 3, max(3, 1))
	assert.Equal(t, 0, max(0, 0))

	rev := &manifest.Review{
		Entries: []manifest.ReviewEntry{
			{ID: "a", Title: "A", Kind: manifest.KindMovie, Candidates: []manifest.Candidate{{Title: "A", TMDB: 1}}},
			{ID: "b", Title: "B", Kind: manifest.KindSeries},
		},
	}
	m := newModel(rev, []int{0, 1}, "/tmp/review.json")
	assert.NotNil(t, m.entry())
	assert.Equal(t, "A", m.entry().Title)

	m2 := newModel(rev, []int{}, "/tmp/x")
	assert.Nil(t, m2.entry())
	m3 := newModel(rev, []int{0}, "/tmp/x")
	m3.pos = 99
	assert.Nil(t, m3.entry())

	assert.Nil(t, m.Init())

	// Window resize path
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	mm := next.(model)
	assert.Equal(t, 100, mm.width)
	assert.Equal(t, 40, mm.height)

	// View renders without panic
	v := mm.View()
	assert.NotEmpty(t, v)
}
