// Package agent contains Primer's durable, provider-independent agent runtime.
//
// The package deliberately keeps persistence models separate from Fantasy's
// request/response types. In particular, provider metadata and prompts are not
// durable operational data.
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ConversationStatus string

const (
	ConversationActive   ConversationStatus = "active"
	ConversationArchived ConversationStatus = "archived"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

type RunStatus string

const (
	RunAwaitingConfirmation RunStatus = "awaiting_confirmation"
	RunQueued               RunStatus = "queued"
	RunRunning              RunStatus = "running"
	RunCancelRequested      RunStatus = "cancel_requested"
	RunSucceeded            RunStatus = "succeeded"
	RunFailed               RunStatus = "failed"
	RunCanceled             RunStatus = "canceled"
)

func (s RunStatus) Terminal() bool { return s == RunSucceeded || s == RunFailed || s == RunCanceled }

type Conversation struct {
	ID, TenantID, ActorID string
	Status                ConversationStatus
	PolicyVersion         string
	CreatedAt, UpdatedAt  time.Time
}

type Message struct {
	ID, TenantID, ConversationID, ClientMessageID string
	Role                                          MessageRole
	Content                                       string
	Sequence                                      int64
	CreatedAt                                     time.Time
}

type Usage struct {
	InputTokens, OutputTokens, TotalTokens, ReasoningTokens int64
}

type Provenance struct {
	Provider, Model, PolicyVersion string
	PromptDigest                   string // digest only; never the prompt
}

type Run struct {
	ID, TenantID, ConversationID, UserMessageID string
	Status                                      RunStatus
	DurableStep                                 int
	Attempt                                     int
	MaxSteps, MaxTokens                         int
	Deadline                                    time.Time
	LeaseOwner                                  string
	LeaseUntil                                  *time.Time
	CancelRequested                             bool
	Usage                                       Usage
	Provenance                                  Provenance
	CreatedAt, UpdatedAt                        time.Time
}

type ConfirmationPreview struct {
	ID, TenantID, ActorID, Action, ActionDigest string
	Payload                                     []byte
	ExpiresAt                                   time.Time
	UsedAt                                      *time.Time
}

var ErrInvalidTransition = errors.New("invalid agent run state transition")
var ErrAlreadyTerminal = errors.New("agent run is already terminal")
var ErrNotFound = errors.New("agent record not found")
var ErrPreviewExpired = errors.New("confirmation preview expired or already used")

func ValidateRunLimits(r Run) error {
	if r.MaxSteps < 1 || r.MaxSteps > 100 {
		return fmt.Errorf("max steps must be between 1 and 100")
	}
	if r.MaxTokens < 1 || r.MaxTokens > 200000 {
		return fmt.Errorf("max tokens must be between 1 and 200000")
	}
	if r.Deadline.IsZero() {
		return errors.New("run deadline is required")
	}
	return nil
}

func CanTransition(from, to RunStatus) bool {
	switch from {
	case RunQueued:
		return to == RunRunning || to == RunCancelRequested || to == RunCanceled || to == RunFailed
	case RunRunning:
		return to == RunAwaitingConfirmation || to == RunCancelRequested || to == RunSucceeded || to == RunFailed || to == RunCanceled
	case RunAwaitingConfirmation:
		return to == RunSucceeded || to == RunCanceled || to == RunCancelRequested || to == RunFailed
	case RunCancelRequested:
		return to == RunCanceled || to == RunFailed
	default:
		return false
	}
}

func ActionDigest(action string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(action)))
	h.Write([]byte{0})
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func ValidatePreview(p ConfirmationPreview, tenant, actor, action string, payload []byte, now time.Time) error {
	if p.TenantID != tenant || p.ActorID != actor || p.Action != action || p.ActionDigest != ActionDigest(action, payload) {
		return ErrPreviewExpired
	}
	if p.UsedAt != nil || !now.Before(p.ExpiresAt) {
		return ErrPreviewExpired
	}
	return nil
}
