package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestAppKeyPathsPairingQueueSummary(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenPairing

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Contains(t, m.err, "pairing code")

	m.pairInput.SetValue("ABCD")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.pairing)
	require.NotNil(t, cmd)

	m2 := NewModel(Options{})
	m2.screen = ScreenPairing
	next, cmd = m2.Update(tea.KeyPressMsg{Code: 'q'})
	m2 = next.(Model)
	assert.True(t, m2.quitting)
	require.NotNil(t, cmd)

	m3 := NewModel(Options{Broker: &broker.Client{}})
	m3.screen = ScreenQueue
	next, cmd = m3.Update(tea.KeyPressMsg{Code: 'q'})
	assert.True(t, next.(Model).quitting)
	require.NotNil(t, cmd)

	m3.screen = ScreenQueue
	m3.quitting = false
	next, cmd = m3.Update(tea.KeyPressMsg{Code: 'r'})
	require.NotNil(t, cmd)
	_ = next

	m4 := NewModel(Options{Broker: &broker.Client{}})
	m4.screen = ScreenSummary
	m4.brokerSessionID = "sid"
	m4.summaryTitle = "done"
	next, cmd = m4.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm := next.(Model)
	assert.Equal(t, "queue", nm.ScreenName())
	require.NotNil(t, cmd)
}

func TestAppHintTermPollSyncWorkMessages(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Broker: &broker.Client{}})
	m.screen = ScreenActivity
	m.brokerSessionID = "s1"
	m.snap.HasTerminal = true
	m.snap.Kind = contracts.KindTerminal

	next, _ := m.Update(hintDoneMsg{hint: "try ls"})
	m = next.(Model)
	assert.Equal(t, "try ls", m.snap.TutorHint)
	require.NotEmpty(t, m.chatLog)

	for i := 0; i < 25; i++ {
		next, _ = m.Update(hintDoneMsg{hint: "h"})
		m = next.(Model)
	}
	assert.LessOrEqual(t, len(m.chatLog), 20)

	// Direct-session branch with nil sess is a no-op for SetTutorHint.
	m2 := NewModel(Options{})
	m2.screen = ScreenActivity
	next, _ = m2.Update(hintDoneMsg{hint: "x"})
	_ = next

	m3 := NewModel(Options{Broker: &broker.Client{}})
	m3.screen = ScreenActivity
	m3.brokerSessionID = "s"
	m3.snap.HasTerminal = true
	next, cmd := m3.Update(termTickMsg{})
	require.NotNil(t, cmd)
	m3 = next.(Model)
	next, cmd = m3.Update(termPollMsg{
		screen: "prompt$",
		snap: engine.SessionSnapshot{
			HasTerminal:    true,
			TerminalScreen: "prompt$",
			Kind:           contracts.KindTerminal,
		},
	})
	m3 = next.(Model)
	assert.Equal(t, "prompt$", m3.termScreen)
	require.NotNil(t, cmd)

	// completeDoneMsg → summary
	m4 := NewModel(Options{Broker: &broker.Client{}})
	m4.screen = ScreenActivity
	m4.brokerSessionID = "s"
	m4.snap = engine.SessionSnapshot{ActivityTitle: "Nav", ActivitySlug: "basic-navigation"}
	next, _ = m4.Update(completeDoneMsg{
		snap: engine.SessionSnapshot{
			ActivityTitle:    "Nav",
			CompletionQueued: true,
			CompletionAcked:  true,
			RequiredPassed:   true,
		},
	})
	m4 = next.(Model)
	assert.Equal(t, "summary", m4.ScreenName())

	// sync + work loaded
	m5 := NewModel(Options{Broker: &broker.Client{}})
	m5.screen = ScreenQueue
	next, _ = m5.Update(syncDoneMsg{res: sync.Result{Status: sync.StatusOnline, WorkItems: 2}})
	m5 = next.(Model)
	assert.Equal(t, sync.StatusOnline, m5.syncSt)

	next, _ = m5.Update(workLoadedMsg{items: []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: "a1", State: "available"},
		Activity:   domain.LearningActivity{Slug: "basic-navigation", Title: "Nav"},
	}}})
	m5 = next.(Model)
	assert.NotEmpty(t, m5.workList.Items())

	next, _ = m5.Update(workLoadedMsg{err: assertErr("load failed")})
	m5 = next.(Model)
	assert.Contains(t, m5.err, "load failed")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
