package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	ExportFormatPDF      = "pdf"
	ExportFormatMarkdown = "markdown"
	ExportFormatDOCX     = "docx"
	ExportFormatCSV      = "csv"
	ExportFormatJSON     = "json"
	ExportFormatICal     = "ical"

	ExportStatusRequested = "requested"
	ExportStatusReady     = "ready"
	ExportStatusFailed    = "failed"
)

// Export records metadata for an object-store artifact; bytes never enter Studio DB.
type Export struct {
	ID                    uuid.UUID  `json:"id" db:"id"`
	WorkspaceID           uuid.UUID  `json:"workspaceId" db:"workspace_id"`
	RunID                 *uuid.UUID `json:"runId,omitempty" db:"run_id"`
	PlanRevisionID        *uuid.UUID `json:"planRevisionId,omitempty" db:"plan_revision_id"`
	Format                string     `json:"format" db:"format"`
	Status                string     `json:"status" db:"status"`
	ArtifactRef           string     `json:"artifactRef" db:"artifact_ref"`
	Checksum              string     `json:"checksum" db:"checksum"`
	RequestedBySubjectRef string     `json:"requestedBySubjectRef" db:"requested_by_subject_ref"`
	CreatedAt             time.Time  `json:"createdAt" db:"created_at"`
	CompletedAt           *time.Time `json:"completedAt,omitempty" db:"completed_at"`
}
