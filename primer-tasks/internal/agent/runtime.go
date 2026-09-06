package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	Agent        fantasy.Agent
	Limits       Limits
	Labels       map[string]string
	allowedTools map[string]struct{}
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
	allowedTools := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		allowedTools[tool.Info().Name] = struct{}{}
	}
	return &Runtime{Agent: fantasy.NewAgent(model, fantasy.WithSystemPrompt(ParentPolicyV1), fantasy.WithTools(tools...), fantasy.WithMaxRetries(limits.MaxRetries)), Limits: limits, Labels: safeToolLabels(), allowedTools: allowedTools}, nil
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
	var emitMu sync.Mutex
	retries := 0
	var pendingConfirmation atomic.Bool
	var callbackMu sync.Mutex
	var callbackErr error
	failCallback := func(err error) error {
		callbackMu.Lock()
		if callbackErr == nil {
			callbackErr = err
		}
		callbackMu.Unlock()
		cancel()
		return err
	}
	next := func(e protocol.Event) error {
		emitMu.Lock()
		defer emitMu.Unlock()
		seq++
		e.Sequence = seq
		e.Cursor = seq
		if err := emit(e); err != nil {
			return failCallback(err)
		}
		return nil
	}
	call := fantasy.AgentStreamCall{Prompt: prompt, MaxOutputTokens: ptr(int64(r.Limits.MaxTokens)), StopWhen: []fantasy.StopCondition{fantasy.StepCountIs(r.Limits.MaxSteps), fantasy.MaxTokensUsed(int64(r.Limits.MaxTokens)), func([]fantasy.StepResult) bool { return pendingConfirmation.Load() }},
		OnTextStart: func(id string) error { return next(protocol.TextStart(runID, 0)) },
		OnTextDelta: func(id, text string) error { return next(protocol.TextDelta(runID, 0, text)) },
		OnTextEnd:   func(id string) error { return next(protocol.TextEnd(runID, 0)) },
		// Reasoning callbacks intentionally discard the content argument.
		OnReasoningStart: func(id string, _ fantasy.ReasoningContent) error { return next(protocol.ThinkingStart(runID, 0)) },
		OnReasoningDelta: func(string, string) error { return nil },
		OnReasoningEnd:   func(id string, _ fantasy.ReasoningContent) error { return next(protocol.ThinkingEnd(runID, 0)) },
		OnToolInputStart: func(id, name string) error {
			return next(protocol.ToolProgress(runID, 0, r.label(name), "started"))
		},
		OnToolInputDelta: func(string, string) error { return nil },
		OnToolInputEnd:   func(id string) error { return nil },
		OnToolCall: func(c fantasy.ToolCallContent) error {
			if c.Invalid || c.ProviderExecuted {
				return errors.New("agent tool call rejected")
			}
			if _, allowed := r.allowedTools[c.ToolName]; !allowed {
				// A provider must not gain authority by emitting a tool call that
				// was omitted from the server-owned active tool set. Stop before
				// any tool result can commit a mutation.
				return errors.New("agent tool unavailable")
			}
			return next(protocol.ToolProgress(runID, 0, r.label(c.ToolName), "called"))
		},
		OnToolResult: func(c fantasy.ToolResultContent) error {
			if err := next(protocol.ToolProgress(runID, 0, r.label(c.ToolName), "completed")); err != nil {
				return err
			}
			if _, failed := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](c.Result); failed {
				// Fantasy validation/not-found failures are untrusted provider/tool
				// details. Stop this run with the outer safe terminal event instead
				// of allowing a scripted next step to claim an effect.
				return failCallback(errors.New("agent tool failed"))
			}
			// A confirmation preview is a safe, server-issued handle. Only its
			// opaque handle, human summary, and expiry may cross the protocol.
			if preview, ok := readConfirmationPreview(c.Result); ok {
				pendingConfirmation.Store(true)
				return next(protocol.Confirmation(runID, 0, preview.Handle, preview.Summary, preview.ExpiresAt))
			}
			return nil
		},
		OnRetry: func(_ *fantasy.ProviderError, delay time.Duration) {
			retries++
			_ = next(protocol.Retry(runID, 0, retries, delay))
		},
	}
	result, err := r.Agent.Stream(ctx, call)
	callbackMu.Lock()
	emittedErr := callbackErr
	callbackMu.Unlock()
	if emittedErr != nil {
		return Execution{}, emittedErr
	}
	if ctx.Err() != nil {
		return Execution{}, ctx.Err()
	}
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

// readConfirmationPreview accepts only a server-issued opaque preview encoded
// in a safe text result; raw tool input/result content never enters the wire.
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

// safeToolLabels is an explicit, server-owned projection. Tool names and
// provider input must never become browser-facing progress text.
func safeToolLabels() map[string]string {
	return map[string]string{
		"list_students": "List students", "get_student": "Inspect student",
		"list_tasks": "List tasks", "get_task": "Inspect task",
		"draft_task": "Draft task", "update_task": "Update task",
		"publish_task": "Publish task", "retire_task": "Retire task",
		"list_schedules": "List schedules", "create_schedule": "Create schedule",
		"update_schedule": "Update schedule", "disable_schedule": "Disable schedule",
		"list_occurrences": "List occurrences", "preview_action": "Prepare change",
		"confirm_action": "Confirm change",
	}
}
func (r *Runtime) label(name string) string {
	if label, ok := r.Labels[name]; ok && label != "" {
		return label
	}
	return "agent tool"
}
