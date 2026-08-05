package validatecmd

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunTUIMissingActivitiesDir(t *testing.T) {
	t.Parallel()
	err := RunTUI(Options{ActivitiesDir: filepath.Join(t.TempDir(), "missing")})
	require.Error(t, err)
}

func TestTUIModelProgramQuitsOnQ(t *testing.T) {
	t.Parallel()
	results := []Result{
		{OK: true, Title: "Nav", Slug: "basic-navigation", Tasks: 1, Checks: 1, Fixtures: 0},
		{OK: false, Title: "Broken", Slug: "broken", Error: "bad yaml"},
	}
	items := make([]list.Item, 0, len(results))
	for _, r := range results {
		items = append(items, activityItem{result: r})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	l := list.New(items, delegate, 80, 20)
	l.Title = "Activity validation"
	m := tuiModel{list: l, results: results}

	p := tea.NewProgram(m, tea.WithInput(strings.NewReader("q")), tea.WithOutput(io.Discard))
	final, err := p.Run()
	require.NoError(t, err)
	fm, ok := final.(tuiModel)
	require.True(t, ok)
	assert.True(t, fm.quitting)
	assert.False(t, AllOK(results))
	assert.Equal(t, 1, failCount(results))
}
