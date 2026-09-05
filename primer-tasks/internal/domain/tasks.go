package domain

import (
	"errors"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskDraft     TaskStatus = "draft"
	TaskPublished TaskStatus = "published"
	TaskRetired   TaskStatus = "retired"
)

type OccurrenceStatus string

const (
	OccurrencePending              OccurrenceStatus = "pending"
	OccurrenceInProgress           OccurrenceStatus = "in_progress"
	OccurrenceAwaitingVerification OccurrenceStatus = "awaiting_verification"
	OccurrenceCompleted            OccurrenceStatus = "completed"
	OccurrenceExcused              OccurrenceStatus = "excused"
	OccurrenceCanceled             OccurrenceStatus = "canceled"
)

var (
	ErrInvalidTask       = errors.New("invalid task")
	ErrInvalidTransition = errors.New("invalid occurrence transition")
)

type TaskRevision struct {
	ID           string                    `json:"id"`
	TemplateID   string                    `json:"templateId"`
	TenantID     string                    `json:"tenantId,omitempty"`
	Version      int                       `json:"version"`
	Title        string                    `json:"title"`
	Instructions string                    `json:"instructions"`
	Status       TaskStatus                `json:"status"`
	Requirements []VerificationRequirement `json:"requirements"`
	CreatedAt    time.Time                 `json:"createdAt"`
}
type VerificationRequirement struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	ConfigVersion int            `json:"configVersion"`
	Config        map[string]any `json:"config"`
	Interaction   string         `json:"interaction"`
	Executor      string         `json:"executor"`
}
type TaskTemplate struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenantId,omitempty"`
	Title           string     `json:"title"`
	Status          TaskStatus `json:"status"`
	CurrentRevision int        `json:"currentRevision"`
	CreatedAt       time.Time  `json:"createdAt"`
	RetiredAt       *time.Time `json:"retiredAt,omitempty"`
}

func ValidateRevision(title, instructions string, requirements []VerificationRequirement) error {
	if strings.TrimSpace(title) == "" || len(requirements) == 0 {
		return ErrInvalidTask
	}
	for _, r := range requirements {
		if r.Kind != "parent_approval" || r.ConfigVersion != 1 {
			return ErrInvalidTask
		}
	}
	return nil
}
func CanTransition(from, to OccurrenceStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case OccurrencePending:
		return to == OccurrenceInProgress || to == OccurrenceExcused || to == OccurrenceCanceled
	case OccurrenceInProgress:
		return to == OccurrenceAwaitingVerification || to == OccurrenceExcused || to == OccurrenceCanceled
	case OccurrenceAwaitingVerification:
		return to == OccurrenceCompleted || to == OccurrencePending || to == OccurrenceExcused || to == OccurrenceCanceled
	default:
		return false
	}
}

// DecisionExpectedStatus is the occurrence status a parent decision may consume.
func DecisionExpectedStatus() OccurrenceStatus { return OccurrenceAwaitingVerification }

func IsTerminal(status OccurrenceStatus) bool {
	return status == OccurrenceCompleted || status == OccurrenceExcused || status == OccurrenceCanceled
}
