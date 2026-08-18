package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ValidationReport is a persisted validation run for a plan revision.
type ValidationReport struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	PlanRevisionID uuid.UUID       `json:"planRevisionId" db:"plan_revision_id"`
	Status         string          `json:"status" db:"status"`
	Summary        json.RawMessage `json:"summary" db:"summary"`
	GeneratedAt    time.Time       `json:"generatedAt" db:"generated_at"`
	CreatedAt      time.Time       `json:"createdAt" db:"created_at"`
}

// ValidationFinding is one deterministic finding in a report.
type ValidationFinding struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	ReportID  uuid.UUID       `json:"reportId" db:"report_id"`
	Severity  string          `json:"severity" db:"severity"`
	Code      string          `json:"code" db:"code"`
	Message   string          `json:"message" db:"message"`
	NodeKind  string          `json:"nodeKind" db:"node_kind"`
	NodeID    *uuid.UUID      `json:"nodeId,omitempty" db:"node_id"`
	Details   json.RawMessage `json:"details" db:"details"`
	CreatedAt time.Time       `json:"createdAt" db:"created_at"`
}
