package agent

import (
	"context"
	"time"

	maftool "github.com/microsoft/agent-framework-go/tool"
)

// Event kinds emitted on the Primer-facing event stream.
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
// Student/default-untrusted policy must set MaxChildren=0.
type AgentSpec struct {
	Type         string
	Name         string
	Instructions string
	// Tools is the full grant surface for this agent.
	Tools []maftool.Tool
	// MaxChildren limits direct StartChild calls (0 = none allowed).
	MaxChildren int
	// MaxDepth limits nested child depth. Root is depth 0; a direct child is
	// depth 1. StartChild is denied when child depth would exceed MaxDepth.
	// MaxDepth <= 0 denies all children.
	MaxDepth int
	// MaxTotalChildren limits total children started across the whole run tree.
	// 0 means no total limit (direct MaxChildren still applies).
	MaxTotalChildren int
}

// ChildSpec describes a tailored child invocation request.
// AllowedTools is a fail-closed allowlist intersected with parent grants.
type ChildSpec struct {
	Type         string
	Name         string
	Instructions string
	// AllowedTools is a fail-closed allowlist. Empty means no tools.
	AllowedTools []string
	// Tools is the child's concrete tool set after factory filtering.
	// Must be a subset of parent grants by name.
	Tools []maftool.Tool
	// MaxChildren limits further StartChild calls from the child runner.
	// 0 = no grandchildren. Negative = inherit parent MaxChildren.
	MaxChildren int
}

// Orchestration is the orchestrator-controlled lineage snapshot for one runner node.
// Callers never supply Depth; it is derived only by Runner.New / StartChild.
type Orchestration struct {
	RunID       string
	RootRunID   string
	ParentRunID string
	Depth       int
	AgentID     string
	AgentType   string
	AgentName   string
}

// RunEvent is a Primer-shaped streaming event with parent/child/root attribution.
// JSON field names are stable; treat the envelope as a versioned public contract.
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

// EventSink receives best-effort stream events. Implementations must be
// non-blocking or bounded: subscriber loss must never fail the underlying run.
type EventSink interface {
	Emit(ctx context.Context, e RunEvent)
}

// NoopSink discards all events.
type NoopSink struct{}

func (NoopSink) Emit(context.Context, RunEvent) {}
