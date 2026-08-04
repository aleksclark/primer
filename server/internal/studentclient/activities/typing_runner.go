package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
	"github.com/aleksclark/primer/server/internal/studentclient/typing"
)

// TypingRunner executes typing-practice activities.
type TypingRunner struct {
	mu      sync.Mutex
	content contracts.ActivityContent
	digest  string
	ws      string
	sess    *typing.Session

	obs            []contracts.Observation
	checks         []CheckStatus
	requiredPassed bool
	checksPassed   int
	currentTaskIdx int

	// one-shot flags for the host after HandleInput/Verify
	emitSample bool
	emitChecks bool
}

// NewTyping returns an unopened typing runner.
func NewTyping() *TypingRunner {
	return &TypingRunner{}
}

// Kind implements Runner.
func (r *TypingRunner) Kind() string { return contracts.KindTyping }

// Open implements Runner.
func (r *TypingRunner) Open(_ context.Context, opts OpenOpts) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if opts.Content.Typing == nil {
		return fmt.Errorf("typing content is required")
	}
	sess, err := typing.NewSession(opts.Content.Typing, opts.Content.Checks)
	if err != nil {
		return err
	}
	r.content = opts.Content
	r.digest = opts.Digest
	r.ws = opts.Workspace
	r.sess = sess
	r.refreshLocked(false)
	return nil
}

// Snapshot implements Runner. Does not clear one-shot emit flags.
func (r *TypingRunner) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(false)
}

// DrainEvents returns a snapshot and clears one-shot emit flags for the host.
func (r *TypingRunner) DrainEvents() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(true)
}

func (r *TypingRunner) snapshotLocked(drain bool) Snapshot {
	snap := Snapshot{
		Kind:           contracts.KindTyping,
		Workspace:      r.ws,
		Cwd:            r.ws,
		RelCwd:         ".",
		CurrentTaskIdx: r.currentTaskIdx,
		Checks:         append([]CheckStatus(nil), r.checks...),
		RequiredPassed: r.requiredPassed,
		ChecksPassed:   r.checksPassed,
		ChecksTotal:    len(r.checks),
		EmitSample:     r.emitSample,
		EmitChecks:     r.emitChecks,
	}
	if r.sess != nil {
		m := r.sess.Metrics()
		id, text := r.sess.CurrentPrompt()
		snap.Typing = &TypingSnapshot{
			PromptID:         id,
			PromptText:       text,
			Input:            r.sess.CurrentInput(),
			PromptIndex:      r.sess.PromptIndex(),
			TotalPrompts:     m.TotalPrompts,
			RemainingPrompts: r.sess.RemainingPrompts(),
			WPM:              m.WPM,
			Accuracy:         m.Accuracy,
			CorrectChars:     m.CorrectChars,
			IncorrectChars:   m.IncorrectChars,
			Done:             m.Done,
			ThresholdsMet:    m.ThresholdsMet,
		}
	}
	if drain {
		r.emitSample = false
		r.emitChecks = false
	}
	return snap
}

// HandleInput implements Runner.
func (r *TypingRunner) HandleInput(_ context.Context, in Input) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess == nil {
		return fmt.Errorf("typing runner not open")
	}
	beforeIdx := r.sess.PromptIndex()
	beforeDone := r.sess.Done()
	switch in.Type {
	case InputKey:
		r.sess.TypeKey(in.Rune)
	case InputBackspace:
		r.sess.Backspace()
	case InputString:
		r.sess.TypeString(in.Text)
	default:
		return fmt.Errorf("typing runner does not accept input type %q", in.Type)
	}
	advanced := beforeIdx != r.sess.PromptIndex() || (!beforeDone && r.sess.Done())
	r.refreshLocked(advanced)
	return nil
}

// Verify implements Runner.
func (r *TypingRunner) Verify(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess == nil {
		return fmt.Errorf("typing runner not open")
	}
	r.refreshLocked(false)
	return nil
}

// CompleteReady implements Runner.
func (r *TypingRunner) CompleteReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requiredPassed
}

// Observations implements Runner.
func (r *TypingRunner) Observations() []contracts.Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]contracts.Observation, len(r.obs))
	copy(out, r.obs)
	return out
}

// EncodeState implements Runner.
func (r *TypingRunner) EncodeState() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess == nil {
		return json.Marshal(struct {
			V    int    `json:"v"`
			Kind string `json:"kind"`
		}{V: 1, Kind: contracts.KindTyping})
	}
	inner, err := r.sess.EncodeState()
	if err != nil {
		return nil, err
	}
	// Wrap with kind for cache inspection; inner is typing.DurableState JSON.
	return json.Marshal(struct {
		V     int             `json:"v"`
		Kind  string          `json:"kind"`
		State json.RawMessage `json:"state"`
	}{V: 1, Kind: contracts.KindTyping, State: inner})
}

// RestoreState implements Runner. Does not re-emit past events.
func (r *TypingRunner) RestoreState(raw []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess == nil {
		return fmt.Errorf("typing runner not open")
	}
	if len(raw) == 0 {
		return nil
	}
	// Accept either wrapped {kind,state} or bare typing.DurableState.
	var wrap struct {
		V     int             `json:"v"`
		Kind  string          `json:"kind"`
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && len(wrap.State) > 0 {
		if err := r.sess.RestoreState(wrap.State); err != nil {
			return err
		}
	} else if err := r.sess.RestoreState(raw); err != nil {
		return err
	}
	// Restore refreshes observations without enqueue flags.
	r.refreshLocked(false)
	r.emitSample = false
	r.emitChecks = false
	return nil
}

// Close implements Runner.
func (r *TypingRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sess = nil
	return nil
}

// Session exposes the underlying typing session for tests.
func (r *TypingRunner) Session() *typing.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sess
}

func (r *TypingRunner) refreshLocked(emitSample bool) {
	if r.sess == nil {
		return
	}
	obs := r.sess.Observations()
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
	// Tasks with completion trees (rare for typing) — evaluate if present.
	for _, task := range r.content.Tasks {
		if task.Optional {
			continue
		}
		if checkTreeEmpty(task.Completion) {
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
		if checkTreeEmpty(task.Completion) {
			r.currentTaskIdx = i
			break
		}
		ok, _ := terminal.EvalTree(task.Completion, byID)
		if !ok {
			r.currentTaskIdx = i
			break
		}
	}

	r.emitSample = emitSample
	r.emitChecks = emitSample
}
