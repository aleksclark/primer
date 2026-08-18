package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	ProfileKindLearner = "learner"
	ProfileKindClass   = "class"

	MaterializationStatusRequested = "requested"
	MaterializationStatusRunning   = "running"
	MaterializationStatusReady     = "ready"
	MaterializationStatusFailed    = "failed"
	MaterializationStatusCancelled = "cancelled"
)

// LearnerProfile is a workspace-scoped learner/class snapshot source.
type LearnerProfile struct {
	ID                    uuid.UUID       `json:"id" db:"id"`
	WorkspaceID           uuid.UUID       `json:"workspaceId" db:"workspace_id"`
	Kind                  string          `json:"kind" db:"kind"`
	Label                 string          `json:"label" db:"label"`
	GradeBand             string          `json:"gradeBand" db:"grade_band"`
	Profile               json.RawMessage `json:"profile" db:"profile"`
	IntegrationIdentityID *uuid.UUID      `json:"integrationIdentityId,omitempty" db:"integration_identity_id"`
	CreatedAt             time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time       `json:"updatedAt" db:"updated_at"`
}

// MaterializationRun freezes every input used to produce generated content.
type MaterializationRun struct {
	ID                    uuid.UUID       `json:"id" db:"id"`
	WorkspaceID           uuid.UUID       `json:"workspaceId" db:"workspace_id"`
	PlanRevisionID        uuid.UUID       `json:"planRevisionId" db:"plan_revision_id"`
	LearnerProfileID      *uuid.UUID      `json:"learnerProfileId,omitempty" db:"learner_profile_id"`
	WindowStart           *time.Time      `json:"windowStart,omitempty" db:"window_start"`
	WindowEnd             *time.Time      `json:"windowEnd,omitempty" db:"window_end"`
	Status                string          `json:"status" db:"status"`
	InputSnapshot         json.RawMessage `json:"inputSnapshot" db:"input_snapshot"`
	InputFingerprint      string          `json:"inputFingerprint" db:"input_fingerprint"`
	RequestedBySubjectRef string          `json:"requestedBySubjectRef" db:"requested_by_subject_ref"`
	StartedAt             *time.Time      `json:"startedAt,omitempty" db:"started_at"`
	CompletedAt           *time.Time      `json:"completedAt,omitempty" db:"completed_at"`
	CreatedAt             time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time       `json:"updatedAt" db:"updated_at"`
}
