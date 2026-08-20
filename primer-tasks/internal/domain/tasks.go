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
		if r.ConfigVersion != 1 {
			return ErrInvalidTask
		}
		switch r.Kind {
		case "parent_approval", "agent_dialogue":
			// Existing phase requirements retain their established boundary.
		case "agent_artifact_rubric":
			if r.Interaction != "artifact_upload" || r.Executor != "fantasy" || !validArtifactRubric(r.Config) {
				return ErrInvalidTask
			}
		default:
			return ErrInvalidTask
		}
	}
	return nil
}

func validArtifactRubric(config map[string]any) bool {
	accepted, ok := config["acceptedKinds"].([]any)
	if !ok || len(accepted) == 0 {
		return false
	}
	for _, value := range accepted {
		kind, ok := value.(string)
		if !ok || (kind != "image" && kind != "video" && kind != "audio") {
			return false
		}
	}
	criteria, ok := config["criteria"].([]any)
	if !ok || len(criteria) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range criteria {
		item, ok := value.(map[string]any)
		if !ok {
			return false
		}
		id, idOK := item["id"].(string)
		label, labelOK := item["label"].(string)
		description, descriptionOK := item["description"].(string)
		if !idOK || id == "" || seen[id] || !labelOK || strings.TrimSpace(label) == "" || !descriptionOK || strings.TrimSpace(description) == "" {
			return false
		}
		seen[id] = true
	}
	pass, passOK := config["passRule"].(string)
	policy, policyOK := config["reviewPolicy"].(string)
	return passOK && pass == "all_required" && policyOK && (policy == "reject" || policy == "parent_review")
}
func CanTransition(from, to OccurrenceStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case OccurrencePending:
		return to == OccurrenceInProgress || to == OccurrenceCanceled
	case OccurrenceInProgress:
		return to == OccurrenceAwaitingVerification || to == OccurrenceCanceled
	case OccurrenceAwaitingVerification:
		return to == OccurrenceCompleted || to == OccurrencePending || to == OccurrenceCanceled
	default:
		return false
	}
}
