// Package domain holds primer-agents domain types and the frozen run lifecycle
// state machine. No persistence logic lives here.
package domain

import (
	"errors"
	"time"
)

// ─── Session ─────────────────────────────────────────────────────────────────

// SessionStatus enumerates the two stable states a session can be in.
type SessionStatus string

const (
	SessionStatusOpen   SessionStatus = "open"
	SessionStatusClosed SessionStatus = "closed"
)

// Session is a durable multi-turn agent session.
type Session struct {
	ID             string        `db:"id"`
	OwnerNamespace string        `db:"owner_namespace"`
	CallerContext  *string       `db:"caller_context"`
	Profile        string        `db:"profile"`
	Status         SessionStatus `db:"status"`
	Revision       int64         `db:"revision"`
	StateRef       *string       `db:"state_ref"`
	CreatedAt      time.Time     `db:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"`
	ExpiresAt      *time.Time    `db:"expires_at"`
}

// ─── Run ─────────────────────────────────────────────────────────────────────

// RunStatus enumerates every durable run state.
type RunStatus string

const (
	// Non-terminal.
	RunStatusQueued          RunStatus = "queued"
	RunStatusRunning         RunStatus = "running"
	RunStatusCancelRequested RunStatus = "cancel_requested"

	// Terminal.
	RunStatusSucceeded  RunStatus = "succeeded"
	RunStatusFailed     RunStatus = "failed"
	RunStatusCanceled   RunStatus = "canceled"
	RunStatusInterrupted RunStatus = "interrupted"
)

// IsTerminal reports whether s is a terminal (non-continuable) run status.
func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunStatusSucceeded, RunStatusFailed, RunStatusCanceled, RunStatusInterrupted:
		return true
	default:
		return false
	}
}

// validTransitions is the frozen lifecycle graph.
var validTransitions = map[RunStatus]map[RunStatus]bool{
	RunStatusQueued: {
		RunStatusRunning:         true,
		RunStatusCancelRequested: true,
	},
	RunStatusRunning: {
		RunStatusSucceeded:       true,
		RunStatusFailed:          true,
		RunStatusInterrupted:     true,
		RunStatusCancelRequested: true,
	},
	RunStatusCancelRequested: {
		RunStatusCanceled: true,
	},
}

// ErrInvalidTransition is returned when a requested state change is not in
// the frozen state machine.
var ErrInvalidTransition = errors.New("invalid run lifecycle transition")

// CanTransition reports whether the from→to transition is permitted.
func CanTransition(from, to RunStatus) bool {
	targets, ok := validTransitions[from]
	return ok && targets[to]
}

// Run is a durable agent run record.
type Run struct {
	ID                 string     `db:"id"`
	SessionID          *string    `db:"session_id"`
	OwnerNamespace     string     `db:"owner_namespace"`
	IdempotencyKey     string     `db:"idempotency_key"`
	IdempotencyHash    string     `db:"idempotency_hash"`
	Profile            string     `db:"profile"`
	InputHash          *string    `db:"input_hash"`
	InputPreview       *string    `db:"input_preview"`
	Status             RunStatus  `db:"status"`
	StateVersion       int64      `db:"state_version"`
	NextEventSeq       int64      `db:"next_event_seq"`
	CancelRequestedAt  *time.Time `db:"cancel_requested_at"`
	CancelReasonClass  *string    `db:"cancel_reason_class"`
	AttemptCount       int        `db:"attempt_count"`
	LeaseExpiresAt     *time.Time `db:"lease_expires_at"`
	ProviderStartedAt  *time.Time `db:"provider_started_at"`
	ResultClass        *string    `db:"result_class"`
	ErrorClass         *string    `db:"error_class"`
	CreatedAt          time.Time  `db:"created_at"`
	StartedAt          *time.Time `db:"started_at"`
	EndedAt            *time.Time `db:"ended_at"`
}

// ─── RunEvent ─────────────────────────────────────────────────────────────────

// SessionTurn records one durable multi-turn interaction within a session.
type SessionTurn struct {
	ID             string    `db:"id"`
	SessionID      string    `db:"session_id"`
	TurnSequence   int64     `db:"turn_sequence"`
	RunID          *string   `db:"run_id"`
	IdempotencyKey string    `db:"idempotency_key"`
	InputPreview   *string   `db:"input_preview"`
	Status         string    `db:"status"`
	CreatedAt      time.Time `db:"created_at"`
}

// RunEvent is a single durable, sequenced event in a run's event log.
type RunEvent struct {
	RunID         string     `db:"run_id"`
	Sequence      int64      `db:"sequence"`
	SchemaVersion int16      `db:"schema_version"`
	RootRunID     *string    `db:"root_run_id"`
	ParentRunID   *string    `db:"parent_run_id"`
	AgentID       *string    `db:"agent_id"`
	AgentType     *string    `db:"agent_type"`
	AgentDepth    *int       `db:"agent_depth"`
	Kind          string     `db:"kind"`
	Payload       *string    `db:"payload"`
	CreatedAt     time.Time  `db:"created_at"`
}
