package domain

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	SharePermissionRead = "read"
	ApprovalApproved    = "approved"
	ApprovalRejected    = "rejected"
)

// PlanComment is a collaborative note on a plan node (outcome, unit, …).
type PlanComment struct {
	ID                uuid.UUID
	WorkspaceID       uuid.UUID
	PlanRevisionID    uuid.UUID
	NodeID            string
	AuthorSubjectRef  string
	AuthorDisplayName string
	Body              string
	CreatedAt         time.Time
}

// PlanApproval records a reviewer decision on a draft revision.
type PlanApproval struct {
	ID                  uuid.UUID
	WorkspaceID         uuid.UUID
	PlanRevisionID      uuid.UUID
	ReviewerSubjectRef  string
	ReviewerDisplayName string
	Status              string
	ContentFingerprint  string
	CreatedAt           time.Time
}

// CurriculumShare is a workspace-to-workspace read-only grant.
type CurriculumShare struct {
	ID                  uuid.UUID
	CurriculumID        uuid.UUID
	SourceWorkspaceID   uuid.UUID
	TargetWorkspaceID   uuid.UUID
	Permission          string
	CreatedBySubjectRef string
	CreatedAt           time.Time
}

// UnitLibraryEntry is a reusable unit blueprint owned by a workspace.
type UnitLibraryEntry struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Name        string
	Blueprint   json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PlanTemplate seeds a new curriculum draft from a brief type.
type PlanTemplate struct {
	ID          uuid.UUID
	WorkspaceID *uuid.UUID
	Code        string
	Name        string
	BriefType   string
	Seed        json.RawMessage
	CreatedAt   time.Time
}

// TemplateSeed is the JSON body stored on plan_templates.seed.
type TemplateSeed struct {
	Outcomes []NamedSeed `json:"outcomes"`
	Units    []NamedSeed `json:"units"`
}

// NamedSeed is a title-preserving template or library fragment.
type NamedSeed struct {
	Code         string   `json:"code,omitempty"`
	Title        string   `json:"title"`
	Name         string   `json:"name,omitempty"`
	OutcomeCodes []string `json:"outcomeCodes,omitempty"`
}

func (s NamedSeed) DisplayName() string {
	if n := strings.TrimSpace(s.Title); n != "" {
		return n
	}
	return strings.TrimSpace(s.Name)
}

// RevisionDiff compares stable outcome codes, not per-revision UUIDs. Renames
// appear as a removed old name and an added new name.
type RevisionDiff struct {
	AddedOutcomes   []OutcomeChange `json:"addedOutcomes"`
	RemovedOutcomes []OutcomeChange `json:"removedOutcomes"`
}
type OutcomeChange struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// UnitLibrarySnapshot is a durable, self-contained copy of a unit's educational
// content. References are remapped on import; no source plan IDs are reused.
type UnitLibrarySnapshot struct {
	Unit            Unit                  `json:"unit"`
	Arc             *LearningArc          `json:"arc,omitempty"`
	Objectives      []Objective           `json:"objectives"`
	Outcomes        []LibraryOutcome      `json:"outcomes"`
	Prerequisites   []OutcomePrerequisite `json:"prerequisites"`
	Projects        []Project             `json:"projects"`
	ProjectOutcomes []ProjectOutcome      `json:"projectOutcomes"`
	Resources       []PlanResource        `json:"resources"`
}
type LibraryOutcome struct {
	Outcome   Outcome                  `json:"outcome"`
	Role      string                   `json:"role"`
	Standards []OutcomeStandardMapping `json:"standards"`
	Evidence  []EvidenceRequirement    `json:"evidence"`
}
