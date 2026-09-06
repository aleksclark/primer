package agent

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"primer-tasks/internal/agent/protocol"
)

type qualificationInput struct {
	Name string `json:"name" jsonschema:"description=the name"`
}
type scriptedModel struct {
	calls  int
	cancel bool
	fail   bool
}

func (m *scriptedModel) Provider() string { return "scripted" }
func (m *scriptedModel) Model() string    { return "qualification" }
func (m *scriptedModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("not used")
}
func (m *scriptedModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, errors.New("not used")
}
func (m *scriptedModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, errors.New("not used")
}
func (m *scriptedModel) Stream(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	if m.fail {
		return nil, errors.New("scripted provider failure")
	}
	if m.cancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	m.calls++
	n := m.calls
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		parts := []fantasy.StreamPart{}
		if n == 1 {
			parts = []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeReasoningStart, ID: "r"}, {Type: fantasy.StreamPartTypeReasoningDelta, ID: "r", Delta: "secret reasoning must not be forwarded"}, {Type: fantasy.StreamPartTypeReasoningEnd, ID: "r"},
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "t", ToolCallName: "qualify_tool"}, {Type: fantasy.StreamPartTypeToolInputDelta, ID: "t", Delta: `{"name":"Ada"}`}, {Type: fantasy.StreamPartTypeToolInputEnd, ID: "t"}, {Type: fantasy.StreamPartTypeToolCall, ID: "t", ToolCallName: "qualify_tool", ToolCallInput: `{"name":"Ada"}`}, {Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			}
		} else {
			parts = []fantasy.StreamPart{{Type: fantasy.StreamPartTypeTextStart, ID: "x"}, {Type: fantasy.StreamPartTypeTextDelta, ID: "x", Delta: "done"}, {Type: fantasy.StreamPartTypeTextEnd, ID: "x"}, {Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}}}
		}
		for _, p := range parts {
			if !yield(p) {
				return
			}
		}
	}), nil
}

func TestFantasyQualificationUsesPublicTypedToolAndSafeCallbacks(t *testing.T) {
	model := &scriptedModel{}
	called := false
	tool := fantasy.NewAgentTool("qualify_tool", "typed qualification tool", func(_ context.Context, in qualificationInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		called = in.Name == "Ada"
		return fantasy.NewTextResponse("tool result contains secret-id"), nil
	})
	r, err := NewFantasyAgent(model, []fantasy.AgentTool{tool}, Limits{MaxSteps: 4, MaxTokens: 100, Deadline: time.Second, MaxRetries: 0})
	if err != nil {
		t.Fatal(err)
	}
	var events []protocol.Event
	result, err := r.Execute(context.Background(), "run-1", "qualify", func(e protocol.Event) error { events = append(events, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalText != "done" || !called {
		t.Fatalf("tool loop did not run: %#v called=%v", result, called)
	}
	var text strings.Builder
	thinking := 0
	toolProgress := 0
	for _, e := range events {
		if e.Kind == protocol.EventTextDelta {
			text.WriteString(e.Text)
		}
		if e.Kind == protocol.EventThinkingStart || e.Kind == protocol.EventThinkingEnd {
			thinking++
		}
		if e.Kind == protocol.EventToolProgress {
			toolProgress++
		}
		if strings.Contains(e.Text, "secret") || strings.Contains(e.Label, "secret") {
			t.Fatalf("unsafe event: %#v", e)
		}
	}
	if text.String() != "done" || thinking != 2 || toolProgress < 2 {
		t.Fatalf("events=%#v text=%q thinking=%d tools=%d", events, text.String(), thinking, toolProgress)
	}
}
func TestFantasyQualificationContextCancelAndProviderFailureShape(t *testing.T) {
	model := &scriptedModel{cancel: true}
	r, err := NewFantasyAgent(model, nil, Limits{MaxSteps: 1, MaxTokens: 10, Deadline: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = r.Execute(ctx, "r", "x", func(protocol.Event) error { return nil }); err == nil {
		t.Fatal("cancel was swallowed")
	}
	if _, err := NewFantasyAgent(nil, nil, Limits{MaxSteps: 1, MaxTokens: 10, Deadline: time.Second}); !errors.Is(err, ErrProviderDisabled) {
		t.Fatal("disabled provider not explicit")
	}
	failing := &scriptedModel{fail: true}
	r, err = NewFantasyAgent(failing, nil, Limits{MaxSteps: 1, MaxTokens: 10, Deadline: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Execute(context.Background(), "r", "x", func(protocol.Event) error { return nil }); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("provider failure was not classified: %v", err)
	}
}
func TestReservedFileShapeIsNotAcceptedByRuntimeWire(t *testing.T) {
	// Phase 5 may add file parts. Phase 3 accepts only text prompt input and
	// therefore has no path that can persist or broadcast provider files.
	b, _ := protocol.SchemaJSON()
	if strings.Contains(string(b), "file") {
		t.Fatal("file support accidentally entered phase 3 schema")
	}
}
