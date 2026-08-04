// Package engine is the headless student activity harness: download work,
// materialize fixtures, run scripted commands, verify checks, and queue
// session events/completions through the local cache + sync loop.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

// Status is a snapshot for the harness TUI / tests.
type Status struct {
	Phase            string
	Sync             sync.Status
	WorkDownloaded   int
	AssignmentID     string
	ActivitySlug     string
	ClientSessionID  string
	ServerSessionID  string
	CommandsRun      int
	ChecksPassed     int
	ChecksTotal      int
	RequiredPassed   bool
	CompletionQueued bool
	CompletionAcked  bool
	Offline          bool
	Message          string
	LastError        string
}

// Options configure the harness.
type Options struct {
	// Client is required unless Offline is true and work is already cached.
	Client *studentapi.Client
	// Store is the local SQLite cache (required).
	Store *cache.Store
	// WorkspaceRoot is where exercise dirs are created (default: temp).
	WorkspaceRoot string
	// Offline skips network for activity run; still uses cache. Completion is queued.
	Offline bool
	// UseSandbox runs commands under bubblewrap when available. If true and
	// bwrap is missing, Run fails closed unless AllowUnsandboxed is set.
	UseSandbox bool
	// AllowUnsandboxed permits plain exec when bwrap is unavailable.
	AllowUnsandboxed bool
	// Sync is optional; when nil a Loop is created from Client+Store.
	Sync *sync.Loop
}

// Engine runs headless activity sessions.
type Engine struct {
	opts   Options
	status Status
}

// New builds an Engine.
func New(opts Options) (*Engine, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("cache store is required")
	}
	if opts.Client == nil && !opts.Offline {
		return nil, fmt.Errorf("api client is required unless offline")
	}
	if opts.Sync == nil && opts.Client != nil {
		opts.Sync = sync.New(opts.Client, opts.Store)
	}
	return &Engine{opts: opts, status: Status{Phase: "init", Sync: sync.StatusIdle}}, nil
}

// Status returns a copy of the last known harness status.
func (e *Engine) Status() Status { return e.status }

// SyncOnce runs one sync pull/flush when online.
func (e *Engine) SyncOnce(ctx context.Context) sync.Result {
	if e.opts.Offline || e.opts.Sync == nil {
		e.status.Offline = true
		e.status.Sync = sync.StatusOffline
		return sync.Result{Status: sync.StatusOffline}
	}
	res := e.opts.Sync.SyncOnce(ctx)
	e.status.Sync = res.Status
	e.status.WorkDownloaded = res.WorkItems
	if res.Err != nil {
		e.status.LastError = res.Err.Error()
	}
	return res
}

// ScriptedCommand is one command to run in the workspace.
type ScriptedCommand struct {
	// Argv is the program and arguments (e.g. ["pwd"] or ["ls", "-la"]).
	Argv []string
	// Dir is relative to workspace root; empty uses activity initial_cwd or root.
	Dir string
	// Shell if true runs via sh -c joining Argv as a single command line.
	Shell bool
}

// RunAssignment downloads (unless offline), materializes, runs commands, verifies,
// queues events + completion. When online, flushes via SyncOnce at the end.
func (e *Engine) RunAssignment(ctx context.Context, assignmentID string, commands []ScriptedCommand) error {
	e.status = Status{Phase: "load_work", Offline: e.opts.Offline, AssignmentID: assignmentID}

	if !e.opts.Offline && e.opts.Sync != nil {
		res := e.SyncOnce(ctx)
		if res.Status == sync.StatusRevoked {
			return fmt.Errorf("device revoked: %w", res.Err)
		}
		// Network errors are OK if work is already cached.
		if res.Err != nil {
			e.status.Message = "sync failed; trying cache"
		}
	}

	item, err := e.opts.Store.GetWork(ctx, assignmentID)
	if err != nil {
		// Try list and match, or pull once more.
		if !e.opts.Offline && e.opts.Client != nil {
			if res := e.SyncOnce(ctx); res.Err == nil {
				item, err = e.opts.Store.GetWork(ctx, assignmentID)
			}
		}
		if err != nil {
			return fmt.Errorf("load assignment %s: %w", assignmentID, err)
		}
	}
	e.status.ActivitySlug = item.Activity.Slug
	e.status.WorkDownloaded = 1
	e.status.Phase = "materialize"

	content, err := cache.DecodeActivityContent(item.Revision.Content)
	if err != nil {
		return err
	}
	if content.Terminal == nil {
		return fmt.Errorf("activity %s is not a terminal activity", item.Activity.Slug)
	}

	wsRoot := e.opts.WorkspaceRoot
	if wsRoot == "" {
		wsRoot, err = os.MkdirTemp("", "primer-ws-*")
		if err != nil {
			return err
		}
	}
	workspace := filepath.Join(wsRoot, "exercise-"+item.Assignment.ID[:8])
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	if err := terminal.Materialize(workspace, content.Terminal.Fixtures); err != nil {
		return fmt.Errorf("materialize fixtures: %w", err)
	}

	clientSessionID := uuid.NewString()
	e.status.ClientSessionID = clientSessionID
	e.status.Phase = "session"

	sess := cache.Session{
		ClientSessionID:    clientSessionID,
		AssignmentID:       item.Assignment.ID,
		ActivityRevisionID: item.Revision.ID,
		State:              "started",
		LastAckedSequence:  -1,
		NextSequence:       0,
		WorkspacePath:      workspace,
	}

	// Start server session when online.
	if !e.opts.Offline && e.opts.Client != nil {
		if tok, _ := e.opts.Store.DeviceToken(ctx); tok != "" {
			e.opts.Client.SetToken(tok)
		}
		serverSess, err := e.opts.Client.StartSession(ctx, clientSessionID, item.Assignment.ID)
		if err != nil {
			// Queue local session anyway for offline resume.
			e.status.LastError = err.Error()
			e.status.Message = "start session failed; continuing offline"
			e.status.Offline = true
		} else {
			sess.ServerSessionID = serverSess.ID
			e.status.ServerSessionID = serverSess.ID
		}
	}
	if err := e.opts.Store.SaveSession(ctx, sess); err != nil {
		return err
	}

	// session_started event
	if _, err := e.enqueue(ctx, clientSessionID, contracts.EventSessionStarted, map[string]any{
		"assignmentId": item.Assignment.ID,
		"activitySlug": item.Activity.Slug,
		"revisionId":   item.Revision.ID,
	}); err != nil {
		return err
	}

	cwd := workspace
	if content.Terminal.InitialCwd != "" {
		joined, err := contracts.JoinUnder(workspace, content.Terminal.InitialCwd)
		if err != nil {
			return err
		}
		cwd = joined
	}

	// task_viewed for each task
	for _, task := range content.Tasks {
		if _, err := e.enqueue(ctx, clientSessionID, contracts.EventTaskViewed, map[string]any{
			"taskId": task.ID,
			"title":  task.Title,
		}); err != nil {
			return err
		}
	}

	e.status.Phase = "run_commands"
	var lastShell *terminal.ShellState
	for i, sc := range commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		runDir := cwd
		if sc.Dir != "" {
			runDir = filepath.Join(workspace, sc.Dir)
		}
		start := time.Now()
		stdout, stderr, exitCode, runErr := e.runCommand(ctx, workspace, runDir, sc)
		dur := time.Since(start).Milliseconds()
		argv := sc.Argv
		if len(argv) == 0 {
			return fmt.Errorf("command %d: empty argv", i)
		}
		execName := argv[0]
		args := argv[1:]
		if sc.Shell {
			execName = "/bin/sh"
			args = []string{"-c", strings.Join(sc.Argv, " ")}
		}
		// Track cd for subsequent commands (scripted relative navigation).
		if execName == "cd" || (sc.Shell && len(sc.Argv) > 0 && sc.Argv[0] == "cd") {
			target := ""
			if !sc.Shell && len(args) > 0 {
				target = args[0]
			} else if sc.Shell && len(sc.Argv) > 1 {
				target = sc.Argv[1]
			}
			if target != "" {
				if filepath.IsAbs(target) {
					// ignore abs host paths; only relative under workspace
				} else {
					next := filepath.Clean(filepath.Join(runDir, target))
					// ensure still under workspace
					if rel, err := filepath.Rel(workspace, next); err == nil && !strings.HasPrefix(rel, "..") {
						cwd = next
						runDir = next
					}
				}
			}
		}

		relCwd, _ := filepath.Rel(workspace, runDir)
		shell := &terminal.ShellState{
			Cwd:        relCwd,
			Executable: execName,
			Args:       args,
			ExitCode:   exitCode,
			Stdout:     stdout,
			Stderr:     stderr,
		}
		lastShell = shell
		e.status.CommandsRun++

		payload := map[string]any{
			"executable": execName,
			"args":       args,
			"exitCode":   exitCode,
			"cwd":        relCwd,
			"durationMs": dur,
			"stdoutNorm": truncate(stdout, 2048),
			"stderrNorm": truncate(stderr, 1024),
		}
		if runErr != nil && exitCode == 0 {
			payload["error"] = runErr.Error()
		}
		if _, err := e.enqueue(ctx, clientSessionID, contracts.EventCommandFinished, payload); err != nil {
			return err
		}

		// Verify after each command
		obs := terminal.VerifyAll(workspace, content.Checks, shell)
		passed := 0
		for _, o := range obs {
			if o.Passed {
				passed++
			}
			checkPayload := map[string]any{
				"activityDigest":  item.Revision.ContentSHA256,
				"verifierVersion": terminal.VerifierVersion,
				"observation":     o,
			}
			if _, err := e.enqueue(ctx, clientSessionID, contracts.EventCheckEvaluated, checkPayload); err != nil {
				return err
			}
		}
		e.status.ChecksPassed = passed
		e.status.ChecksTotal = len(obs)
	}

	// Final verification
	e.status.Phase = "verify"
	finalObs := terminal.VerifyAll(workspace, content.Checks, lastShell)
	byID := map[string]contracts.Observation{}
	passed := 0
	requiredOK := true
	for _, o := range finalObs {
		byID[o.CheckID] = o
		if o.Passed {
			passed++
		}
	}
	for _, ch := range content.Checks {
		if ch.Optional {
			continue
		}
		o, ok := byID[ch.ID]
		if !ok || !o.Passed {
			requiredOK = false
			break
		}
	}
	for _, task := range content.Tasks {
		if task.Optional {
			continue
		}
		ok, _ := terminal.EvalTree(task.Completion, byID)
		if !ok {
			requiredOK = false
			break
		}
	}
	e.status.ChecksPassed = passed
	e.status.ChecksTotal = len(finalObs)
	e.status.RequiredPassed = requiredOK

	if !requiredOK {
		e.status.Phase = "incomplete"
		e.status.Message = "required checks not passed"
		// Still try to flush events.
		e.flush(ctx)
		return fmt.Errorf("required checks not passed (%d/%d)", passed, len(finalObs))
	}

	// Queue completion
	e.status.Phase = "complete"
	completionID := uuid.NewString()
	digest := requestDigest(item.Revision.ContentSHA256, finalObs)
	req := contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  completionID,
		RequestDigest: digest,
		Observations:  finalObs,
		ClientTime:    time.Now().UTC(),
		Summary:       fmt.Sprintf("harness completed %s", item.Activity.Slug),
	}
	serverID := e.status.ServerSessionID
	if err := e.opts.Store.SaveCompletionIntent(ctx, clientSessionID, serverID, req); err != nil {
		return err
	}
	if _, err := e.enqueue(ctx, clientSessionID, contracts.EventSessionCompleted, map[string]any{
		"completionId": completionID,
	}); err != nil {
		return err
	}
	e.status.CompletionQueued = true

	// Flush when online
	if err := e.flush(ctx); err != nil {
		e.status.Message = "completion queued; awaiting sync"
		e.status.Phase = "awaiting_sync"
		// Offline completion is success for the harness local path.
		if e.opts.Offline || e.status.Offline {
			return nil
		}
		return err
	}

	// Check if completion acked
	if cmp, err := e.opts.Store.GetCompletion(ctx, completionID); err == nil && cmp.Acked {
		e.status.CompletionAcked = true
		e.status.Phase = "done"
		e.status.Message = "completed and synced"
	} else {
		e.status.Phase = "awaiting_sync"
		e.status.Message = "completion queued; awaiting sync"
	}
	return nil
}

// FindAssignmentBySlug returns the first cached assignment matching activity slug.
func (e *Engine) FindAssignmentBySlug(ctx context.Context, slug string) (string, error) {
	items, err := e.opts.Store.ListWork(ctx)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Activity.Slug == slug && it.Assignment.State != "cancelled" {
			return it.Assignment.ID, nil
		}
	}
	return "", cache.ErrNotFound
}

// ResumeAndSync reopens the store path semantics: flush any pending outbox.
func (e *Engine) ResumeAndSync(ctx context.Context) error {
	return e.flush(ctx)
}

func (e *Engine) flush(ctx context.Context) error {
	if e.opts.Offline || e.opts.Sync == nil {
		e.status.Sync = sync.StatusOffline
		return nil
	}
	res := e.opts.Sync.SyncOnce(ctx)
	e.status.Sync = res.Status
	if res.Err != nil {
		e.status.LastError = res.Err.Error()
		return res.Err
	}
	return nil
}

func (e *Engine) enqueue(ctx context.Context, clientSessionID, typ string, payload map[string]any) (contracts.SessionEvent, error) {
	return e.opts.Store.EnqueueEvent(ctx, clientSessionID, contracts.SessionEvent{
		SchemaVersion: contracts.EventSchemaVersion,
		EventID:       uuid.NewString(),
		Type:          typ,
		Sequence:      -1,
		ClientTime:    time.Now().UTC(),
		Payload:       payload,
	})
}

func (e *Engine) runCommand(ctx context.Context, workspace, dir string, sc ScriptedCommand) (stdout, stderr string, exitCode int, err error) {
	argv := sc.Argv
	if len(argv) == 0 {
		return "", "", 1, fmt.Errorf("empty command")
	}
	name := argv[0]
	args := argv[1:]
	if sc.Shell {
		name = "/bin/sh"
		args = []string{"-c", strings.Join(sc.Argv, " ")}
	}

	// Built-in cd: no process, just for scripting convenience.
	if name == "cd" || (len(sc.Argv) > 0 && sc.Argv[0] == "cd" && !sc.Shell) {
		return "", "", 0, nil
	}

	useSandbox := e.opts.UseSandbox
	if useSandbox && !sandbox.Available() {
		if !e.opts.AllowUnsandboxed {
			return "", "", 1, sandbox.ErrUnavailable
		}
		useSandbox = false
	}

	if useSandbox {
		// Map dir into /workspace-relative path.
		rel, relErr := filepath.Rel(workspace, dir)
		workDir := "/workspace"
		if relErr == nil && rel != "." && rel != "" {
			workDir = filepath.Join("/workspace", rel)
		}
		cfg := sandbox.Config{Workspace: workspace, WorkDir: workDir}
		cmd, cerr := sandbox.Command(ctx, cfg, name, args...)
		if cerr != nil {
			return "", "", 1, cerr
		}
		var outBuf, errBuf strings.Builder
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		runErr := cmd.Run()
		exitCode = 0
		if runErr != nil {
			if ee, ok := runErr.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
				runErr = nil
			} else {
				exitCode = 1
			}
		}
		return outBuf.String(), errBuf.String(), exitCode, runErr
	}

	// Unsandboxed: Dir = workspace path (phase 2 default for harness tests).
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "HOME=" + workspace, "TERM=xterm"}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	exitCode = 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
			runErr = nil
		} else {
			exitCode = 1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode, runErr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func requestDigest(activityDigest string, obs []contracts.Observation) string {
	raw, _ := json.Marshal(struct {
		Activity string                  `json:"activity"`
		Obs      []contracts.Observation `json:"obs"`
	}{Activity: activityDigest, Obs: obs})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// BasicNavigationScript returns commands that satisfy basic-navigation checks.
// Fixtures already contain the required files; checks are filesystem-based, so
// discovery commands are run for realistic event streams.
func BasicNavigationScript() []ScriptedCommand {
	return []ScriptedCommand{
		{Argv: []string{"pwd"}},
		{Argv: []string{"ls", "-la"}},
		{Argv: []string{"cd", "docs"}},
		{Argv: []string{"ls"}},
		{Argv: []string{"cat", "guide.txt"}},
	}
}
