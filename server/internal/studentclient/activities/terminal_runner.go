package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

// TerminalRunner executes terminal activities (fixtures + checks).
// Live PTY is owned by the engine facade; this runner tracks durable cwd,
// command count, check observations, and task index — not a fictitious shell.
type TerminalRunner struct {
	mu      sync.Mutex
	content contracts.ActivityContent
	digest  string
	ws      string
	cwd     string // absolute under workspace

	runShell func(ctx context.Context, workspace, cwd, line string) (stdout, stderr string, exitCode int, err error)

	lastShell      *terminal.ShellState
	obs            []contracts.Observation
	checks         []CheckStatus
	requiredPassed bool
	checksPassed   int
	commandsRun    int
	currentTaskIdx int
	lastOutput     string
	lastError      string

	// one-shot host flags
	emitChecks   bool
	emitCommand  bool
	pendingShell *ShellResult
}

// NewTerminal returns an unopened terminal runner.
func NewTerminal() *TerminalRunner {
	return &TerminalRunner{}
}

// Kind implements Runner.
func (r *TerminalRunner) Kind() string { return contracts.KindTerminal }

// Open implements Runner.
func (r *TerminalRunner) Open(_ context.Context, opts OpenOpts) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if opts.Content.Terminal == nil {
		return fmt.Errorf("terminal content is required")
	}
	if opts.Workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	r.content = opts.Content
	r.digest = opts.Digest
	r.ws = opts.Workspace
	r.runShell = opts.RunShell

	if !opts.SkipMaterialize {
		if err := terminal.Materialize(opts.Workspace, opts.Content.Terminal.Fixtures); err != nil {
			return fmt.Errorf("materialize fixtures: %w", err)
		}
	}
	cwd := opts.Workspace
	if opts.Content.Terminal.InitialCwd != "" {
		joined, err := contracts.JoinUnder(opts.Workspace, opts.Content.Terminal.InitialCwd)
		if err != nil {
			return err
		}
		cwd = joined
	}
	r.cwd = cwd
	r.refreshLocked(nil, false)
	return nil
}

// Snapshot implements Runner. Does not clear one-shot emit flags.
func (r *TerminalRunner) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(false)
}

// DrainEvents returns a snapshot and clears one-shot emit flags for the host.
func (r *TerminalRunner) DrainEvents() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(true)
}

func (r *TerminalRunner) snapshotLocked(drain bool) Snapshot {
	rel := "."
	if r.ws != "" && r.cwd != "" {
		if rr, err := filepath.Rel(r.ws, r.cwd); err == nil {
			rel = rr
		}
	}
	snap := Snapshot{
		Kind:           contracts.KindTerminal,
		Workspace:      r.ws,
		Cwd:            r.cwd,
		RelCwd:         rel,
		CurrentTaskIdx: r.currentTaskIdx,
		Checks:         append([]CheckStatus(nil), r.checks...),
		RequiredPassed: r.requiredPassed,
		ChecksPassed:   r.checksPassed,
		ChecksTotal:    len(r.checks),
		CommandsRun:    r.commandsRun,
		LastOutput:     r.lastOutput,
		LastError:      r.lastError,
		EmitChecks:     r.emitChecks,
		EmitCommand:    r.emitCommand,
		LastShell:      r.pendingShell,
	}
	if drain {
		r.emitChecks = false
		r.emitCommand = false
		r.pendingShell = nil
	}
	return snap
}

// HandleInput implements Runner.
func (r *TerminalRunner) HandleInput(ctx context.Context, in Input) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ws == "" {
		return fmt.Errorf("terminal runner not open")
	}
	switch in.Type {
	case InputCommand:
		return r.runLineLocked(ctx, in.Line)
	case InputShellResult:
		if in.Shell == nil {
			return fmt.Errorf("shell_result requires shell payload")
		}
		return r.applyShellLocked(in.Shell)
	default:
		return fmt.Errorf("terminal runner does not accept input type %q", in.Type)
	}
}

func (r *TerminalRunner) runLineLocked(ctx context.Context, line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Built-in cd so subsequent commands use the new directory.
	if isCD, target := parseCD(line); isCD {
		if err := r.applyCDLocked(target); err != nil {
			r.lastError = err.Error()
			r.lastOutput = ""
			return err
		}
		r.lastOutput = ""
		r.lastError = ""
		r.commandsRun++
		rel, _ := filepath.Rel(r.ws, r.cwd)
		shell := &terminal.ShellState{
			Cwd:        rel,
			Executable: "cd",
			Args:       []string{target},
			ExitCode:   0,
		}
		r.lastShell = shell
		r.pendingShell = &ShellResult{
			Cwd: rel, Executable: "cd", Args: []string{target}, ExitCode: 0, CountCommand: true,
		}
		r.emitCommand = true
		r.refreshLocked(shell, true)
		return nil
	}

	if r.runShell == nil {
		return fmt.Errorf("terminal runner has no shell executor")
	}
	stdout, stderr, exitCode, runErr := r.runShell(ctx, r.ws, r.cwd, line)
	r.commandsRun++

	out := stdout
	if stderr != "" {
		if out != "" {
			out += "\n"
		}
		out += stderr
	}
	r.lastOutput = truncate(out, 4000)
	if runErr != nil && exitCode == 0 {
		r.lastError = runErr.Error()
	} else {
		r.lastError = ""
	}

	rel, _ := filepath.Rel(r.ws, r.cwd)
	shell := &terminal.ShellState{
		Cwd:        rel,
		Executable: "/bin/sh",
		Args:       []string{"-c", line},
		ExitCode:   exitCode,
		Stdout:     stdout,
		Stderr:     stderr,
	}
	r.lastShell = shell
	sr := &ShellResult{
		Cwd: rel, Executable: "/bin/sh", Args: []string{"-c", line},
		ExitCode: exitCode, Stdout: stdout, Stderr: stderr, CountCommand: true,
	}
	if runErr != nil && exitCode == 0 {
		// surface in payload via host
	}
	r.pendingShell = sr
	r.emitCommand = true
	r.refreshLocked(shell, true)
	return nil
}

func (r *TerminalRunner) applyShellLocked(sr *ShellResult) error {
	// External PTY observation: update last shell + optional command count.
	if sr.CountCommand {
		r.commandsRun++
	}
	if sr.Cwd != "" {
		// Cwd may be relative to workspace.
		next := sr.Cwd
		if !filepath.IsAbs(next) {
			joined, err := contracts.JoinUnder(r.ws, next)
			if err == nil {
				if info, err := os.Stat(joined); err == nil && info.IsDir() {
					r.cwd = joined
				}
			}
		} else {
			rel, err := filepath.Rel(r.ws, next)
			if err == nil && !strings.HasPrefix(rel, "..") {
				r.cwd = filepath.Clean(next)
			}
		}
	}
	rel, _ := filepath.Rel(r.ws, r.cwd)
	shell := &terminal.ShellState{
		Cwd:        rel,
		Executable: sr.Executable,
		Args:       sr.Args,
		ExitCode:   sr.ExitCode,
		Stdout:     sr.Stdout,
		Stderr:     sr.Stderr,
	}
	if shell.Executable == "" {
		shell.Executable = "pty-shell"
	}
	r.lastShell = shell
	r.lastOutput = truncate(sr.Stdout, 4000)
	r.lastError = ""
	// Host already has the shell event path for PTY; still mark checks emit.
	r.pendingShell = sr
	r.emitCommand = sr.CountCommand
	r.refreshLocked(shell, true)
	return nil
}

// Verify implements Runner.
func (r *TerminalRunner) Verify(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked(r.lastShell, true)
	return nil
}

// CompleteReady implements Runner.
func (r *TerminalRunner) CompleteReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requiredPassed
}

// Observations implements Runner.
func (r *TerminalRunner) Observations() []contracts.Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]contracts.Observation, len(r.obs))
	copy(out, r.obs)
	return out
}

// terminalDurable is the JSON shape for EncodeState (no live PTY).
type terminalDurable struct {
	V              int                     `json:"v"`
	Kind           string                  `json:"kind"`
	RelCwd         string                  `json:"relCwd"`
	CommandsRun    int                     `json:"commandsRun"`
	CurrentTaskIdx int                     `json:"currentTaskIdx"`
	RequiredPassed bool                    `json:"requiredPassed"`
	ChecksPassed   int                     `json:"checksPassed"`
	Checks         []CheckStatus           `json:"checks,omitempty"`
	Observations   []contracts.Observation `json:"observations,omitempty"`
	LastOutput     string                  `json:"lastOutput,omitempty"`
	LastError      string                  `json:"lastError,omitempty"`
	// LastShellCwd is relative cwd from last shell observation.
	LastShellCwd string `json:"lastShellCwd,omitempty"`
	LastShellExe string `json:"lastShellExe,omitempty"`
	LastExitCode int    `json:"lastExitCode,omitempty"`
	LastStdout   string `json:"lastStdout,omitempty"`
	LastStderr   string `json:"lastStderr,omitempty"`
	SavedAt      string `json:"savedAt,omitempty"`
}

// EncodeState implements Runner.
func (r *TerminalRunner) EncodeState() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel := "."
	if r.ws != "" && r.cwd != "" {
		if rr, err := filepath.Rel(r.ws, r.cwd); err == nil {
			rel = rr
		}
	}
	st := terminalDurable{
		V:              1,
		Kind:           contracts.KindTerminal,
		RelCwd:         rel,
		CommandsRun:    r.commandsRun,
		CurrentTaskIdx: r.currentTaskIdx,
		RequiredPassed: r.requiredPassed,
		ChecksPassed:   r.checksPassed,
		Checks:         append([]CheckStatus(nil), r.checks...),
		Observations:   append([]contracts.Observation(nil), r.obs...),
		LastOutput:     r.lastOutput,
		LastError:      r.lastError,
		SavedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if r.lastShell != nil {
		st.LastShellCwd = r.lastShell.Cwd
		st.LastShellExe = r.lastShell.Executable
		st.LastExitCode = r.lastShell.ExitCode
		st.LastStdout = r.lastShell.Stdout
		st.LastStderr = r.lastShell.Stderr
	}
	return json.Marshal(st)
}

// RestoreState implements Runner. Does not re-emit past events.
func (r *TerminalRunner) RestoreState(raw []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ws == "" {
		return fmt.Errorf("terminal runner not open")
	}
	if len(raw) == 0 {
		return nil
	}
	var st terminalDurable
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("terminal restore: %w", err)
	}
	if st.V != 0 && st.V != 1 {
		return fmt.Errorf("terminal restore: unsupported state version %d", st.V)
	}
	if st.RelCwd != "" {
		joined, err := contracts.JoinUnder(r.ws, st.RelCwd)
		if err == nil {
			if info, err := os.Stat(joined); err == nil && info.IsDir() {
				r.cwd = joined
			}
		}
	}
	r.commandsRun = st.CommandsRun
	r.currentTaskIdx = st.CurrentTaskIdx
	r.lastOutput = st.LastOutput
	r.lastError = st.LastError
	if st.LastShellExe != "" || st.LastStdout != "" || st.LastShellCwd != "" {
		r.lastShell = &terminal.ShellState{
			Cwd:        st.LastShellCwd,
			Executable: st.LastShellExe,
			ExitCode:   st.LastExitCode,
			Stdout:     st.LastStdout,
			Stderr:     st.LastStderr,
		}
	}
	// Re-verify against live workspace so filesystem progress is current.
	// Do not set emit flags — restore must not double-count events.
	r.refreshLocked(r.lastShell, false)
	// Prefer restored counters when reverify would zero them incorrectly...
	// refreshLocked overwrites checks from live verify which is correct.
	_ = st
	return nil
}

// Close implements Runner.
func (r *TerminalRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runShell = nil
	return nil
}

// Workspace returns the absolute workspace path.
func (r *TerminalRunner) Workspace() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ws
}

// Cwd returns the absolute current working directory.
func (r *TerminalRunner) Cwd() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cwd
}

// SetCwd updates cwd when the host tracks PTY directory changes.
func (r *TerminalRunner) SetCwd(abs string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, err := filepath.Rel(r.ws, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("cwd outside workspace")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	r.cwd = abs
	return nil
}

func (r *TerminalRunner) refreshLocked(shell *terminal.ShellState, enqueue bool) {
	obs := terminal.VerifyAll(r.ws, r.content.Checks, shell)
	r.obs = obs
	byID := map[string]contracts.Observation{}
	passed := 0
	checks := make([]CheckStatus, 0, len(obs))
	for _, o := range obs {
		byID[o.CheckID] = o
		if o.Passed {
			passed++
		}
		checks = append(checks, CheckStatus{
			ID: o.CheckID, Passed: o.Passed, Optional: o.Optional, Message: o.Message,
		})
	}
	r.checks = checks
	r.checksPassed = passed

	requiredOK := true
	for _, ch := range r.content.Checks {
		if ch.Optional {
			continue
		}
		o, ok := byID[ch.ID]
		if !ok || !o.Passed {
			requiredOK = false
			break
		}
	}
	for _, task := range r.content.Tasks {
		if task.Optional {
			continue
		}
		ok, _ := terminal.EvalTree(task.Completion, byID)
		if !ok {
			requiredOK = false
			break
		}
	}
	r.requiredPassed = requiredOK

	r.currentTaskIdx = len(r.content.Tasks)
	for i, task := range r.content.Tasks {
		if task.Optional {
			continue
		}
		ok, _ := terminal.EvalTree(task.Completion, byID)
		if !ok {
			r.currentTaskIdx = i
			break
		}
	}
	r.emitChecks = enqueue
}

func (r *TerminalRunner) applyCDLocked(target string) error {
	if target == "" {
		target = "."
	}
	var next string
	if filepath.IsAbs(target) {
		rel, err := filepath.Rel(r.ws, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("cd: path outside workspace")
		}
		next = filepath.Clean(target)
	} else {
		next = filepath.Clean(filepath.Join(r.cwd, target))
		rel, err := filepath.Rel(r.ws, next)
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
	r.cwd = next
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
	return true, strings.Join(fields[1:], " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
