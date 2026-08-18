package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Curriculum is a durable curriculum identity scoped to a Studio workspace.
type Curriculum struct {
	ID                     uuid.UUID       `json:"id" db:"id"`
	WorkspaceID            uuid.UUID       `json:"workspaceId" db:"workspace_id"`
	Slug                   string          `json:"slug" db:"slug"`
	Title                  string          `json:"title" db:"title"`
	Description            string          `json:"description" db:"description"`
	Approach               string          `json:"approach" db:"approach"`
	GradeBand              string          `json:"gradeBand" db:"grade_band"`
	Status                 string          `json:"status" db:"status"`
	CurrentDraftRevisionID *uuid.UUID      `json:"currentDraftRevisionId,omitempty" db:"current_draft_revision_id"`
	PublishedRevisionID    *uuid.UUID      `json:"publishedRevisionId,omitempty" db:"published_revision_id"`
	Metadata               json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt              time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt              time.Time       `json:"updatedAt" db:"updated_at"`
}

// PlanRevision is an immutable published version of a curriculum plan.
type PlanRevision struct {
	ID                    uuid.UUID       `json:"id" db:"id"`
	CurriculumID          uuid.UUID       `json:"curriculumId" db:"curriculum_id"`
	Revision              int             `json:"revision" db:"revision"`
	Title                 string          `json:"title" db:"title"`
	Brief                 json.RawMessage `json:"brief" db:"brief"`
	Status                string          `json:"status" db:"status"`
	PublishedAt           *time.Time      `json:"publishedAt,omitempty" db:"published_at"`
	PublishedBySubjectRef string          `json:"publishedBySubjectRef,omitempty" db:"published_by_subject_ref"`
	CreatedAt             time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time       `json:"updatedAt" db:"updated_at"`
}

// Objective is a high-level goal in a plan revision.
type Objective struct {
	ID             uuid.UUID `json:"id" db:"id"`
	PlanRevisionID uuid.UUID `json:"planRevisionId" db:"plan_revision_id"`
	Code           string    `json:"code" db:"code"`
	Title          string    `json:"title" db:"title"`
	Description    string    `json:"description" db:"description"`
	Position       int       `json:"position" db:"position"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" db:"updated_at"`
}

// Outcome is an observable outcome in a plan revision.
type Outcome struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	PlanRevisionID  uuid.UUID  `json:"planRevisionId" db:"plan_revision_id"`
	ObjectiveID     *uuid.UUID `json:"objectiveId,omitempty" db:"objective_id"`
	Code            string     `json:"code" db:"code"`
	Title           string     `json:"title" db:"title"`
	Description     string     `json:"description" db:"description"`
	MasteryCriteria string     `json:"masteryCriteria" db:"mastery_criteria"`
	Position        int        `json:"position" db:"position"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
}

// OutcomeStandardMapping maps an outcome to a catalog standard.
type OutcomeStandardMapping struct {
	ID         uuid.UUID
	OutcomeID  uuid.UUID
	StandardID uuid.UUID
	Alignment  string
	Notes      string
	CreatedAt  time.Time
}

// OutcomePrerequisite is a directed prerequisite edge among outcomes.
type OutcomePrerequisite struct {
	ID             uuid.UUID
	PlanRevisionID uuid.UUID
	OutcomeID      uuid.UUID
	PrerequisiteID uuid.UUID
	Requirement    string
}

// LearningArc groups units in a revision.
type LearningArc struct {
	ID             uuid.UUID
	PlanRevisionID uuid.UUID
	Code           string
	Title          string
	Description    string
	Position       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Unit is a unit blueprint.
type Unit struct {
	ID                 uuid.UUID
	PlanRevisionID     uuid.UUID
	LearningArcID      *uuid.UUID
	Code               string
	Title              string
	EssentialQuestions []string
	EstimatedMinutes   *int
	Position           int
	Blueprint          json.RawMessage
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Project is a project blueprint.
type Project struct {
	ID               uuid.UUID
	PlanRevisionID   uuid.UUID
	UnitID           *uuid.UUID
	Code             string
	Title            string
	Description      string
	Phases           json.RawMessage
	EstimatedMinutes *int
	Position         int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// UnitOutcome and ProjectOutcome attach outcomes to plan containers.
type UnitOutcome struct {
	UnitID    uuid.UUID
	OutcomeID uuid.UUID
	Role      string
}
type ProjectOutcome struct {
	ProjectID uuid.UUID
	OutcomeID uuid.UUID
	Role      string
}

// EvidenceRequirement describes required evidence for an outcome.
type EvidenceRequirement struct {
	ID             uuid.UUID
	PlanRevisionID uuid.UUID
	OutcomeID      uuid.UUID
	Kind           string
	Description    string
	Criteria       json.RawMessage
	CreatedAt      time.Time
}

// SchedulingConstraint stores one scheduling/policy constraint.
type SchedulingConstraint struct {
	ID             uuid.UUID
	PlanRevisionID uuid.UUID
	Kind           string
	Payload        json.RawMessage
	CreatedAt      time.Time
}

// PlanResource attaches resource metadata to a revision or its container.
type PlanResource struct {
	ID             uuid.UUID
	PlanRevisionID uuid.UUID
	ResourceID     uuid.UUID
	UnitID         *uuid.UUID
	ProjectID      *uuid.UUID
	Role           string
	Notes          string
}

// PlanGraph is a consistent, revision-scoped graph snapshot. Each collection is
// loaded with a revision/workspace predicate; no child is returned from another
// revision or workspace.
type PlanGraph struct {
	Revision                PlanRevision
	Objectives              []Objective
	Outcomes                []Outcome
	OutcomeStandardMappings []OutcomeStandardMapping
	OutcomePrerequisites    []OutcomePrerequisite
	LearningArcs            []LearningArc
	Units                   []Unit
	Projects                []Project
	UnitOutcomes            []UnitOutcome
	ProjectOutcomes         []ProjectOutcome
	EvidenceRequirements    []EvidenceRequirement
	SchedulingConstraints   []SchedulingConstraint
	PlanResources           []PlanResource
}
