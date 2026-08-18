package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	ItemKindLesson       = "lesson"
	ItemKindAssessment   = "assessment"
	ItemKindRubric       = "rubric"
	ItemKindAnswerKey    = "answer_key"
	ItemStatusDraft      = "draft"
	ItemStatusReady      = "ready"
	ItemStatusPublished  = "published"
	ItemStatusSuperseded = "superseded"
)

// MaterializedItem is generated content with durable plan/run provenance.
type MaterializedItem struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	RunID              uuid.UUID       `json:"runId" db:"run_id"`
	PlanRevisionID     uuid.UUID       `json:"planRevisionId" db:"plan_revision_id"`
	UnitID             *uuid.UUID      `json:"unitId,omitempty" db:"unit_id"`
	ProjectID          *uuid.UUID      `json:"projectId,omitempty" db:"project_id"`
	OutcomeID          *uuid.UUID      `json:"outcomeId,omitempty" db:"outcome_id"`
	Kind               string          `json:"kind" db:"kind"`
	Title              string          `json:"title" db:"title"`
	Body               json.RawMessage `json:"body" db:"body"`
	Status             string          `json:"status" db:"status"`
	Locked             bool            `json:"locked" db:"locked"`
	LockedAt           *time.Time      `json:"lockedAt,omitempty" db:"locked_at"`
	LockedBySubjectRef string          `json:"lockedBySubjectRef,omitempty" db:"locked_by_subject_ref"`
	SupersedesItemID   *uuid.UUID      `json:"supersedesItemId,omitempty" db:"supersedes_item_id"`
	Provenance         json.RawMessage `json:"provenance" db:"provenance"`
	CreatedAt          time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time       `json:"updatedAt" db:"updated_at"`
}

// MaterializedItemEdit records an author patch without replacing edit history.
type MaterializedItemEdit struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	ItemID           uuid.UUID       `json:"itemId" db:"item_id"`
	EditorSubjectRef string          `json:"editorSubjectRef" db:"editor_subject_ref"`
	Patch            json.RawMessage `json:"patch" db:"patch"`
	CreatedAt        time.Time       `json:"createdAt" db:"created_at"`
}

// AssessmentSupport links an assessment to a rubric or answer key.
type AssessmentSupport struct {
	AssessmentItemID uuid.UUID `json:"assessmentItemId" db:"assessment_item_id"`
	SupportItemID    uuid.UUID `json:"supportItemId" db:"support_item_id"`
}
