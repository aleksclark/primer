package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	stdsync "sync"
	"time"

	"github.com/google/uuid"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

// CheckStatus is one check row for the activity TUI.
type CheckStatus struct {
	ID       string
	Passed   bool
	Optional bool
	Message  string
}

// SessionSnapshot is a read-only view of an interactive activity session.
type SessionSnapshot struct {
	AssignmentID    string
	ActivitySlug    string
	ActivityTitle   string
	ClientSessionID string
	ServerSessionID string
	Workspace       string
	Cwd             string // absolute path
	RelCwd          string // path relative to workspace
	Objective       string
	Instructions    string
	Tasks           []contracts.Task
	CurrentTaskIdx  int
	Checks          []CheckStatus
	RequiredPassed  bool
	ChecksPassed    int
	ChecksTotal     int
	CommandsRun     int
	LastOutput      string
	LastError       string
	Message         string
	Completed       bool
	CompletionQueued bool
	CompletionAcked  bool
	Offline         bool
	Sync            sync.Status
	Hints           []contracts.Hint
	TutorHint       string
}

// Session is an interactive terminal activity session driven by the TUI.
type Session struct {
	mu stdsync.Mutex

	eng     *Engine
	item    studentapi.WorkItem
	content contracts.ActivityContent
	digest  string

	clientSessionID string
	serverSessionID string
	workspace       string
	cwd             string

	lastShell *terminal.ShellState
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
}

// OpenSession loads work, materializes fixtures, starts a local (and optional
// server) session, and returns an interactive Session for the TUI.
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
	if content.Terminal == nil {
		return nil, fmt.Errorf("activity %s is not a terminal activity", item.Activity.Slug)
	}

	wsRoot := e.opts.WorkspaceRoot
	if wsRoot == "" {
		wsRoot, err = os.MkdirTemp("", "primer-ws-*")
		if err != nil {
			return nil, err
		}
	}
	workspace := filepath.Join(wsRoot, "exercise-"+item.Assignment.ID[:8])
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}
	if err := terminal.Materialize(workspace, content.Terminal.Fixtures); err != nil {
		return nil, fmt.Errorf("materialize fixtures: %w", err)
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
	}); err != nil {
		return nil, err
	}

	cwd := workspace
	if content.Terminal.InitialCwd != "" {
		joined, err := contracts.JoinUnder(workspace, content.Terminal.InitialCwd)
		if err != nil {
			return nil, err
		}
		cwd = joined
	}
	s.cwd = cwd

	for _, task := range content.Tasks {
		if _, err := e.enqueue(ctx, clientSessionID, contracts.EventTaskViewed, map[string]any{
			"taskId": task.ID,
			"title":  task.Title,
		}); err != nil {
			return nil, err
		}
	}

	// Initial verify (fixtures alone may satisfy filesystem checks).
	s.reverify(ctx, nil)
	return s, nil
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
	if r, err := filepath.Rel(s.workspace, s.cwd); err == nil {
		rel = r
	}
	title := s.item.Activity.Title
	if title == "" {
		title = s.item.Activity.Slug
	}
	return SessionSnapshot{
		AssignmentID:     s.item.Assignment.ID,
		ActivitySlug:     s.item.Activity.Slug,
		ActivityTitle:    title,
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
}

// RunLine runs one shell command line in the session workspace (via sh -c).
// Built-in "cd" is handled without spawning a process.
func (s *Session) RunLine(ctx context.Context, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return fmt.Errorf("session already completed")
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

// Verify re-runs deterministic checks against the current workspace.
func (s *Session) Verify(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reverify(ctx, s.lastShell)
	return nil
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
func (s *Session) Pause(ctx context.Context) error {
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
	obs := terminal.VerifyAll(s.workspace, s.content.Checks, shell)
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
		// Only enqueue check events when we have a shell action or completion path;
		// still useful on open for TUI display without flooding — skip enqueue on pure display.
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

	if shell != nil {
		for _, o := range obs {
			checkPayload := map[string]any{
				"activityDigest":  s.digest,
				"verifierVersion": terminal.VerifierVersion,
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
