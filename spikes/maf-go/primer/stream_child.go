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
	sink        EventSink
	runOpts     []agent.Option
	// onChildRun is optional hook for tests (e.g. capture child ctx).
	onChildRun func(ctx context.Context)
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
func (t *StreamingChildTool) Call(ctx context.Context, args string) (any, error) {
	if t == nil || t.child == nil {
		return "", fmt.Errorf("streaming child tool: nil child")
	}
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

	childRunID := t.child.ID()
	if childRunID == "" {
		childRunID = "child-run"
	}
	t.sink.Emit(ctx, RunEvent{
		RunID:       childRunID,
		ParentRunID: t.parentRunID,
		AgentType:   t.childType,
		AgentName:   t.child.Name(),
		AgentID:     t.child.ID(),
		Kind:        KindChildStart,
		Text:        in.Query,
	})

	var b strings.Builder
	var runErr error
	stream := t.child.RunText(ctx, in.Query, t.runOpts...)
	for update, err := range stream {
		if err != nil {
			runErr = err
			t.sink.Emit(ctx, RunEvent{
				RunID:       childRunID,
				ParentRunID: t.parentRunID,
				AgentType:   t.childType,
				AgentName:   t.child.Name(),
				AgentID:     t.child.ID(),
				Kind:        KindError,
				Err:         err.Error(),
			})
			break
		}
		if update == nil {
			continue
		}
		// Attribute tool call contents if present.
		for _, c := range update.Contents {
			switch cc := c.(type) {
			case *message.FunctionCallContent:
				t.sink.Emit(ctx, RunEvent{
					RunID:       childRunID,
					ParentRunID: t.parentRunID,
					AgentType:   t.childType,
					AgentName:   t.child.Name(),
					AgentID:     t.child.ID(),
					Kind:        KindToolStart,
					ToolName:    cc.Name,
					Text:        cc.Arguments,
				})
			case *message.FunctionResultContent:
				name := ""
				if cc.CallID != "" {
					name = cc.CallID
				}
				text := ""
				if cc.Result != nil {
					text = fmt.Sprint(cc.Result)
				}
				errStr := ""
				if cc.Error != nil {
					errStr = cc.Error.Error()
				}
				t.sink.Emit(ctx, RunEvent{
					RunID:       childRunID,
					ParentRunID: t.parentRunID,
					AgentType:   t.childType,
					AgentName:   t.child.Name(),
					AgentID:     t.child.ID(),
					Kind:        KindToolEnd,
					ToolName:    name,
					Text:        text,
					Err:         errStr,
				})
			case *message.TextContent:
				if cc.Text != "" {
					b.WriteString(cc.Text)
					t.sink.Emit(ctx, RunEvent{
						RunID:       childRunID,
						ParentRunID: t.parentRunID,
						AgentType:   t.childType,
						AgentName:   t.child.Name(),
						AgentID:     t.child.ID(),
						Kind:        KindText,
						Text:        cc.Text,
					})
				}
			}
		}
		// Also surface update.String() when contents empty but String non-empty.
		if len(update.Contents) == 0 {
			if s := update.String(); s != "" {
				b.WriteString(s)
				t.sink.Emit(ctx, RunEvent{
					RunID:       childRunID,
					ParentRunID: t.parentRunID,
					AgentType:   t.childType,
					AgentName:   t.child.Name(),
					AgentID:     t.child.ID(),
					Kind:        KindText,
					Text:        s,
				})
			}
		}
	}

	final := b.String()
	t.sink.Emit(ctx, RunEvent{
		RunID:       childRunID,
		ParentRunID: t.parentRunID,
		AgentType:   t.childType,
		AgentName:   t.child.Name(),
		AgentID:     t.child.ID(),
		Kind:        KindChildEnd,
		Text:        final,
		Err: func() string {
			if runErr != nil {
				return runErr.Error()
			}
			return ""
		}(),
	})
	if runErr != nil {
		return final, runErr
	}
	return final, nil
}

// Ensure StreamingChildTool implements tool.FuncTool.
var _ tool.FuncTool = (*StreamingChildTool)(nil)
