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
	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal/observe"
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
	// Blocks are student-visible instructional blocks (parent_note excluded).
	Blocks           []contracts.InstructionBlock `json:"blocks,omitempty"`
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
	// ResponseDraft is the durable draft body for the current short_response task.
	ResponseDraft string `json:"responseDraft,omitempty"`
	// ResponseQueued is true when a conceptual response is awaiting sync.
	ResponseQueued bool `json:"responseQueued,omitempty"`
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

	// observeSpool / observeReader collect bash instrumentation events (Phase 2).
	// nil when instrumentation could not be installed (fail closed for command checks).
	observeOK     bool
	observeReader interface {
		Drain() ([]contracts.ShellEvent, error)
	}
	observeClose func() error

	message          string
	completed        bool
	completionQueued bool
	completionAcked  bool
	offline          bool
	syncStatus       sync.Status
	tutorHint        string
	// lastPTYInputAt tracks recent WriteTerminal activity for observe drain debounce.
	lastPTYInputAt time.Time
	pendingObserve bool
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

	// Capability policy is enforced locally first so offline mode and network
	// failures cannot bypass structured-command gates (Phase 0 honesty).
	caps := e.sessionCapabilities()
	capSet := map[string]bool{}
	for _, c := range caps {
		capSet[c] = true
	}
	if err := contracts.RejectIncompatibleRevision(content, capSet); err != nil {
		_ = runner.Close()
		return nil, err
	}

	offline := e.opts.Offline
	var serverSessionID string
	if !e.opts.Offline && e.opts.Client != nil {
		if tok, _ := e.opts.Store.DeviceToken(ctx); tok != "" {
			e.opts.Client.SetToken(tok)
		}
		serverSess, err := e.opts.Client.StartSession(ctx, clientSessionID, item.Assignment.ID, caps...)
		if err != nil {
			if isIncompatibleRevisionError(err) {
				_ = runner.Close()
				return nil, err
			}
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
	s.restoreSubmittedResponses(ctx)
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

	// Re-check capability policy on resume (content may have been cached offline).
	caps := e.sessionCapabilities()
	capSet := map[string]bool{}
	for _, c := range caps {
		capSet[c] = true
	}
	if err := contracts.RejectIncompatibleRevision(content, capSet); err != nil {
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
	s.restoreSubmittedResponses(ctx)
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

// startPTY launches an interactive bash shell with observe instrumentation.
// Instrumentation failure is recorded (observeOK=false) so command-sensitive
// checks fail closed; the PTY may still start for display when AllowUnsandboxed.
func (s *Session) startPTY(rows, cols uint16) error {
	cwd := s.workspace
	if tr, ok := s.runner.(*activities.TerminalRunner); ok {
		if c := tr.Cwd(); c != "" {
			cwd = c
		}
	}

	// Prefer bash; observe hooks require bash. Without bash, refuse structured path.
	bashPath, bashErr := exec.LookPath("bash")
	useObserve := bashErr == nil

	var spool *observe.Spool
	var hostEvents, hostRC string
	var sandWS string
	if useObserve {
		base := filepath.Join(s.eng.opts.WorkspaceRoot, ".primer-observe")
		if s.eng.opts.WorkspaceRoot == "" {
			base = filepath.Join(os.TempDir(), "primer-observe")
		}
		var err error
		spool, err = observe.Prepare(base)
		if err != nil {
			useObserve = false
			s.message = "observe spool unavailable: " + err.Error()
		} else {
			hostEvents = spool.EventsPath()
			hostRC = spool.RCPath()
			sandWS = "/workspace"
			// Write RC using paths the shell will see (sandbox mounts spool at /primer-observe).
			rcEvents, rcWS := hostEvents, s.workspace
			if s.eng.opts.UseSandbox && sandbox.Available() {
				rcEvents, rcWS = "/primer-observe/events.ndjson", sandWS
			}
			if err := observe.WriteBashRC(hostRC, rcEvents, rcWS); err != nil {
				_ = spool.Close()
				useObserve = false
				s.message = "observe rc unavailable: " + err.Error()
			}
		}
	}

	cmd, sandboxed, err := s.eng.shellCommand(s.workspace, cwd, bashPath, hostRC, spool)
	if err != nil {
		if spool != nil {
			_ = spool.Close()
		}
		return err
	}

	term, err := ptyterm.Start(ptyterm.Options{Cmd: cmd, Rows: rows, Cols: cols})
	if err != nil {
		if spool != nil {
			_ = spool.Close()
		}
		return err
	}
	s.pty = term

	if useObserve && spool != nil {
		reader := observe.NewReader(spool)
		reader.SessionID = s.clientSessionID
		reader.Workspace = s.workspace
		reader.RunnerVersion = activities.RunnerVersion
		reader.VerifierVersion = terminal.VerifierVersion
		if sandboxed {
			reader.SandboxWorkspace = sandWS
		}
		s.observeReader = reader
		s.observeClose = spool.Close
		s.observeOK = true
	} else {
		s.observeOK = false
		if s.message == "" {
			s.message = "bash observe instrumentation unavailable; command checks fail closed"
		}
		if spool != nil {
			_ = spool.Close()
		}
	}
	return nil
}

// shellCommand builds the *exec.Cmd for an interactive PTY shell.
// When spool is non-nil and bashPath is set, launches bash with --rcfile observe hooks.
func (e *Engine) shellCommand(workspace, cwd, bashPath, hostRC string, spool *observe.Spool) (*exec.Cmd, bool, error) {
	useSandbox := e.opts.UseSandbox
	if useSandbox && !sandbox.Available() {
		if !e.opts.AllowUnsandboxed {
			return nil, false, sandbox.ErrUnavailable
		}
		useSandbox = false
	}

	shell := bashPath
	if shell == "" {
		if p, err := exec.LookPath("bash"); err == nil {
			shell = p
		} else if p, err := exec.LookPath("sh"); err == nil {
			shell = p
		} else {
			return nil, false, fmt.Errorf("no shell found")
		}
	}

	useBashRC := spool != nil && hostRC != "" && filepath.Base(shell) == "bash"

	if useSandbox {
		rel, relErr := filepath.Rel(workspace, cwd)
		workDir := "/workspace"
		if relErr == nil && rel != "." && rel != "" {
			workDir = filepath.Join("/workspace", rel)
		}
		cfg := sandbox.Config{Workspace: workspace, WorkDir: workDir}
		if p := e.runtimeProfile(); p != "" {
			if perr := sandbox.ApplyProfile(&cfg, p); perr != nil {
				return nil, true, perr
			}
		}
		if useBashRC {
			// Bind observe spool (events + rc) read-write so bash can append events.
			cfg.ExtraBinds = append(cfg.ExtraBinds, sandbox.Bind{
				Host: spool.Dir, Dest: "/primer-observe", ReadOnly: false,
			})
			cfg.Env = append([]string{
				"PATH=/usr/bin:/bin:/usr/local/bin",
				"HOME=/workspace",
				"TERM=xterm-256color",
				"PS1=$ ",
			}, cfg.Env...)
		}
		name := shell
		var args []string
		if useBashRC {
			args = []string{"--noprofile", "--rcfile", "/primer-observe/rc.bash", "-i"}
		} else if filepath.Base(shell) == "bash" {
			args = []string{"--noprofile", "--norc", "-i"}
		} else {
			args = []string{"-i"}
		}
		cmd, err := sandbox.Command(context.Background(), cfg, name, args...)
		if err != nil {
			return nil, true, err
		}
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd, true, nil
	}

	var cmd *exec.Cmd
	if useBashRC {
		cmd = exec.Command(shell, "--noprofile", "--rcfile", hostRC, "-i")
	} else if filepath.Base(shell) == "bash" {
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
	return cmd, false, nil
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
		Blocks:           contracts.StudentBlocks(s.content.Blocks),
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
	// Surface draft for current short_response task when store is available.
	if rs.CurrentTaskIdx >= 0 && rs.CurrentTaskIdx < len(tasks) {
		t := tasks[rs.CurrentTaskIdx]
		if contracts.TaskKindOrDefault(t) == contracts.TaskKindShortResponse && s.eng != nil && s.eng.opts.Store != nil {
			if body, err := s.eng.opts.Store.GetResponseDraft(context.Background(), s.clientSessionID, t.ID); err == nil {
				snap.ResponseDraft = body
			}
			if pending, err := s.eng.opts.Store.ListPendingResponses(context.Background()); err == nil {
				for _, p := range pending {
					if p.ClientSessionID == s.clientSessionID && p.TaskID == t.ID {
						snap.ResponseQueued = true
						break
					}
				}
			}
		}
	}
	return snap
}

// SaveResponseDraft persists a local draft for a short_response task.
func (s *Session) SaveResponseDraft(ctx context.Context, taskID, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eng == nil || s.eng.opts.Store == nil {
		return fmt.Errorf("no local store")
	}
	return s.eng.opts.Store.SaveResponseDraft(ctx, s.clientSessionID, taskID, body)
}

// SubmitResponse queues an idempotent conceptual response and marks the task submitted.
// Body must be the student's own text — never tutor-generated coaching.
func (s *Session) SubmitResponse(ctx context.Context, taskID, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("response body is required")
	}
	var task *contracts.Task
	for i := range s.content.Tasks {
		if s.content.Tasks[i].ID == taskID {
			task = &s.content.Tasks[i]
			break
		}
	}
	if task == nil {
		return fmt.Errorf("unknown task %q", taskID)
	}
	if contracts.TaskKindOrDefault(*task) != contracts.TaskKindShortResponse || task.Response == nil {
		return fmt.Errorf("task %q is not a short_response task", taskID)
	}
	max := task.Response.MaxChars
	if max <= 0 {
		max = contracts.DefaultResponseMaxChars
	}
	if len([]rune(body)) > max {
		return fmt.Errorf("response exceeds %d characters", max)
	}
	if s.eng == nil || s.eng.opts.Store == nil {
		return fmt.Errorf("no local store")
	}

	req := contracts.ResponseSubmission{
		SchemaVersion: contracts.ResponseSchemaVersion,
		SubmissionID:  uuid.NewString(),
		TaskID:        taskID,
		Body:          body,
		ClientTime:    time.Now().UTC(),
	}
	if err := s.eng.opts.Store.SaveResponseIntent(ctx, s.clientSessionID, s.serverSessionID, req); err != nil {
		return err
	}
	_ = s.eng.opts.Store.SaveResponseDraft(ctx, s.clientSessionID, taskID, body)

	if tr, ok := s.runner.(*activities.TerminalRunner); ok {
		tr.MarkResponseSubmitted(taskID)
		s.applyRunnerEvents(ctx)
		_ = s.persistRunnerState(ctx)
	}

	// Best-effort immediate flush when online.
	if !s.offline && s.eng.opts.Sync != nil && s.serverSessionID != "" {
		go func() {
			_ = s.eng.SyncOnce(context.Background())
		}()
	} else {
		s.message = "Response saved — awaiting sync"
	}
	return nil
}

// ContentBlocks returns student-visible instructional blocks.
func (s *Session) ContentBlocks() []contracts.InstructionBlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	return contracts.StudentBlocks(s.content.Blocks)
}

func (s *Session) restoreSubmittedResponses(ctx context.Context) {
	if s.eng == nil || s.eng.opts.Store == nil {
		return
	}
	ids, err := s.eng.opts.Store.ListAckedResponseTaskIDs(ctx, s.clientSessionID)
	if err != nil || len(ids) == 0 {
		return
	}
	if tr, ok := s.runner.(*activities.TerminalRunner); ok {
		for id := range ids {
			tr.MarkResponseSubmitted(id)
		}
	}
}

// WriteTerminal sends raw bytes to the live PTY (keystrokes).
// On newline, schedules an observe drain — never synthesizes success from screen text.
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
		s.pendingObserve = true
		go s.drainObserveAfterIdle()
	}
	return nil
}

// drainObserveAfterIdle waits for the shell to return to a prompt, then drains
// structured observe events. Does not scrape the PTY screen into command evidence.
func (s *Session) drainObserveAfterIdle() {
	// Allow bash PROMPT_COMMAND to fire after the command completes.
	time.Sleep(500 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pendingObserve || s.completed {
		return
	}
	if time.Since(s.lastPTYInputAt) < 400*time.Millisecond {
		// More input arrived; another drain will be scheduled.
		return
	}
	s.pendingObserve = false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.drainObserveLocked(ctx)
	s.applyRunnerEvents(ctx)
	_ = s.persistRunnerState(ctx)
}

// drainObserveLocked reads new shell events and feeds them to the runner.
// Must hold s.mu.
func (s *Session) drainObserveLocked(ctx context.Context) {
	if s.observeReader == nil {
		return
	}
	events, err := s.observeReader.Drain()
	if err != nil {
		s.message = "observe drain error: " + err.Error()
		return
	}
	for i := range events {
		ev := events[i]
		// Attach workspace manifest digest around the command when feasible.
		if man, merr := terminal.CaptureManifest(s.workspace); merr == nil {
			if ev.ManifestAfter == "" {
				ev.ManifestAfter = man.Digest
			}
		}
		_ = s.runner.HandleInput(ctx, activities.Input{
			Type:  activities.InputShellResult,
			Event: &ev,
		})
	}
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

// Verify drains pending observe events then re-runs deterministic checks.
func (s *Session) Verify(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainObserveLocked(ctx)
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
	// Drain observe events before final verify so recent commands are not lost
	// when Complete is called without an intervening Verify.
	s.drainObserveLocked(ctx)
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

// Close releases the runner, PTY, and observe spool.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pty != nil {
		_ = s.pty.Close()
		s.pty = nil
	}
	if s.observeClose != nil {
		_ = s.observeClose()
		s.observeClose = nil
		s.observeReader = nil
		s.observeOK = false
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
	if rs.EmitCommand {
		if rs.LastEvent != nil {
			ev := rs.LastEvent
			payload := map[string]any{
				"schemaVersion":        ev.SchemaVersion,
				"sequence":             ev.Sequence,
				"executable":           ev.Executable,
				"argv":                 ev.Argv,
				"argvAvailable":        ev.ArgvAvailable,
				"exitCode":             ev.ExitCode,
				"exitAvailable":        ev.ExitAvailable,
				"cwd":                  ev.CwdAfter,
				"cwdBefore":            ev.CwdBefore,
				"cwdAvailable":         ev.CwdAvailable,
				"submittedLine":        ev.SubmittedLine,
				"stdoutNorm":           truncate(ev.Stdout.Text, 2048),
				"stderrNorm":           truncate(ev.Stderr.Text, 1024),
				"stdoutTrusted":        ev.Stdout.Trusted,
				"stderrTrusted":        ev.Stderr.Trusted,
				"source":               ev.Source,
				"structured":           ev.Structured,
				"runnerVersion":        ev.RunnerVersion,
				"shellInstrumentation": ev.ShellInstrumentation,
				"verifierVersion":      ev.VerifierVersion,
				"manifestBefore":       ev.ManifestBefore,
				"manifestAfter":        ev.ManifestAfter,
				"writeSet":             ev.WriteSet,
				"quality":              ev.Quality,
			}
			_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCommandFinished, payload)
		} else if rs.LastShell != nil {
			sh := rs.LastShell
			payload := map[string]any{
				"executable": sh.Executable,
				"args":       sh.Args,
				"exitCode":   sh.ExitCode,
				"cwd":        sh.Cwd,
				"stdoutNorm": truncate(sh.Stdout, 2048),
				"stderrNorm": truncate(sh.Stderr, 1024),
				"source":     sh.Source,
				"structured": sh.Structured,
			}
			_, _ = s.eng.enqueue(ctx, s.clientSessionID, contracts.EventCommandFinished, payload)
		}
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

