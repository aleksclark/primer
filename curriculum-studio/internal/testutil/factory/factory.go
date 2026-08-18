// Package factory provides FactoryBot-style test data builders for Studio
// authorization tables. Each helper inserts through the repo layer.
package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

var seq atomic.Uint64

func n() uint64 { return seq.Add(1) }

// Tenant creates a tenant with unique slug.
func Tenant(t *testing.T, q repo.Querier, overrides ...func(*domain.Tenant)) *domain.Tenant {
	t.Helper()
	i := n()
	in := &domain.Tenant{
		Slug:   fmt.Sprintf("tenant-%d-%s", i, uuid.NewString()[:8]),
		Name:   fmt.Sprintf("Tenant %d", i),
		Status: domain.TenantStatusActive,
	}
	for _, o := range overrides {
		o(in)
	}
	out, err := repo.NewTenantRepo(q).Create(context.Background(), in)
	require.NoError(t, err)
	return out
}

// Workspace creates a workspace, creating a tenant unless TenantID is set.
func Workspace(t *testing.T, q repo.Querier, overrides ...func(*domain.Workspace)) *domain.Workspace {
	t.Helper()
	i := n()
	in := &domain.Workspace{
		Slug:   fmt.Sprintf("ws-%d-%s", i, uuid.NewString()[:8]),
		Name:   fmt.Sprintf("Workspace %d", i),
		Kind:   domain.WorkspaceKindFamily,
		Status: domain.WorkspaceStatusActive,
	}
	for _, o := range overrides {
		o(in)
	}
	if in.TenantID == uuid.Nil {
		in.TenantID = Tenant(t, q).ID
	}
	out, err := repo.NewWorkspaceRepo(q).Create(context.Background(), in)
	require.NoError(t, err)
	return out
}

// Membership creates a workspace membership. Creates a workspace unless set.
func Membership(t *testing.T, q repo.Querier, overrides ...func(*domain.WorkspaceMembership)) *domain.WorkspaceMembership {
	t.Helper()
	in := &domain.WorkspaceMembership{
		SubjectRef:  domain.HumanSubjectRef(uuid.New()),
		SubjectKind: domain.SubjectKindHuman,
		Role:        domain.MembershipRoleAuthor,
		Status:      domain.MembershipStatusActive,
		DisplayName: "Test Member",
	}
	for _, o := range overrides {
		o(in)
	}
	if in.WorkspaceID == uuid.Nil {
		in.WorkspaceID = Workspace(t, q).ID
	}
	out, err := repo.NewMembershipRepo(q).Create(context.Background(), in)
	require.NoError(t, err)
	return out
}

// IntegrationIdentity upserts an integration identity. Creates workspace unless set.
func IntegrationIdentity(t *testing.T, q repo.Querier, overrides ...func(*domain.IntegrationIdentity)) *domain.IntegrationIdentity {
	t.Helper()
	i := n()
	in := &domain.IntegrationIdentity{
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  fmt.Sprintf("learner_%d", i),
		DisplayLabel: fmt.Sprintf("Learner %d", i),
		Snapshot:     json.RawMessage(`{"grade":6}`),
	}
	for _, o := range overrides {
		o(in)
	}
	if in.WorkspaceID == uuid.Nil {
		in.WorkspaceID = Workspace(t, q).ID
	}
	out, err := repo.NewIntegrationIdentityRepo(q).Upsert(context.Background(), in)
	require.NoError(t, err)
	return out
}

// Framework creates a global standards framework unless WorkspaceID is set.
func Framework(t *testing.T, q repo.Querier, overrides ...func(*domain.StandardFramework)) *domain.StandardFramework {
	t.Helper()
	i := n()
	in := &domain.StandardFramework{
		Code:         fmt.Sprintf("FW-%d-%s", i, uuid.NewString()[:8]),
		Name:         fmt.Sprintf("Framework %d", i),
		Jurisdiction: "TN",
	}
	for _, o := range overrides {
		o(in)
	}
	out, err := repo.NewFrameworkRepo(q).Create(context.Background(), in)
	require.NoError(t, err)
	return out
}

// CatalogStandard creates a catalog standard. Creates a framework unless set.
func CatalogStandard(t *testing.T, q repo.Querier, overrides ...func(*domain.CatalogStandard)) *domain.CatalogStandard {
	t.Helper()
	i := n()
	in := &domain.CatalogStandard{
		Code:        fmt.Sprintf("STD.%d", i),
		Description: fmt.Sprintf("Standard %d", i),
	}
	for _, o := range overrides {
		o(in)
	}
	if in.FrameworkID == uuid.Nil {
		in.FrameworkID = Framework(t, q).ID
	}
	// Catalog repository operations require a requested workspace scope. Global
	// frameworks are visible to any workspace; custom frameworks use their
	// owning workspace.
	var frameworkWorkspace *uuid.UUID
	err := q.QueryRow(context.Background(), `
SELECT workspace_id FROM curriculum_studio.standard_frameworks WHERE id = $1`, in.FrameworkID).Scan(&frameworkWorkspace)
	require.NoError(t, err)
	scope := Workspace(t, q).ID
	if frameworkWorkspace != nil {
		scope = *frameworkWorkspace
	}
	out, err := repo.NewCatalogStandardRepo(q).Create(context.Background(), scope, in)
	require.NoError(t, err)
	return out
}

// Resource creates resource metadata. Creates a tenant unless TenantID is set.
func Resource(t *testing.T, q repo.Querier, overrides ...func(*domain.Resource)) *domain.Resource {
	t.Helper()
	i := n()
	in := &domain.Resource{
		Kind:  domain.ResourceKindBook,
		Title: fmt.Sprintf("Resource %d", i),
	}
	for _, o := range overrides {
		o(in)
	}
	if in.TenantID == uuid.Nil {
		in.TenantID = Tenant(t, q).ID
	}
	out, err := repo.NewResourceRepo(q).Create(context.Background(), in)
	require.NoError(t, err)
	return out
}
