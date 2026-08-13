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

// Runner orchestrates parent runs and budgeted StartChild calls using public MAF APIs.
type Runner struct {
	spec   AgentSpec
	agent  *agent.Agent
	sink   EventSink
	mu     sync.Mutex
	depth  int
	direct int
	total  int
	// activeChildren tracks in-flight child starts for budget accounting.
	activeChildren int
}

// NewRunner wraps a prebuilt MAF agent with Primer budgets and event fanout.
func NewRunner(spec AgentSpec, a *agent.Agent, sink EventSink) *Runner {
	if sink == nil {
		sink = NoopSink{}
	}
	if spec.MaxDepth <= 0 {
		// 0 means no nesting beyond parent unless MaxChildren also 0.
		// Keep 0 as "no children depth budget" when MaxChildren==0.
	}
	return &Runner{spec: spec, agent: a, sink: sink}
}

// Run executes the parent agent once (session optional via opts) and emits events.
func (r *Runner) Run(ctx context.Context, userText string, opts ...agent.Option) error {
	if r == nil || r.agent == nil {
		return fmt.Errorf("runner: nil agent")
	}
	runID := r.agent.ID()
	if runID == "" {
		runID = uuid.NewString()
	}
	r.sink.Emit(ctx, RunEvent{
		RunID:     runID,
		AgentType: r.spec.Type,
		AgentName: r.agent.Name(),
		AgentID:   r.agent.ID(),
		Kind:      KindStart,
		Text:      userText,
	})

	var runErr error
	for update, err := range r.agent.RunText(ctx, userText, opts...) {
		if err != nil {
			runErr = err
			r.sink.Emit(ctx, RunEvent{
				RunID:     runID,
				AgentType: r.spec.Type,
				AgentName: r.agent.Name(),
				AgentID:   r.agent.ID(),
				Kind:      KindError,
				Err:       err.Error(),
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
					r.sink.Emit(ctx, RunEvent{
						RunID:     runID,
						AgentType: r.spec.Type,
						AgentName: r.agent.Name(),
						AgentID:   r.agent.ID(),
						Kind:      KindText,
						Text:      cc.Text,
					})
				}
			case *message.FunctionCallContent:
				r.sink.Emit(ctx, RunEvent{
					RunID:     runID,
					AgentType: r.spec.Type,
					AgentName: r.agent.Name(),
					AgentID:   r.agent.ID(),
					Kind:      KindToolStart,
					ToolName:  cc.Name,
					Text:      cc.Arguments,
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
				r.sink.Emit(ctx, RunEvent{
					RunID:     runID,
					AgentType: r.spec.Type,
					AgentName: r.agent.Name(),
					AgentID:   r.agent.ID(),
					Kind:      KindToolEnd,
					ToolName:  cc.CallID,
					Text:      text,
					Err:       errStr,
				})
			}
		}
	}

	r.sink.Emit(ctx, RunEvent{
		RunID:     runID,
		AgentType: r.spec.Type,
		AgentName: r.agent.Name(),
		AgentID:   r.agent.ID(),
		Kind:      KindEnd,
		Err: func() string {
			if runErr != nil {
				return runErr.Error()
			}
			return ""
		}(),
	})
	return runErr
}

// StartChild enforces budgets and authority intersection, then returns a
// StreamingChildTool bound to the child. Does not broaden tools.
func (r *Runner) StartChild(ctx context.Context, parentRunID string, child ChildSpec) (tool.FuncTool, error) {
	if r == nil {
		return nil, fmt.Errorf("runner: nil")
	}
	if child.Agent == nil {
		return nil, fmt.Errorf("start child: nil child agent")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Student/default-untrusted: MaxChildren=0 blocks all children.
	if r.spec.MaxChildren <= 0 {
		return nil, fmt.Errorf("start child: max_children=0 (untrusted policy)")
	}
	if r.direct >= r.spec.MaxChildren {
		return nil, fmt.Errorf("start child: direct child budget exhausted (%d)", r.spec.MaxChildren)
	}
	if r.spec.MaxDepth > 0 && r.depth >= r.spec.MaxDepth {
		return nil, fmt.Errorf("start child: max depth %d exceeded", r.spec.MaxDepth)
	}
	if r.spec.MaxTotalChildren > 0 && r.total >= r.spec.MaxTotalChildren {
		return nil, fmt.Errorf("start child: total child budget exhausted (%d)", r.spec.MaxTotalChildren)
	}

	// Authority: child tools must be subset of parent grants.
	if err := AssertNoAuthorityExpansion(r.spec.Tools, child.Tools); err != nil {
		return nil, err
	}
	// Also re-filter by allowlist names for defense in depth.
	filtered := FilterToolsFailClosed(r.spec.Tools, child.AllowedTools)
	// Child.Tools should already be filtered; ensure names ⊆ filtered.
	allowedNames := make(map[string]struct{}, len(filtered))
	for _, t := range filtered {
		allowedNames[t.Name()] = struct{}{}
	}
	for _, t := range child.Tools {
		if t == nil {
			continue
		}
		if _, ok := allowedNames[t.Name()]; !ok && len(child.AllowedTools) > 0 {
			return nil, fmt.Errorf("start child: tool %q not in allowlist intersection", t.Name())
		}
	}

	r.direct++
	r.total++
	r.activeChildren++

	st := NewStreamingChildTool(child.Agent, child.Type, parentRunID, r.sink)
	return st, nil
}

// ChildBudgetSnapshot exposes counters for tests.
func (r *Runner) ChildBudgetSnapshot() (direct, total, active int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.direct, r.total, r.activeChildren
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
