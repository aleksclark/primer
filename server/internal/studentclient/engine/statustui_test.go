package engine

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusModelRendersPhasesAndQuit(t *testing.T) {
	t.Parallel()
	m := NewStatusModel("", Status{
		Phase:            "activity",
		Sync:             "online",
		WorkDownloaded:   3,
		ActivitySlug:     "basic-navigation",
		AssignmentID:     "1234567890abcdef",
		CommandsRun:      2,
		ChecksPassed:     1,
		ChecksTotal:      2,
		RequiredPassed:   false,
		CompletionQueued: true,
		Offline:          true,
		Message:          "keep going",
		LastError:        "boom",
	})
	assert.Equal(t, "Primer student harness", m.title)
	assert.Nil(t, m.Init())

	view := m.View().Content
	assert.Contains(t, view, "Primer student harness")
	assert.Contains(t, view, "Phase:")
	assert.Contains(t, view, "basic-navigation")
	assert.Contains(t, view, "12345678…")
	assert.Contains(t, view, "Required checks: incomplete")
	assert.Contains(t, view, "Completion: awaiting sync")
	assert.Contains(t, view, "Mode: offline")
	assert.Contains(t, view, "keep going")
	assert.Contains(t, view, "Error: boom")

	// Required pass + completion acked branch.
	m.SetStatus(Status{
		Phase:           "done",
		ChecksTotal:     1,
		ChecksPassed:    1,
		RequiredPassed:  true,
		CompletionAcked: true,
	})
	view = m.View().Content
	assert.Contains(t, view, "Required checks: PASS")
	assert.Contains(t, view, "Completion: synced")

	// No checks branch.
	m.SetStatus(Status{ChecksTotal: 0})
	assert.Contains(t, m.View().Content, "Required checks: —")
	assert.Contains(t, m.View().Content, "Completion: —")

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	nm := next.(StatusModel)
	assert.True(t, nm.quitting)
	require.NotNil(t, cmd)
	assert.Equal(t, "", nm.View().Content)

	// Status message update path.
	m2 := NewStatusModel("Custom", Status{})
	next, _ = m2.Update(Status{Phase: "queue", WorkDownloaded: 9})
	m2 = next.(StatusModel)
	assert.Equal(t, "queue", m2.status.Phase)
	assert.Equal(t, 9, m2.status.WorkDownloaded)
	assert.Contains(t, m2.View().Content, "Custom")
}

func TestShortID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short", shortID("short"))
	assert.Equal(t, "12345678…", shortID("1234567890"))
}
