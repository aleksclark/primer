// Package primer is a throwaway Primer-facing adapter over public MAF Go APIs.
// It exists only to answer short-term feasibility questions F1–F8.
package primer

import (
	"context"
	"time"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/tool"
)

// Event kinds emitted on the Primer-facing stream surface.
const (
	KindStart      = "start"
	KindText       = "text"
	KindToolStart  = "tool_start"
	KindToolEnd    = "tool_end"
	KindChildStart = "child_start"
	KindChildEnd   = "child_end"
	KindError      = "error"
	KindEnd        = "end"
)

// AgentSpec describes a parent or standalone agent.
// Student/default-untrusted policy should set MaxChildren=0.
type AgentSpec struct {
	Type         string
	Name         string
	Instructions string
	// Tools are the full grant surface for this agent.
	Tools []tool.Tool
	// MaxChildren limits direct StartChild calls from this agent (0 = none).
	MaxChildren int
	// MaxDepth limits nested child depth. Root orchestration is depth 0;
	// a direct child runs at depth 1. StartChild is denied when the child
	// would exceed MaxDepth (i.e. when current depth >= MaxDepth).
	// MaxDepth <= 0 means no nested children beyond the root's own policy
	// combined with MaxChildren (depth-0 StartChild still allowed when MaxChildren > 0
	// only if MaxDepth > 0; MaxDepth == 0 denies all children by depth).
	MaxDepth int
	// MaxTotalChildren limits total children started under a root run tree.
	// 0 means unlimited total (direct MaxChildren still applies).
	MaxTotalChildren int
}

// ChildSpec describes a tailored child invocation request.
// AllowedTools is a fail-closed allowlist intersected with parent grants.
type ChildSpec struct {
	Type         string
	Name         string
	Instructions string
	// AllowedTools fail-closed allowlist. Empty means no tools.
	AllowedTools []string
	// Agent is the prebuilt MAF child agent (own identity/model/tools already applied
	// by the factory). Runner still enforces budgets and never broadens tools.
	Agent *agent.Agent
	// Tools is the child's concrete tool set after factory filtering.
	// Must be a subset of parent grants by name.
	Tools []tool.Tool
	// MaxChildren limits further StartChild calls from the child runner.
	// If 0, the child cannot start grandchildren (in addition to depth limits).
	// Negative means inherit parent MaxChildren.
	MaxChildren int
}

// Orchestration is the orchestrator-controlled lineage for one runner node.
// Callers never supply Depth; it is derived only by Runner.New / StartChild.
type Orchestration struct {
	// RunID is fresh per Run invocation (not the agent ID).
	RunID string
	// RootRunID is the root invocation id for the tree (equals RunID at root).
	RootRunID string
	// ParentRunID is the immediate parent run (empty at root).
	ParentRunID string
	// Depth is 0 at root; each StartChild child-runner is parent.Depth+1.
	Depth int
	// AgentID is the stable agent identity (distinct from RunID).
	AgentID string
	// AgentType / AgentName are attribution labels.
	AgentType string
	AgentName string
}

// RunEvent is a Primer-shaped streaming event with parent/child attribution.
type RunEvent struct {
	RunID       string    `json:"run_id"`
	RootRunID   string    `json:"root_run_id,omitempty"`
	ParentRunID string    `json:"parent_run_id,omitempty"`
	AgentType   string    `json:"agent_type,omitempty"`
	AgentName   string    `json:"agent_name,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	Depth       int       `json:"depth,omitempty"`
	Kind        string    `json:"kind"`
	Text        string    `json:"text,omitempty"`
	ToolName    string    `json:"tool_name,omitempty"`
	Err         string    `json:"error,omitempty"`
	At          time.Time `json:"at"`
}

// EventSink receives best-effort stream events. Implementations must not
// permanently block the runner; subscriber loss must not fail the underlying run.
type EventSink interface {
	Emit(ctx context.Context, e RunEvent)
}

// NoopSink discards events.
type NoopSink struct{}

func (NoopSink) Emit(context.Context, RunEvent) {}
