// Package engine is the headless student activity harness: download work,
// materialize fixtures, run scripted commands, verify checks, and queue
// session events/completions through the local cache + sync loop.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	// RuntimeProfile is the activity terminal.runtime_profile name. When set,
	// sandboxed commands bind the matching Nix tool closure (or host fallback).
	RuntimeProfile string
	// StructuredCommandEvidence advertises CapStructuredCommandEvidence when
	// starting sessions. The headless harness/scripted RunShell path may set
	// this true. Interactive PTY mode must leave it false: synthetic screen
	// text is not structured command evidence.
	StructuredCommandEvidence bool
	// Sync is optional; when nil a Loop is created from Client+Store.
	Sync *sync.Loop
}

// Engine runs headless activity sessions.
type Engine struct {
	opts   Options
	status Status
	// activeRuntimeProfile is taken from the current activity's terminal
	// content (or Options.RuntimeProfile when set as an override).
	activeRuntimeProfile string
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

// sessionCapabilities lists runner flags advertised on StartSession.
// Interactive PTY does not produce structured command evidence (Phase 2).
func (e *Engine) sessionCapabilities() []string {
	if e.opts.StructuredCommandEvidence {
		return []string{contracts.CapStructuredCommandEvidence}
	}
	return nil
}

// isIncompatibleRevisionError reports server/local capability policy rejections.
func isIncompatibleRevisionError(err error) bool {
	if err == nil {
		return false
	}
	var inv contracts.ErrIncompatibleRevision
	if errors.As(err, &inv) {
		return true
	}
	var httpErr *studentapi.ErrHTTP
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest {
		return strings.Contains(httpErr.Body, "structured_command_evidence") ||
			strings.Contains(httpErr.Error(), "structured_command_evidence")
	}
	return strings.Contains(err.Error(), "structured_command_evidence")
}

// Status returns a copy of the last known harness status.
func (e *Engine) Status() Status { return e.status }

// Store returns the engine's durable cache (for tests and broker diagnostics).
func (e *Engine) Store() *cache.Store { return e.opts.Store }

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

// RunAssignment downloads (unless offline), opens a session via the runner
// registry, runs scripted terminal commands or auto-types typing prompts,
// verifies, and queues completion. When online, flushes via SyncOnce at the end.
//
// For typing activities, commands are ignored; prompts are typed from content.
func (e *Engine) RunAssignment(ctx context.Context, assignmentID string, commands []ScriptedCommand) error {
	e.status = Status{Phase: "load_work", Offline: e.opts.Offline, AssignmentID: assignmentID}

	if !e.opts.Offline && e.opts.Sync != nil {
		res := e.SyncOnce(ctx)
		if res.Status == sync.StatusRevoked {
			return fmt.Errorf("device revoked: %w", res.Err)
		}
		if res.Err != nil {
			e.status.Message = "sync failed; trying cache"
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
			return fmt.Errorf("load assignment %s: %w", assignmentID, err)
		}
	}
	e.status.ActivitySlug = item.Activity.Slug
	e.status.WorkDownloaded = 1
	e.status.Phase = "session"

	// Headless path uses OpenSession so runners + durable state stay unified.
	// Prefer a fresh session for scripted runs: mark any open session completed-local
	// only if it has no runner state; otherwise OpenSession resumes (tests use fresh stores).
	sess, err := e.OpenSession(ctx, assignmentID)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()

	e.status.ClientSessionID = sess.clientSessionID
	e.status.ServerSessionID = sess.serverSessionID
	e.status.Offline = sess.offline

	switch sess.Kind() {
	case contracts.KindTyping:
		e.status.Phase = "run_typing"
		if sess.content.Typing == nil {
			return fmt.Errorf("activity %s is kind typing but has no typing content", item.Activity.Slug)
		}
		for _, p := range sess.content.Typing.Prompts {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := sess.TypeString(ctx, p.Text); err != nil {
				return err
			}
		}
	case contracts.KindTerminal:
		e.status.Phase = "run_commands"
		for _, sc := range commands {
			if err := ctx.Err(); err != nil {
				return err
			}
			line := strings.Join(sc.Argv, " ")
			if sc.Shell {
				line = strings.Join(sc.Argv, " ")
			}
			if sc.Dir != "" {
				// Best-effort: cd into Dir first when relative.
				if err := sess.RunLine(ctx, "cd "+sc.Dir); err != nil {
					return err
				}
			}
			if err := sess.RunLine(ctx, line); err != nil {
				// Non-zero exit is OK for discovery; RunLine only errors on cd/path.
				if !strings.Contains(err.Error(), "cd:") {
					// keep going for ordinary command failures (exit codes)
				} else {
					return err
				}
			}
			e.status.CommandsRun++
		}
	default:
		return fmt.Errorf("unsupported activity kind %q", sess.Kind())
	}

	e.status.Phase = "verify"
	if err := sess.Verify(ctx); err != nil {
		return err
	}
	snap := sess.Snapshot()
	e.status.ChecksPassed = snap.ChecksPassed
	e.status.ChecksTotal = snap.ChecksTotal
	e.status.RequiredPassed = snap.RequiredPassed
	e.status.CommandsRun = snap.CommandsRun

	if !snap.RequiredPassed {
		e.status.Phase = "incomplete"
		e.status.Message = "required checks not passed"
		e.flush(ctx)
		return fmt.Errorf("required checks not passed (%d/%d)", snap.ChecksPassed, snap.ChecksTotal)
	}

	e.status.Phase = "complete"
	if err := sess.Complete(ctx); err != nil {
		return err
	}
	snap = sess.Snapshot()
	e.status.CompletionQueued = snap.CompletionQueued
	e.status.CompletionAcked = snap.CompletionAcked
	e.status.Message = snap.Message
	if snap.CompletionAcked {
		e.status.Phase = "done"
	} else {
		e.status.Phase = "awaiting_sync"
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

// runtimeProfile returns the active sandbox profile name.
func (e *Engine) runtimeProfile() string {
	if p := strings.TrimSpace(e.activeRuntimeProfile); p != "" {
		return p
	}
	return strings.TrimSpace(e.opts.RuntimeProfile)
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
		if p := e.runtimeProfile(); p != "" {
			if perr := sandbox.ApplyProfile(&cfg, p); perr != nil {
				return "", "", 1, perr
			}
		}
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
