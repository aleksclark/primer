package primer

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/microsoft/agent-framework-go/tool"
)

// StreamingChildTool is a public-API-only FuncTool that invokes a child agent
// via child.Run (streaming) and surfaces child start/text/tool/end events on sink.
// Unlike stock agenttool.New, this does NOT Collect() away intermediate updates.
type StreamingChildTool struct {
	child       *agent.Agent
	childType   string
	parentRunID string
	rootRunID   string
	depth       int
	sink        EventSink
	runOpts     []agent.Option
	// onChildRun is optional hook for tests (e.g. capture child ctx).
	onChildRun func(ctx context.Context)
	// onComplete is invoked once when Call returns (success, error, or cancel).
	// Used by Runner to close activeChildren accounting.
	onComplete func()
	// childRunner is the orchestrator-controlled child Runner (depth parent+1).
	// Exposed for nested StartChild proofs; nil when constructed outside Runner.
	childRunner *Runner
	// runIDGen optional override for child run ids (tests).
	runIDGen func() string
}

// NewStreamingChildTool builds a streaming child tool adapter.
func NewStreamingChildTool(child *agent.Agent, childType, parentRunID string, sink EventSink, runOpts ...agent.Option) *StreamingChildTool {
	if sink == nil {
		sink = NoopSink{}
	}
	return &StreamingChildTool{
		child:       child,
		childType:   childType,
		parentRunID: parentRunID,
		sink:        sink,
		runOpts:     runOpts,
	}
}

// ChildRunner returns the orchestrator-controlled child runner, if any.
func (t *StreamingChildTool) ChildRunner() *Runner {
	if t == nil {
		return nil
	}
	return t.childRunner
}

var invalidNameChars = regexp.MustCompile(`[^0-9A-Za-z]+`)

func (t *StreamingChildTool) Name() string {
	if t == nil || t.child == nil {
		return "child"
	}
	n := t.child.Name()
	if n == "" {
		n = "child"
	}
	return invalidNameChars.ReplaceAllString(n, "_")
}

func (t *StreamingChildTool) Description() string {
	if t == nil || t.child == nil {
		return "Invoke a child agent (streaming)."
	}
	if d := t.child.Description(); d != "" {
		return d
	}
	return "Invoke a child agent (streaming)."
}

func (t *StreamingChildTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "input query to invoke the child agent",
			},
		},
		"required": []string{"query"},
	}
}

func (t *StreamingChildTool) ReturnSchema() any {
	return map[string]any{"type": "string"}
}

// Call runs the child with streaming and emits attributed events.
// Always invokes onComplete on return (success, error, cancel).
func (t *StreamingChildTool) Call(ctx context.Context, args string) (any, error) {
	if t == nil || t.child == nil {
		return "", fmt.Errorf("streaming child tool: nil child")
	}
	defer func() {
		if t.onComplete != nil {
			t.onComplete()
		}
	}()

	var in struct {
		Query string `json:"query"`
	}
	if args == "" {
		args = "{}"
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", err
	}
	if t.onChildRun != nil {
		t.onChildRun(ctx)
	}

	gen := t.runIDGen
	if gen == nil {
		gen = newRunID
	}
	childRunID := gen()
	// If child runner exists, record this as its last run id for grandchild attribution.
	if t.childRunner != nil {
		t.childRunner.lastRunID.Store(childRunID)
		// Ensure root id is established for the tree.
		if t.childRunner.lineage != nil {
			t.childRunner.lineage.mu.Lock()
			if t.childRunner.lineage.rootID == "" && t.rootRunID != "" {
				t.childRunner.lineage.rootID = t.rootRunID
			}
			if t.rootRunID == "" {
				t.rootRunID = t.childRunner.lineage.rootID
			}
			t.childRunner.lineage.mu.Unlock()
		}
	}
	rootID := t.rootRunID
	if rootID == "" {
		rootID = t.parentRunID
	}

	base := RunEvent{
		RunID:       childRunID,
		RootRunID:   rootID,
		ParentRunID: t.parentRunID,
		AgentType:   t.childType,
		AgentName:   t.child.Name(),
		AgentID:     t.child.ID(),
		Depth:       t.depth,
	}

	t.sink.Emit(ctx, withKind(base, KindChildStart, in.Query, ""))

	var b strings.Builder
	var runErr error
	stream := t.child.RunText(ctx, in.Query, t.runOpts...)
	for update, err := range stream {
		if err != nil {
			runErr = err
			t.sink.Emit(ctx, withKind(base, KindError, "", err.Error()))
			break
		}
		if update == nil {
			continue
		}
		for _, c := range update.Contents {
			switch cc := c.(type) {
			case *message.FunctionCallContent:
				ev := withKind(base, KindToolStart, cc.Arguments, "")
				ev.ToolName = cc.Name
				t.sink.Emit(ctx, ev)
			case *message.FunctionResultContent:
				text := ""
				if cc.Result != nil {
					text = fmt.Sprint(cc.Result)
				}
				errStr := ""
				if cc.Error != nil {
					errStr = cc.Error.Error()
				}
				ev := withKind(base, KindToolEnd, text, errStr)
				ev.ToolName = cc.CallID
				t.sink.Emit(ctx, ev)
			case *message.TextContent:
				if cc.Text != "" {
					b.WriteString(cc.Text)
					t.sink.Emit(ctx, withKind(base, KindText, cc.Text, ""))
				}
			}
		}
		if len(update.Contents) == 0 {
			if s := update.String(); s != "" {
				b.WriteString(s)
				t.sink.Emit(ctx, withKind(base, KindText, s, ""))
			}
		}
	}

	final := b.String()
	errStr := ""
	if runErr != nil {
		errStr = runErr.Error()
	}
	t.sink.Emit(ctx, withKind(base, KindChildEnd, final, errStr))
	if runErr != nil {
		return final, runErr
	}
	return final, nil
}

func withKind(base RunEvent, kind, text, errStr string) RunEvent {
	base.Kind = kind
	base.Text = text
	base.Err = errStr
	return base
}

// Ensure StreamingChildTool implements tool.FuncTool.
var _ tool.FuncTool = (*StreamingChildTool)(nil)
