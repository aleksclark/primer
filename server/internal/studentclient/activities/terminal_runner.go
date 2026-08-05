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
// command history, check observations, and task index.
type TerminalRunner struct {
	mu      sync.Mutex
	content contracts.ActivityContent
	digest  string
	ws      string
	cwd     string // absolute under workspace

	runShell func(ctx context.Context, workspace, cwd, line string) (stdout, stderr string, exitCode int, err error)

	history        terminal.History
	nextSeq        int64
	lastShell      *terminal.ShellState
	lastManifest   string
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
	pendingEvent *contracts.ShellEvent

	// submittedTasks tracks local conceptual response submissions for response_submitted checks.
	submittedTasks map[string]bool
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
	r.nextSeq = 1
	r.history = terminal.History{TaskStartSeq: map[int]int64{0: 1}}

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
	if r.submittedTasks == nil {
		r.submittedTasks = map[string]bool{}
	}
	if man, err := terminal.CaptureManifest(r.ws); err == nil {
		r.lastManifest = man.Digest
	}
	r.refreshLocked(nil, false)
	return nil
}

// MarkResponseSubmitted records that taskID has a durable conceptual response
// and re-evaluates checks (response_submitted).
func (r *TerminalRunner) MarkResponseSubmitted(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.submittedTasks == nil {
		r.submittedTasks = map[string]bool{}
	}
	if taskID != "" {
		r.submittedTasks[taskID] = true
	}
	r.refreshLocked(r.shellStateLocked(), true)
}

// SubmittedTasks returns a copy of locally known submitted task IDs.
func (r *TerminalRunner) SubmittedTasks() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]bool, len(r.submittedTasks))
	for k, v := range r.submittedTasks {
		out[k] = v
	}
	return out
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
		LastEvent:      r.pendingEvent,
	}
	if drain {
		r.emitChecks = false
		r.emitCommand = false
		r.pendingShell = nil
		r.pendingEvent = nil
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
		if in.Shell == nil && in.Event == nil {
			return fmt.Errorf("shell_result requires shell or event payload")
		}
		if in.Event != nil {
			return r.applyEventLocked(*in.Event)
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

	cwdBefore := r.relCwdLocked()
	manBefore, _ := terminal.CaptureManifest(r.ws)

	// Built-in cd so subsequent commands use the new directory.
	if isCD, target := parseCD(line); isCD {
		if err := r.applyCDLocked(target); err != nil {
			r.lastError = err.Error()
			r.lastOutput = ""
			// Still record failed cd as structured evidence.
			ev := terminal.BuildStructuredEvent(
				r.nextSeq, "", cwdBefore, cwdBefore,
				"cd", []string{target}, line, 1, "", err.Error(),
				RunnerVersion, terminal.VerifierVersion, manBefore, manBefore,
			)
			ev.ExitCode = 1
			return r.recordEventLocked(ev)
		}
		r.lastOutput = ""
		r.lastError = ""
		manAfter, _ := terminal.CaptureManifest(r.ws)
		ev := terminal.BuildStructuredEvent(
			r.nextSeq, "", cwdBefore, r.relCwdLocked(),
			"cd", []string{target}, line, 0, "", "",
			RunnerVersion, terminal.VerifierVersion, manBefore, manAfter,
		)
		return r.recordEventLocked(ev)
	}

	if r.runShell == nil {
		return fmt.Errorf("terminal runner has no shell executor")
	}
	stdout, stderr, exitCode, runErr := r.runShell(ctx, r.ws, r.cwd, line)
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

	manAfter, _ := terminal.CaptureManifest(r.ws)
	// Best-effort: detect cwd change if command was cd via shell.
	cwdAfter := r.relCwdLocked()
	if isCD, target := parseCD(line); isCD {
		_ = target
		cwdAfter = r.relCwdLocked()
	}

	ev := terminal.BuildStructuredEvent(
		r.nextSeq, "", cwdBefore, cwdAfter,
		"/bin/sh", []string{"-c", line}, line, exitCode, stdout, stderr,
		RunnerVersion, terminal.VerifierVersion, manBefore, manAfter,
	)
	return r.recordEventLocked(ev)
}

func (r *TerminalRunner) applyEventLocked(ev contracts.ShellEvent) error {
	if ev.Sequence <= 0 {
		ev.Sequence = r.nextSeq
	}
	// Enrich manifests when missing.
	if ev.ManifestAfter == "" {
		if man, err := terminal.CaptureManifest(r.ws); err == nil {
			ev.ManifestAfter = man.Digest
			r.lastManifest = man.Digest
			if ev.ManifestBefore == "" {
				ev.ManifestBefore = man.Digest
			}
		}
	} else {
		r.lastManifest = ev.ManifestAfter
	}
	if ev.CwdAfter != "" {
		joined, err := contracts.JoinUnder(r.ws, ev.CwdAfter)
		if err == nil {
			if info, err := os.Stat(joined); err == nil && info.IsDir() {
				r.cwd = joined
			}
		}
	}
	return r.recordEventLocked(ev)
}

func (r *TerminalRunner) applyShellLocked(sr *ShellResult) error {
	// Legacy path: convert ShellResult into a ShellEvent.
	if sr.Cwd != "" {
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
	structured := sr.Structured
	source := sr.Source
	if source == "" {
		if sr.Executable == "pty-shell" || !structured {
			source = contracts.SourcePTYShell
			structured = false
		} else {
			source = contracts.SourceStructured
		}
	}
	if sr.Executable == "pty-shell" || source == contracts.SourcePTYShell || source == contracts.SourceSyntheticPTY || source == contracts.SourceScreen {
		// Never accept screen scrape as structured evidence.
		structured = false
		source = contracts.SourcePTYShell
	}
	rel := r.relCwdLocked()
	q := contracts.EvidenceQuality{
		Exit:   structured,
		Cwd:    structured && rel != "",
		Argv:   structured && sr.Executable != "" && sr.Executable != "pty-shell",
		Stdout: structured && source == contracts.SourceStructured,
		Stderr: structured && source == contracts.SourceStructured,
	}
	if !q.MeetsStructuredBar() {
		structured = false
	}
	man, _ := terminal.CaptureManifest(r.ws)
	ev := contracts.ShellEvent{
		SchemaVersion: contracts.ShellEventSchemaVersion,
		Sequence:      r.nextSeq,
		FinishedAt:    time.Now().UTC(),
		Executable:    sr.Executable,
		Argv:          append([]string(nil), sr.Args...),
		ArgvAvailable: q.Argv,
		CwdAfter:      rel,
		CwdAvailable:  q.Cwd,
		ExitCode:      sr.ExitCode,
		ExitAvailable: q.Exit,
		Stdout:        contracts.Excerpt{Text: truncate(sr.Stdout, 2048), Trusted: q.Stdout},
		Stderr:        contracts.Excerpt{Text: truncate(sr.Stderr, 1024), Trusted: q.Stderr},
		ManifestAfter: man.Digest,
		Source:        source,
		Structured:    structured,
		Quality:       q,
		RunnerVersion: RunnerVersion,
		VerifierVersion: terminal.VerifierVersion,
	}
	if !sr.CountCommand && !structured {
		// Non-counting untrusted observation: refresh FS checks only.
		r.lastOutput = truncate(sr.Stdout, 4000)
		r.refreshLocked(r.shellStateLocked(), true)
		return nil
	}
	return r.recordEventLocked(ev)
}

func (r *TerminalRunner) recordEventLocked(ev contracts.ShellEvent) error {
	if ev.Sequence < r.nextSeq {
		ev.Sequence = r.nextSeq
	}
	r.nextSeq = ev.Sequence + 1
	r.commandsRun++

	taskID := ""
	if r.currentTaskIdx >= 0 && r.currentTaskIdx < len(r.content.Tasks) {
		taskID = r.content.Tasks[r.currentTaskIdx].ID
	}
	r.history.MarkTaskStart(r.currentTaskIdx, ev.Sequence)
	obs := contracts.ObservationFromShellEvent(ev, taskID, r.currentTaskIdx)
	r.history.Append(obs)

	shell := ShellStateFromEvent(ev, &r.history, r.currentTaskIdx, r.lastManifest)
	r.lastShell = shell
	r.lastOutput = truncate(ev.Stdout.Text, 4000)
	if ev.Stderr.Text != "" && ev.ExitCode != 0 {
		r.lastError = truncate(ev.Stderr.Text, 512)
	} else {
		r.lastError = ""
	}
	if ev.ManifestAfter != "" {
		r.lastManifest = ev.ManifestAfter
	}

	r.pendingEvent = &ev
	r.pendingShell = &ShellResult{
		Cwd:          ev.CwdAfter,
		Executable:   ev.Executable,
		Args:         append([]string(nil), ev.Argv...),
		ExitCode:     ev.ExitCode,
		Stdout:       ev.Stdout.Text,
		Stderr:       ev.Stderr.Text,
		CountCommand: true,
		Structured:   ev.Structured,
		Source:       ev.Source,
	}
	r.emitCommand = true
	r.refreshLocked(shell, true)
	return nil
}

// Verify implements Runner.
func (r *TerminalRunner) Verify(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked(r.shellStateLocked(), true)
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
	V              int                         `json:"v"`
	Kind           string                      `json:"kind"`
	RelCwd         string                      `json:"relCwd"`
	CommandsRun    int                         `json:"commandsRun"`
	CurrentTaskIdx int                         `json:"currentTaskIdx"`
	RequiredPassed bool                        `json:"requiredPassed"`
	ChecksPassed   int                         `json:"checksPassed"`
	Checks         []CheckStatus               `json:"checks,omitempty"`
	Observations   []contracts.Observation     `json:"observations,omitempty"`
	LastOutput     string                      `json:"lastOutput,omitempty"`
	LastError      string                      `json:"lastError,omitempty"`
	LastShellCwd   string                      `json:"lastShellCwd,omitempty"`
	LastShellExe   string                      `json:"lastShellExe,omitempty"`
	LastExitCode   int                         `json:"lastExitCode,omitempty"`
	LastStdout     string                      `json:"lastStdout,omitempty"`
	LastStderr     string                      `json:"lastStderr,omitempty"`
	LastSource     string                      `json:"lastSource,omitempty"`
	LastStructured bool                        `json:"lastStructured,omitempty"`
	History        *terminal.History           `json:"history,omitempty"`
	NextSeq        int64                       `json:"nextSeq,omitempty"`
	LastManifest   string                      `json:"lastManifest,omitempty"`
	SubmittedTasks []string                    `json:"submittedTasks,omitempty"`
	SavedAt        string                      `json:"savedAt,omitempty"`
}

// EncodeState implements Runner.
func (r *TerminalRunner) EncodeState() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel := r.relCwdLocked()
	hist := r.history
	var submitted []string
	for id, ok := range r.submittedTasks {
		if ok {
			submitted = append(submitted, id)
		}
	}
	st := terminalDurable{
		V:              2,
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
		History:        &hist,
		NextSeq:        r.nextSeq,
		LastManifest:   r.lastManifest,
		SubmittedTasks: submitted,
		SavedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if r.lastShell != nil {
		st.LastShellCwd = r.lastShell.Cwd
		st.LastShellExe = r.lastShell.Executable
		st.LastExitCode = r.lastShell.ExitCode
		st.LastStdout = r.lastShell.Stdout
		st.LastStderr = r.lastShell.Stderr
		st.LastSource = r.lastShell.Source
		st.LastStructured = r.lastShell.StructuredCommandEvidence
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
	if st.V != 0 && st.V != 1 && st.V != 2 {
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
	r.lastManifest = st.LastManifest
	if st.NextSeq > 0 {
		r.nextSeq = st.NextSeq
	}
	if st.History != nil {
		r.history = *st.History
		if r.history.TaskStartSeq == nil {
			r.history.TaskStartSeq = map[int]int64{}
		}
	}
	if st.LastShellExe != "" || st.LastStdout != "" || st.LastShellCwd != "" || st.LastSource != "" {
		r.lastShell = &terminal.ShellState{
			Cwd:                       st.LastShellCwd,
			Executable:                st.LastShellExe,
			ExitCode:                  st.LastExitCode,
			Stdout:                    st.LastStdout,
			Stderr:                    st.LastStderr,
			Source:                    st.LastSource,
			StructuredCommandEvidence: st.LastStructured,
		}
	}
	r.submittedTasks = map[string]bool{}
	for _, id := range st.SubmittedTasks {
		if id != "" {
			r.submittedTasks[id] = true
		}
	}
	r.refreshLocked(r.shellStateLocked(), false)
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

// History returns a copy of command history (for tests).
func (r *TerminalRunner) History() terminal.History {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.history
}

func (r *TerminalRunner) relCwdLocked() string {
	if r.ws == "" || r.cwd == "" {
		return "."
	}
	if rr, err := filepath.Rel(r.ws, r.cwd); err == nil {
		return rr
	}
	return "."
}

func (r *TerminalRunner) shellStateLocked() *terminal.ShellState {
	if r.lastShell != nil {
		s := *r.lastShell
		s.History = &r.history
		s.TaskIndex = r.currentTaskIdx
		s.ManifestDigest = r.lastManifest
		s.SubmittedTasks = r.submittedTasks
		return &s
	}
	return &terminal.ShellState{
		Cwd:            r.relCwdLocked(),
		History:        &r.history,
		TaskIndex:      r.currentTaskIdx,
		ManifestDigest: r.lastManifest,
		SubmittedTasks: r.submittedTasks,
	}
}

func (r *TerminalRunner) refreshLocked(shell *terminal.ShellState, enqueue bool) {
	if shell == nil {
		shell = r.shellStateLocked()
	} else {
		// Ensure history is attached for task-scoped predicates.
		cp := *shell
		cp.History = &r.history
		cp.TaskIndex = r.currentTaskIdx
		if cp.ManifestDigest == "" {
			cp.ManifestDigest = r.lastManifest
		}
		cp.SubmittedTasks = r.submittedTasks
		shell = &cp
	}
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

	prevTask := r.currentTaskIdx
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
	if r.currentTaskIdx != prevTask && r.currentTaskIdx < len(r.content.Tasks) {
		r.history.MarkTaskStart(r.currentTaskIdx, r.nextSeq)
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

// ShellStateFromEvent builds verifier ShellState from a ShellEvent + history.
func ShellStateFromEvent(ev contracts.ShellEvent, hist *terminal.History, taskIdx int, manifest string) *terminal.ShellState {
	return &terminal.ShellState{
		Cwd:                       ev.CwdAfter,
		Executable:                ev.Executable,
		Args:                      append([]string(nil), ev.Argv...),
		ExitCode:                  ev.ExitCode,
		Stdout:                    ev.Stdout.Text,
		Stderr:                    ev.Stderr.Text,
		StructuredCommandEvidence: ev.Structured && ev.Quality.MeetsStructuredBar(),
		Source:                    ev.Source,
		History:                   hist,
		TaskIndex:                 taskIdx,
		ManifestDigest:            manifest,
	}
}
