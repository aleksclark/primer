// Package app is the Bubble Tea student client: pairing, work queue, activity
// session (command runner), and completion summary.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
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
	ScreenSummary
)

// Options configure the student TUI.
type Options struct {
	BaseURL       string
	Store         *cache.Store
	Client        *studentapi.Client
	WorkspaceRoot string
	DeviceName    string
	// AllowUnsandboxed runs commands without bubblewrap (default true for Phase 3).
	AllowUnsandboxed bool
	// Offline skips network after cache is populated.
	Offline bool
}

// Model is the root Bubble Tea model.
type Model struct {
	opts   Options
	screen Screen
	width  int
	height int
	err    string
	status string // free-form status line message
	syncSt sync.Status
	quitting bool

	// pairing
	pairInput textinput.Model
	pairing   bool

	// queue
	workList list.Model
	work     []studentapi.WorkItem

	// activity
	sess        *engine.Session
	snap        engine.SessionSnapshot
	cmdInput    textinput.Model
	busy        bool
	focusCmd    bool
	activityMsg string

	// summary
	summaryTitle string
	summaryBody  string
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
	pi.Width = 24
	pi.Prompt = "> "

	ci := textinput.New()
	ci.Placeholder = "command (e.g. ls, pwd, cd docs)"
	ci.CharLimit = 512
	ci.Width = 60
	ci.Prompt = "$ "

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
		workList:  l,
		syncSt:    sync.StatusIdle,
		focusCmd:  true,
	}
}

// Run starts the interactive TUI (blocking).
func Run(opts Options) error {
	p := tea.NewProgram(NewModel(opts), tea.WithAltScreen())
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

// --- lifecycle ---------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return m.boot
}

func (m Model) boot() tea.Msg {
	if m.opts.Store == nil {
		return bootMsg{err: fmt.Errorf("cache store is required")}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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
		return m, nil

	case bootMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.screen = ScreenPairing
			m.pairInput.Focus()
			return m, textinput.Blink
		}
		if !msg.hasToken {
			m.screen = ScreenPairing
			m.pairInput.Focus()
			m.status = "Enter the pairing code from a parent."
			return m, textinput.Blink
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
		m.screen = ScreenActivity
		m.cmdInput.SetValue("")
		if msg.snap.Kind == contracts.KindTyping {
			m.cmdInput.Blur()
			m.focusCmd = false
			m.activityMsg = "Type the prompt exactly. backspace fixes · ctrl+s complete · esc back"
			return m, nil
		}
		m.cmdInput.Focus()
		m.focusCmd = true
		m.activityMsg = "Type a command and press Enter. verify/complete/hint as commands · esc=back"
		return m, textinput.Blink

	case cmdDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.activityMsg = msg.err.Error()
		} else {
			m.activityMsg = ""
		}
		m.snap = msg.snap
		m.cmdInput.SetValue("")
		m.cmdInput.Focus()
		return m, textinput.Blink

	case verifyDoneMsg:
		m.busy = false
		m.snap = msg.snap
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
		if m.sess != nil {
			m.sess.SetTutorHint(msg.hint)
			m.snap = m.sess.Snapshot()
		}
		m.activityMsg = "Hint ready"
		return m, nil

	case tea.KeyMsg:
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
		if m.focusCmd && !m.busy {
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		// Global activity keys when not typing specials into input — use ctrl combos
		// and single keys only when we intentionally intercept.
		switch key {
		case "esc":
			if m.sess != nil {
				_ = m.sess.Pause(context.Background())
			}
			m.sess = nil
			m.screen = ScreenQueue
			m.status = "Session paused"
			return m, m.loadWorkCmd()
		case "ctrl+v":
			if m.busy || m.sess == nil {
				return m, nil
			}
			m.busy = true
			return m, m.verifyCmd()
		case "ctrl+s":
			if m.busy || m.sess == nil {
				return m, nil
			}
			if !m.snap.RequiredPassed {
				m.activityMsg = "Required checks not passed yet — run verify after commands."
				return m, nil
			}
			m.busy = true
			return m, m.completeCmd()
		case "ctrl+h":
			if m.sess == nil {
				return m, nil
			}
			return m, m.hintCmd()
		case "enter":
			if m.busy || m.sess == nil {
				return m, nil
			}
			line := strings.TrimSpace(m.cmdInput.Value())
			// Test-friendly shortcuts typed as commands:
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
				if m.sess != nil {
					_ = m.sess.Pause(context.Background())
				}
				m.sess = nil
				m.screen = ScreenQueue
				return m, m.loadWorkCmd()
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

	case ScreenSummary:
		switch key {
		case "enter", "q", "esc":
			m.sess = nil
			m.screen = ScreenQueue
			m.summaryBody = ""
			return m, tea.Batch(m.syncCmd(), m.loadWorkCmd())
		}
	}
	return m, nil
}

// --- commands ----------------------------------------------------------------

func (m Model) pairCmd(code string) tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		if opts.Client == nil {
			return pairedMsg{err: fmt.Errorf("API client not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pair, err := opts.Client.Pair(ctx, code, opts.DeviceName)
		if err != nil {
			return pairedMsg{err: err}
		}
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
		if opts.Offline || opts.Client == nil {
			return syncDoneMsg{res: sync.Result{Status: sync.StatusOffline}}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if tok, _ := opts.Store.DeviceToken(ctx); tok != "" {
			opts.Client.SetToken(tok)
		}
		loop := sync.New(opts.Client, opts.Store)
		res := loop.SyncOnce(ctx)
		return syncDoneMsg{res: res}
	}
}

func (m Model) loadWorkCmd() tea.Cmd {
	store := m.opts.Store
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		items, err := store.ListWork(ctx)
		return workLoadedMsg{items: items, err: err}
	}
}

func (m Model) openSessionCmd(assignmentID string) tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if tok, _ := opts.Store.DeviceToken(ctx); tok != "" && opts.Client != nil {
			opts.Client.SetToken(tok)
		}
		eng, err := engine.New(engine.Options{
			Client:           opts.Client,
			Store:            opts.Store,
			WorkspaceRoot:    opts.WorkspaceRoot,
			Offline:          opts.Offline,
			AllowUnsandboxed: true, // Phase 3: command runner without required bwrap
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
	sess := m.sess
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := sess.RunLine(ctx, line)
		return cmdDoneMsg{snap: sess.Snapshot(), err: err}
	}
}

func (m Model) handleTypingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		if m.sess != nil {
			_ = m.sess.Pause(context.Background())
		}
		m.sess = nil
		m.screen = ScreenQueue
		m.status = "Session paused"
		return m, m.loadWorkCmd()
	case "ctrl+s":
		if m.busy || m.sess == nil {
			return m, nil
		}
		if !m.snap.RequiredPassed {
			m.activityMsg = "Keep typing until WPM and accuracy thresholds are met."
			return m, nil
		}
		m.busy = true
		return m, m.completeCmd()
	case "ctrl+h":
		if m.sess == nil {
			return m, nil
		}
		return m, m.hintCmd()
	case "ctrl+v":
		if m.busy || m.sess == nil {
			return m, nil
		}
		m.busy = true
		return m, m.verifyCmd()
	case "enter":
		// Allow test-friendly complete when thresholds already met.
		if m.snap.RequiredPassed && !m.busy && m.sess != nil {
			m.busy = true
			return m, m.completeCmd()
		}
		return m, nil
	}

	if m.busy || m.sess == nil {
		return m, nil
	}

	// Typing input is applied synchronously so keystroke order is preserved
	// (Bubble Tea runs Cmds concurrently, which would race TypeRune).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if key == "backspace" {
		if err := m.sess.TypeBackspace(ctx); err != nil {
			m.activityMsg = err.Error()
		} else {
			m.activityMsg = ""
		}
		m.snap = m.sess.Snapshot()
		if m.snap.RequiredPassed {
			m.activityMsg = "Thresholds met — press enter or ctrl+s to finish."
		}
		return m, nil
	}

	var runes []rune
	switch msg.Type {
	case tea.KeyRunes:
		runes = msg.Runes
	case tea.KeySpace:
		runes = []rune{' '}
	case tea.KeyTab:
		runes = []rune{'\t'}
	default:
		// Fallback for single printable characters reported only via String().
		if key == " " {
			runes = []rune{' '}
		} else if rs := []rune(key); len(rs) == 1 && unicode.IsPrint(rs[0]) &&
			!strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") &&
			!strings.HasPrefix(key, "shift+") {
			runes = rs
		}
	}
	if len(runes) == 0 {
		return m, nil
	}
	for _, r := range runes {
		if r == 0 || (!unicode.IsPrint(r) && r != '\t') {
			continue
		}
		if err := m.sess.TypeRune(ctx, r); err != nil {
			m.activityMsg = err.Error()
			m.snap = m.sess.Snapshot()
			return m, nil
		}
	}
	m.snap = m.sess.Snapshot()
	m.activityMsg = ""
	if m.snap.RequiredPassed {
		m.activityMsg = "Thresholds met — press enter or ctrl+s to finish."
	}
	return m, nil
}

func (m Model) verifyCmd() tea.Cmd {
	sess := m.sess
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = sess.Verify(ctx)
		return verifyDoneMsg{snap: sess.Snapshot()}
	}
}

func (m Model) completeCmd() tea.Cmd {
	sess := m.sess
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := sess.Complete(ctx)
		return completeDoneMsg{snap: sess.Snapshot(), err: err}
	}
}

func (m Model) hintCmd() tea.Cmd {
	sess := m.sess
	opts := m.opts
	return func() tea.Msg {
		// Prefer server tutor stub when online + session id known.
		snap := sess.Snapshot()
		if !opts.Offline && opts.Client != nil && snap.ServerSessionID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
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
	m.cmdInput.Width = max(20, w-8)
	m.pairInput.Width = min(32, w-10)
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
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	switch m.screen {
	case ScreenBoot:
		return "Starting Primer student…\n"
	case ScreenPairing:
		return m.viewPairing()
	case ScreenQueue:
		return m.viewQueue()
	case ScreenActivity:
		return m.viewActivity()
	case ScreenSummary:
		return m.viewSummary()
	default:
		return ""
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

	w := m.width
	if w < 60 {
		w = 60
	}
	leftW := w * 3 / 5
	if leftW < 30 {
		leftW = 30
	}
	rightW := w - leftW - 4
	if rightW < 20 {
		rightW = 20
	}
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
	b.WriteString(dimStyle.Render("type to match prompt · backspace · ctrl+s complete · esc back"))
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
	} else if s.RequiredPassed {
		taskTitle = "All tasks complete"
		taskBody = "Required checks passed. Press complete (or type complete) to finish."
	}
	instr := fmt.Sprintf("%s\n\n%s", taskTitle, taskBody)
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

	w := m.width
	if w < 60 {
		w = 60
	}
	leftW := w * 3 / 5
	if leftW < 30 {
		leftW = 30
	}
	rightW := w - leftW - 4
	if rightW < 20 {
		rightW = 20
	}
	left := panelStyle.Width(leftW).Render(instr)
	right := panelStyle.Width(rightW).Render(strings.TrimRight(checks.String(), "\n"))
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
	b.WriteString("\n")

	// Output + command
	cwd := s.RelCwd
	if cwd == "" {
		cwd = "."
	}
	fmt.Fprintf(&b, "cwd: %s  commands: %d\n", cwd, s.CommandsRun)
	out := s.LastOutput
	if out == "" {
		out = dimStyle.Render("(no output yet)")
	} else {
		// Cap displayed lines.
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
	if m.activityMsg != "" {
		b.WriteString(statusStyle.Render(m.activityMsg))
		b.WriteString("\n")
	}
	if s.LastError != "" {
		b.WriteString(errStyle.Render(s.LastError))
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("enter run · verify/complete/hint as commands · esc back"))
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
	case ScreenSummary:
		return "summary"
	default:
		return "unknown"
	}
}
