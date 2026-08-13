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
	// MaxDepth limits nested child depth (parent depth=0).
	MaxDepth int
	// MaxTotalChildren limits total children started under a root run.
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
}

// RunEvent is a Primer-shaped streaming event with parent/child attribution.
type RunEvent struct {
	RunID       string    `json:"run_id"`
	ParentRunID string    `json:"parent_run_id,omitempty"`
	AgentType   string    `json:"agent_type,omitempty"`
	AgentName   string    `json:"agent_name,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
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
