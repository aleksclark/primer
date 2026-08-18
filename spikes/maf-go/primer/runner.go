package primer

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/microsoft/agent-framework-go/tool"
)

// newRunID generates a fresh per-invocation run id.
// Tests may override via SetRunIDGenerator.
var newRunID = func() string { return uuid.NewString() }

// SetRunIDGenerator installs a deterministic run-id factory for tests.
// Pass nil to restore the default UUID generator. Not safe for concurrent use
// outside tests.
func SetRunIDGenerator(fn func() string) {
	if fn == nil {
		newRunID = func() string { return uuid.NewString() }
		return
	}
	newRunID = fn
}

// Runner orchestrates parent runs and budgeted StartChild calls using public MAF APIs.
// Depth and lineage are orchestrator-controlled: callers never supply Depth.
type Runner struct {
	spec   AgentSpec
	agent  *agent.Agent
	sink   EventSink
	mu     sync.Mutex
	depth  int // current orchestration depth (0 = root)
	// lineage shared across the tree for root/parent attribution + total budget.
	lineage *runLineage
	// direct counts StartChild attempts that passed budget checks on THIS runner.
	direct int
	// activeChildren counts in-flight child tool executions started from this runner.
	activeChildren int
	// parentRunID is set only on child runners (immediate parent run id).
	parentRunID atomic.Value // string
	// lastRunID is the most recent Run invocation id on this runner.
	lastRunID atomic.Value // string
	// pendingRunID, when set, is consumed by the next Run() so StartChild-before-Run
	// (e.g. SSE BuildChildTool) shares the same run identity as parent events.
	pendingRunID atomic.Value // string
}

// runLineage is shared root state for a delegation tree.
type runLineage struct {
	mu      sync.Mutex
	rootID  string // set on first root Run; may be empty until then
	total   int    // total children started under this root tree
	agentID string // root agent id (stable)
}

// NewRunner wraps a prebuilt MAF agent as a root orchestrator (depth 0).
func NewRunner(spec AgentSpec, a *agent.Agent, sink EventSink) *Runner {
	if sink == nil {
		sink = NoopSink{}
	}
	agentID := ""
	if a != nil {
		agentID = a.ID()
	}
	return &Runner{
		spec:  spec,
		agent: a,
		sink:  sink,
		depth: 0,
		lineage: &runLineage{
			agentID: agentID,
		},
	}
}

// Orchestration returns a snapshot of orchestrator-controlled lineage fields.
func (r *Runner) Orchestration() Orchestration {
	runID := r.LastRunID()
	return Orchestration{
		RunID:       runID,
		RootRunID:   r.rootRunIDOr(runID),
		ParentRunID: r.parentRun(),
		Depth:       r.Depth(),
		AgentID:     r.agentID(),
		AgentType:   r.spec.Type,
		AgentName:   r.agentName(),
	}
}

// LastRunID returns the most recent run id assigned by Run/PrepareRun on this runner.
func (r *Runner) LastRunID() string {
	if v := r.lastRunID.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Depth returns the orchestrator-controlled depth of this runner.
func (r *Runner) Depth() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.depth
}

// PrepareRun assigns the run identity that the next Run will use and that
// StartChild should attribute as parent_run_id. Call before StartChild when
// children are wired prior to Run (SSE path). Idempotent while a pending id
// remains unconsumed: returns the existing pending/last id.
func (r *Runner) PrepareRun() string {
	if r == nil {
		return ""
	}
	if v := r.pendingRunID.Load(); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	runID := newRunID()
	r.pendingRunID.Store(runID)
	r.lastRunID.Store(runID)
	if r.lineage != nil {
		r.lineage.mu.Lock()
		if r.lineage.rootID == "" {
			r.lineage.rootID = runID
		}
		r.lineage.mu.Unlock()
	}
	return runID
}

// consumeRunID returns the pending prepared id or a fresh one, and clears pending.
func (r *Runner) consumeRunID() string {
	if v := r.pendingRunID.Swap(""); v != nil {
		if s, ok := v.(string); ok && s != "" {
			r.lastRunID.Store(s)
			return s
		}
	}
	runID := newRunID()
	r.lastRunID.Store(runID)
	return runID
}

// Run executes the agent once and emits events with a RunID.
// If PrepareRun was called, that id is reused so pre-wired children share lineage.
func (r *Runner) Run(ctx context.Context, userText string, opts ...agent.Option) error {
	if r == nil || r.agent == nil {
		return fmt.Errorf("runner: nil agent")
	}
	runID := r.consumeRunID()

	// Establish root run id once for the tree (matches PrepareRun if used).
	rootID := runID
	if r.lineage != nil {
		r.lineage.mu.Lock()
		if r.lineage.rootID == "" {
			r.lineage.rootID = runID
		}
		rootID = r.lineage.rootID
		r.lineage.mu.Unlock()
	}
	parentID := r.parentRun()
	depth := r.Depth()

	r.emit(ctx, RunEvent{
		RunID: runID, RootRunID: rootID, ParentRunID: parentID,
		AgentType: r.spec.Type, AgentName: r.agentName(), AgentID: r.agent.ID(),
		Depth: depth, Kind: KindStart, Text: userText,
	})

	var runErr error
	for update, err := range r.agent.RunText(ctx, userText, opts...) {
		if err != nil {
			runErr = err
			r.emit(ctx, RunEvent{
				RunID: runID, RootRunID: rootID, ParentRunID: parentID,
				AgentType: r.spec.Type, AgentName: r.agentName(), AgentID: r.agent.ID(),
				Depth: depth, Kind: KindError, Err: err.Error(),
			})
			break
		}
		if update == nil {
			continue
		}
		for _, c := range update.Contents {
			switch cc := c.(type) {
			case *message.TextContent:
				if cc.Text != "" {
					r.emit(ctx, RunEvent{
						RunID: runID, RootRunID: rootID, ParentRunID: parentID,
						AgentType: r.spec.Type, AgentName: r.agentName(), AgentID: r.agent.ID(),
						Depth: depth, Kind: KindText, Text: cc.Text,
					})
				}
			case *message.FunctionCallContent:
				r.emit(ctx, RunEvent{
					RunID: runID, RootRunID: rootID, ParentRunID: parentID,
					AgentType: r.spec.Type, AgentName: r.agentName(), AgentID: r.agent.ID(),
					Depth: depth, Kind: KindToolStart, ToolName: cc.Name, Text: cc.Arguments,
				})
			case *message.FunctionResultContent:
				text := ""
				if cc.Result != nil {
					text = fmt.Sprint(cc.Result)
				}
				errStr := ""
				if cc.Error != nil {
					errStr = cc.Error.Error()
				}
				r.emit(ctx, RunEvent{
					RunID: runID, RootRunID: rootID, ParentRunID: parentID,
					AgentType: r.spec.Type, AgentName: r.agentName(), AgentID: r.agent.ID(),
					Depth: depth, Kind: KindToolEnd, ToolName: cc.CallID, Text: text, Err: errStr,
				})
			}
		}
	}

	errStr := ""
	if runErr != nil {
		errStr = runErr.Error()
	}
	r.emit(ctx, RunEvent{
		RunID: runID, RootRunID: rootID, ParentRunID: parentID,
		AgentType: r.spec.Type, AgentName: r.agentName(), AgentID: r.agent.ID(),
		Depth: depth, Kind: KindEnd, Err: errStr,
	})
	return runErr
}

// StartChild enforces budgets and authority intersection, then returns a
// StreamingChildTool bound to a child Runner at depth+1.
// Depth is NEVER taken from the caller; it is parent.depth+1 only.
//
// parentRunID for attribution comes from the parent's LastRunID (or root id).
// The returned *Runner is the child orchestrator (for nested StartChild proofs).
func (r *Runner) StartChild(ctx context.Context, child ChildSpec) (tool.FuncTool, *Runner, error) {
	if r == nil {
		return nil, nil, fmt.Errorf("runner: nil")
	}
	if child.Agent == nil {
		return nil, nil, fmt.Errorf("start child: nil child agent")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	r.mu.Lock()
	// Student/default-untrusted: MaxChildren=0 blocks all children.
	if r.spec.MaxChildren <= 0 {
		r.mu.Unlock()
		return nil, nil, fmt.Errorf("start child: max_children=0 (untrusted policy)")
	}
	if r.direct >= r.spec.MaxChildren {
		r.mu.Unlock()
		return nil, nil, fmt.Errorf("start child: direct child budget exhausted (%d)", r.spec.MaxChildren)
	}
	// Child would run at depth r.depth+1. Deny when that exceeds MaxDepth.
	// MaxDepth <= 0 denies all children (no nesting budget).
	childDepth := r.depth + 1
	if r.spec.MaxDepth <= 0 || childDepth > r.spec.MaxDepth {
		maxD := r.spec.MaxDepth
		r.mu.Unlock()
		return nil, nil, fmt.Errorf("start child: max depth %d exceeded (child would be depth %d)", maxD, childDepth)
	}
	if r.lineage != nil {
		r.lineage.mu.Lock()
		if r.spec.MaxTotalChildren > 0 && r.lineage.total >= r.spec.MaxTotalChildren {
			total := r.lineage.total
			maxT := r.spec.MaxTotalChildren
			r.lineage.mu.Unlock()
			r.mu.Unlock()
			return nil, nil, fmt.Errorf("start child: total child budget exhausted (%d, current=%d)", maxT, total)
		}
		r.lineage.total++
		r.lineage.mu.Unlock()
	}
	r.direct++
	r.activeChildren++
	parentSpec := r.spec
	lineage := r.lineage
	sink := r.sink
	r.mu.Unlock()

	// Parent attribution must match the current/upcoming parent Run id.
	// If neither PrepareRun nor Run has assigned identity yet, prepare one now
	// so a later Run() reuses the same id (SSE StartChild-before-Run path).
	parentRunID := r.LastRunID()
	if parentRunID == "" {
		parentRunID = r.PrepareRun()
	}
	_ = lineage // shared via r.lineage on child runner

	// Authority checks (may fail — roll back budget reservation).
	if err := AssertNoAuthorityExpansion(parentSpec.Tools, child.Tools); err != nil {
		r.rollbackChildBudget()
		return nil, nil, err
	}
	filtered := FilterToolsFailClosed(parentSpec.Tools, child.AllowedTools)
	allowedNames := make(map[string]struct{}, len(filtered))
	for _, t := range filtered {
		allowedNames[t.Name()] = struct{}{}
	}
	for _, t := range child.Tools {
		if t == nil {
			continue
		}
		if _, ok := allowedNames[t.Name()]; !ok {
			r.rollbackChildBudget()
			return nil, nil, fmt.Errorf("start child: tool %q not in allowlist intersection", t.Name())
		}
	}

	// Child runner: orchestrator-controlled depth = parent+1.
	childMaxChildren := child.MaxChildren
	if childMaxChildren < 0 {
		childMaxChildren = parentSpec.MaxChildren
	}
	childSpec := AgentSpec{
		Type:             child.Type,
		Name:             child.Name,
		Instructions:     child.Instructions,
		Tools:            child.Tools,
		MaxChildren:      childMaxChildren,
		MaxDepth:         parentSpec.MaxDepth,
		MaxTotalChildren: parentSpec.MaxTotalChildren,
	}
	if childSpec.Name == "" && child.Agent != nil {
		childSpec.Name = child.Agent.Name()
	}

	childRunner := &Runner{
		spec:    childSpec,
		agent:   child.Agent,
		sink:    sink,
		depth:   childDepth,
		lineage: lineage,
	}
	childRunner.parentRunID.Store(parentRunID)

	// Build child run options: instructions + tools from ChildSpec (not parent).
	var runOpts []agent.Option
	if child.Instructions != "" {
		runOpts = append(runOpts, agent.WithInstructions(child.Instructions))
	}
	for _, tl := range child.Tools {
		if tl != nil {
			runOpts = append(runOpts, agent.WithTool(tl))
		}
	}

	st := NewStreamingChildTool(child.Agent, child.Type, parentRunID, sink, runOpts...)
	st.rootRunID = r.rootRunIDOr(parentRunID)
	st.depth = childDepth
	st.childRunner = childRunner
	st.onComplete = func() {
		r.releaseChildSlot()
	}

	return st, childRunner, nil
}

// rollbackChildBudget undoes direct/total/active increments after a failed StartChild
// that had already reserved budget slots.
func (r *Runner) rollbackChildBudget() {
	r.mu.Lock()
	if r.direct > 0 {
		r.direct--
	}
	if r.activeChildren > 0 {
		r.activeChildren--
	}
	r.mu.Unlock()
	if r.lineage != nil {
		r.lineage.mu.Lock()
		if r.lineage.total > 0 {
			r.lineage.total--
		}
		r.lineage.mu.Unlock()
	}
}

// releaseChildSlot decrements activeChildren when a child tool finishes
// (success, error, or cancel). Direct/total budgets count starts, not concurrency.
func (r *Runner) releaseChildSlot() {
	r.mu.Lock()
	if r.activeChildren > 0 {
		r.activeChildren--
	}
	r.mu.Unlock()
}

// ChildBudgetSnapshot exposes counters for tests.
func (r *Runner) ChildBudgetSnapshot() (direct, total, active int) {
	r.mu.Lock()
	direct = r.direct
	active = r.activeChildren
	r.mu.Unlock()
	if r.lineage != nil {
		r.lineage.mu.Lock()
		total = r.lineage.total
		r.lineage.mu.Unlock()
	}
	return direct, total, active
}

func (r *Runner) emit(ctx context.Context, e RunEvent) {
	if r.sink != nil {
		r.sink.Emit(ctx, e)
	}
}

func (r *Runner) agentName() string {
	if r.spec.Name != "" {
		return r.spec.Name
	}
	if r.agent != nil {
		return r.agent.Name()
	}
	return ""
}

func (r *Runner) agentID() string {
	if r.agent != nil {
		return r.agent.ID()
	}
	return ""
}

func (r *Runner) rootRunID() string {
	if r.lineage == nil {
		return ""
	}
	r.lineage.mu.Lock()
	defer r.lineage.mu.Unlock()
	return r.lineage.rootID
}

func (r *Runner) rootRunIDOr(fallback string) string {
	if id := r.rootRunID(); id != "" {
		return id
	}
	return fallback
}

func (r *Runner) parentRun() string {
	if v := r.parentRunID.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ScriptedProvider builds a MAF agent.ProviderConfig from a deterministic update sequence.
// Used so tests exercise real agent.Agent / tool paths without network.
type ScriptedProvider struct {
	Name    string
	Updates []*agent.ResponseUpdate
	// RunFn overrides Updates when set.
	RunFn func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error]
	// Started counts invocations.
	Started atomic.Int64
	// LastCtx is the most recent run context (tests).
	LastCtx atomic.Value // context.Context
	// LastOptions captures the most recent options slice (tests).
	LastOptions atomic.Value // []agent.Option
	// LastMessages captures the most recent messages (tests).
	LastMessages atomic.Value // []*message.Message
}

// ProviderConfig returns a MAF provider config.
func (s *ScriptedProvider) ProviderConfig() agent.ProviderConfig {
	name := s.Name
	if name == "" {
		name = "scripted"
	}
	return agent.ProviderConfig{
		ProviderName: name,
		Run: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			s.Started.Add(1)
			s.LastCtx.Store(ctx)
			optsCopy := append([]agent.Option(nil), options...)
			s.LastOptions.Store(optsCopy)
			msgsCopy := append([]*message.Message(nil), messages...)
			s.LastMessages.Store(msgsCopy)
			if s.RunFn != nil {
				return s.RunFn(ctx, messages, options...)
			}
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				for _, u := range s.Updates {
					select {
					case <-ctx.Done():
						yield(nil, ctx.Err())
						return
					default:
					}
					if !yield(u, nil) {
						return
					}
				}
			}
		},
	}
}

// NewScriptedAgent constructs a MAF agent with the scripted provider.
func NewScriptedAgent(cfg agent.Config, prov *ScriptedProvider) *agent.Agent {
	return agent.New(prov.ProviderConfig(), cfg)
}
