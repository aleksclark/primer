package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Resource kinds (DB CHECK on curriculum_studio.resources.kind).
const (
	ResourceKindBook          = "book"
	ResourceKindDocument      = "document"
	ResourceKindVideo         = "video"
	ResourceKindTool          = "tool"
	ResourceKindProjectSupply = "project_supply"
	ResourceKindURL           = "url"
	ResourceKindOther         = "other"
)

// Crosswalk relationships (DB CHECK).
const (
	CrosswalkEquivalent = "equivalent"
	CrosswalkBroader    = "broader"
	CrosswalkNarrower   = "narrower"
	CrosswalkRelated    = "related"
)

// StandardFramework is a global or workspace-owned standards catalog.
// WorkspaceID nil means a global (shared) framework.
type StandardFramework struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	WorkspaceID  *uuid.UUID `json:"workspaceId,omitempty" db:"workspace_id"`
	Code         string     `json:"code" db:"code"`
	Name         string     `json:"name" db:"name"`
	Jurisdiction string     `json:"jurisdiction" db:"jurisdiction"`
	Version      string     `json:"version" db:"version"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// CatalogStandard is a hierarchical standard inside a framework.
type CatalogStandard struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	FrameworkID uuid.UUID       `json:"frameworkId" db:"framework_id"`
	ParentID    *uuid.UUID      `json:"parentId,omitempty" db:"parent_id"`
	Code        string          `json:"code" db:"code"`
	SubjectCode string          `json:"subjectCode" db:"subject_code"`
	GradeBand   string          `json:"gradeBand" db:"grade_band"`
	Domain      string          `json:"domain" db:"domain"`
	Cluster     string          `json:"cluster" db:"cluster"`
	Description string          `json:"description" db:"description"`
	Metadata    json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time       `json:"updatedAt" db:"updated_at"`
}

// StandardCrosswalk maps one catalog standard onto another.
type StandardCrosswalk struct {
	ID             uuid.UUID `json:"id" db:"id"`
	FromStandardID uuid.UUID `json:"fromStandardId" db:"from_standard_id"`
	ToStandardID   uuid.UUID `json:"toStandardId" db:"to_standard_id"`
	Relationship   string    `json:"relationship" db:"relationship"`
	Notes          string    `json:"notes" db:"notes"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
}

// CatalogPrerequisite is a directed edge in the catalog DAG.
type CatalogPrerequisite struct {
	StandardID     uuid.UUID `json:"standardId" db:"standard_id"`
	PrerequisiteID uuid.UUID `json:"prerequisiteId" db:"prerequisite_id"`
}

// Resource is catalog metadata only — no file bytes live in Postgres.
type Resource struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	TenantID    uuid.UUID       `json:"tenantId" db:"tenant_id"`
	WorkspaceID *uuid.UUID      `json:"workspaceId,omitempty" db:"workspace_id"`
	Kind        string          `json:"kind" db:"kind"`
	Title       string          `json:"title" db:"title"`
	Authors     string          `json:"authors" db:"authors"`
	ISBN        string          `json:"isbn" db:"isbn"`
	URL         string          `json:"url" db:"url"`
	ArtifactRef string          `json:"artifactRef" db:"artifact_ref"`
	Metadata    json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time       `json:"updatedAt" db:"updated_at"`
}
