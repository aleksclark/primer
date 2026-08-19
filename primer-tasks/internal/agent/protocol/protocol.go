// Package protocol defines the versioned, safe wire contract. It contains no
// websocket implementation: socket adapters can use these values without
// gaining access to provider callbacks or durable store internals.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const Version = 1

type CommandKind string

const (
	CommandHello       CommandKind = "hello"
	CommandSubscribe   CommandKind = "subscribe"
	CommandUnsubscribe CommandKind = "unsubscribe"
	CommandUserMessage CommandKind = "user_message"
	CommandCancel      CommandKind = "cancel"
	CommandConfirm     CommandKind = "confirm"
)

type CommandEnvelope struct {
	Protocol        int         `json:"protocol"`
	Kind            CommandKind `json:"kind"`
	RequestID       string      `json:"requestId,omitempty"`
	RunID           string      `json:"runId,omitempty"`
	ConversationID  string      `json:"conversationId,omitempty"`
	Cursor          int64       `json:"cursor,omitempty"`
	ClientMessageID string      `json:"clientMessageId,omitempty"`
	Text            string      `json:"text,omitempty"`
	ConfirmationID  string      `json:"confirmationId,omitempty"`
}

func (c CommandEnvelope) Validate() error {
	if c.Protocol != Version {
		return fmt.Errorf("unsupported protocol version %d", c.Protocol)
	}
	switch c.Kind {
	case CommandHello:
	case CommandSubscribe:
		// A conversation-only subscription is the durable remount boundary:
		// the client may not know the last run yet, but the server can replay
		// the conversation from its cursor. Reconnects may include both.
		if c.RunID == "" && c.ConversationID == "" {
			return errors.New("runId or conversationId is required")
		}
	case CommandUnsubscribe:
		if c.RunID == "" {
			return errors.New("runId is required")
		}
	case CommandCancel:
		if c.RunID == "" {
			return errors.New("runId is required")
		}
	case CommandUserMessage:
		if c.ConversationID == "" || c.ClientMessageID == "" || c.Text == "" {
			return errors.New("conversationId, clientMessageId and text are required")
		}
	case CommandConfirm:
		if c.RunID == "" || c.ConfirmationID == "" {
			return errors.New("runId and confirmationId are required")
		}
	default:
		return fmt.Errorf("unknown command kind %q", c.Kind)
	}
	return nil
}

type EventKind string

const (
	EventHello         EventKind = "hello"
	EventTextStart     EventKind = "text_start"
	EventTextDelta     EventKind = "text_delta"
	EventTextEnd       EventKind = "text_end"
	EventThinkingStart EventKind = "thinking_start"
	EventThinkingEnd   EventKind = "thinking_end"
	EventToolProgress  EventKind = "tool_progress"
	EventRetry         EventKind = "retry"
	EventConfirmation  EventKind = "confirmation_required"
	EventTerminal      EventKind = "terminal"
	EventError         EventKind = "error"
	EventReplayGap     EventKind = "replay_gap"
)

type Event struct {
	Protocol       int       `json:"protocol"`
	Kind           EventKind `json:"kind"`
	RunID          string    `json:"runId"`
	Sequence       int64     `json:"sequence"`
	Cursor         int64     `json:"cursor"`
	Time           time.Time `json:"time"`
	Text           string    `json:"text,omitempty"`
	Label          string    `json:"label,omitempty"`
	Phase          string    `json:"phase,omitempty"`
	Status         string    `json:"status,omitempty"`
	Code           string    `json:"code,omitempty"`
	Retry          int       `json:"retry,omitempty"`
	RetryAfterMS   int       `json:"retryAfterMs,omitempty"`
	ConfirmationID string    `json:"confirmationId,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt,omitempty"`
}

func base(kind EventKind, run string, sequence int64) Event {
	return Event{Protocol: Version, Kind: kind, RunID: run, Sequence: sequence, Cursor: sequence, Time: time.Now().UTC()}
}
func TextStart(run string, seq int64) Event { return base(EventTextStart, run, seq) }
func TextDelta(run string, seq int64, text string) Event {
	e := base(EventTextDelta, run, seq)
	e.Text = text
	return e
}
func TextEnd(run string, seq int64) Event       { return base(EventTextEnd, run, seq) }
func ThinkingStart(run string, seq int64) Event { return base(EventThinkingStart, run, seq) }
func ThinkingEnd(run string, seq int64) Event   { return base(EventThinkingEnd, run, seq) }
func ToolProgress(run string, seq int64, label, phase string) Event {
	e := base(EventToolProgress, run, seq)
	e.Label = label
	e.Phase = phase
	return e
}
func Retry(run string, seq int64, n int, after time.Duration) Event {
	e := base(EventRetry, run, seq)
	e.Retry = n
	e.RetryAfterMS = int(after / time.Millisecond)
	return e
}
func Confirmation(run string, seq int64, id, summary string, expires time.Time) Event {
	e := base(EventConfirmation, run, seq)
	e.ConfirmationID, e.Summary, e.ExpiresAt = id, summary, expires
	return e
}
func Terminal(run string, seq int64, status string) Event {
	e := base(EventTerminal, run, seq)
	e.Status = status
	return e
}
func Error(run string, seq int64, code string) Event {
	e := base(EventError, run, seq)
	e.Code = code
	return e
}

func (e Event) MarshalJSON() ([]byte, error) {
	// Explicitly marshal the allowlisted event, rather than provider objects.
	type wire Event
	return json.Marshal(wire(e))
}

// Schema is a small offline-generated contract consumed by client generation.
// It is returned as data so CI can compare checked-in generated output without
// requiring a running server.
func Schema() map[string]any {
	return map[string]any{
		"protocol":    Version,
		"commands":    []string{"hello", "subscribe", "unsubscribe", "user_message", "cancel", "confirm"},
		"events":      []string{"hello", "text_start", "text_delta", "text_end", "thinking_start", "thinking_end", "tool_progress", "confirmation_required", "retry", "terminal", "error", "replay_gap"},
		"eventFields": map[string][]string{"text_delta": {"text"}, "tool_progress": {"label", "phase"}, "confirmation_required": {"confirmationId", "summary", "expiresAt"}, "retry": {"retry", "retryAfterMs"}, "terminal": {"status"}, "error": {"code"}},
	}
}
func SchemaJSON() ([]byte, error) { return json.MarshalIndent(Schema(), "", "  ") }
