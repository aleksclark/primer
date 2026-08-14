// Package domain holds Curriculum Studio persistence entity types.
//
// JSON tags are camelCase for future API surfaces. db tags map columns for
// pgx struct scanning. Authorization tables store role projections only —
// never credentials or secrets.
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Tenant statuses (DB CHECK).
const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusRetired   = "retired"
)

// Workspace kinds (DB CHECK).
const (
	WorkspaceKindSchool       = "school"
	WorkspaceKindFamily       = "family"
	WorkspaceKindCoop         = "coop"
	WorkspaceKindTeacher      = "teacher"
	WorkspaceKindOrganization = "organization"
)

// Workspace statuses (DB CHECK).
const (
	WorkspaceStatusActive   = "active"
	WorkspaceStatusArchived = "archived"
)

// Membership subject kinds (DB CHECK).
const (
	SubjectKindHuman   = "human"
	SubjectKindService = "service"
)

// Membership roles (DB CHECK).
const (
	MembershipRoleOwner    = "owner"
	MembershipRoleAdmin    = "admin"
	MembershipRoleAuthor   = "author"
	MembershipRoleReviewer = "reviewer"
	MembershipRoleViewer   = "viewer"
)

// Membership statuses (DB CHECK).
const (
	MembershipStatusActive  = "active"
	MembershipStatusInvited = "invited"
	MembershipStatusRevoked = "revoked"
)

// Integration identity systems (DB CHECK).
const (
	IntegrationSystemPrimerLMS      = "primer_lms"
	IntegrationSystemPrimerIdentity = "primer_identity"
	IntegrationSystemOIDC           = "oidc"
	IntegrationSystemOther          = "other"
)

// Integration identity external kinds (DB CHECK).
const (
	ExternalKindLearner     = "learner"
	ExternalKindEducator    = "educator"
	ExternalKindClass       = "class"
	ExternalKindAuthSubject = "auth_subject"
	ExternalKindService     = "service"
)

// Tenant is a Studio billing / org root. Independent of Identity tenants and LMS families.
type Tenant struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Slug      string    `json:"slug" db:"slug"`
	Name      string    `json:"name" db:"name"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// Workspace is a Studio authoring boundary owned by a tenant.
type Workspace struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenantId" db:"tenant_id"`
	Slug      string    `json:"slug" db:"slug"`
	Name      string    `json:"name" db:"name"`
	Kind      string    `json:"kind" db:"kind"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// WorkspaceMembership is an authorization projection keyed by opaque subject_ref.
// No credential material is persisted on this type.
type WorkspaceMembership struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId" db:"workspace_id"`
	SubjectRef  string    `json:"subjectRef" db:"subject_ref"`
	SubjectKind string    `json:"subjectKind" db:"subject_kind"`
	DisplayName string    `json:"displayName" db:"display_name"`
	Role        string    `json:"role" db:"role"`
	Status      string    `json:"status" db:"status"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

// IntegrationIdentity is a bounded snapshot of an external Primer/IdP identifier.
// external_ref is opaque text; snapshot is private JSON (never logged at info).
type IntegrationIdentity struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	WorkspaceID  uuid.UUID       `json:"workspaceId" db:"workspace_id"`
	System       string          `json:"system" db:"system"`
	ExternalKind string          `json:"externalKind" db:"external_kind"`
	ExternalRef  string          `json:"externalRef" db:"external_ref"`
	DisplayLabel string          `json:"displayLabel" db:"display_label"`
	Snapshot     json.RawMessage `json:"snapshot,omitempty" db:"snapshot"`
	LastSeenAt   time.Time       `json:"lastSeenAt" db:"last_seen_at"`
	CreatedAt    time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time       `json:"updatedAt" db:"updated_at"`
}
