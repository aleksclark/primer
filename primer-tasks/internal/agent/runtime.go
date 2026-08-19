package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"charm.land/fantasy"
	"primer-tasks/internal/agent/protocol"
)

type Limits struct {
	MaxSteps, MaxTokens int
	Deadline            time.Duration
	MaxRetries          int
}

func (l Limits) valid() error {
	if l.MaxSteps < 1 || l.MaxSteps > 100 {
		return errors.New("invalid max steps")
	}
	if l.MaxTokens < 1 || l.MaxTokens > 200000 {
		return errors.New("invalid max tokens")
	}
	if l.Deadline <= 0 || l.Deadline > 30*time.Minute {
		return errors.New("invalid deadline")
	}
	if l.MaxRetries < 0 || l.MaxRetries > 5 {
		return errors.New("invalid retries")
	}
	return nil
}

type Runtime struct {
	Agent  fantasy.Agent
	Limits Limits
	Labels map[string]string
}
type Execution struct {
	// FinalText is the only model output exposed by the durable runtime. The
	// Fantasy result (which may contain reasoning/provider metadata) is never
	// returned to callers or eligible for persistence.
	FinalText string
	Usage     Usage
	Provider  string
	Model     string
	Steps     int
}

var ErrProviderDisabled = errors.New("agent provider is disabled")

// NewFantasyAgent is the sole construction point used by production code. It
// uses Fantasy public APIs and applies finite stop/retry limits at the agent
// boundary; callers cannot accidentally create an unlimited loop.
func NewFantasyAgent(model fantasy.LanguageModel, tools []fantasy.AgentTool, limits Limits) (*Runtime, error) {
	if model == nil {
		return nil, ErrProviderDisabled
	}
	if err := limits.valid(); err != nil {
		return nil, err
	}
	return &Runtime{Agent: fantasy.NewAgent(model, fantasy.WithTools(tools...), fantasy.WithMaxRetries(limits.MaxRetries)), Limits: limits, Labels: map[string]string{}}, nil
}

func (r *Runtime) Execute(ctx context.Context, runID, prompt string, emit func(protocol.Event) error) (Execution, error) {
	if r == nil || r.Agent == nil {
		return Execution{}, ErrProviderDisabled
	}
	if err := r.Limits.valid(); err != nil {
		return Execution{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Limits.Deadline)
	defer cancel()
	seq := int64(0)
	retries := 0
	next := func(e protocol.Event) error { seq++; e.Sequence = seq; e.Cursor = seq; return emit(e) }
	call := fantasy.AgentStreamCall{Prompt: prompt, MaxOutputTokens: ptr(int64(r.Limits.MaxTokens)), StopWhen: []fantasy.StopCondition{fantasy.StepCountIs(r.Limits.MaxSteps), fantasy.MaxTokensUsed(int64(r.Limits.MaxTokens))},
		OnTextStart: func(id string) error { return next(protocol.TextStart(runID, seq+1)) },
		OnTextDelta: func(id, text string) error { return next(protocol.TextDelta(runID, seq+1, text)) },
		OnTextEnd:   func(id string) error { return next(protocol.TextEnd(runID, seq+1)) },
		// Reasoning callbacks intentionally discard the content argument.
		OnReasoningStart: func(id string, _ fantasy.ReasoningContent) error { return next(protocol.ThinkingStart(runID, seq+1)) },
		OnReasoningDelta: func(string, string) error { return nil },
		OnReasoningEnd:   func(id string, _ fantasy.ReasoningContent) error { return next(protocol.ThinkingEnd(runID, seq+1)) },
		OnToolInputStart: func(id, name string) error {
			return next(protocol.ToolProgress(runID, seq+1, r.label(name), "started"))
		},
		OnToolInputDelta: func(string, string) error { return nil },
		OnToolInputEnd:   func(id string) error { return nil },
		OnToolCall: func(c fantasy.ToolCallContent) error {
			return next(protocol.ToolProgress(runID, seq+1, r.label(c.ToolName), "called"))
		},
		OnToolResult: func(c fantasy.ToolResultContent) error {
			if err := next(protocol.ToolProgress(runID, seq+1, r.label(c.ToolName), "completed")); err != nil {
				return err
			}
			// A confirmation preview is a safe, server-issued handle. Only its
			// opaque handle, human summary, and expiry may cross the protocol.
			if preview, ok := readConfirmationPreview(c.Result); ok {
				return next(protocol.Confirmation(runID, seq+1, preview.Handle, preview.Summary, preview.ExpiresAt))
			}
			return nil
		},
		OnRetry: func(_ *fantasy.ProviderError, delay time.Duration) {
			retries++
			_ = next(protocol.Retry(runID, seq+1, retries, delay))
		},
	}
	result, err := r.Agent.Stream(ctx, call)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Execution{}, ctx.Err()
		}
		return Execution{}, fmt.Errorf("provider: %w", err)
	}
	usage := Usage{InputTokens: result.TotalUsage.InputTokens, OutputTokens: result.TotalUsage.OutputTokens, TotalTokens: result.TotalUsage.TotalTokens, ReasoningTokens: result.TotalUsage.ReasoningTokens}
	return Execution{FinalText: result.Response.Content.Text(), Usage: usage, Provider: "fantasy", Model: "fantasy", Steps: len(result.Steps)}, nil
}
func ptr[T any](v T) *T { return &v }

type previewEnvelope struct {
	Handle    string    `json:"handle"`
	Summary   string    `json:"summary"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// confirmationPreview accepts only a server-issued opaque preview encoded in
// the safe text result. Fantasy's typed tool result wrapper is intentionally
// unwrapped here; raw tool input/result content never enters the protocol.
func readConfirmationPreview(result fantasy.ToolResultOutputContent) (previewEnvelope, bool) {
	text, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result)
	if !ok {
		return previewEnvelope{}, false
	}
	var preview previewEnvelope
	if json.Unmarshal([]byte(text.Text), &preview) != nil || preview.Handle == "" || preview.ExpiresAt.IsZero() {
		return previewEnvelope{}, false
	}
	return preview, true
}

func (r *Runtime) label(name string) string {
	if label, ok := r.Labels[name]; ok && label != "" {
		return label
	}
	return "agent tool"
}
