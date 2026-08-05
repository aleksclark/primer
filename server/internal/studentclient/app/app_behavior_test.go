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

func openStore(t *testing.T) *cache.Store {
	t.Helper()
	store, err := cache.Open(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestNewModelDefaultsAndScreenNames(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{BaseURL: "http://example.test"})
	assert.Equal(t, "workstation", m.opts.DeviceName)
	assert.NotNil(t, m.opts.Client)
	assert.Equal(t, "boot", m.ScreenName())

	for _, tc := range []struct {
		s    Screen
		name string
	}{
		{ScreenBoot, "boot"},
		{ScreenPairing, "pairing"},
		{ScreenQueue, "queue"},
		{ScreenActivity, "activity"},
		{ScreenSummary, "summary"},
		{Screen(99), "unknown"},
	} {
		m.screen = tc.s
		assert.Equal(t, tc.name, m.ScreenName())
	}
}

func TestBootWithTokenGoesToQueue(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	require.NoError(t, store.SetDeviceToken(t.Context(), "tok-1"))
	m := NewModel(Options{Store: store, Client: studentapi.New("http://127.0.0.1:1", ""), Offline: true})
	msg := m.boot()
	bm := msg.(bootMsg)
	require.NoError(t, bm.err)
	assert.True(t, bm.hasToken)

	next, cmd := m.Update(bm)
	nm := next.(Model)
	assert.Equal(t, "queue", nm.ScreenName())
	require.NotNil(t, cmd)
	assert.Contains(t, nm.View().Content, "work queue")
}

func TestBootWithoutTokenShowsPairing(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	m := NewModel(Options{Store: store, Offline: true})
	msg := m.boot()
	bm := msg.(bootMsg)
	require.NoError(t, bm.err)
	assert.False(t, bm.hasToken)

	next, _ := m.Update(bm)
	nm := next.(Model)
	assert.Equal(t, "pairing", nm.ScreenName())
	assert.Contains(t, nm.View().Content, "pairing code")
}

func TestMessageFlowPairSyncWorkOpenCompleteSummary(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	m := NewModel(Options{Store: store, Offline: true, AllowUnsandboxed: true, WorkspaceRoot: t.TempDir()})
	m.width, m.height = 100, 40

	// paired -> queue
	next, _ := m.Update(pairedMsg{})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	assert.Contains(t, m.status, "Paired")

	// sync done refreshes work
	next, cmd := m.Update(syncDoneMsg{res: sync.Result{Status: sync.StatusOnline, WorkItems: 2}})
	m = next.(Model)
	assert.Equal(t, sync.StatusOnline, m.syncSt)
	assert.Contains(t, m.status, "2 work item")
	require.NotNil(t, cmd)

	// work loaded populates list + view
	items := []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: "asg-1", State: "available"},
		Activity:   domain.LearningActivity{Title: "Basic nav", Slug: "basic-navigation"},
		Revision:   domain.LearningActivityRevision{ID: "rev-1"},
	}}
	next, _ = m.Update(workLoadedMsg{items: items})
	m = next.(Model)
	require.Len(t, m.work, 1)
	assert.Contains(t, m.View().Content, "Basic nav")
	wi := workItem{it: items[0]}
	assert.Equal(t, "Basic nav", wi.Title())
	assert.Equal(t, "basic-navigation · available", wi.Description())
	assert.Contains(t, wi.FilterValue(), "basic-navigation")
	// Title falls back to slug.
	assert.Equal(t, "only-slug", workItem{it: studentapi.WorkItem{Activity: domain.LearningActivity{Slug: "only-slug"}}}.Title())

	// open session -> terminal activity (no PTY fallback)
	snap := engine.SessionSnapshot{
		ClientSessionID: "cs-1",
		ActivityTitle:   "Basic nav",
		Kind:            contracts.KindTerminal,
		Instructions:    "Use pwd and ls.",
		ChecksTotal:     2,
		ChecksPassed:    0,
		Tasks: []contracts.Task{{
			ID: "t1", Title: "Orient", Instructions: "Print the working directory.",
		}},
		CurrentTaskIdx: 0,
		RelCwd:         "home",
		LastOutput:     "hello\nworld",
	}
	next, cmd = m.Update(sessionOpenedMsg{snap: snap})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())
	assert.Contains(t, m.activityMsg, "command")
	require.NotNil(t, cmd)
	view := m.View().Content
	assert.Contains(t, view, "Basic nav")
	assert.Contains(t, view, "Orient")
	assert.Contains(t, view, "Checks 0/2")

	// verify / complete failure stays on activity
	next, _ = m.Update(verifyDoneMsg{snap: engine.SessionSnapshot{
		ActivityTitle: "Basic nav", Kind: contracts.KindTerminal,
		ChecksPassed: 1, ChecksTotal: 2, RequiredPassed: false,
		Checks: []engine.CheckStatus{{ID: "pwd", Passed: true}, {ID: "ls", Passed: false}},
	}})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "Checks: 1/2")
	assert.Contains(t, m.View().Content, "pwd")

	next, _ = m.Update(completeDoneMsg{err: assert.AnError, snap: m.snap})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())
	assert.Contains(t, m.activityMsg, assert.AnError.Error())

	// successful complete -> summary (queued path)
	next, _ = m.Update(completeDoneMsg{snap: engine.SessionSnapshot{
		ActivityTitle: "Basic nav", ChecksPassed: 2, ChecksTotal: 2,
		CompletionQueued: true, Message: "nice work",
	}})
	m = next.(Model)
	assert.Equal(t, "summary", m.ScreenName())
	assert.Contains(t, m.View().Content, "awaiting sync")
	assert.Contains(t, m.View().Content, "nice work")

	// summary enter returns to queue
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	assert.False(t, m.hasSession())
	require.NotNil(t, cmd)
}

func TestCompleteSummaryAckedAndGeneric(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	next, _ := m.Update(completeDoneMsg{snap: engine.SessionSnapshot{
		ActivityTitle: "A", ChecksPassed: 1, ChecksTotal: 1, CompletionAcked: true,
	}})
	m = next.(Model)
	assert.Contains(t, m.summaryBody, "accepted and synced")

	next, _ = m.Update(completeDoneMsg{snap: engine.SessionSnapshot{
		ActivityTitle: "B", ChecksPassed: 1, ChecksTotal: 1,
	}})
	m = next.(Model)
	assert.Contains(t, m.summaryBody, "Status:   completed")
}

func TestTypingActivityViewAndKeys(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 90, 30
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "sess"
	m.snap = engine.SessionSnapshot{
		Kind:            contracts.KindTyping,
		ActivityTitle:   "Touch typing",
		ClientSessionID: "sess",
		ChecksTotal:     1,
		ChecksPassed:    0,
		Instructions:    "Type carefully.",
		Tasks:           []contracts.Task{{Title: "Prompt 1", Instructions: "Match the line."}},
		CurrentTaskIdx:  0,
		Typing: &engine.TypingSnapshot{
			PromptText: "hello", Input: "he", WPM: 20, Accuracy: 0.9,
			TotalPrompts: 2, RemainingPrompts: 1, IncorrectChars: 1,
		},
		Checks: []engine.CheckStatus{{ID: "accuracy", Passed: false}},
	}
	view := m.View().Content
	assert.Contains(t, view, "Touch typing")
	assert.Contains(t, view, "hello")
	assert.Contains(t, view, "WPM")
	assert.Contains(t, view, "Miss 1")

	// complete blocked until required pass
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "Keep typing")
	assert.Nil(t, cmd)

	// thresholds met messaging
	m.snap.RequiredPassed = true
	m.snap.Typing.Done = true
	m.snap.Typing.RemainingPrompts = 0
	m.snap.ChecksPassed = 1
	m.snap.Checks = []engine.CheckStatus{{ID: "accuracy", Passed: true}}
	assert.Contains(t, m.View().Content, "REQUIRED PASS")
	assert.Contains(t, m.View().Content, "all prompts complete")

	// enter completes when thresholds met
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)

	// ctrl+g pauses (broker Pause error is ignored; session is cleared locally)
	m.busy = false
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'g'})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	assert.Equal(t, "Session paused", m.status)
	require.NotNil(t, cmd) // loadWorkCmd
}

func TestTerminalActivityKeysAndViews(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.width, m.height = 120, 40
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "cs"
	m.snap = engine.SessionSnapshot{
		Kind: contracts.KindTerminal, HasTerminal: true, ActivityTitle: "Shell",
		ClientSessionID: "cs", ChecksTotal: 1, Instructions: "Explore.",
		Tasks: []contracts.Task{{Title: "Task", Instructions: "Run pwd."}}, CurrentTaskIdx: 0,
		TerminalScreen: "user@host:~$ ",
		Checks:         []engine.CheckStatus{{ID: "pwd", Passed: false}},
	}
	m.termScreen = "user@host:~$ ls\n"
	m.chatLog = []string{"tutor: try pwd"}
	view := m.View().Content
	assert.Contains(t, view, "Terminal")
	assert.Contains(t, view, "Instructions")
	assert.Contains(t, view, "try pwd")
	assert.Contains(t, view, "user@host")

	// tab cycles panes
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	assert.Equal(t, paneInstructions, m.activePane)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	assert.Equal(t, paneChat, m.activePane)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	assert.Equal(t, paneTerminal, m.activePane)

	// verify shortcut
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'v'})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)
	m.busy = false

	// complete blocked
	next, _ = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "Required checks")

	// complete allowed
	m.snap.RequiredPassed = true
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)
	m.busy = false

	// hint
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'h'})
	require.NotNil(t, cmd)
	next, _ = m.Update(hintDoneMsg{hint: "Look at the prompt."})
	m = next.(Model)
	assert.Contains(t, m.chatLog[len(m.chatLog)-1], "Look at the prompt")
	assert.Equal(t, "Hint ready", m.activityMsg)

	// long chat log trims
	m.chatLog = make([]string, 25)
	for i := range m.chatLog {
		m.chatLog[i] = "old"
	}
	next, _ = m.Update(hintDoneMsg{hint: "newest"})
	m = next.(Model)
	require.LessOrEqual(t, len(m.chatLog), 20)
	assert.Contains(t, m.chatLog[len(m.chatLog)-1], "newest")

	// fallback command box path (no PTY)
	m.snap.HasTerminal = false
	m.focusCmd = true
	m.cmdInput.SetValue("verify")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)
	m.busy = false

	m.cmdInput.SetValue("hint")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	m.cmdInput.SetValue("complete")
	m.snap.RequiredPassed = false
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "Required checks")

	m.cmdInput.SetValue("back")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
	require.NotNil(t, cmd)
}

func TestKeyToPTYMappings(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"enter":     "\r",
		"backspace": "\x7f",
		"tab":       "\t",
		"esc":       "\x1b",
		"ctrl+c":    "\x03",
		"ctrl+d":    "\x04",
		"ctrl+z":    "\x1a",
		"ctrl+l":    "\x0c",
		"up":        "\x1b[A",
		"down":      "\x1b[B",
		"right":     "\x1b[C",
		"left":      "\x1b[D",
		"space":     " ",
	}
	for key, want := range cases {
		msg := tea.KeyPressMsg{}
		// String() is derived from Code/Mod; set via known constructors where possible.
		switch key {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "left":
			msg = tea.KeyPressMsg{Code: tea.KeyLeft}
		case "right":
			msg = tea.KeyPressMsg{Code: tea.KeyRight}
		case "ctrl+c":
			msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		case "ctrl+d":
			msg = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
		case "ctrl+z":
			msg = tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
		case "ctrl+l":
			msg = tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
		}
		got := keyToPTY(msg)
		assert.Equal(t, want, got, "key %s (String=%q)", key, msg.String())
	}
	// Printable via Text.
	assert.Equal(t, "a", keyToPTY(tea.KeyPressMsg{Code: 'a', Text: "a"}))
	// Unknown modifier combo yields empty.
	assert.Equal(t, "", keyToPTY(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl | tea.ModAlt}))
}

func TestPairingAndQueueKeys(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	m := NewModel(Options{Store: store, Offline: true})
	m.screen = ScreenPairing

	// empty enter errors
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Equal(t, "pairing code required", m.err)

	// q quits when empty
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	m = next.(Model)
	assert.True(t, m.quitting)
	require.NotNil(t, cmd)

	m = NewModel(Options{Store: store, Offline: true})
	m.screen = ScreenQueue
	// r refreshes
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'r'})
	require.NotNil(t, cmd)
	// enter with no selection is no-op
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
	// q quits
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'q'})
	m = next.(Model)
	assert.True(t, m.quitting)

	// enter with selection opens session
	m = NewModel(Options{Store: store, Offline: true})
	m.screen = ScreenQueue
	it := workItem{it: studentapi.WorkItem{Assignment: domain.StudentAssignment{ID: "asg"}}}
	m.workList.SetItems([]list.Item{it})
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.True(t, m.busy)
	require.NotNil(t, cmd)
}

func TestTermTickPollAndResize(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.screen = ScreenActivity
	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "cs"
	m.snap.HasTerminal = true
	m.snap.TutorHint = "keep"

	// tick schedules poll
	next, cmd := m.Update(termTickMsg{})
	require.NotNil(t, cmd)

	// poll updates screen and preserves hint
	next, cmd = m.Update(termPollMsg{
		screen: "new screen",
		snap:   engine.SessionSnapshot{HasTerminal: true, TerminalScreen: "from-snap", TutorHint: ""},
	})
	m = next.(Model)
	assert.Equal(t, "from-snap", m.termScreen)
	assert.Equal(t, "keep", m.snap.TutorHint)
	require.NotNil(t, cmd)

	// window size with terminal schedules resize
	next, cmd = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(Model)
	assert.Equal(t, 100, m.width)
	require.NotNil(t, cmd)

	// resizeTerminalCmd nil without session
	m2 := NewModel(Options{})
	assert.Nil(t, m2.resizeTerminalCmd(40, 100))
}

func TestSyncCmdOfflineAndLoadWork(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	require.NoError(t, store.SaveWork(t.Context(), []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: "a1", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "s", Title: "T"},
	}}))
	m := NewModel(Options{Store: store, Offline: true})
	msg := m.syncCmd()()
	sd := msg.(syncDoneMsg)
	assert.Equal(t, sync.StatusOffline, sd.res.Status)

	msg = m.loadWorkCmd()()
	wl := msg.(workLoadedMsg)
	require.NoError(t, wl.err)
	require.Len(t, wl.items, 1)

	// pairCmd without client
	msg = m.pairCmd("ABCD")()
	pm := msg.(pairedMsg)
	require.Error(t, pm.err)
}

func TestSyncDoneErrorAndWorkError(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenQueue
	next, _ := m.Update(syncDoneMsg{res: sync.Result{Status: sync.StatusOffline, Err: assert.AnError}})
	m = next.(Model)
	// offline errors are not surfaced as status failures
	assert.NotContains(t, m.status, assert.AnError.Error())

	next, _ = m.Update(syncDoneMsg{res: sync.Result{Status: sync.StatusOnline, Err: assert.AnError}})
	m = next.(Model)
	assert.Contains(t, m.status, "sync:")

	next, _ = m.Update(workLoadedMsg{err: assert.AnError})
	m = next.(Model)
	assert.Equal(t, assert.AnError.Error(), m.err)

	next, _ = m.Update(sessionOpenedMsg{err: assert.AnError})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())
}

func TestCmdDoneAndHasSessionClear(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	m.snap.HasTerminal = false
	next, cmd := m.Update(cmdDoneMsg{
		snap: engine.SessionSnapshot{HasTerminal: false, LastOutput: "out", RelCwd: "."},
		err:  assert.AnError,
	})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, assert.AnError.Error())
	require.NotNil(t, cmd) // refocus cmd input

	m.opts.Broker = &broker.Client{}
	m.brokerSessionID = "x"
	assert.True(t, m.hasSession())
	m.clearSession()
	assert.False(t, m.hasSession())
	assert.Equal(t, "", m.brokerSessionID)
}

func TestSyncLineStyles(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	for _, st := range []sync.Status{
		sync.StatusOnline, sync.StatusOffline, sync.StatusAwaiting,
		sync.StatusSyncing, sync.StatusRevoked, sync.StatusIdle, "",
	} {
		m.syncSt = st
		assert.Contains(t, m.syncLine(), "sync:")
	}
}

func TestCtrlCQuitsFromAnyScreen(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenActivity
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	assert.True(t, m.quitting)
	require.NotNil(t, cmd)
	assert.Equal(t, "", m.View().Content)
}

func TestOpenSessionCmdOfflineMissingAssignment(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	require.NoError(t, store.SetDeviceToken(t.Context(), "tok"))
	m := NewModel(Options{
		Store: store, Offline: true, AllowUnsandboxed: true,
		WorkspaceRoot: t.TempDir(), Client: studentapi.New("http://127.0.0.1:1", "tok"),
	})
	msg := m.openSessionCmd("missing-asg")()
	som := msg.(sessionOpenedMsg)
	require.Error(t, som.err)
}
