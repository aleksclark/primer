package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	stdsync "sync"
	"time"

	"github.com/google/uuid"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal/ptyterm"
	"github.com/aleksclark/primer/server/internal/studentclient/typing"
)

// CheckStatus is one check row for the activity TUI.
type CheckStatus struct {
	ID       string `json:"id"`
	Passed   bool   `json:"passed"`
	Optional bool   `json:"optional"`
	Message  string `json:"message,omitempty"`
}

// TypingSnapshot is the typing-mode portion of a session view.
type TypingSnapshot struct {
	PromptID         string  `json:"promptId,omitempty"`
	PromptText       string  `json:"promptText,omitempty"`
	Input            string  `json:"input,omitempty"`
	PromptIndex      int     `json:"promptIndex"`
	TotalPrompts     int     `json:"totalPrompts"`
	RemainingPrompts int     `json:"remainingPrompts"`
	WPM              float64 `json:"wpm"`
	Accuracy         float64 `json:"accuracy"`
	CorrectChars     int     `json:"correctChars"`
	IncorrectChars   int     `json:"incorrectChars"`
	Done             bool    `json:"done"`
	ThresholdsMet    bool    `json:"thresholdsMet"`
}

// SessionSnapshot is a read-only view of an interactive activity session.
type SessionSnapshot struct {
	AssignmentID     string           `json:"assignmentId"`
	ActivitySlug     string           `json:"activitySlug"`
	ActivityTitle    string           `json:"activityTitle"`
	Kind             string           `json:"kind"` // terminal | typing
	ClientSessionID  string           `json:"clientSessionId"`
	ServerSessionID  string           `json:"serverSessionId,omitempty"`
	Workspace        string           `json:"workspace,omitempty"`
	Cwd              string           `json:"cwd,omitempty"`    // absolute path (terminal)
	RelCwd           string           `json:"relCwd,omitempty"` // path relative to workspace (terminal)
	Objective        string           `json:"objective,omitempty"`
	Instructions     string           `json:"instructions,omitempty"`
	Tasks            []contracts.Task `json:"tasks,omitempty"`
	CurrentTaskIdx   int              `json:"currentTaskIdx"`
	Checks           []CheckStatus    `json:"checks,omitempty"`
	RequiredPassed   bool             `json:"requiredPassed"`
	ChecksPassed     int              `json:"checksPassed"`
	ChecksTotal      int              `json:"checksTotal"`
	CommandsRun      int              `json:"commandsRun"`
	LastOutput       string           `json:"lastOutput,omitempty"`
	LastError        string           `json:"lastError,omitempty"`
	Message          string           `json:"message,omitempty"`
	Completed        bool             `json:"completed"`
	CompletionQueued bool             `json:"completionQueued"`
	CompletionAcked  bool             `json:"completionAcked"`
	Offline          bool             `json:"offline"`
	Sync             sync.Status      `json:"sync,omitempty"`
	Hints            []contracts.Hint `json:"hints,omitempty"`
	TutorHint        string           `json:"tutorHint,omitempty"`
	Typing           *TypingSnapshot  `json:"typing,omitempty"`
	// TerminalScreen is the bounded PTY scrollback when a live PTY is attached.
	TerminalScreen string `json:"terminalScreen,omitempty"`
	// HasTerminal is true when a PTY shell is available for this session.
	HasTerminal bool `json:"hasTerminal,omitempty"`
}

// Session is an interactive terminal or typing activity session driven by the TUI.
type Session struct {
	mu stdsync.Mutex

	eng     *Engine
	item    studentapi.WorkItem
	content contracts.ActivityContent
	digest  string
	kind    string

	clientSessionID string
	serverSessionID string
	workspace       string
	cwd             string

	lastShell *terminal.ShellState
	typing    *typing.Session
	pty       *ptyterm.Terminal
	obs       []contracts.Observation
	checks    []CheckStatus

	commandsRun      int
	requiredPassed   bool
	checksPassed     int
	lastOutput       string
	lastError        string
	message          string
	completed        bool
	completionQueued bool
	completionAcked  bool
	offline          bool
	syncStatus       sync.Status
	tutorHint        string
	currentTaskIdx   int
	// lastPTYInput tracks whether recent WriteTerminal ended with newline (idle poll).
	lastPTYInputAt time.Time
	pendingVerify  bool
}

// OpenSession loads work, prepares the activity runner (terminal fixtures or
// typing session), starts a local (and optional server) session, and returns
// an interactive Session for the TUI.
func (e *Engine) OpenSession(ctx context.Context, assignmentID string) (*Session, error) {
	if !e.opts.Offline && e.opts.Sync != nil {
		res := e.SyncOnce(ctx)
		if res.Status == sync.StatusRevoked {
			return nil, fmt.Errorf("device revoked: %w", res.Err)
		}
	}

	item, err := e.opts.Store.GetWork(ctx, assignmentID)
	if err != nil {
		if !e.opts.Offline && e.opts.Client != nil {
			if res := e.SyncOnce(ctx); res.Err == nil {
				item, err = e.opts.Store.GetWork(ctx, assignmentID)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("load assignment %s: %w", assignmentID, err)
		}
	}

	content, err := cache.DecodeActivityContent(item.Revision.Content)
	if err != nil {
		return nil, err
	}

	kind := item.Activity.Kind
	if kind == "" {
		switch {
		case content.Terminal != nil:
			kind = contracts.KindTerminal
		case content.Typing != nil:
			kind = contracts.KindTyping
		default:
			return nil, fmt.Errorf("activity %s has no terminal or typing content", item.Activity.Slug)
		}
	}

	var workspace string
	var cwd string
	var typingSess *typing.Session

	switch kind {
	case contracts.KindTerminal:
		if content.Terminal == nil {
			return nil, fmt.Errorf("activity %s is kind terminal but has no terminal content", item.Activity.Slug)
		}
		e.activeRuntimeProfile = content.Terminal.RuntimeProfile
		if e.opts.RuntimeProfile != "" {
			e.activeRuntimeProfile = e.opts.RuntimeProfile
		}
		wsRoot := e.opts.WorkspaceRoot
		if wsRoot == "" {
			wsRoot, err = os.MkdirTemp("", "primer-ws-*")
			if err != nil {
				return nil, err
			}
		}
		workspace = filepath.Join(wsRoot, "exercise-"+item.Assignment.ID[:8])
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return nil, err
		}
		if err := terminal.Materialize(workspace, content.Terminal.Fixtures); err != nil {
			return nil, fmt.Errorf("materialize fixtures: %w", err)
		}
		cwd = workspace
		if content.Terminal.InitialCwd != "" {
			joined, err := contracts.JoinUnder(workspace, content.Terminal.InitialCwd)
			if err != nil {
				return nil, err
			}
			cwd = joined
		}
	case contracts.KindTyping:
		if content.Typing == nil {
			return nil, fmt.Errorf("activity %s is kind typing but has no typing content", item.Activity.Slug)
		}
		typingSess, err = typing.NewSession(content.Typing, content.Checks)
		if err != nil {
			return nil, fmt.Errorf("typing session: %w", err)
		}
		// No sandbox workspace for typing; keep an empty marker path for cache.
		wsRoot := e.opts.WorkspaceRoot
		if wsRoot == "" {
			wsRoot, err = os.MkdirTemp("", "primer-ws-*")
			if err != nil {
				return nil, err
			}
		}
		workspace = filepath.Join(wsRoot, "typing-"+item.Assignment.ID[:8])
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return nil, err
		}
		cwd = workspace
	default:
		return nil, fmt.Errorf("unsupported activity kind %q", kind)
	}

	clientSessionID := uuid.NewString()
	sessRow := cache.Session{
		ClientSessionID:    clientSessionID,
		AssignmentID:       item.Assignment.ID,
		ActivityRevisionID: item.Revision.ID,
		State:              "started",
		LastAckedSequence:  -1,
		NextSequence:       0,
		WorkspacePath:      workspace,
	}

	offline := e.opts.Offline
	var serverSessionID string
	if !e.opts.Offline && e.opts.Client != nil {
		if tok, _ := e.opts.Store.DeviceToken(ctx); tok != "" {
			e.opts.Client.SetToken(tok)
		}
		serverSess, err := e.opts.Client.StartSession(ctx, clientSessionID, item.Assignment.ID)
		if err != nil {
			offline = true
		} else {
			serverSessionID = serverSess.ID
			sessRow.ServerSessionID = serverSess.ID
		}
	}
	if err := e.opts.Store.SaveSession(ctx, sessRow); err != nil {
		return nil, err
	}

	s := &Session{
		eng:             e,
		item:            *item,
		content:         content,
		digest:          item.Revision.ContentSHA256,
		kind:            kind,
		clientSessionID: clientSessionID,
		serverSessionID: serverSessionID,
		workspace:       workspace,
		cwd:             cwd,
		typing:          typingSess,
		offline:         offline,
		syncStatus:      sync.StatusIdle,
	}
	if offline {
		s.syncStatus = sync.StatusOffline
		s.message = "working offline"
	}

	if _, err := e.enqueue(ctx, clientSessionID, contracts.EventSessionStarted, map[string]any{
		"assignmentId": item.Assignment.ID,
		"activitySlug": item.Activity.Slug,
		"revisionId":   item.Revision.ID,
		"kind":         kind,
	}); err != nil {
		return nil, err
	}

	for _, task := range content.Tasks {
		if _, err := e.enqueue(ctx, clientSessionID, contracts.EventTaskViewed, map[string]any{
			"taskId": task.ID,
			"title":  task.Title,
		}); err != nil {
			return nil, err
		}
	}

	// Start embedded PTY for terminal activities (interactive shell).
	// Failures are non-fatal when AllowUnsandboxed is set so RunLine tests still work;
	// production expects a live PTY and surfaces the error.
	if kind == contracts.KindTerminal {
		if err := s.startPTY(24, 80); err != nil {
			if !e.opts.AllowUnsandboxed {
				return nil, fmt.Errorf("start terminal pty: %w", err)
			}
			s.message = "pty unavailable; command box only: " + err.Error()
		}
	}

	// Initial verify (fixtures alone may satisfy filesystem checks; typing starts incomplete).
	s.reverify(ctx, nil)
	return s, nil
}

// startPTY launches an interactive shell in the session workspace.
// Prefer sandbox.bwrap when available; otherwise plain sh/bash in the workspace Dir
// (only when AllowUnsandboxed — PTY shell is the interface, not arbitrary host commands).
func (s *Session) startPTY(rows, cols uint16) error {
	cmd, err := s.eng.shellCommand(s.workspace, s.cwd)
	if err != nil {
		return err
	}
	term, err := ptyterm.Start(ptyterm.Options{Cmd: cmd, Rows: rows, Cols: cols})
	if err != nil {
		return err
	}
	s.pty = term
	return nil
}

// shellCommand builds the *exec.Cmd for an interactive PTY shell.
func (e *Engine) shellCommand(workspace, cwd string) (*exec.Cmd, error) {
	useSandbox := e.opts.UseSandbox
	if useSandbox && !sandbox.Available() {
		if !e.opts.AllowUnsandboxed {
			return nil, sandbox.ErrUnavailable
		}
		useSandbox = false
	}

	shell := "sh"
	if p, err := exec.LookPath("bash"); err == nil {
		shell = p
	} else if p, err := exec.LookPath("sh"); err == nil {
		shell = p
	}

	if useSandbox {
		rel, relErr := filepath.Rel(workspace, cwd)
		workDir := "/workspace"
		if relErr == nil && rel != "." && rel != "" {
			workDir = filepath.Join("/workspace", rel)
		}
		cfg := sandbox.Config{Workspace: workspace, WorkDir: workDir}
		if p := e.runtimeProfile(); p != "" {
			if perr := sandbox.ApplyProfile(&cfg, p); perr != nil {
				return nil, perr
			}
		}
		// Interactive shell inside bwrap.
		name := shell
		args := []string{"-i"}
		if filepath.Base(shell) == "bash" {
			args = []string{"--noprofile", "--norc", "-i"}
		}
		cmd, err := sandbox.Command(context.Background(), cfg, name, args...)
		if err != nil {
			return nil, err
		}
		// PTY manages stdio; clear any inherited pipes.
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd, nil
	}

	// Plain shell in workspace directory (tests without bwrap).
	var cmd *exec.Cmd
	if filepath.Base(shell) == "bash" {
		cmd = exec.Command(shell, "--noprofile", "--norc", "-i")
	} else {
		cmd = exec.Command(shell, "-i")
	}
	cmd.Dir = cwd
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/local/bin",
		"HOME=" + workspace,
		"TERM=xterm-256color",
		"PS1=$ ",
	}
	return cmd, nil
}

// Snapshot returns a copy of the current session state for rendering.
func (s *Session) Snapshot() SessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Session) snapshotLocked() SessionSnapshot {
	checks := make([]CheckStatus, len(s.checks))
	copy(checks, s.checks)
	tasks := make([]contracts.Task, len(s.content.Tasks))
	copy(tasks, s.content.Tasks)
	hints := make([]contracts.Hint, len(s.content.Hints))
	copy(hints, s.content.Hints)

	rel := "."
	if s.workspace != "" && s.cwd != "" {
		if r, err := filepath.Rel(s.workspace, s.cwd); err == nil {
			rel = r
		}
	}
	title := s.item.Activity.Title
	if title == "" {
		title = s.item.Activity.Slug
	}
	snap := SessionSnapshot{
		AssignmentID:     s.item.Assignment.ID,
		ActivitySlug:     s.item.Activity.Slug,
		ActivityTitle:    title,
		Kind:             s.kind,
		ClientSessionID:  s.clientSessionID,
		ServerSessionID:  s.serverSessionID,
		Workspace:        s.workspace,
		Cwd:              s.cwd,
		RelCwd:           rel,
		Objective:        s.content.Objective,
		Instructions:     s.content.Instructions,
		Tasks:            tasks,
		CurrentTaskIdx:   s.currentTaskIdx,
		Checks:           checks,
		RequiredPassed:   s.requiredPassed,
		ChecksPassed:     s.checksPassed,
		ChecksTotal:      len(s.checks),
		CommandsRun:      s.commandsRun,
		LastOutput:       s.lastOutput,
		LastError:        s.lastError,
		Message:          s.message,
		Completed:        s.completed,
		CompletionQueued: s.completionQueued,
		CompletionAcked:  s.completionAcked,
		Offline:          s.offline,
		Sync:             s.syncStatus,
		Hints:            hints,
		TutorHint:        s.tutorHint,
	}
	if s.typing != nil {
		m := s.typing.Metrics()
		id, text := s.typing.CurrentPrompt()
		snap.Typing = &TypingSnapshot{
			PromptID:         id,
			PromptText:       text,
			Input:            s.typing.CurrentInput(),
			PromptIndex:      s.typing.PromptIndex(),
			TotalPrompts:     m.TotalPrompts,
			RemainingPrompts: s.typing.RemainingPrompts(),
			WPM:              m.WPM,
			Accuracy:         m.Accuracy,
			CorrectChars:     m.CorrectChars,
			IncorrectChars:   m.IncorrectChars,
			Done:             m.Done,
			ThresholdsMet:    m.ThresholdsMet,
		}
	}
	if s.pty != nil && s.pty.Alive() {
		snap.HasTerminal = true
		snap.TerminalScreen = s.pty.ScreenContent()
		if snap.TerminalScreen != "" {
			snap.LastOutput = truncate(snap.TerminalScreen, 4000)
		}
	}
	return snap
}

// WriteTerminal sends raw bytes to the live PTY (keystrokes).
// After a newline, schedules a poll-based VerifyAll once the shell is idle.
func (s *Session) WriteTerminal(ctx context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	if s.kind == contracts.KindTyping {
		return fmt.Errorf("WriteTerminal is not valid for typing activities")
	}
	if s.pty == nil || !s.pty.Alive() {
		return fmt.Errorf("no live terminal")
	}
	if _, err := s.pty.Write(data); err != nil {
		return err
	}
	s.lastPTYInputAt = time.Now()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		s.commandsRun++
		s.pendingVerify = true
		// Best-effort: re-verify filesystem checks after short idle.
		// Full command observation capture is Phase 10 / DEBUG trap.
		go s.idleVerify()
	}
	return nil
}

// idleVerify waits briefly after newline input then re-runs checks.
func (s *Session) idleVerify() {
	time.Sleep(400 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pendingVerify || s.completed || s.pty == nil {
		return
	}
	// If more input arrived recently, skip; another idleVerify will run.
	if time.Since(s.lastPTYInputAt) < 350*time.Millisecond {
		return
	}
	s.pendingVerify = false
	// Build a lightweight shell state from screen content for stdout checks.
	screen := s.pty.ScreenPlain()
	rel, _ := filepath.Rel(s.workspace, s.cwd)
	shell := &terminal.ShellState{
		Cwd:        rel,
		Executable: "pty-shell",
		Args:       nil,
		ExitCode:   0,
		Stdout:     screen,
	}
	s.lastShell = shell
	s.lastOutput = truncate(screen, 4000)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.reverify(ctx, shell)
	_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCommandFinished, map[string]any{
		"executable": "pty-shell",
		"args":       []string{},
		"exitCode":   0,
		"cwd":        rel,
		"source":     "pty-idle",
	})
}

// TerminalScreen returns the current PTY scrollback (empty if no PTY).
func (s *Session) TerminalScreen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pty == nil {
		return ""
	}
	return s.pty.ScreenContent()
}

// ResizeTerminal updates the PTY window size.
func (s *Session) ResizeTerminal(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pty == nil || !s.pty.Alive() {
		return fmt.Errorf("no live terminal")
	}
	return s.pty.Resize(rows, cols)
}

// CloseTerminal shuts down the PTY if present.
func (s *Session) CloseTerminal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pty != nil {
		_ = s.pty.Close()
		s.pty = nil
	}
}

// TypeRune handles one character of input in a typing session.
func (s *Session) TypeRune(ctx context.Context, r rune) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	if s.kind != contracts.KindTyping || s.typing == nil {
		return fmt.Errorf("TypeRune is only valid for typing activities")
	}
	beforeIdx := s.typing.PromptIndex()
	s.typing.TypeKey(r)
	s.reverifyTyping(ctx, beforeIdx != s.typing.PromptIndex() || s.typing.Done())
	return nil
}

// TypeBackspace removes the last typed character in a typing session.
func (s *Session) TypeBackspace(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	if s.kind != contracts.KindTyping || s.typing == nil {
		return fmt.Errorf("TypeBackspace is only valid for typing activities")
	}
	s.typing.Backspace()
	s.reverifyTyping(ctx, false)
	return nil
}

// TypeString types an entire string into a typing session (tests / automation).
func (s *Session) TypeString(ctx context.Context, text string) error {
	for _, r := range text {
		if err := s.TypeRune(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// RunLine runs one shell command line in the session workspace (via sh -c).
// Built-in "cd" is handled without spawning a process.
// Typing sessions reject RunLine — use TypeRune instead.
func (s *Session) RunLine(ctx context.Context, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	if s.kind == contracts.KindTyping {
		return fmt.Errorf("RunLine is not valid for typing activities")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Handle cd specially so subsequent commands use the new directory.
	if isCD, target := parseCD(line); isCD {
		if err := s.applyCD(target); err != nil {
			s.lastError = err.Error()
			s.lastOutput = ""
			return err
		}
		s.lastOutput = ""
		s.lastError = ""
		s.commandsRun++
		rel, _ := filepath.Rel(s.workspace, s.cwd)
		shell := &terminal.ShellState{
			Cwd:        rel,
			Executable: "cd",
			Args:       []string{target},
			ExitCode:   0,
		}
		s.lastShell = shell
		if _, err := s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCommandFinished, map[string]any{
			"executable": "cd",
			"args":       []string{target},
			"exitCode":   0,
			"cwd":        rel,
		}); err != nil {
			return err
		}
		s.reverify(ctx, shell)
		return nil
	}

	start := time.Now()
	stdout, stderr, exitCode, runErr := s.eng.runCommand(ctx, s.workspace, s.cwd, ScriptedCommand{
		Argv:  []string{line},
		Shell: true,
	})
	dur := time.Since(start).Milliseconds()
	s.commandsRun++

	out := stdout
	if stderr != "" {
		if out != "" {
			out += "\n"
		}
		out += stderr
	}
	s.lastOutput = truncate(out, 4000)
	if runErr != nil && exitCode == 0 {
		s.lastError = runErr.Error()
	} else {
		s.lastError = ""
	}

	rel, _ := filepath.Rel(s.workspace, s.cwd)
	shell := &terminal.ShellState{
		Cwd:        rel,
		Executable: "/bin/sh",
		Args:       []string{"-c", line},
		ExitCode:   exitCode,
		Stdout:     stdout,
		Stderr:     stderr,
	}
	s.lastShell = shell

	payload := map[string]any{
		"executable": "/bin/sh",
		"args":       []string{"-c", line},
		"exitCode":   exitCode,
		"cwd":        rel,
		"durationMs": dur,
		"stdoutNorm": truncate(stdout, 2048),
		"stderrNorm": truncate(stderr, 1024),
	}
	if runErr != nil && exitCode == 0 {
		payload["error"] = runErr.Error()
	}
	if _, err := s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCommandFinished, payload); err != nil {
		return err
	}
	s.reverify(ctx, shell)
	return nil
}

// Verify re-runs deterministic checks against the current workspace or typing metrics.
func (s *Session) Verify(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reverify(ctx, s.lastShell)
	return nil
}

// Kind returns the activity kind (terminal or typing).
func (s *Session) Kind() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kind
}

// Complete queues a completion intent when required checks pass and flushes sync.
func (s *Session) Complete(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return nil
	}
	s.reverify(ctx, s.lastShell)
	if !s.requiredPassed {
		return fmt.Errorf("required checks not passed (%d/%d)", s.checksPassed, len(s.checks))
	}

	completionID := uuid.NewString()
	digest := requestDigest(s.digest, s.obs)
	req := contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  completionID,
		RequestDigest: digest,
		Observations:  s.obs,
		ClientTime:    time.Now().UTC(),
		Summary:       fmt.Sprintf("student completed %s", s.item.Activity.Slug),
	}
	if err := s.eng.opts.Store.SaveCompletionIntent(ctx, s.clientSessionID, s.serverSessionID, req); err != nil {
		return err
	}
	if _, err := s.eng.enqueue(ctx, s.clientSessionID, contracts.EventSessionCompleted, map[string]any{
		"completionId": completionID,
	}); err != nil {
		return err
	}
	s.completionQueued = true
	s.completed = true
	s.message = "completion queued"

	// Flush when online.
	if err := s.eng.flush(ctx); err != nil {
		s.syncStatus = sync.StatusAwaiting
		s.message = "completion queued; awaiting sync"
		if s.offline || s.eng.opts.Offline {
			return nil
		}
		// Still success locally; outbox will retry.
		s.message = "completion queued; awaiting sync"
		return nil
	}
	if cmp, err := s.eng.opts.Store.GetCompletion(ctx, completionID); err == nil && cmp.Acked {
		s.completionAcked = true
		s.syncStatus = sync.StatusOnline
		s.message = "completed and synced"
	} else {
		s.syncStatus = sync.StatusAwaiting
		s.message = "completion queued; awaiting sync"
	}
	return nil
}

// SetTutorHint stores a coaching string shown in the session view.
func (s *Session) SetTutorHint(hint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tutorHint = hint
}

// LocalHint returns the first unused activity hint text for the current task,
// or a generic nudge when no hints are defined.
func (s *Session) LocalHint() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentTaskIdx >= 0 && s.currentTaskIdx < len(s.content.Tasks) {
		task := s.content.Tasks[s.currentTaskIdx]
		byID := map[string]string{}
		for _, h := range s.content.Hints {
			byID[h.ID] = h.Text
		}
		for _, id := range task.HintIDs {
			if t, ok := byID[id]; ok && t != "" {
				return t
			}
		}
	}
	if len(s.content.Hints) > 0 {
		return s.content.Hints[0].Text
	}
	return "Try discovering with short commands like pwd and ls before changing directories."
}

// Pause marks the session paused in the local cache (events stay durable).
// The live PTY is closed; durable session state remains for resume of task UI.
func (s *Session) Pause(ctx context.Context) error {
	s.mu.Lock()
	if s.pty != nil {
		_ = s.pty.Close()
		s.pty = nil
	}
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.eng.opts.Store.GetSession(ctx, s.clientSessionID)
	if err != nil {
		return err
	}
	row.State = "paused"
	return s.eng.opts.Store.SaveSession(ctx, *row)
}

func (s *Session) reverify(ctx context.Context, shell *terminal.ShellState) {
	if s.kind == contracts.KindTyping {
		s.reverifyTyping(ctx, false)
		return
	}
	obs := terminal.VerifyAll(s.workspace, s.content.Checks, shell)
	s.applyObservations(obs, shell != nil, terminal.VerifierVersion, ctx)
}

// reverifyTyping refreshes observations from the typing runner.
// emitSample enqueues a typing_sample (+ check_evaluated) when true.
func (s *Session) reverifyTyping(ctx context.Context, emitSample bool) {
	if s.typing == nil {
		return
	}
	obs := s.typing.Observations()
	s.applyObservations(obs, emitSample, typing.VerifierVersion, ctx)
	if emitSample {
		m := s.typing.Metrics()
		payload := map[string]any{
			"wpm":              m.WPM,
			"accuracy":         m.Accuracy,
			"correctChars":     m.CorrectChars,
			"incorrectChars":   m.IncorrectChars,
			"completedPrompts": m.CompletedPrompts,
			"totalPrompts":     m.TotalPrompts,
			"elapsedMs":        m.Elapsed.Milliseconds(),
			"done":             m.Done,
			"thresholdsMet":    m.ThresholdsMet,
		}
		if id, text := s.typing.CurrentPrompt(); id != "" {
			payload["promptId"] = id
			payload["promptText"] = text
		}
		_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventTypingSample, payload)
	}
}

func (s *Session) applyObservations(obs []contracts.Observation, enqueueChecks bool, verifierVersion string, ctx context.Context) {
	s.obs = obs
	byID := map[string]contracts.Observation{}
	passed := 0
	checks := make([]CheckStatus, 0, len(obs))
	for _, o := range obs {
		byID[o.CheckID] = o
		if o.Passed {
			passed++
		}
		checks = append(checks, CheckStatus{
			ID:       o.CheckID,
			Passed:   o.Passed,
			Optional: o.Optional,
			Message:  o.Message,
		})
	}
	s.checks = checks
	s.checksPassed = passed

	requiredOK := true
	for _, ch := range s.content.Checks {
		if ch.Optional {
			continue
		}
		o, ok := byID[ch.ID]
		if !ok || !o.Passed {
			requiredOK = false
			break
		}
	}
	for _, task := range s.content.Tasks {
		if task.Optional {
			continue
		}
		ok, _ := terminal.EvalTree(task.Completion, byID)
		if !ok {
			requiredOK = false
			break
		}
	}
	s.requiredPassed = requiredOK

	// Advance current task to first incomplete required task.
	s.currentTaskIdx = len(s.content.Tasks)
	for i, task := range s.content.Tasks {
		if task.Optional {
			continue
		}
		ok, _ := terminal.EvalTree(task.Completion, byID)
		if !ok {
			s.currentTaskIdx = i
			break
		}
	}

	if enqueueChecks {
		for _, o := range obs {
			checkPayload := map[string]any{
				"activityDigest":  s.digest,
				"verifierVersion": verifierVersion,
				"observation":     o,
			}
			_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCheckEvaluated, checkPayload)
		}
	}
}

func (s *Session) applyCD(target string) error {
	if target == "" {
		target = "."
	}
	var next string
	if filepath.IsAbs(target) {
		// Only allow absolute paths under workspace.
		rel, err := filepath.Rel(s.workspace, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("cd: path outside workspace")
		}
		next = filepath.Clean(target)
	} else {
		next = filepath.Clean(filepath.Join(s.cwd, target))
		rel, err := filepath.Rel(s.workspace, next)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("cd: path outside workspace")
		}
	}
	info, err := os.Stat(next)
	if err != nil {
		return fmt.Errorf("cd: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cd: not a directory")
	}
	s.cwd = next
	return nil
}

func parseCD(line string) (bool, string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false, ""
	}
	if fields[0] != "cd" {
		return false, ""
	}
	if len(fields) == 1 {
		return true, "."
	}
	// Join remaining for paths with spaces (quoted handling is minimal).
	return true, strings.Join(fields[1:], " ")
}
