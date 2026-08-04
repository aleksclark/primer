package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdsync "sync"
	"time"

	"github.com/google/uuid"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal/ptyterm"
	"github.com/aleksclark/primer/server/internal/studentclient/typing"
)

// CheckStatus is one check row for the activity TUI.
type CheckStatus = activities.CheckStatus

// TypingSnapshot is the typing-mode portion of a session view.
type TypingSnapshot = activities.TypingSnapshot

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

// Session is an interactive activity session driven by the TUI.
// Kind-specific logic lives on activities.Runner; Session owns PTY, outbox,
// completion, and durable runner_state persistence.
type Session struct {
	mu stdsync.Mutex

	eng     *Engine
	item    studentapi.WorkItem
	content contracts.ActivityContent
	digest  string
	kind    string
	runner  activities.Runner

	clientSessionID string
	serverSessionID string
	workspace       string

	pty *ptyterm.Terminal

	message          string
	completed        bool
	completionQueued bool
	completionAcked  bool
	offline          bool
	syncStatus       sync.Status
	tutorHint        string
	// lastPTYInput tracks whether recent WriteTerminal ended with newline (idle poll).
	lastPTYInputAt time.Time
	pendingVerify  bool
	// restored is true when this session resumed durable runner state (no re-emit of start events).
	restored bool
}

// OpenSession loads work, prepares the activity runner (terminal fixtures or
// typing session), starts a local (and optional server) session, and returns
// an interactive Session for the TUI.
//
// If a non-completed session already exists for the assignment with durable
// runner state, it is restored instead of starting fresh (no double-count events).
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

	kind, err := activities.ResolveKind(item.Activity.Kind, content)
	if err != nil {
		return nil, fmt.Errorf("activity %s: %w", item.Activity.Slug, err)
	}
	if !activities.Supports(kind) {
		return nil, fmt.Errorf("%w: %q (supported: %v, runnerVersion=%s)",
			activities.ErrUnsupportedKind, kind, activities.SupportedKinds(), activities.RunnerVersion)
	}

	// Prefer resuming an open session for this assignment.
	if existing, err := e.opts.Store.FindOpenSessionByAssignment(ctx, item.Assignment.ID); err == nil && existing != nil {
		if rs, rerr := e.opts.Store.GetRunnerState(ctx, existing.ClientSessionID); rerr == nil && rs != nil {
			sess, rerr := e.resumeSession(ctx, item, content, kind, existing, rs)
			if rerr == nil {
				return sess, nil
			}
			// Fall through to fresh open if resume fails (corrupt state, missing ws, …).
			e.status.LastError = "resume failed: " + rerr.Error()
		}
	}

	return e.openFreshSession(ctx, item, content, kind)
}

func (e *Engine) openFreshSession(ctx context.Context, item *studentapi.WorkItem, content contracts.ActivityContent, kind string) (*Session, error) {
	wsRoot := e.opts.WorkspaceRoot
	var err error
	if wsRoot == "" {
		wsRoot, err = os.MkdirTemp("", "primer-ws-*")
		if err != nil {
			return nil, err
		}
	}
	var workspace string
	switch kind {
	case contracts.KindTerminal:
		if content.Terminal != nil {
			e.activeRuntimeProfile = content.Terminal.RuntimeProfile
		}
		if e.opts.RuntimeProfile != "" {
			e.activeRuntimeProfile = e.opts.RuntimeProfile
		}
		workspace = filepath.Join(wsRoot, "exercise-"+item.Assignment.ID[:8])
	case contracts.KindTyping:
		workspace = filepath.Join(wsRoot, "typing-"+item.Assignment.ID[:8])
	default:
		workspace = filepath.Join(wsRoot, "activity-"+item.Assignment.ID[:8])
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}

	runner, err := activities.New(kind)
	if err != nil {
		return nil, err
	}
	openOpts := activities.OpenOpts{
		Workspace:      workspace,
		Content:        content,
		Digest:         item.Revision.ContentSHA256,
		RuntimeProfile: e.runtimeProfile(),
		RunShell: func(ctx context.Context, ws, cwd, line string) (string, string, int, error) {
			return e.runCommand(ctx, ws, cwd, ScriptedCommand{Argv: []string{line}, Shell: true})
		},
	}
	if err := runner.Open(ctx, openOpts); err != nil {
		_ = runner.Close()
		return nil, err
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
		_ = runner.Close()
		return nil, err
	}

	s := &Session{
		eng:             e,
		item:            *item,
		content:         content,
		digest:          item.Revision.ContentSHA256,
		kind:            kind,
		runner:          runner,
		clientSessionID: clientSessionID,
		serverSessionID: serverSessionID,
		workspace:       workspace,
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
		_ = runner.Close()
		return nil, err
	}

	for _, task := range content.Tasks {
		if _, err := e.enqueue(ctx, clientSessionID, contracts.EventTaskViewed, map[string]any{
			"taskId": task.ID,
			"title":  task.Title,
		}); err != nil {
			_ = runner.Close()
			return nil, err
		}
	}

	// Start embedded PTY for terminal activities (interactive shell).
	if kind == contracts.KindTerminal {
		if err := s.startPTY(24, 80); err != nil {
			if !e.opts.AllowUnsandboxed {
				_ = runner.Close()
				return nil, fmt.Errorf("start terminal pty: %w", err)
			}
			s.message = "pty unavailable; command box only: " + err.Error()
		}
	}

	// Initial verify (fixtures alone may satisfy filesystem checks; typing starts incomplete).
	_ = s.runner.Verify(ctx)
	_ = s.persistRunnerState(ctx)
	return s, nil
}

func (e *Engine) resumeSession(
	ctx context.Context,
	item *studentapi.WorkItem,
	content contracts.ActivityContent,
	kind string,
	row *cache.Session,
	rs *cache.RunnerState,
) (*Session, error) {
	if rs.Kind != "" && rs.Kind != kind {
		return nil, fmt.Errorf("runner state kind %q does not match activity kind %q", rs.Kind, kind)
	}
	workspace := row.WorkspacePath
	if workspace == "" {
		return nil, fmt.Errorf("session %s has no workspace path", row.ClientSessionID)
	}
	if _, err := os.Stat(workspace); err != nil {
		return nil, fmt.Errorf("workspace missing for resume: %w", err)
	}

	if kind == contracts.KindTerminal && content.Terminal != nil {
		e.activeRuntimeProfile = content.Terminal.RuntimeProfile
	}
	if e.opts.RuntimeProfile != "" {
		e.activeRuntimeProfile = e.opts.RuntimeProfile
	}

	runner, err := activities.New(kind)
	if err != nil {
		return nil, err
	}
	openOpts := activities.OpenOpts{
		Workspace:       workspace,
		Content:         content,
		Digest:          item.Revision.ContentSHA256,
		RuntimeProfile:  e.runtimeProfile(),
		SkipMaterialize: true, // keep student filesystem mutations
		RunShell: func(ctx context.Context, ws, cwd, line string) (string, string, int, error) {
			return e.runCommand(ctx, ws, cwd, ScriptedCommand{Argv: []string{line}, Shell: true})
		},
	}
	if err := runner.Open(ctx, openOpts); err != nil {
		_ = runner.Close()
		return nil, err
	}
	if err := runner.RestoreState(rs.StateJSON); err != nil {
		_ = runner.Close()
		return nil, err
	}

	// Mark session active again.
	row.State = "started"
	if err := e.opts.Store.SaveSession(ctx, *row); err != nil {
		_ = runner.Close()
		return nil, err
	}

	offline := e.opts.Offline
	s := &Session{
		eng:             e,
		item:            *item,
		content:         content,
		digest:          item.Revision.ContentSHA256,
		kind:            kind,
		runner:          runner,
		clientSessionID: row.ClientSessionID,
		serverSessionID: row.ServerSessionID,
		workspace:       workspace,
		offline:         offline,
		syncStatus:      sync.StatusIdle,
		restored:        true,
		message:         "resumed session",
	}
	if offline {
		s.syncStatus = sync.StatusOffline
	}

	// Fresh PTY only — durable state does not include live shell.
	if kind == contracts.KindTerminal {
		if err := s.startPTY(24, 80); err != nil {
			if !e.opts.AllowUnsandboxed {
				_ = runner.Close()
				return nil, fmt.Errorf("start terminal pty: %w", err)
			}
			s.message = "resumed; pty unavailable: " + err.Error()
		}
	}
	// Re-verify without emitting past events.
	_ = s.runner.Verify(ctx)
	return s, nil
}

// startPTY launches an interactive shell in the session workspace.
func (s *Session) startPTY(rows, cols uint16) error {
	cwd := s.workspace
	if tr, ok := s.runner.(*activities.TerminalRunner); ok {
		if c := tr.Cwd(); c != "" {
			cwd = c
		}
	}
	cmd, err := s.eng.shellCommand(s.workspace, cwd)
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
		name := shell
		args := []string{"-i"}
		if filepath.Base(shell) == "bash" {
			args = []string{"--noprofile", "--norc", "-i"}
		}
		cmd, err := sandbox.Command(context.Background(), cfg, name, args...)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd, nil
	}

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
	rs := s.runner.Snapshot()
	tasks := make([]contracts.Task, len(s.content.Tasks))
	copy(tasks, s.content.Tasks)
	hints := make([]contracts.Hint, len(s.content.Hints))
	copy(hints, s.content.Hints)

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
		Workspace:        rs.Workspace,
		Cwd:              rs.Cwd,
		RelCwd:           rs.RelCwd,
		Objective:        s.content.Objective,
		Instructions:     s.content.Instructions,
		Tasks:            tasks,
		CurrentTaskIdx:   rs.CurrentTaskIdx,
		Checks:           rs.Checks,
		RequiredPassed:   rs.RequiredPassed,
		ChecksPassed:     rs.ChecksPassed,
		ChecksTotal:      rs.ChecksTotal,
		CommandsRun:      rs.CommandsRun,
		LastOutput:       rs.LastOutput,
		LastError:        rs.LastError,
		Message:          s.message,
		Completed:        s.completed,
		CompletionQueued: s.completionQueued,
		CompletionAcked:  s.completionAcked,
		Offline:          s.offline,
		Sync:             s.syncStatus,
		Hints:            hints,
		TutorHint:        s.tutorHint,
		Typing:           rs.Typing,
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
		s.pendingVerify = true
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
	if time.Since(s.lastPTYInputAt) < 350*time.Millisecond {
		return
	}
	s.pendingVerify = false
	screen := s.pty.ScreenPlain()
	rel := "."
	if tr, ok := s.runner.(*activities.TerminalRunner); ok {
		if r, err := filepath.Rel(s.workspace, tr.Cwd()); err == nil {
			rel = r
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.runner.HandleInput(ctx, activities.Input{
		Type: activities.InputShellResult,
		Shell: &activities.ShellResult{
			Cwd:          rel,
			Executable:   "pty-shell",
			ExitCode:     0,
			Stdout:       screen,
			CountCommand: true,
		},
	})
	// applyRunnerEvents records command_finished + checks once (no second enqueue).
	s.applyRunnerEvents(ctx)
	_ = s.persistRunnerState(ctx)
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
	if s.kind != contracts.KindTyping {
		return fmt.Errorf("TypeRune is only valid for typing activities")
	}
	if err := s.runner.HandleInput(ctx, activities.Input{Type: activities.InputKey, Rune: r}); err != nil {
		return err
	}
	s.applyRunnerEvents(ctx)
	return s.persistRunnerState(ctx)
}

// TypeBackspace removes the last typed character in a typing session.
func (s *Session) TypeBackspace(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	if s.kind != contracts.KindTyping {
		return fmt.Errorf("TypeBackspace is only valid for typing activities")
	}
	if err := s.runner.HandleInput(ctx, activities.Input{Type: activities.InputBackspace}); err != nil {
		return err
	}
	s.applyRunnerEvents(ctx)
	return s.persistRunnerState(ctx)
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
func (s *Session) RunLine(ctx context.Context, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	if s.kind == contracts.KindTyping {
		return fmt.Errorf("RunLine is not valid for typing activities")
	}
	if err := s.runner.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: line}); err != nil {
		return err
	}
	s.applyRunnerEvents(ctx)
	return s.persistRunnerState(ctx)
}

// Verify re-runs deterministic checks against the current workspace or typing metrics.
func (s *Session) Verify(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.runner.Verify(ctx); err != nil {
		return err
	}
	s.applyRunnerEvents(ctx)
	return s.persistRunnerState(ctx)
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
	_ = s.runner.Verify(ctx)
	if !s.runner.CompleteReady() {
		rs := s.runner.Snapshot()
		return fmt.Errorf("required checks not passed (%d/%d)", rs.ChecksPassed, rs.ChecksTotal)
	}

	obs := s.runner.Observations()
	completionID := uuid.NewString()
	digest := requestDigest(s.digest, obs)
	req := contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  completionID,
		RequestDigest: digest,
		Observations:  obs,
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

	// Mark session completed and drop durable runner progress.
	if row, err := s.eng.opts.Store.GetSession(ctx, s.clientSessionID); err == nil {
		row.State = "completed"
		_ = s.eng.opts.Store.SaveSession(ctx, *row)
	}
	_ = s.eng.opts.Store.DeleteRunnerState(ctx, s.clientSessionID)

	if err := s.eng.flush(ctx); err != nil {
		s.syncStatus = sync.StatusAwaiting
		s.message = "completion queued; awaiting sync"
		if s.offline || s.eng.opts.Offline {
			return nil
		}
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
	rs := s.runner.Snapshot()
	if rs.CurrentTaskIdx >= 0 && rs.CurrentTaskIdx < len(s.content.Tasks) {
		task := s.content.Tasks[rs.CurrentTaskIdx]
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
	_ = s.persistRunnerState(ctx)
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

// Close releases the runner and PTY.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pty != nil {
		_ = s.pty.Close()
		s.pty = nil
	}
	if s.runner != nil {
		return s.runner.Close()
	}
	return nil
}

// applyRunnerEvents drains one-shot emit flags from the runner and
// enqueues corresponding session events. Must hold s.mu.
func (s *Session) applyRunnerEvents(ctx context.Context) {
	var rs activities.Snapshot
	if d, ok := s.runner.(activities.EventDrainer); ok {
		rs = d.DrainEvents()
	} else {
		rs = s.runner.Snapshot()
	}
	if rs.EmitCommand && rs.LastShell != nil {
		sh := rs.LastShell
		payload := map[string]any{
			"executable": sh.Executable,
			"args":       sh.Args,
			"exitCode":   sh.ExitCode,
			"cwd":        sh.Cwd,
			"stdoutNorm": truncate(sh.Stdout, 2048),
			"stderrNorm": truncate(sh.Stderr, 1024),
		}
		_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCommandFinished, payload)
	}
	if rs.EmitSample && s.kind == contracts.KindTyping && rs.Typing != nil {
		payload := map[string]any{
			"wpm":              rs.Typing.WPM,
			"accuracy":         rs.Typing.Accuracy,
			"correctChars":     rs.Typing.CorrectChars,
			"incorrectChars":   rs.Typing.IncorrectChars,
			"completedPrompts": rs.Typing.PromptIndex,
			"totalPrompts":     rs.Typing.TotalPrompts,
			"done":             rs.Typing.Done,
			"thresholdsMet":    rs.Typing.ThresholdsMet,
		}
		if rs.Typing.PromptID != "" {
			payload["promptId"] = rs.Typing.PromptID
			payload["promptText"] = rs.Typing.PromptText
		}
		_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventTypingSample, payload)
	}
	if rs.EmitChecks {
		obs := s.runner.Observations()
		ver := terminal.VerifierVersion
		if s.kind == contracts.KindTyping {
			ver = typing.VerifierVersion
		}
		for _, o := range obs {
			checkPayload := map[string]any{
				"activityDigest":  s.digest,
				"verifierVersion": ver,
				"observation":     o,
			}
			_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCheckEvaluated, checkPayload)
		}
	}
}

// persistRunnerState encodes and stores durable runner progress. Must hold s.mu.
func (s *Session) persistRunnerState(ctx context.Context) error {
	if s.runner == nil || s.eng == nil || s.eng.opts.Store == nil {
		return nil
	}
	raw, err := s.runner.EncodeState()
	if err != nil {
		return err
	}
	return s.eng.opts.Store.SaveRunnerState(ctx, s.clientSessionID, s.kind, raw)
}

