package app

import (
	"strings"
	"testing"
	"time"

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

func TestViewTypingAndTerminalBranches(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 100, 40
	m.screen = ScreenActivity
	m.syncSt = sync.StatusOnline

	// Typing view: tasks, required pass, tutor hint, metrics, checks, busy, activityMsg, done prompt
	m.snap = engine.SessionSnapshot{
		Kind: contracts.KindTyping, ActivityTitle: "Type", Instructions: "base",
		Tasks: []contracts.Task{{Title: "Task", Instructions: "Do it"}}, CurrentTaskIdx: 0,
		ChecksPassed: 1, ChecksTotal: 1, RequiredPassed: true,
		TutorHint: "slow down",
		Typing: &engine.TypingSnapshot{
			PromptText: "hi", Input: "h", WPM: 20, Accuracy: 0.9,
			TotalPrompts: 2, RemainingPrompts: 1, IncorrectChars: 2, Done: false,
		},
		Checks: []engine.CheckStatus{
			{ID: "pass", Passed: true},
			{ID: "fail", Passed: false, Optional: false},
			{ID: "opt", Passed: false, Optional: true},
		},
	}
	m.busy = true
	m.activityMsg = "working"
	view := m.View().Content
	assert.Contains(t, view, "Type")
	assert.Contains(t, view, "REQUIRED PASS")
	assert.Contains(t, view, "Hint:")
	assert.Contains(t, view, "Miss")
	assert.Contains(t, view, "Working")

	// Typing complete branch (no current task)
	m.busy = false
	m.activityMsg = ""
	m.snap.CurrentTaskIdx = 99
	m.snap.RequiredPassed = true
	m.snap.Typing.Done = true
	view = m.View().Content
	assert.Contains(t, view, "All prompts complete")
	assert.Contains(t, view, "all prompts complete")

	// Typing incomplete metrics
	m.snap.RequiredPassed = false
	m.snap.Typing = &engine.TypingSnapshot{PromptText: "x", TotalPrompts: 1, RemainingPrompts: 1}
	view = m.View().Content
	assert.Contains(t, view, "incomplete")

	// Terminal view without PTY: required pass + tutor
	m.snap = engine.SessionSnapshot{
		Kind: contracts.KindTerminal, HasTerminal: false, ActivityTitle: "Term",
		Instructions: "instr", RequiredPassed: true, ChecksPassed: 1, ChecksTotal: 1,
		TutorHint:      "tip",
		Checks:         []engine.CheckStatus{{ID: "c", Passed: true}},
		CurrentTaskIdx: 99,
	}
	m.focusCmd = true
	view = m.View().Content
	assert.Contains(t, view, "All tasks complete")
	assert.Contains(t, view, "Hint:")

	// Terminal with PTY panes + long screen truncation + empty screen
	m.snap.HasTerminal = true
	m.snap.CurrentTaskIdx = 0
	m.snap.Tasks = []contracts.Task{{Title: "T1", Instructions: "Go"}}
	m.snap.RequiredPassed = false
	m.termScreen = ""
	m.activePane = paneTerminal
	view = m.View().Content
	assert.Contains(t, view, "starting shell")

	// long screen
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line " + strings.Repeat("x", 10)
	}
	m.termScreen = strings.Join(lines, "\n")
	m.activePane = paneInstructions
	m.chatLog = []string{"a", "b", "c"}
	view = m.View().Content
	assert.Contains(t, view, "Instructions (active)")

	m.activePane = paneChat
	view = m.View().Content
	assert.Contains(t, view, "Tutor (active)")

	// Pairing/queue views with errors and empty work
	m.screen = ScreenPairing
	m.err = "bad code"
	view = m.View().Content
	assert.Contains(t, view, "bad code")

	m.screen = ScreenQueue
	m.err = "load failed"
	m.workList.SetItems(nil)
	view = m.View().Content
	assert.Contains(t, view, "load failed")

	m.err = ""
	m.status = "ok"
	view = m.View().Content
	assert.NotEmpty(t, view)
}

func TestBootBrokerFallbackAndPairingKeys(t *testing.T) {
	t.Parallel()
	// Broker boot with failing profile falls back to health error path.
	m := NewModel(Options{Broker: &broker.Client{}, Offline: true})
	msg := m.boot()
	// no live broker → error boot
	bm := msg.(bootMsg)
	assert.Error(t, bm.err)

	// ctrl+c quits
	m2 := NewModel(Options{Offline: true})
	next, cmd := m2.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'c'})
	assert.True(t, next.(Model).quitting)
	require.NotNil(t, cmd)

	// pairing q with empty input quits
	m3 := NewModel(Options{Offline: true})
	m3.screen = ScreenPairing
	next, cmd = m3.Update(tea.KeyPressMsg{Code: 'q'})
	assert.True(t, next.(Model).quitting)
	require.NotNil(t, cmd)

	// queue r refreshes
	m4 := NewModel(Options{Offline: true, Store: openStore(t)})
	m4.screen = ScreenQueue
	next, cmd = m4.Update(tea.KeyPressMsg{Code: 'r'})
	require.NotNil(t, cmd)
	_ = next

	// queue enter without selection
	m4.workList.SetItems(nil)
	next, cmd = m4.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)

	// summary esc
	m4.screen = ScreenSummary
	next, cmd = m4.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, "queue", next.(Model).ScreenName())
	require.NotNil(t, cmd)
}

func TestOpenSessionCmdAndWorkItemMethods(t *testing.T) {
	t.Parallel()
	// workItem methods
	wi := workItem{it: studentapi.WorkItem{
		Activity:   domain.LearningActivity{Title: "Hello", Slug: "hello"},
		Assignment: domain.StudentAssignment{State: "available", UpdatedAt: time.Now().UTC()},
	}}
	assert.Contains(t, wi.Title(), "Hello")
	assert.NotEmpty(t, wi.Description())
	assert.Contains(t, wi.FilterValue(), "hello")

	// openSessionCmd broker missing session error
	m := NewModel(Options{Broker: &broker.Client{}, Offline: true})
	msg := m.openSessionCmd("asg")()
	som := msg.(sessionOpenedMsg)
	assert.Error(t, som.err)

	// termTickCmd returns tick msg
	m.screen = ScreenActivity
	m.snap.HasTerminal = true
	m.brokerSessionID = "x"
	cmd := m.termTickCmd()
	require.NotNil(t, cmd)
	_ = cmd()
}
