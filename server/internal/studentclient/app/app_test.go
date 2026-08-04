package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewModelStartsBoot(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, "boot", m.ScreenName())
	assert.NotNil(t, m.Init())
}

func TestBootWithoutStoreShowsPairingErrorPath(t *testing.T) {
	m := NewModel(Options{})
	// boot cmd reports missing store
	msg := m.boot()
	bm, ok := msg.(bootMsg)
	require.True(t, ok)
	require.Error(t, bm.err)

	next, _ := m.Update(bm)
	nm := next.(Model)
	assert.Equal(t, "pairing", nm.ScreenName())
	assert.Contains(t, nm.View(), "pair device")
}

func TestWindowSizeDoesNotPanic(t *testing.T) {
	m := NewModel(Options{})
	m.screen = ScreenQueue
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	nm := next.(Model)
	assert.Equal(t, 100, nm.width)
	assert.Equal(t, 40, nm.height)
	_ = nm.View()

	next, _ = nm.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	nm = next.(Model)
	_ = nm.View()
}

func TestSummaryThenQuitKeys(t *testing.T) {
	m := NewModel(Options{})
	m.screen = ScreenSummary
	m.summaryTitle = "Activity complete"
	m.summaryBody = "Status:   accepted and synced\n"
	view := m.View()
	assert.Contains(t, view, "Activity complete")
	assert.Contains(t, view, "accepted and synced")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(Model)
	assert.True(t, nm.quitting)
	require.NotNil(t, cmd)
}

func TestPairingViewChrome(t *testing.T) {
	m := NewModel(Options{})
	m.screen = ScreenPairing
	m.status = "Enter the pairing code from a parent."
	v := m.View()
	assert.Contains(t, v, "pair device")
	assert.Contains(t, v, "pairing code")
}
