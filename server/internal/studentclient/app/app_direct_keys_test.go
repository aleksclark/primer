package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
)

func TestKeyToPTYMoreMappings(t *testing.T) {
	t.Parallel()
	// Cover branches not already hit by TestKeyToPTYMappings.
	cases := []struct {
		msg  tea.KeyPressMsg
		want string
	}{
		{tea.KeyPressMsg{Code: tea.KeyEnter}, "\r"},
		{tea.KeyPressMsg{Code: tea.KeyBackspace}, "\x7f"},
		{tea.KeyPressMsg{Code: tea.KeyTab}, "\t"},
		{tea.KeyPressMsg{Code: tea.KeyEscape}, "\x1b"},
		{tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'c'}, "\x03"},
		{tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'd'}, "\x04"},
		{tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'z'}, "\x1a"},
		{tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'l'}, "\x0c"},
		{tea.KeyPressMsg{Code: tea.KeyUp}, "\x1b[A"},
		{tea.KeyPressMsg{Code: tea.KeyDown}, "\x1b[B"},
		{tea.KeyPressMsg{Code: tea.KeyRight}, "\x1b[C"},
		{tea.KeyPressMsg{Code: tea.KeyLeft}, "\x1b[D"},
		{tea.KeyPressMsg{Code: tea.KeySpace}, " "},
		{tea.KeyPressMsg{Text: "A"}, "A"},
		{tea.KeyPressMsg{Code: 'q'}, "q"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, keyToPTY(tc.msg), "msg=%+v str=%q", tc.msg, tc.msg.String())
	}
	// Non-printable / multi-char without text → empty
	assert.Empty(t, keyToPTY(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'x'})) // ctrl+x not mapped
}

func TestTerminalPTYKeyForwardAndCommandBox(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true, Broker: &broker.Client{}})
	m.width, m.height = 120, 40
	m.screen = ScreenActivity
	m.brokerSessionID = "pty-sess"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal, HasTerminal: true}
	m.activePane = paneTerminal
	m.busy = false

	// Printable key → termWriteCmd
	next, cmd := m.Update(tea.KeyPressMsg{Text: "l"})
	m = next.(Model)
	require.NotNil(t, cmd)

	// Esc while terminal focused stays (does not pause)
	m.activePane = paneTerminal
	m.snap.HasTerminal = true
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())

	// Esc without terminal focus pauses
	m.snap.HasTerminal = false
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Equal(t, "queue", m.ScreenName())

	// ctrl+g pause
	m.screen = ScreenActivity
	m.brokerSessionID = "pty-sess"
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal}
	next, _ = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'g'})
	assert.Equal(t, "queue", next.(Model).ScreenName())
}

func TestDirectSessionTypingKeysAndCmds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Minimal typing work item.
	asg := "asg-type-direct"
	require.NoError(t, store.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: asg, State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "type-me", Title: "Type", Kind: contracts.KindTyping},
		Revision: domain.LearningActivityRevision{
			ID: "rev-t", ContentSHA256: "digest",
			Content: map[string]any{
				"objective": "type",
				"typing": map[string]any{
					"prompts":        []any{map[string]any{"id": "p1", "text": "ab"}},
					"minWpm":         1,
					"minAccuracy":    0.5,
					"allowBackspace": true,
				},
				"tasks":  []any{map[string]any{"id": "t1", "title": "T", "instructions": "type", "completion": map[string]any{"checkId": "c1"}}},
				"checks": []any{map[string]any{"id": "c1", "kind": "typing_thresholds", "params": map[string]any{}}},
				"hints":  []any{map[string]any{"id": "h1", "text": "slow down"}},
			},
		},
	}}))

	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))
	eng, err := engine.New(engine.Options{
		Store: store, WorkspaceRoot: ws, Offline: true, AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	sess, err := eng.OpenSession(ctx, asg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	m := NewModel(Options{Store: store, Offline: true, AllowUnsandboxed: true, WorkspaceRoot: ws})
	m.width, m.height = 100, 40
	m.screen = ScreenActivity
	m.sess = sess
	m.snap = sess.Snapshot()
	m.opts.Broker = nil

	// Type a rune via direct session path
	next, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, contracts.KindTyping, m.snap.Kind)

	// Backspace direct
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	assert.Nil(t, cmd)

	// ctrl+h hintCmd body (local hint)
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'h'})
	require.NotNil(t, cmd)
	msg := cmd()
	hd := msg.(hintDoneMsg)
	assert.NotEmpty(t, hd.hint)

	// verifyCmd direct
	m.busy = false
	m.snap = m.sess.Snapshot()
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'v'})
	m = next.(Model)
	require.NotNil(t, cmd)
	_ = cmd()

	// termPollCmd with direct sess
	msg = m.termPollCmd()()
	_ = msg.(termPollMsg)

	// termWriteCmd / resize with direct sess (may error without PTY)
	msg = m.termWriteCmd("x")()
	_ = msg.(termPollMsg)
	if c := m.resizeTerminalCmd(30, 80); c != nil {
		_ = c()
	}

	// completeCmd may fail thresholds
	msg = m.completeCmd()()
	_ = msg.(completeDoneMsg)

	// busy blocks typing
	m.busy = true
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTyping}
	next, cmd = m.Update(tea.KeyPressMsg{Text: "z"})
	assert.Nil(t, cmd)
	_ = next
}

func TestPairingInputUpdateAndQueueNoSelection(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.screen = ScreenPairing
	// Non-enter key updates text input
	next, _ := m.Update(tea.KeyPressMsg{Text: "A"})
	m = next.(Model)
	assert.Equal(t, "pairing", m.ScreenName())

	// Queue enter with no selection
	m.screen = ScreenQueue
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
	_ = next

	// Unknown screen key returns nil
	m.screen = Screen(99)
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x'})
	assert.Nil(t, cmd)
	_ = next
}
