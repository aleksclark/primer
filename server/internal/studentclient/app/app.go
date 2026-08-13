// Package app is the Bubble Tea student client: pairing, work queue, activity
// session (PTY terminal / typing), and completion summary.
//
// Phase 9: Bubble Tea v2 + embedded PTY three-pane activity view.
// Leave activity: ctrl+g (avoids shell conflict). Tab cycles pane focus.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

// Screen identifies the active root view.
type Screen int

const (
	ScreenBoot Screen = iota
	ScreenPairing
	ScreenQueue
	ScreenActivity
	ScreenLesson
	ScreenSummary
)

// activityPane is focus within the terminal activity three-pane layout.
type activityPane int

const (
	paneTerminal activityPane = iota
	paneInstructions
	paneChat
)

// Options configure the student TUI.
type Options struct {
	BaseURL       string
	Store         *cache.Store
	Client        *studentapi.Client
	WorkspaceRoot string
	DeviceName    string
	// Broker is the privileged IPC client. When non-nil, the TUI never touches
	// Store/Client token paths and all durable ops go through the broker.
	Broker *broker.Client
	// AllowUnsandboxed runs commands without bubblewrap.
	// Production broker path never sets this; direct-mode tests may.
	AllowUnsandboxed bool
	// Offline skips network after cache is populated.
	Offline bool
}

// Model is the root Bubble Tea model.
type Model struct {
	opts     Options
	screen   Screen
	width    int
	height   int
	err      string
	status   string // free-form status line message
	syncSt   sync.Status
	quitting bool

	// pairing
	pairInput textinput.Model
	pairing   bool

	// queue
	workList list.Model
	work     []studentapi.WorkItem

	// activity
	sess            *engine.Session // direct mode only
	brokerSessionID string          // broker mode session handle
	snap            engine.SessionSnapshot
	cmdInput        textinput.Model // fallback RunLine box when no PTY / instructions pane
	busy            bool
	focusCmd        bool
	activityMsg     string
	activePane      activityPane
	termScreen      string
	chatLog         []string

	// lesson reading (blocks from immutable revision)
	lessonLines  []string
	lessonOffset int
	lessonIdx    int // focused block index in student-visible blocks

	// response drafting (short_response tasks)
	respInput textinput.Model

	// summary
	summaryTitle string
	summaryBody  string
}

func (m Model) brokerMode() bool { return m.opts.Broker != nil }

func (m Model) hasSession() bool {
	if m.brokerMode() {
		return m.brokerSessionID != ""
	}
	return m.sess != nil
}

func (m *Model) clearSession() {
	m.sess = nil
	m.brokerSessionID = ""
	m.termScreen = ""
	m.chatLog = nil
	m.activePane = paneTerminal
}

// NewModel builds the root model. Call Init via the tea program.
func NewModel(opts Options) Model {
	if opts.DeviceName == "" {
		opts.DeviceName = "workstation"
	}
	if opts.Client == nil && opts.BaseURL != "" {
		opts.Client = studentapi.New(opts.BaseURL, "")
	}
	pi := textinput.New()
	pi.Placeholder = "pairing code"
	pi.CharLimit = 32
	pi.SetWidth(24)
	pi.Prompt = "> "

	ci := textinput.New()
	ci.Placeholder = "command (fallback when no PTY)"
	ci.CharLimit = 512
	ci.SetWidth(60)
	ci.Prompt = "$ "

	ri := textinput.New()
	ri.Placeholder = "type your response (your own words)"
	ri.CharLimit = contracts.MaxResponseMaxChars
	ri.SetWidth(70)
	ri.Prompt = "> "

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	l := list.New(nil, delegate, 80, 16)
	l.Title = "Work queue"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	return Model{
		opts:      opts,
		screen:    ScreenBoot,
		pairInput: pi,
		cmdInput:  ci,
		respInput: ri,
		workList:  l,
		syncSt:    sync.StatusIdle,
		focusCmd:  true,
	}
}

// Run starts the interactive TUI (blocking).
func Run(opts Options) error {
	p := tea.NewProgram(NewModel(opts))
	_, err := p.Run()
	return err
}

// --- messages ----------------------------------------------------------------

type bootMsg struct {
	hasToken bool
	err      error
}

type pairedMsg struct {
	err error
}

type syncDoneMsg struct {
	res sync.Result
}

type workLoadedMsg struct {
	items []studentapi.WorkItem
	err   error
}

type sessionOpenedMsg struct {
	sess *engine.Session
	snap engine.SessionSnapshot
	err  error
}

type cmdDoneMsg struct {
	snap engine.SessionSnapshot
	err  error
}

type verifyDoneMsg struct {
	snap engine.SessionSnapshot
}

type completeDoneMsg struct {
	snap engine.SessionSnapshot
	err  error
}

type hintDoneMsg struct {
	hint string
}

type termTickMsg struct{}

type termPollMsg struct {
	screen string
	snap   engine.SessionSnapshot
	err    error
}

// --- lifecycle ---------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return m.boot
}

func (m Model) boot() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if m.brokerMode() {
		prof, err := m.opts.Broker.Profile(ctx)
		if err != nil {
			// Fall back to health.paired.
			h, herr := m.opts.Broker.Health(ctx)
			if herr != nil {
				return bootMsg{err: err}
			}
			return bootMsg{hasToken: h.Paired}
		}
		return bootMsg{hasToken: prof.Paired}
	}
	if m.opts.Store == nil {
		return bootMsg{err: fmt.Errorf("cache store is required (or set Broker)")}
	}
	tok, err := m.opts.Store.DeviceToken(ctx)
	if err != nil {
		return bootMsg{err: err}
	}
	if tok != "" && m.opts.Client != nil {
		m.opts.Client.SetToken(tok)
	}
	return bootMsg{hasToken: tok != ""}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		var cmds []tea.Cmd
		if m.screen == ScreenActivity && m.snap.HasTerminal {
			cmds = append(cmds, m.resizeTerminalCmd(msg.Height, msg.Width))
		}
		return m, tea.Batch(cmds...)

	case bootMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.screen = ScreenPairing
			return m, m.pairInput.Focus()
		}
		if !msg.hasToken {
			m.screen = ScreenPairing
			m.status = "Enter the pairing code from a parent."
			return m, m.pairInput.Focus()
		}
		m.screen = ScreenQueue
		return m, tea.Batch(m.syncCmd(), m.loadWorkCmd())

	case pairedMsg:
		m.pairing = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.screen = ScreenQueue
		m.status = "Paired. Syncing work…"
		return m, tea.Batch(m.syncCmd(), m.loadWorkCmd())

	case syncDoneMsg:
		m.syncSt = msg.res.Status
		if msg.res.Err != nil && msg.res.Status != sync.StatusOffline {
			m.status = "sync: " + msg.res.Err.Error()
		} else {
			m.status = fmt.Sprintf("sync %s · %d work item(s)", msg.res.Status, msg.res.WorkItems)
		}
		if m.screen == ScreenQueue {
			return m, m.loadWorkCmd()
		}
		return m, nil

	case workLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.work = msg.items
		items := make([]list.Item, 0, len(msg.items))
		for _, it := range msg.items {
			items = append(items, workItem{it: it})
		}
		m.workList.SetItems(items)
		return m, nil

	case sessionOpenedMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.screen = ScreenQueue
			return m, nil
		}
		m.err = ""
		m.sess = msg.sess
		m.snap = msg.snap
		if m.brokerMode() {
			m.brokerSessionID = msg.snap.ClientSessionID
			m.sess = nil
		} else {
			m.brokerSessionID = ""
		}
		m.screen = ScreenActivity
		m.cmdInput.SetValue("")
		m.termScreen = msg.snap.TerminalScreen
		m.activePane = paneTerminal
		if msg.snap.Kind == contracts.KindTyping {
			m.cmdInput.Blur()
			m.focusCmd = false
			m.activityMsg = "Type the prompt exactly. backspace fixes · ctrl+s complete · ctrl+g back"
			return m, nil
		}
		if msg.snap.HasTerminal {
			m.cmdInput.Blur()
			m.focusCmd = false
			m.activityMsg = "PTY terminal · tab focus · ctrl+v verify · ctrl+s complete · ctrl+g back"
			return m, tea.Batch(m.termTickCmd(), m.resizeTerminalCmd(m.height, m.width))
		}
		// Fallback command box (no PTY).
		m.focusCmd = true
		m.activityMsg = "Type a command and press Enter. verify/complete/hint as commands · ctrl+g back"
		return m, m.cmdInput.Focus()

	case cmdDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.activityMsg = msg.err.Error()
		} else {
			m.activityMsg = ""
		}
		m.snap = msg.snap
		if msg.snap.TerminalScreen != "" {
			m.termScreen = msg.snap.TerminalScreen
		}
		m.cmdInput.SetValue("")
		if m.snap.HasTerminal && m.activePane == paneTerminal {
			return m, nil
		}
		return m, m.cmdInput.Focus()

	case verifyDoneMsg:
		m.busy = false
		m.snap = msg.snap
		if msg.snap.TerminalScreen != "" {
			m.termScreen = msg.snap.TerminalScreen
		}
		m.activityMsg = fmt.Sprintf("Checks: %d/%d · required=%v", msg.snap.ChecksPassed, msg.snap.ChecksTotal, msg.snap.RequiredPassed)
		return m, nil

	case completeDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.activityMsg = msg.err.Error()
			m.snap = msg.snap
			return m, nil
		}
		m.snap = msg.snap
		m.screen = ScreenSummary
		m.summaryTitle = "Activity complete"
		body := strings.Builder{}
		fmt.Fprintf(&body, "Activity: %s\n", msg.snap.ActivityTitle)
		fmt.Fprintf(&body, "Checks:   %d/%d passed\n", msg.snap.ChecksPassed, msg.snap.ChecksTotal)
		if msg.snap.CompletionAcked {
			body.WriteString("Status:   accepted and synced\n")
		} else if msg.snap.CompletionQueued {
			body.WriteString("Status:   awaiting sync\n")
			body.WriteString("Evidence is stored locally and will upload when online.\n")
		} else {
			body.WriteString("Status:   completed\n")
		}
		if msg.snap.Message != "" {
			fmt.Fprintf(&body, "Note:     %s\n", msg.snap.Message)
		}
		m.summaryBody = body.String()
		return m, nil

	case hintDoneMsg:
		if m.brokerMode() {
			if msg.hint != "" {
				m.snap.TutorHint = msg.hint
			}
		} else if m.sess != nil {
			m.sess.SetTutorHint(msg.hint)
			m.snap = m.sess.Snapshot()
		}
		if msg.hint != "" {
			m.chatLog = append(m.chatLog, "tutor: "+msg.hint)
			if len(m.chatLog) > 20 {
				m.chatLog = m.chatLog[len(m.chatLog)-20:]
			}
		}
		m.activityMsg = "Hint ready"
		return m, nil

	case termTickMsg:
		if m.screen != ScreenActivity || !m.hasSession() || !m.snap.HasTerminal {
			return m, nil
		}
		return m, m.termPollCmd()

	case termPollMsg:
		if m.screen != ScreenActivity {
			return m, nil
		}
		if msg.err == nil {
			if msg.screen != "" {
				m.termScreen = msg.screen
			}
			// Preserve tutor hint / local activityMsg when refreshing snap.
			hint := m.snap.TutorHint
			m.snap = msg.snap
			if m.snap.TutorHint == "" {
				m.snap.TutorHint = hint
			}
			if msg.snap.TerminalScreen != "" {
				m.termScreen = msg.snap.TerminalScreen
			}
		}
		return m, m.termTickCmd()

	case responseDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.activityMsg = msg.err.Error()
		} else {
			m.snap = msg.snap
			m.activityMsg = "Response submitted"
			if msg.snap.ResponseQueued {
				m.activityMsg = "Response submitted — awaiting sync"
			}
			m.respInput.Blur()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Delegate to focused inputs / list.
	switch m.screen {
	case ScreenPairing:
		var cmd tea.Cmd
		m.pairInput, cmd = m.pairInput.Update(msg)
		return m, cmd
	case ScreenQueue:
		var cmd tea.Cmd
		m.workList, cmd = m.workList.Update(msg)
		return m, cmd
	case ScreenActivity:
		if m.snap.Kind == contracts.KindTyping {
			return m, nil
		}
		if !m.snap.HasTerminal && m.focusCmd && !m.busy {
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
		if m.snap.HasTerminal && m.activePane == paneInstructions && m.focusCmd && !m.busy {
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	switch m.screen {
	case ScreenPairing:
		switch key {
		case "enter":
			if m.pairing {
				return m, nil
			}
			code := strings.TrimSpace(m.pairInput.Value())
			if code == "" {
				m.err = "pairing code required"
				return m, nil
			}
			m.pairing = true
			m.err = ""
			return m, m.pairCmd(code)
		case "q":
			// Only quit on bare q when input empty so codes can contain q.
			if m.pairInput.Value() == "" {
				m.quitting = true
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.pairInput, cmd = m.pairInput.Update(msg)
		return m, cmd

	case ScreenQueue:
		switch key {
		case "q":
			m.quitting = true
			return m, tea.Quit
		case "r":
			return m, tea.Batch(m.syncCmd(), m.loadWorkCmd())
		case "enter":
			it, ok := m.workList.SelectedItem().(workItem)
			if !ok {
				return m, nil
			}
			m.busy = true
			m.status = "Opening assignment…"
			return m, m.openSessionCmd(it.it.Assignment.ID)
		}
		var cmd tea.Cmd
		m.workList, cmd = m.workList.Update(msg)
		return m, cmd

	case ScreenActivity:
		if m.snap.Kind == contracts.KindTyping {
			return m.handleTypingKey(msg)
		}
		return m.handleTerminalActivityKey(msg)

	case ScreenLesson:
		return m.handleLessonKey(msg)

	case ScreenSummary:
		switch key {
		case "enter", "q", "esc", "ctrl+g":
			m.clearSession()
			m.screen = ScreenQueue
			m.summaryBody = ""
			return m, tea.Batch(m.syncCmd(), m.loadWorkCmd())
		}
	}
	return m, nil
}

func (m Model) handleTerminalActivityKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global activity keys (work even when PTY focused).
	switch key {
	case "ctrl+g":
		// Documented leave-activity key (avoids shell ESC / ctrl+c conflict).
		return m.pauseToQueue()
	case "esc":
		// Esc only leaves when not focused on the live terminal.
		if !(m.snap.HasTerminal && m.activePane == paneTerminal) {
			return m.pauseToQueue()
		}
	case "tab":
		if m.snap.HasTerminal {
			m.activePane = (m.activePane + 1) % 3
			m.focusCmd = m.activePane == paneInstructions
			if m.focusCmd {
				return m, m.cmdInput.Focus()
			}
			m.cmdInput.Blur()
			return m, nil
		}
	case "ctrl+v":
		if m.busy || !m.hasSession() {
			return m, nil
		}
		m.busy = true
		return m, m.verifyCmd()
	case "ctrl+s":
		if m.busy || !m.hasSession() {
			return m, nil
		}
		if !m.snap.RequiredPassed {
			m.activityMsg = "Required checks not passed yet — run verify after commands."
			return m, nil
		}
		m.busy = true
		return m, m.completeCmd()
	case "ctrl+h":
		if !m.hasSession() {
			return m, nil
		}
		return m, m.hintCmd()
	case "ctrl+l":
		// Open lesson reader from immutable revision blocks.
		if !m.hasSession() {
			return m, nil
		}
		m.openLesson()
		m.screen = ScreenLesson
		return m, nil
	}

	// Short-response task: route typing to response field when instructions pane focused
	// or when there is no PTY and current task is short_response.
	if m.currentTaskIsResponse() && (!m.snap.HasTerminal || m.activePane == paneInstructions) {
		if key == "ctrl+enter" || key == "alt+enter" {
			return m, m.submitResponseCmd()
		}
		if !m.busy {
			var cmd tea.Cmd
			m.respInput, cmd = m.respInput.Update(msg)
			// Persist draft periodically on enter-less edits.
			if key != "enter" {
				return m, tea.Batch(cmd, m.saveDraftCmd())
			}
			// Enter alone inserts nothing special; keep draft.
			return m, tea.Batch(cmd, m.saveDraftCmd())
		}
	}

	// Live PTY: forward keystrokes when terminal pane focused.
	if m.snap.HasTerminal && m.activePane == paneTerminal && !m.busy {
		data := keyToPTY(msg)
		if data == "" {
			return m, nil
		}
		return m, m.termWriteCmd(data)
	}

	// Fallback / instructions-pane command box (RunLine + verify/complete shortcuts).
	if key == "enter" {
		if m.busy || !m.hasSession() {
			return m, nil
		}
		line := strings.TrimSpace(m.cmdInput.Value())
		switch line {
		case ":verify", "verify":
			m.busy = true
			m.cmdInput.SetValue("")
			return m, m.verifyCmd()
		case ":complete", "complete":
			if !m.snap.RequiredPassed {
				m.activityMsg = "Required checks not passed yet"
				m.cmdInput.SetValue("")
				return m, nil
			}
			m.busy = true
			m.cmdInput.SetValue("")
			return m, m.completeCmd()
		case ":hint", "hint":
			m.cmdInput.SetValue("")
			return m, m.hintCmd()
		case ":back", "back":
			return m.pauseToQueue()
		}
		if line == "" {
			return m, nil
		}
		m.busy = true
		return m, m.runCmd(line)
	}
	if !m.busy {
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// keyToPTY maps a Bubble Tea v2 key press to PTY input bytes.
func keyToPTY(msg tea.KeyPressMsg) string {
	k := msg.Key()
	// Printable text (including shifted symbols) prefers Key.Text.
	if k.Text != "" {
		return k.Text
	}
	switch msg.String() {
	case "enter":
		return "\r"
	case "backspace":
		return "\x7f"
	case "tab":
		return "\t"
	case "esc":
		return "\x1b"
	case "ctrl+c":
		return "\x03"
	case "ctrl+d":
		return "\x04"
	case "ctrl+z":
		return "\x1a"
	case "ctrl+l":
		return "\x0c"
	case "up":
		return "\x1b[A"
	case "down":
		return "\x1b[B"
	case "right":
		return "\x1b[C"
	case "left":
		return "\x1b[D"
	case "space":
		return " "
	}
	// Single printable rune fallback.
	s := msg.String()
	if rs := []rune(s); len(rs) == 1 && unicode.IsPrint(rs[0]) &&
		!strings.HasPrefix(s, "ctrl+") && !strings.HasPrefix(s, "alt+") {
		return s
	}
	return ""
}

func (m Model) pauseToQueue() (tea.Model, tea.Cmd) {
	if m.brokerMode() && m.brokerSessionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = m.opts.Broker.Pause(ctx, m.brokerSessionID)
		cancel()
	} else if m.sess != nil {
		_ = m.sess.Pause(context.Background())
	}
	m.clearSession()
	m.screen = ScreenQueue
	m.status = "Session paused"
	return m, m.loadWorkCmd()
}

// --- commands ----------------------------------------------------------------

func (m Model) pairCmd(code string) tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if opts.Broker != nil {
			_, err := opts.Broker.Pair(ctx, code, opts.DeviceName)
			return pairedMsg{err: err}
		}
		if opts.Client == nil {
			return pairedMsg{err: fmt.Errorf("API client not configured")}
		}
		pair, err := opts.Client.Pair(ctx, code, opts.DeviceName)
		if err != nil {
			return pairedMsg{err: err}
		}
		// Direct mode (tests): store token locally. Production uses broker.
		if err := opts.Store.SetDeviceToken(ctx, pair.Token); err != nil {
			return pairedMsg{err: err}
		}
		if err := opts.Store.SetDeviceIdentity(ctx, pair.DeviceID, pair.Student.ID, pair.Device.Name); err != nil {
			return pairedMsg{err: err}
		}
		opts.Client.SetToken(pair.Token)
		return pairedMsg{}
	}
}

func (m Model) syncCmd() tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if opts.Broker != nil {
			if opts.Offline {
				return syncDoneMsg{res: sync.Result{Status: sync.StatusOffline}}
			}
			r, err := opts.Broker.SyncWork(ctx)
			res := sync.Result{
				Status:           broker.SyncStatus(r.Status),
				WorkItems:        r.WorkItems,
				EventsFlushed:    r.EventsFlushed,
				CompletionsSent:  r.CompletionsSent,
				ArtifactsSent:    r.ArtifactsSent,
				PendingEvents:    r.PendingEvents,
				PendingCompletes: r.PendingCompletes,
			}
			if err != nil {
				res.Err = err
			} else if r.Error != "" {
				res.Err = fmt.Errorf("%s", r.Error)
			}
			return syncDoneMsg{res: res}
		}
		if opts.Offline || opts.Client == nil {
			return syncDoneMsg{res: sync.Result{Status: sync.StatusOffline}}
		}
		if tok, _ := opts.Store.DeviceToken(ctx); tok != "" {
			opts.Client.SetToken(tok)
		}
		loop := sync.New(opts.Client, opts.Store)
		res := loop.SyncOnce(ctx)
		return syncDoneMsg{res: res}
	}
}

func (m Model) loadWorkCmd() tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if opts.Broker != nil {
			items, err := opts.Broker.ListWork(ctx)
			return workLoadedMsg{items: items, err: err}
		}
		items, err := opts.Store.ListWork(ctx)
		return workLoadedMsg{items: items, err: err}
	}
}

func (m Model) openSessionCmd(assignmentID string) tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if opts.Broker != nil {
			resp, err := opts.Broker.OpenSession(ctx, assignmentID)
			if err != nil {
				return sessionOpenedMsg{err: err}
			}
			return sessionOpenedMsg{snap: resp.Snapshot, err: nil}
		}
		if tok, _ := opts.Store.DeviceToken(ctx); tok != "" && opts.Client != nil {
			opts.Client.SetToken(tok)
		}
		eng, err := engine.New(engine.Options{
			Client:           opts.Client,
			Store:            opts.Store,
			WorkspaceRoot:    opts.WorkspaceRoot,
			Offline:          opts.Offline,
			UseSandbox:       !opts.AllowUnsandboxed,
			AllowUnsandboxed: opts.AllowUnsandboxed,
		})
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		sess, err := eng.OpenSession(ctx, assignmentID)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		return sessionOpenedMsg{sess: sess, snap: sess.Snapshot()}
	}
}

func (m Model) runCmd(line string) tea.Cmd {
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if opts.Broker != nil {
			snap, err := opts.Broker.RunCommand(ctx, sid, line)
			return cmdDoneMsg{snap: snap, err: err}
		}
		err := sess.RunLine(ctx, line)
		return cmdDoneMsg{snap: sess.Snapshot(), err: err}
	}
}

func (m Model) termWriteCmd(data string) tea.Cmd {
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if opts.Broker != nil {
			snap, err := opts.Broker.TerminalWrite(ctx, sid, data)
			screen := snap.TerminalScreen
			return termPollMsg{screen: screen, snap: snap, err: err}
		}
		err := sess.WriteTerminal(ctx, []byte(data))
		snap := sess.Snapshot()
		return termPollMsg{screen: snap.TerminalScreen, snap: snap, err: err}
	}
}

func (m Model) termPollCmd() tea.Cmd {
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if opts.Broker != nil {
			resp, err := opts.Broker.TerminalRead(ctx, sid)
			return termPollMsg{screen: resp.Screen, snap: resp.Snapshot, err: err}
		}
		if sess == nil {
			return termPollMsg{err: fmt.Errorf("no session")}
		}
		snap := sess.Snapshot()
		return termPollMsg{screen: sess.TerminalScreen(), snap: snap}
	}
}

func (m Model) termTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return termTickMsg{}
	})
}

func (m Model) resizeTerminalCmd(height, width int) tea.Cmd {
	if !m.hasSession() {
		return nil
	}
	// Approximate terminal pane size (left ~55% width, most of height minus chrome).
	cols := uint16(max(20, width*55/100-4))
	rows := uint16(max(8, height-8))
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if opts.Broker != nil {
			snap, err := opts.Broker.TerminalResize(ctx, sid, rows, cols)
			return termPollMsg{screen: snap.TerminalScreen, snap: snap, err: err}
		}
		if sess != nil {
			_ = sess.ResizeTerminal(rows, cols)
			snap := sess.Snapshot()
			return termPollMsg{screen: snap.TerminalScreen, snap: snap}
		}
		return termPollMsg{}
	}
}

func (m Model) handleTypingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+g", "esc":
		return m.pauseToQueue()
	case "ctrl+s":
		if m.busy || !m.hasSession() {
			return m, nil
		}
		if !m.snap.RequiredPassed {
			m.activityMsg = "Keep typing until WPM and accuracy thresholds are met."
			return m, nil
		}
		m.busy = true
		return m, m.completeCmd()
	case "ctrl+h":
		if !m.hasSession() {
			return m, nil
		}
		return m, m.hintCmd()
	case "ctrl+v":
		if m.busy || !m.hasSession() {
			return m, nil
		}
		m.busy = true
		return m, m.verifyCmd()
	case "enter":
		// Allow test-friendly complete when thresholds already met.
		if m.snap.RequiredPassed && !m.busy && m.hasSession() {
			m.busy = true
			return m, m.completeCmd()
		}
		return m, nil
	}

	if m.busy || !m.hasSession() {
		return m, nil
	}

	// Typing input is applied synchronously so keystroke order is preserved
	// (Bubble Tea runs Cmds concurrently, which would race TypeRune).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if key == "backspace" {
		if m.brokerMode() {
			snap, err := m.opts.Broker.TypeBackspace(ctx, m.brokerSessionID)
			if err != nil {
				m.activityMsg = err.Error()
			} else {
				m.activityMsg = ""
			}
			m.snap = snap
		} else {
			if err := m.sess.TypeBackspace(ctx); err != nil {
				m.activityMsg = err.Error()
			} else {
				m.activityMsg = ""
			}
			m.snap = m.sess.Snapshot()
		}
		if m.snap.RequiredPassed {
			m.activityMsg = "Thresholds met — press enter or ctrl+s to finish."
		}
		return m, nil
	}

	var runes []rune
	k := msg.Key()
	if k.Text != "" {
		runes = []rune(k.Text)
	} else if key == "space" {
		runes = []rune{' '}
	} else if key == "tab" {
		runes = []rune{'\t'}
	} else if rs := []rune(key); len(rs) == 1 && unicode.IsPrint(rs[0]) &&
		!strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") {
		runes = rs
	}
	if len(runes) == 0 {
		return m, nil
	}
	for _, r := range runes {
		if r == 0 || (!unicode.IsPrint(r) && r != '\t') {
			continue
		}
		if m.brokerMode() {
			snap, err := m.opts.Broker.TypeRune(ctx, m.brokerSessionID, r)
			if err != nil {
				m.activityMsg = err.Error()
				m.snap = snap
				return m, nil
			}
			m.snap = snap
		} else {
			if err := m.sess.TypeRune(ctx, r); err != nil {
				m.activityMsg = err.Error()
				m.snap = m.sess.Snapshot()
				return m, nil
			}
			m.snap = m.sess.Snapshot()
		}
	}
	if !m.brokerMode() && m.sess != nil {
		m.snap = m.sess.Snapshot()
	}
	m.activityMsg = ""
	if m.snap.RequiredPassed {
		m.activityMsg = "Thresholds met — press enter or ctrl+s to finish."
	}
	return m, nil
}

func (m Model) verifyCmd() tea.Cmd {
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if opts.Broker != nil {
			snap, _ := opts.Broker.Verify(ctx, sid)
			return verifyDoneMsg{snap: snap}
		}
		_ = sess.Verify(ctx)
		return verifyDoneMsg{snap: sess.Snapshot()}
	}
}

func (m Model) completeCmd() tea.Cmd {
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if opts.Broker != nil {
			snap, err := opts.Broker.Complete(ctx, sid)
			return completeDoneMsg{snap: snap, err: err}
		}
		err := sess.Complete(ctx)
		return completeDoneMsg{snap: sess.Snapshot(), err: err}
	}
}

func (m Model) hintCmd() tea.Cmd {
	opts := m.opts
	sess := m.sess
	sid := m.brokerSessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if opts.Broker != nil {
			resp, err := opts.Broker.Tutor(ctx, sid, "Need a hint for the current task.")
			if err != nil {
				return hintDoneMsg{hint: ""}
			}
			return hintDoneMsg{hint: resp.Hint}
		}
		// Prefer server tutor stub when online + session id known.
		snap := sess.Snapshot()
		if !opts.Offline && opts.Client != nil && snap.ServerSessionID != "" {
			if tok, _ := opts.Store.DeviceToken(ctx); tok != "" {
				opts.Client.SetToken(tok)
			}
			if resp, err := opts.Client.TutorMessage(ctx, snap.ServerSessionID, "Need a hint for the current task."); err == nil && resp != nil && resp.Reply != "" {
				return hintDoneMsg{hint: resp.Reply}
			}
		}
		return hintDoneMsg{hint: sess.LocalHint()}
	}
}

func (m *Model) layout() {
	w, h := m.width, m.height
	if w < 40 {
		w = 40
	}
	if h < 12 {
		h = 12
	}
	m.workList.SetSize(w-2, h-6)
	m.cmdInput.SetWidth(max(20, w-8))
	m.pairInput.SetWidth(min(32, w-10))
}

// --- view --------------------------------------------------------------------

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	focusBorder = lipgloss.Color("212")
	idleBorder  = lipgloss.Color("62")
)

func (m Model) View() tea.View {
	var content string
	if m.quitting {
		content = ""
	} else {
		switch m.screen {
		case ScreenBoot:
			content = "Starting Primer student…\n"
		case ScreenPairing:
			content = m.viewPairing()
		case ScreenQueue:
			content = m.viewQueue()
		case ScreenActivity:
			content = m.viewActivity()
		case ScreenLesson:
			content = m.viewLesson()
		case ScreenSummary:
			content = m.viewSummary()
		default:
			content = ""
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *Model) openLesson() {
	blocks := m.snap.Blocks
	var lines []string
	if len(blocks) == 0 {
		lines = append(lines, "No typed instructional blocks on this revision.")
		lines = append(lines, "")
		lines = append(lines, m.snap.Objective)
		lines = append(lines, "")
		for _, ln := range wrapWords(m.snap.Instructions, 72) {
			lines = append(lines, ln)
		}
	} else {
		for i, b := range blocks {
			title := b.Title
			if title == "" {
				title = b.Kind
			}
			lines = append(lines, fmt.Sprintf("── %d. %s ──", i+1, title))
			lines = append(lines, formatBlockLines(b, 72)...)
			lines = append(lines, "")
		}
	}
	m.lessonLines = lines
	m.lessonOffset = 0
	m.lessonIdx = 0
}

func formatBlockLines(b contracts.InstructionBlock, width int) []string {
	var out []string
	switch b.Kind {
	case contracts.BlockVocabulary:
		for _, t := range b.Terms {
			out = append(out, wrapWords(fmt.Sprintf("• %s — %s", t.Term, t.Definition), width)...)
		}
	case contracts.BlockExample:
		if b.Input != "" {
			out = append(out, "Input:")
			out = append(out, wrapWords(b.Input, width)...)
		}
		if b.Output != "" {
			out = append(out, "Output:")
			out = append(out, wrapWords(b.Output, width)...)
		}
		if b.Explanation != "" {
			out = append(out, wrapWords(b.Explanation, width)...)
		}
	case contracts.BlockResource:
		if b.Resource != nil {
			out = append(out, wrapWords(fmt.Sprintf("[%s] %s", b.Resource.MediaType, b.Resource.Label), width)...)
		}
	default:
		out = append(out, wrapWords(b.Text, width)...)
	}
	return out
}

func wrapWords(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if width < 20 {
		width = 20
	}
	words := strings.Fields(s)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
			continue
		}
		if cur.Len()+1+len(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func (m Model) handleLessonKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	page := max(8, m.height-6)
	switch key {
	case "q", "esc", "ctrl+g", "ctrl+l":
		m.screen = ScreenActivity
		return m, nil
	case "up", "k":
		if m.lessonOffset > 0 {
			m.lessonOffset--
		}
	case "down", "j":
		if m.lessonOffset+page < len(m.lessonLines) {
			m.lessonOffset++
		}
	case "pgup", "b":
		m.lessonOffset -= page
		if m.lessonOffset < 0 {
			m.lessonOffset = 0
		}
	case "pgdown", "f", "space":
		m.lessonOffset += page
		if m.lessonOffset > len(m.lessonLines) {
			m.lessonOffset = max(0, len(m.lessonLines)-1)
		}
	case "home", "g":
		m.lessonOffset = 0
	case "end", "G":
		m.lessonOffset = max(0, len(m.lessonLines)-page)
	case "n":
		if m.lessonIdx+1 < len(m.snap.Blocks) {
			m.lessonIdx++
			// Jump offset near block header lines (best-effort).
			m.lessonOffset = min(len(m.lessonLines)-1, m.lessonIdx*4)
		}
	case "p":
		if m.lessonIdx > 0 {
			m.lessonIdx--
			m.lessonOffset = max(0, m.lessonIdx*4)
		}
	}
	return m, nil
}

func (m Model) viewLesson() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Lesson — " + m.snap.ActivityTitle))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("immutable revision blocks · parent notes hidden"))
	b.WriteString("\n\n")
	h := max(12, m.height-6)
	w := max(60, m.width-2)
	end := min(len(m.lessonLines), m.lessonOffset+h)
	if m.lessonOffset >= len(m.lessonLines) {
		m.lessonOffset = max(0, len(m.lessonLines)-1)
	}
	for i := m.lessonOffset; i < end; i++ {
		line := m.lessonLines[i]
		if len(line) > w {
			line = line[:w-1] + "…"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(m.lessonLines) == 0 {
		b.WriteString(dimStyle.Render("(empty)"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s\n", dimStyle.Render(fmt.Sprintf("lines %d-%d / %d · j/k scroll · n/p section · esc back",
		m.lessonOffset+1, end, len(m.lessonLines))))
	return b.String()
}

func (m Model) currentTaskIsResponse() bool {
	idx := m.snap.CurrentTaskIdx
	if idx < 0 || idx >= len(m.snap.Tasks) {
		return false
	}
	return contracts.TaskKindOrDefault(m.snap.Tasks[idx]) == contracts.TaskKindShortResponse
}

func (m Model) currentResponseTask() *contracts.Task {
	idx := m.snap.CurrentTaskIdx
	if idx < 0 || idx >= len(m.snap.Tasks) {
		return nil
	}
	t := m.snap.Tasks[idx]
	if contracts.TaskKindOrDefault(t) != contracts.TaskKindShortResponse {
		return nil
	}
	return &t
}

func (m Model) saveDraftCmd() tea.Cmd {
	t := m.currentResponseTask()
	if t == nil || m.sess == nil {
		return nil
	}
	body := m.respInput.Value()
	taskID := t.ID
	sess := m.sess
	return func() tea.Msg {
		_ = sess.SaveResponseDraft(context.Background(), taskID, body)
		return nil
	}
}

type responseDoneMsg struct {
	snap engine.SessionSnapshot
	err  error
}

func (m Model) submitResponseCmd() tea.Cmd {
	t := m.currentResponseTask()
	if t == nil || m.sess == nil {
		return nil
	}
	body := m.respInput.Value()
	taskID := t.ID
	sess := m.sess
	return func() tea.Msg {
		err := sess.SubmitResponse(context.Background(), taskID, body)
		return responseDoneMsg{snap: sess.Snapshot(), err: err}
	}
}

func (m Model) viewPairing() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Primer student — pair device"))
	b.WriteString("\n\n")
	b.WriteString("This workstation is not paired yet.\n")
	b.WriteString("Ask a parent for a pairing code, then enter it below.\n\n")
	b.WriteString(m.pairInput.View())
	b.WriteString("\n")
	if m.pairing {
		b.WriteString(dimStyle.Render("Pairing…"))
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString(errStyle.Render("error: " + m.err))
		b.WriteString("\n")
	}
	if m.status != "" {
		b.WriteString(dimStyle.Render(m.status))
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("\nenter submit · q quit (when empty)"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewQueue() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Primer student — work queue"))
	b.WriteString("\n")
	b.WriteString(m.syncLine())
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(errStyle.Render("error: " + m.err))
		b.WriteString("\n")
	}
	if m.status != "" {
		b.WriteString(dimStyle.Render(m.status))
		b.WriteString("\n")
	}
	if m.busy {
		b.WriteString(dimStyle.Render("Opening…"))
		b.WriteString("\n")
	}
	if len(m.workList.Items()) == 0 {
		b.WriteString("\nNo assignments yet. Press r to sync.\n")
	} else {
		b.WriteString(m.workList.View())
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter open · r refresh · q quit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewActivity() string {
	if m.snap.Kind == contracts.KindTyping {
		return m.viewTypingActivity()
	}
	return m.viewTerminalActivity()
}

func (m Model) viewTypingActivity() string {
	s := m.snap
	var b strings.Builder
	b.WriteString(titleStyle.Render("Primer student — " + s.ActivityTitle))
	b.WriteString("\n")
	b.WriteString(m.syncLine())
	b.WriteString("\n")

	instr := strings.TrimSpace(s.Instructions)
	if s.CurrentTaskIdx >= 0 && s.CurrentTaskIdx < len(s.Tasks) {
		t := s.Tasks[s.CurrentTaskIdx]
		instr = fmt.Sprintf("%s\n\n%s", t.Title, strings.TrimSpace(t.Instructions))
	} else if s.RequiredPassed {
		instr = "All prompts complete. Thresholds met — press enter or ctrl+s to finish."
	}
	if s.TutorHint != "" {
		instr += "\n\nHint: " + s.TutorHint
	}

	var metrics strings.Builder
	fmt.Fprintf(&metrics, "Checks %d/%d", s.ChecksPassed, s.ChecksTotal)
	if s.RequiredPassed {
		metrics.WriteString("  ")
		metrics.WriteString(okStyle.Render("REQUIRED PASS"))
	} else {
		metrics.WriteString("  ")
		metrics.WriteString(warnStyle.Render("incomplete"))
	}
	metrics.WriteString("\n")
	if ty := s.Typing; ty != nil {
		fmt.Fprintf(&metrics, "WPM  %.1f\n", ty.WPM)
		fmt.Fprintf(&metrics, "Acc  %.0f%%\n", ty.Accuracy*100)
		fmt.Fprintf(&metrics, "Done %d/%d\n", ty.TotalPrompts-ty.RemainingPrompts, ty.TotalPrompts)
		if ty.IncorrectChars > 0 {
			fmt.Fprintf(&metrics, "Miss %d\n", ty.IncorrectChars)
		}
	}
	for _, c := range s.Checks {
		mark := "·"
		style := dimStyle
		if c.Passed {
			mark = "✓"
			style = okStyle
		} else if !c.Optional {
			mark = "✗"
			style = errStyle
		}
		fmt.Fprintf(&metrics, "%s %s\n", style.Render(mark), c.ID)
	}

	w := max(60, m.width)
	leftW := max(30, w*3/5)
	rightW := max(20, w-leftW-4)
	b.WriteString(lipgloss.JoinHorizontal(
		lipgloss.Top,
		panelStyle.Width(leftW).Render(instr),
		" ",
		panelStyle.Width(rightW).Render(strings.TrimRight(metrics.String(), "\n")),
	))
	b.WriteString("\n")

	prompt := ""
	input := ""
	if ty := s.Typing; ty != nil {
		prompt = ty.PromptText
		input = ty.Input
		if ty.Done {
			prompt = "(all prompts complete)"
		}
	}
	body := fmt.Sprintf("Prompt:\n  %s\n\nYou typed:\n  %s█", prompt, input)
	b.WriteString(panelStyle.Width(max(40, w-4)).Render(body))
	b.WriteString("\n")
	if m.busy {
		b.WriteString(dimStyle.Render("Working…"))
		b.WriteString("\n")
	}
	if m.activityMsg != "" {
		b.WriteString(statusStyle.Render(m.activityMsg))
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("type to match prompt · backspace · ctrl+s complete · ctrl+g back"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewTerminalActivity() string {
	s := m.snap
	var b strings.Builder
	b.WriteString(titleStyle.Render("Primer student — " + s.ActivityTitle))
	b.WriteString("\n")
	b.WriteString(m.syncLine())
	b.WriteString("\n")

	// Instructions / current task
	taskTitle := "Activity"
	taskBody := strings.TrimSpace(s.Instructions)
	if s.CurrentTaskIdx >= 0 && s.CurrentTaskIdx < len(s.Tasks) {
		t := s.Tasks[s.CurrentTaskIdx]
		taskTitle = t.Title
		taskBody = strings.TrimSpace(t.Instructions)
		if contracts.TaskKindOrDefault(t) == contracts.TaskKindShortResponse && t.Response != nil {
			taskBody = strings.TrimSpace(t.Response.Prompt)
			if taskBody == "" {
				taskBody = strings.TrimSpace(t.Instructions)
			}
			taskBody += "\n\n(Write your own words. Tutor text cannot be submitted as evidence.)"
		}
	} else if s.RequiredPassed {
		taskTitle = "All tasks complete"
		taskBody = "Required checks passed. Press ctrl+s (or complete) to finish."
	}
	instr := fmt.Sprintf("%s\n\n%s", taskTitle, taskBody)
	if len(s.Blocks) > 0 {
		instr += fmt.Sprintf("\n\n[ctrl+l lesson · %d blocks]", len(s.Blocks))
	}
	if s.TutorHint != "" {
		instr += "\n\nHint: " + s.TutorHint
	}

	// Checks
	var checks strings.Builder
	fmt.Fprintf(&checks, "Checks %d/%d", s.ChecksPassed, s.ChecksTotal)
	if s.RequiredPassed {
		checks.WriteString("  ")
		checks.WriteString(okStyle.Render("REQUIRED PASS"))
	} else {
		checks.WriteString("  ")
		checks.WriteString(warnStyle.Render("incomplete"))
	}
	checks.WriteString("\n")
	for _, c := range s.Checks {
		mark := "·"
		style := dimStyle
		if c.Passed {
			mark = "✓"
			style = okStyle
		} else if !c.Optional {
			mark = "✗"
			style = errStyle
		}
		fmt.Fprintf(&checks, "%s %s\n", style.Render(mark), c.ID)
	}

	w := max(60, m.width)
	h := max(16, m.height)

	// Three-pane when PTY is available: left terminal, top-right instructions+checks, bottom-right chat.
	if s.HasTerminal {
		leftW := max(30, w*55/100)
		rightW := max(22, w-leftW-3)
		termH := max(8, h-6)
		instrH := max(4, termH*55/100)
		chatH := max(3, termH-instrH-1)

		termBorder := idleBorder
		instrBorder := idleBorder
		chatBorder := idleBorder
		termTitle := " Terminal "
		instrTitle := " Instructions "
		chatTitle := " Tutor "
		switch m.activePane {
		case paneTerminal:
			termBorder = focusBorder
			termTitle = " Terminal (active) "
		case paneInstructions:
			instrBorder = focusBorder
			instrTitle = " Instructions (active) "
		case paneChat:
			chatBorder = focusBorder
			chatTitle = " Tutor (active) "
		}

		screen := m.termScreen
		if screen == "" {
			screen = s.TerminalScreen
		}
		if screen == "" {
			screen = dimStyle.Render("(starting shell…)")
		} else {
			// Cap displayed lines to pane height.
			lines := strings.Split(screen, "\n")
			maxLines := max(3, int(termH)-3)
			if len(lines) > maxLines {
				lines = lines[len(lines)-maxLines:]
			}
			// Trim trailing spaces per line for cleaner view.
			for i, ln := range lines {
				lines[i] = strings.TrimRight(ln, " \t")
			}
			screen = strings.Join(lines, "\n")
		}

		termPanel := panelStyle.
			BorderForeground(termBorder).
			Width(leftW).
			Height(termH).
			Render(termTitle + "\n" + screen)

		rightTop := panelStyle.
			BorderForeground(instrBorder).
			Width(rightW).
			Height(instrH).
			Render(instrTitle + "\n" + instr + "\n\n" + strings.TrimRight(checks.String(), "\n"))

		chatBody := strings.Join(m.chatLog, "\n")
		if chatBody == "" {
			chatBody = dimStyle.Render("ctrl+h for a hint")
		}
		rightBot := panelStyle.
			BorderForeground(chatBorder).
			Width(rightW).
			Height(chatH).
			Render(chatTitle + "\n" + chatBody)

		rightCol := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightBot)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, termPanel, " ", rightCol))
		b.WriteString("\n")

		if m.activePane == paneInstructions {
			if m.currentTaskIsResponse() {
				if m.snap.ResponseDraft != "" && m.respInput.Value() == "" {
					m.respInput.SetValue(m.snap.ResponseDraft)
				}
				b.WriteString(m.respInput.View())
				b.WriteString("\n")
				b.WriteString(dimStyle.Render("ctrl+enter submit response · drafts save locally"))
				b.WriteString("\n")
			} else {
				b.WriteString(m.cmdInput.View())
				b.WriteString("\n")
			}
		}
	} else {
		// Legacy two-panel + command box when no PTY.
		leftW := max(30, w*3/5)
		rightW := max(20, w-leftW-4)
		left := panelStyle.Width(leftW).Render(instr)
		right := panelStyle.Width(rightW).Render(strings.TrimRight(checks.String(), "\n"))
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
		b.WriteString("\n")

		cwd := s.RelCwd
		if cwd == "" {
			cwd = "."
		}
		fmt.Fprintf(&b, "cwd: %s  commands: %d\n", cwd, s.CommandsRun)
		out := s.LastOutput
		if out == "" {
			out = dimStyle.Render("(no output yet)")
		} else {
			lines := strings.Split(out, "\n")
			if len(lines) > 8 {
				lines = lines[len(lines)-8:]
			}
			out = strings.Join(lines, "\n")
		}
		b.WriteString(panelStyle.Width(max(40, w-4)).Render(out))
		b.WriteString("\n")
		if m.busy {
			b.WriteString(dimStyle.Render("Running…"))
			b.WriteString("\n")
		} else {
			b.WriteString(m.cmdInput.View())
			b.WriteString("\n")
		}
	}

	if m.activityMsg != "" {
		b.WriteString(statusStyle.Render(m.activityMsg))
		b.WriteString("\n")
	}
	if s.LastError != "" {
		b.WriteString(errStyle.Render(s.LastError))
		b.WriteString("\n")
	}
	if s.HasTerminal {
		b.WriteString(dimStyle.Render("tab focus · ctrl+l lesson · ctrl+v verify · ctrl+s complete · ctrl+h hint · ctrl+g back"))
	} else {
		b.WriteString(dimStyle.Render("enter run · ctrl+l lesson · verify/complete/hint · ctrl+g back"))
	}
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewSummary() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.summaryTitle))
	b.WriteString("\n\n")
	b.WriteString(m.summaryBody)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter / q  return to work queue"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) syncLine() string {
	label := string(m.syncSt)
	if label == "" {
		label = "idle"
	}
	var style lipgloss.Style
	switch m.syncSt {
	case sync.StatusOnline:
		style = okStyle
	case sync.StatusOffline, sync.StatusAwaiting:
		style = warnStyle
	case sync.StatusSyncing:
		style = statusStyle
	case sync.StatusRevoked:
		style = errStyle
	default:
		style = dimStyle
	}
	return "sync: " + style.Render(label)
}

// --- list item ---------------------------------------------------------------

type workItem struct {
	it studentapi.WorkItem
}

func (w workItem) Title() string {
	t := w.it.Activity.Title
	if t == "" {
		t = w.it.Activity.Slug
	}
	return t
}

func (w workItem) Description() string {
	return fmt.Sprintf("%s · %s", w.it.Activity.Slug, w.it.Assignment.State)
}

func (w workItem) FilterValue() string {
	return w.Title() + " " + w.it.Activity.Slug
}

// ScreenName returns a stable name for tests.
func (m Model) ScreenName() string {
	switch m.screen {
	case ScreenBoot:
		return "boot"
	case ScreenPairing:
		return "pairing"
	case ScreenQueue:
		return "queue"
	case ScreenActivity:
		return "activity"
	case ScreenLesson:
		return "lesson"
	case ScreenSummary:
		return "summary"
	default:
		return "unknown"
	}
}
