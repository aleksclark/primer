package app

import (
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestSessionOpenedTerminalAndTypingBranches(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 100, 40

	// PTY terminal session opened
	next, cmd := m.Update(sessionOpenedMsg{
		snap: engine.SessionSnapshot{
			Kind: contracts.KindTerminal, HasTerminal: true,
			ActivityTitle: "Shell", TerminalScreen: "$ ",
			ClientSessionID: "cs-pty",
		},
	})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())
	assert.False(t, m.focusCmd)
	assert.Contains(t, m.activityMsg, "PTY")
	require.NotNil(t, cmd)

	// Typing session opened
	next, cmd = m.Update(sessionOpenedMsg{
		snap: engine.SessionSnapshot{
			Kind: contracts.KindTyping, ActivityTitle: "Type",
			Typing: &engine.TypingSnapshot{PromptText: "ab", TotalPrompts: 1},
		},
	})
	m = next.(Model)
	assert.Equal(t, contracts.KindTyping, m.snap.Kind)
	assert.Contains(t, m.activityMsg, "Type the prompt")
	assert.Nil(t, cmd)

	// Broker mode stamps brokerSessionID and clears direct sess
	m.opts.Broker = &broker.Client{}
	fakeSess := &engine.Session{}
	next, _ = m.Update(sessionOpenedMsg{
		sess: fakeSess,
		snap: engine.SessionSnapshot{
			Kind: contracts.KindTerminal, ClientSessionID: "broker-cs", HasTerminal: false,
		},
	})
	m = next.(Model)
	assert.Equal(t, "broker-cs", m.brokerSessionID)
	assert.Nil(t, m.sess)
}

func TestBootErrorAndPairingError(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	next, cmd := m.Update(bootMsg{err: assert.AnError})
	m = next.(Model)
	assert.Equal(t, "pairing", m.ScreenName())
	assert.Contains(t, m.err, assert.AnError.Error())
	require.NotNil(t, cmd)

	next, _ = m.Update(pairedMsg{err: assert.AnError})
	m = next.(Model)
	assert.False(t, m.pairing)
	assert.Contains(t, m.err, assert.AnError.Error())
}

func TestTerminalPTYKeyForwardAndEscLeave(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 120, 40
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "cs"
	m.snap = engine.SessionSnapshot{
		Kind: contracts.KindTerminal, HasTerminal: true,
		ClientSessionID: "cs", RequiredPassed: false,
	}
	m.activePane = paneTerminal

	// Printable key schedules term write
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	require.NotNil(t, cmd)
	_ = next

	// Esc while on terminal pane does not leave
	m = next.(Model)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())

	// Esc from instructions leaves
	m.activePane = paneInstructions
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	require.NotNil(t, cmd)
}

func TestTypingKeysRuneBackspaceComplete(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 90, 30
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "ts"
	typingSnap := func(passed bool) engine.SessionSnapshot {
		return engine.SessionSnapshot{
			Kind: contracts.KindTyping, ClientSessionID: "ts", RequiredPassed: passed,
			Typing: &engine.TypingSnapshot{PromptText: "hi", Input: "h"},
		}
	}
	m.snap = typingSnap(false)

	// Rune/backspace apply synchronously via broker (error path ok) — no Cmd returned.
	// Broker error path replaces snap with the empty response; restore typing kind after.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	assert.Nil(t, cmd)
	m = next.(Model)
	assert.NotEmpty(t, m.activityMsg) // broker TypeRune fails without live session
	m.snap = typingSnap(false)

	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Nil(t, cmd)
	m = next.(Model)
	assert.NotEmpty(t, m.activityMsg) // broker TypeBackspace fails without live session
	m.snap = typingSnap(true)

	// Complete when ready returns completeCmd (ctrl+s on typing activity).
	m.busy = false
	m.brokerSessionID = "ts"
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// enter completes when thresholds met (typing-specific path).
	m.busy = false
	m.snap = typingSnap(true)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// enter is a no-op when thresholds are not met.
	m.busy = false
	m.snap = typingSnap(false)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.False(t, m.busy)
	assert.Nil(t, cmd)
}

func TestActivityFallbackCommandCompleteAndHint(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "cs"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: false, RequiredPassed: true}
	m.focusCmd = true

	m.cmdInput.SetValue("complete")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// Instructions pane cmd box when HasTerminal
	m.busy = false
	m.snap.HasTerminal = true
	m.activePane = paneInstructions
	m.focusCmd = true
	m.cmdInput.SetValue("pwd")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	// Non-key msg delegated to cmd input
	m = next.(Model)
	m.busy = false
	m.snap.HasTerminal = false
	m.focusCmd = true
	next, _ = m.Update(tea.MouseClickMsg{})
	_ = next
}

func TestTermTickSkippedWithoutSession(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	m.snap.HasTerminal = true
	// no session
	next, cmd := m.Update(termTickMsg{})
	assert.Nil(t, cmd)
	// poll when not on activity
	m.screen = ScreenQueue
	next, cmd = m.Update(termPollMsg{screen: "x"})
	assert.Nil(t, cmd)
	_ = next
}

func TestSyncDoneOutsideQueue(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	next, cmd := m.Update(syncDoneMsg{res: sync.Result{Status: sync.StatusOnline, WorkItems: 3}})
	m = next.(Model)
	assert.Equal(t, sync.StatusOnline, m.syncSt)
	assert.Nil(t, cmd)
}

func TestCmdDoneWithTerminalScreen(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	m.snap.HasTerminal = true
	m.activePane = paneTerminal
	next, cmd := m.Update(cmdDoneMsg{
		snap: engine.SessionSnapshot{HasTerminal: true, TerminalScreen: "updated"},
	})
	m = next.(Model)
	assert.Equal(t, "updated", m.termScreen)
	assert.Nil(t, cmd)
}

func TestLayoutAndViewAllScreens(t *testing.T) {
	t.Parallel()
	store, err := cache.Open(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	m := NewModel(Options{Store: store, Offline: true})
	m.width, m.height = 80, 24

	for _, sc := range []Screen{ScreenBoot, ScreenPairing, ScreenQueue, ScreenActivity, ScreenSummary} {
		m.screen = sc
		switch sc {
		case ScreenActivity:
			m.snap = engine.SessionSnapshot{
				Kind: contracts.KindTerminal, ActivityTitle: "A",
				Tasks:  []contracts.Task{{Title: "T", Instructions: "Do"}},
				Checks: []engine.CheckStatus{{ID: "c", Passed: true}},
			}
		case ScreenSummary:
			m.summaryTitle = "Done"
			m.summaryBody = "body"
		case ScreenQueue:
			m.workList.SetItems([]list.Item{workItem{it: studentapi.WorkItem{
				Activity:   domain.LearningActivity{Title: "Q", Slug: "q"},
				Assignment: domain.StudentAssignment{State: "available", UpdatedAt: time.Now().UTC()},
			}}})
		}
		view := m.View()
		assert.NotNil(t, view)
	}

	// Init returns boot cmd
	cmd := m.Init()
	require.NotNil(t, cmd)
}

func TestPauseToQueueDirectMode(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	m := NewModel(Options{Store: store, Offline: true})
	m.screen = ScreenActivity
	// Direct mode without real session — pause still clears and returns queue.
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'g'})
	// Without session/hasSession false paths on activity terminal handler:
	m.screen = ScreenActivity
	m.snap.Kind = contracts.KindTerminal
	m.snap.HasTerminal = false
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'g'})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	require.NotNil(t, cmd)
	_ = next
}

func TestVerifyAndCompleteBlockedWhenBusy(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "cs"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: true, RequiredPassed: true}
	m.busy = true
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'v'})
	assert.Nil(t, cmd)
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	assert.Nil(t, cmd)
	_ = next
}
