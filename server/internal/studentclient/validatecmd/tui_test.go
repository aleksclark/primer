package validatecmd

import (
	"path/filepath"
	"runtime"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityItemRendering(t *testing.T) {
	t.Parallel()
	ok := activityItem{result: Result{OK: true, Title: "Nav", Slug: "basic-navigation", Tasks: 2, Checks: 3, Fixtures: 1}}
	assert.Contains(t, ok.Title(), "PASS")
	assert.Contains(t, ok.Title(), "Nav")
	assert.Contains(t, ok.Description(), "2 tasks")
	assert.Contains(t, ok.FilterValue(), "basic-navigation")

	fail := activityItem{result: Result{OK: false, Slug: "broken", Error: "bad yaml"}}
	assert.Contains(t, fail.Title(), "FAIL")
	assert.Contains(t, fail.Title(), "broken") // title falls back to slug
	assert.Equal(t, "bad yaml", fail.Description())
	var _ list.Item = fail
}

func TestTUIModelUpdateAndView(t *testing.T) {
	t.Parallel()
	results := []Result{
		{OK: true, Title: "A", Slug: "a", Tasks: 1, Checks: 1},
		{OK: false, Title: "B", Slug: "b", Error: "nope"},
	}
	items := []list.Item{activityItem{result: results[0]}, activityItem{result: results[1]}}
	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	m := tuiModel{list: l, results: results}

	assert.Nil(t, m.Init())
	view := m.View().Content
	assert.Contains(t, view, "Primer activity-validate")
	assert.Contains(t, view, "1/2 passed")
	assert.Contains(t, view, "q quit")

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(tuiModel)
	assert.Equal(t, 100, m.width)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	m = next.(tuiModel)
	assert.True(t, m.quitting)
	require.NotNil(t, cmd)
	assert.Equal(t, "", m.View().Content)

	assert.Equal(t, 1, failCount(results))
	assert.Equal(t, 0, failCount([]Result{{OK: true}}))
}

func TestDefaultActivitiesDirAndRun(t *testing.T) {
	// Resolve from repo root so DefaultActivitiesDir can find curriculum/activities.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	t.Chdir(root)

	dir := DefaultActivitiesDir()
	require.DirExists(t, dir)

	results, err := Run(Options{ActivitiesDir: dir, Materialize: false})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	// At least one known activity should validate.
	found := false
	for _, r := range results {
		if r.Slug == "basic-navigation" {
			found = true
			assert.True(t, r.OK, r.Error)
		}
	}
	assert.True(t, found)
}
