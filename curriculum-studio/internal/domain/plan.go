package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	ID           uuid.UUID
	OutcomeID    uuid.UUID
	StandardID   uuid.UUID
	Alignment    string
	Notes        string
	CreatedAt    time.Time
	StandardCode string
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

// ProjectPhase is one ordered stage of a multi-subject project blueprint.
type ProjectPhase struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Position   int               `json:"position"`
	OffScreen  bool              `json:"offScreen,omitempty"`
	Activities []ProjectActivity `json:"activities,omitempty"`
}

// ProjectActivity is a phase-scoped task. Kind off_screen becomes a project_task.
type ProjectActivity struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

const (
	ProjectOutcomeRoleTarget  = "target"
	ProjectOutcomeRolePrior   = "prior"
	ProjectOutcomeRoleStretch = "stretch"
	ActivityKindOffScreen     = "off_screen"
)

// ParseProjectPhases decodes and orders a project's phases JSON. Empty or nil
// payloads are a valid empty list. Duplicate ids and missing names fail closed.
func ParseProjectPhases(raw json.RawMessage) ([]ProjectPhase, error) {
	if len(raw) == 0 {
		return []ProjectPhase{}, nil
	}
	var phases []ProjectPhase
	if err := json.Unmarshal(raw, &phases); err != nil {
		return nil, fmt.Errorf("phases must be a JSON array: %w", err)
	}
	seen := map[string]struct{}{}
	for i := range phases {
		p := &phases[i]
		p.ID = strings.TrimSpace(p.ID)
		p.Name = strings.TrimSpace(p.Name)
		if p.ID == "" {
			return nil, fmt.Errorf("phase %d is missing id", i)
		}
		if p.Name == "" {
			return nil, fmt.Errorf("phase %q is missing name", p.ID)
		}
		if _, ok := seen[p.ID]; ok {
			return nil, fmt.Errorf("duplicate phase id %q", p.ID)
		}
		seen[p.ID] = struct{}{}
		if p.Position == 0 {
			p.Position = i + 1
		}
	}
	sort.SliceStable(phases, func(i, j int) bool {
		if phases[i].Position != phases[j].Position {
			return phases[i].Position < phases[j].Position
		}
		return phases[i].ID < phases[j].ID
	})
	return phases, nil
}

// EncodeProjectPhases stores an ordered phase list as JSONB.
func EncodeProjectPhases(phases []ProjectPhase) (json.RawMessage, error) {
	if phases == nil {
		phases = []ProjectPhase{}
	}
	ordered, err := ParseProjectPhases(mustJSON(phases))
	if err != nil {
		return nil, err
	}
	return json.Marshal(ordered)
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return raw
}

// PhaseByID returns the named phase or false when the blueprint has no match.
func PhaseByID(phases []ProjectPhase, id string) (ProjectPhase, bool) {
	id = strings.TrimSpace(id)
	for _, p := range phases {
		if p.ID == id {
			return p, true
		}
	}
	return ProjectPhase{}, false
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
	ResourceKind   string
	ResourceTitle  string
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
