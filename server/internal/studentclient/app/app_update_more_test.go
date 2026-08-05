package app

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
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

func TestUpdateNonKeyDelegatesAndWindowResize(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 80, 24

	// Pairing screen delegates non-key msgs to pair input.
	m.screen = ScreenPairing
	next, _ := m.Update(tea.MouseClickMsg{})
	m = next.(Model)

	// Queue screen delegates to work list.
	m.screen = ScreenQueue
	m.workList.SetItems([]list.Item{workItem{it: studentapi.WorkItem{
		Activity:   domain.LearningActivity{Title: "Q", Slug: "q"},
		Assignment: domain.StudentAssignment{State: "available", UpdatedAt: time.Now().UTC()},
	}}})
	next, _ = m.Update(tea.MouseClickMsg{})
	m = next.(Model)

	// Activity + typing ignores non-key.
	m.screen = ScreenActivity
	m.snap.Kind = contracts.KindTyping
	next, cmd := m.Update(tea.MouseClickMsg{})
	assert.Nil(t, cmd)
	m = next.(Model)

	// Activity fallback cmd box gets non-key when focused.
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: false}
	m.focusCmd = true
	m.busy = false
	next, _ = m.Update(tea.MouseClickMsg{})
	m = next.(Model)

	// Activity instructions pane with terminal + focusCmd.
	m.snap.HasTerminal = true
	m.activePane = paneInstructions
	m.focusCmd = true
	next, _ = m.Update(tea.MouseClickMsg{})
	m = next.(Model)

	// Window resize on activity with terminal schedules resize.
	m.screen = ScreenActivity
	m.snap.HasTerminal = true
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "cs"
	next, cmd = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	assert.Equal(t, 120, m.width)
	require.NotNil(t, cmd)
}

func TestUpdateMessageBranchesCoverage(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 100, 40

	// workLoaded error
	next, _ := m.Update(workLoadedMsg{err: assert.AnError})
	m = next.(Model)
	assert.Contains(t, m.err, assert.AnError.Error())

	// workLoaded success
	next, _ = m.Update(workLoadedMsg{items: []studentapi.WorkItem{{
		Activity:   domain.LearningActivity{Title: "A", Slug: "a"},
		Assignment: domain.StudentAssignment{ID: "asg", State: "available", UpdatedAt: time.Now().UTC()},
	}}})
	m = next.(Model)
	assert.Empty(t, m.err)
	assert.Len(t, m.work, 1)

	// sessionOpened error returns to queue
	m.busy = true
	next, _ = m.Update(sessionOpenedMsg{err: assert.AnError})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	assert.False(t, m.busy)

	// sessionOpened fallback (no PTY)
	next, cmd := m.Update(sessionOpenedMsg{
		snap: engine.SessionSnapshot{
			Kind: contracts.KindTerminal, HasTerminal: false, ActivityTitle: "NoPTY",
		},
	})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())
	assert.True(t, m.focusCmd)
	require.NotNil(t, cmd)

	// cmdDone error + focus path
	m.snap.HasTerminal = false
	m.activePane = paneInstructions
	next, _ = m.Update(cmdDoneMsg{err: assert.AnError, snap: engine.SessionSnapshot{HasTerminal: false}})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, assert.AnError.Error())

	// verifyDone
	next, _ = m.Update(verifyDoneMsg{snap: engine.SessionSnapshot{
		ChecksPassed: 1, ChecksTotal: 2, RequiredPassed: false, TerminalScreen: "v",
	}})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "Checks:")
	assert.Equal(t, "v", m.termScreen)

	// completeDone error stays on activity
	m.screen = ScreenActivity
	next, _ = m.Update(completeDoneMsg{err: assert.AnError, snap: engine.SessionSnapshot{}})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())
	assert.Contains(t, m.activityMsg, assert.AnError.Error())

	// completeDone success branches: acked / queued / generic + message
	next, _ = m.Update(completeDoneMsg{snap: engine.SessionSnapshot{
		ActivityTitle: "DoneA", ChecksPassed: 2, ChecksTotal: 2, CompletionAcked: true, Message: "nice",
	}})
	m = next.(Model)
	assert.Equal(t, "summary", m.ScreenName())
	assert.Contains(t, m.summaryBody, "accepted and synced")
	assert.Contains(t, m.summaryBody, "nice")

	m.screen = ScreenActivity
	next, _ = m.Update(completeDoneMsg{snap: engine.SessionSnapshot{
		ActivityTitle: "DoneQ", CompletionQueued: true,
	}})
	m = next.(Model)
	assert.Contains(t, m.summaryBody, "awaiting sync")

	m.screen = ScreenActivity
	next, _ = m.Update(completeDoneMsg{snap: engine.SessionSnapshot{ActivityTitle: "DoneG"}})
	m = next.(Model)
	assert.Contains(t, m.summaryBody, "completed")

	// hintDone broker mode with long chat log truncation
	m = NewModel(Options{Offline: true, Broker: &broker.Client{}})
	m.chatLog = make([]string, 25)
	for i := range m.chatLog {
		m.chatLog[i] = "old"
	}
	next, _ = m.Update(hintDoneMsg{hint: "fresh tip"})
	m = next.(Model)
	assert.Equal(t, "fresh tip", m.snap.TutorHint)
	assert.LessOrEqual(t, len(m.chatLog), 20)
	assert.Contains(t, m.activityMsg, "Hint")

	// empty hint
	next, _ = m.Update(hintDoneMsg{})
	m = next.(Model)

	// termTick with session + terminal
	m.screen = ScreenActivity
	m.brokerSessionID = "cs"
	m.snap.HasTerminal = true
	next, cmd = m.Update(termTickMsg{})
	require.NotNil(t, cmd)

	// termPoll success preserves tutor hint
	m.snap.TutorHint = "keep"
	next, cmd = m.Update(termPollMsg{
		screen: "from-poll",
		snap:   engine.SessionSnapshot{HasTerminal: true, TerminalScreen: "snap-term", TutorHint: ""},
	})
	m = next.(Model)
	assert.Equal(t, "keep", m.snap.TutorHint)
	assert.Equal(t, "snap-term", m.termScreen)
	require.NotNil(t, cmd)

	// termPoll with error still schedules tick
	next, cmd = m.Update(termPollMsg{err: assert.AnError})
	require.NotNil(t, cmd)
	_ = next

	// sync error status path on queue
	m.screen = ScreenQueue
	next, cmd = m.Update(syncDoneMsg{res: sync.Result{Status: sync.StatusOnline, Err: assert.AnError}})
	m = next.(Model)
	assert.Contains(t, m.status, "sync:")
	require.NotNil(t, cmd)
}

func TestPairingQueueSummaryAndTypingKeyBranches(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	m := NewModel(Options{Store: store, Offline: true})
	m.width, m.height = 90, 30

	// Pairing enter while already pairing is no-op.
	m.screen = ScreenPairing
	m.pairing = true
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
	m = next.(Model)

	// Pairing enter with code starts pair.
	m.pairing = false
	m.pairInput.SetValue("ABCD")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.pairing)
	require.NotNil(t, cmd)

	// Queue enter with selection opens session.
	m.screen = ScreenQueue
	m.busy = false
	m.workList.SetItems([]list.Item{workItem{it: studentapi.WorkItem{
		Assignment: domain.StudentAssignment{ID: "asg1", State: "available"},
		Activity:   domain.LearningActivity{Title: "OpenMe", Slug: "open-me"},
	}}})
	m.workList.Select(0)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// Summary keys return to queue.
	m.screen = ScreenSummary
	m.summaryBody = "x"
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	require.NotNil(t, cmd)

	// Typing: ctrl+g pause, ctrl+h hint, ctrl+v verify, thresholds message, space/tab runes.
	m = NewModel(Options{Offline: true, Broker: &broker.Client{}})
	m.screen = ScreenActivity
	m.brokerSessionID = "ts"
	m.snap = engine.SessionSnapshot{
		Kind: contracts.KindTyping, ClientSessionID: "ts",
		Typing: &engine.TypingSnapshot{PromptText: "ab", Input: ""},
	}
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'g'})
	// pause may move to queue
	m = next.(Model)
	_ = cmd

	m.screen = ScreenActivity
	m.brokerSessionID = "ts"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTyping, ClientSessionID: "ts", Typing: &engine.TypingSnapshot{PromptText: "ab"}}
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'h'})
	require.NotNil(t, cmd)
	m = next.(Model)

	m.busy = false
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'v'})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// ctrl+s without thresholds
	m.busy = false
	m.snap.RequiredPassed = false
	m.snap.Kind = contracts.KindTyping
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.Contains(t, m.activityMsg, "thresholds")

	// space / tab typing keys (broker error path ok)
	m.busy = false
	m.brokerSessionID = "ts"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTyping, ClientSessionID: "ts", Typing: &engine.TypingSnapshot{PromptText: "x"}}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.Nil(t, cmd)
	m = next.(Model)
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTyping, ClientSessionID: "ts", Typing: &engine.TypingSnapshot{PromptText: "x"}}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Nil(t, cmd)
	_ = next
}

func TestTerminalActivityKeyBranches(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true, Broker: &broker.Client{}})
	m.width, m.height = 100, 40
	m.screen = ScreenActivity
	m.brokerSessionID = "cs"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: true, RequiredPassed: false}
	m.activePane = paneTerminal

	// tab cycles panes
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	assert.NotEqual(t, paneTerminal, m.activePane)

	// ctrl+v verify
	m.busy = false
	m.snap.HasTerminal = true
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'v'})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// ctrl+s blocked when required not passed
	m.busy = false
	m.snap.RequiredPassed = false
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.Contains(t, m.activityMsg, "Required checks")

	// ctrl+s when passed
	m.busy = false
	m.snap.RequiredPassed = true
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// ctrl+h hint
	m.busy = false
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'h'})
	require.NotNil(t, cmd)

	// fallback commands on non-pty terminal activity
	m.snap.HasTerminal = false
	m.focusCmd = true
	m.busy = false
	for _, line := range []string{"verify", "hint", "complete", "back", "pwd"} {
		m.busy = false
		m.screen = ScreenActivity
		m.brokerSessionID = "cs"
		m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: false, RequiredPassed: true}
		m.cmdInput.SetValue(line)
		next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(Model)
		if line == "complete" || line == "verify" || line == "pwd" {
			// these set busy / return cmd when session present
			_ = cmd
		}
		if line == "back" {
			assert.Equal(t, "queue", m.ScreenName())
		}
	}

	// complete blocked when required not passed
	m.screen = ScreenActivity
	m.brokerSessionID = "cs"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: false, RequiredPassed: false}
	m.busy = false
	m.cmdInput.SetValue("complete")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "Required checks")
}
