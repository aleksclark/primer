// Package activities defines the shared runner interface for local activity kinds.
//
// Terminal and typing runners own kind-specific interaction, snapshots, checks,
// and durable state encoding. The host engine owns assignment loading, event
// outbox, completion submission, and live PTY attachment.
//
//	Open(opts) → HandleInput / Verify → CompleteReady → EncodeState / Close
//
// New activity kinds register a Factory; the engine selects by activity kind
// without kind switches for input handling or state persistence.
package activities

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// RunnerVersion is the durable state / capability version advertised by this
// client. Bump when EncodeState shape or runner semantics change incompatibly.
const RunnerVersion = "1"

// Runner is a local activity executor for one activity kind (terminal, typing, …).
// Implementations own kind-specific interaction and durable state; the host
// owns assignment loading, event outbox, and completion submission.
type Runner interface {
	// Kind returns the activity kind this runner handles.
	Kind() string

	// Open prepares workspace/state for the given immutable content.
	Open(ctx context.Context, opts OpenOpts) error

	// Snapshot returns a read-only view for TUI/broker rendering.
	Snapshot() Snapshot

	// HandleInput applies one unit of student input (command, key, backspace, …).
	HandleInput(ctx context.Context, in Input) error

	// Verify re-evaluates deterministic checks against current state.
	Verify(ctx context.Context) error

	// CompleteReady reports whether required checks currently pass.
	CompleteReady() bool

	// Observations returns the latest check observations for completion evidence.
	Observations() []contracts.Observation

	// EncodeState serializes durable runner progress (not live PTY).
	EncodeState() ([]byte, error)

	// RestoreState loads previously encoded progress after restart.
	RestoreState([]byte) error

	// Close releases runner resources.
	Close() error
}

// OpenOpts configures runner initialization.
type OpenOpts struct {
	// Workspace is an absolute path the runner may write under.
	Workspace string
	// Content is the immutable activity revision payload.
	Content contracts.ActivityContent
	// Digest is the content SHA-256 (for event payloads).
	Digest string
	// RuntimeProfile overrides terminal.runtime_profile when non-empty.
	RuntimeProfile string
	// RunShell executes one shell command line under workspace/cwd.
	// Required for terminal runners (scripted RunLine path).
	RunShell func(ctx context.Context, workspace, cwd, line string) (stdout, stderr string, exitCode int, err error)
	// SkipMaterialize leaves an existing workspace untouched (resume path).
	SkipMaterialize bool
}

// InputType classifies HandleInput payloads.
type InputType string

const (
	// InputCommand is a full shell command line (terminal).
	InputCommand InputType = "command"
	// InputKey is one typed character (typing).
	InputKey InputType = "key"
	// InputBackspace deletes one character (typing).
	InputBackspace InputType = "backspace"
	// InputString types multiple characters (typing automation / tests).
	InputString InputType = "string"
	// InputShellResult applies an external shell observation (PTY idle verify).
	InputShellResult InputType = "shell_result"
)

// Input is one unit of student or harness input.
type Input struct {
	Type InputType `json:"type"`
	// Line is the shell command for InputCommand.
	Line string `json:"line,omitempty"`
	// Rune is a single character for InputKey.
	Rune rune `json:"rune,omitempty"`
	// Text is bulk input for InputString.
	Text string `json:"text,omitempty"`
	// Shell carries observation state for InputShellResult.
	Shell *ShellResult `json:"shell,omitempty"`
}

// ShellResult is a lightweight shell observation from PTY or scripted exec.
type ShellResult struct {
	Cwd        string   `json:"cwd,omitempty"` // relative to workspace
	Executable string   `json:"executable,omitempty"`
	Args       []string `json:"args,omitempty"`
	ExitCode   int      `json:"exitCode"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	// CountCommand increments CommandsRun when true.
	CountCommand bool `json:"countCommand,omitempty"`
	// Structured is true only for trusted command instrumentation (not PTY screen).
	Structured bool `json:"structured,omitempty"`
	// Source labels the observation origin (structured, pty-shell, …).
	Source string `json:"source,omitempty"`
}

// CheckStatus is one check row for TUI / snapshots.
type CheckStatus struct {
	ID       string `json:"id"`
	Passed   bool   `json:"passed"`
	Optional bool   `json:"optional"`
	Message  string `json:"message,omitempty"`
}

// TypingSnapshot is the typing-mode portion of a runner snapshot.
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

// Snapshot is a read-only view of runner progress for rendering and broker IPC.
type Snapshot struct {
	Kind           string          `json:"kind"`
	Workspace      string          `json:"workspace,omitempty"`
	Cwd            string          `json:"cwd,omitempty"`
	RelCwd         string          `json:"relCwd,omitempty"`
	CurrentTaskIdx int             `json:"currentTaskIdx"`
	Checks         []CheckStatus   `json:"checks,omitempty"`
	RequiredPassed bool            `json:"requiredPassed"`
	ChecksPassed   int             `json:"checksPassed"`
	ChecksTotal    int             `json:"checksTotal"`
	CommandsRun    int             `json:"commandsRun"`
	LastOutput     string          `json:"lastOutput,omitempty"`
	LastError      string          `json:"lastError,omitempty"`
	Typing         *TypingSnapshot `json:"typing,omitempty"`
	// EmitSample is set after typing input that completed a prompt (host enqueues typing_sample).
	EmitSample bool `json:"-"`
	// EmitChecks is set when the host should enqueue check_evaluated for current observations.
	EmitChecks bool `json:"-"`
	// LastShell is the most recent shell observation (terminal); host may enqueue command_finished.
	LastShell *ShellResult `json:"-"`
	// EmitCommand is true when LastShell represents a newly finished command to record.
	EmitCommand bool `json:"-"`
}

// EventDrainer is optionally implemented by runners that buffer one-shot host events.
// DrainEvents returns the current snapshot and clears emit flags.
type EventDrainer interface {
	DrainEvents() Snapshot
}

// Factory constructs a fresh runner instance for a kind.
type Factory func() Runner

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register adds a runner factory for kind. Panics on duplicate registration.
func Register(kind string, factory Factory) {
	if kind == "" {
		panic("activities: empty kind")
	}
	if factory == nil {
		panic("activities: nil factory for " + kind)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[kind]; exists {
		panic("activities: duplicate registration for " + kind)
	}
	registry[kind] = factory
}

// New constructs a runner for kind, or an error when unsupported.
func New(kind string) (Runner, error) {
	registryMu.RLock()
	f, ok := registry[kind]
	registryMu.RUnlock()
	if !ok || f == nil {
		return nil, fmt.Errorf("%w: %q (supported: %v)", ErrUnsupportedKind, kind, SupportedKinds())
	}
	return f(), nil
}

// Supports reports whether kind has a registered runner.
func Supports(kind string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[kind]
	return ok
}

// SupportedKinds returns registered kinds in stable order.
func SupportedKinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ErrUnsupportedKind is returned when opening or constructing an unknown kind.
var ErrUnsupportedKind = fmt.Errorf("unsupported activity kind")

// KindOf normalizes a content-derived kind string.
func KindOf(kind string) string {
	switch kind {
	case contracts.KindTerminal, contracts.KindTyping:
		return kind
	default:
		return kind
	}
}

// ResolveKind picks terminal/typing from explicit kind or content payload.
func ResolveKind(explicit string, content contracts.ActivityContent) (string, error) {
	kind := explicit
	if kind == "" {
		switch {
		case content.Terminal != nil:
			kind = contracts.KindTerminal
		case content.Typing != nil:
			kind = contracts.KindTyping
		default:
			return "", fmt.Errorf("activity has no terminal or typing content")
		}
	}
	return kind, nil
}
