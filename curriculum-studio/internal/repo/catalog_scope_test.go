package repo_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestCatalogWorkspaceScopeAndDatabasePolicies(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	tenA := factory.Tenant(t, tx)
	tenB := factory.Tenant(t, tx)
	wsA := factory.Workspace(t, tx, func(w *domain.Workspace) { w.TenantID = tenA.ID })
	wsB := factory.Workspace(t, tx, func(w *domain.Workspace) { w.TenantID = tenB.ID })

	fwA, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		WorkspaceID: &wsA.ID, Code: "CUSTOM-A", Name: "A",
	})
	require.NoError(t, err)
	global, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		Code: "GLOBAL", Name: "Global",
	})
	require.NoError(t, err)
	fwB, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		WorkspaceID: &wsB.ID, Code: "CUSTOM-B", Name: "B",
	})
	require.NoError(t, err)

	a, err := f.CatalogStandards().Create(ctx, wsA.ID, &domain.CatalogStandard{
		FrameworkID: fwA.ID, Code: "A", Description: "A",
	})
	require.NoError(t, err)
	g, err := f.CatalogStandards().Create(ctx, wsA.ID, &domain.CatalogStandard{
		FrameworkID: global.ID, Code: "G", Description: "G",
	})
	require.NoError(t, err)
	parentB, err := f.CatalogStandards().Create(ctx, wsB.ID, &domain.CatalogStandard{
		FrameworkID: fwB.ID, Code: "B", Description: "B",
	})
	require.NoError(t, err)

	_, err = f.Frameworks().Get(ctx, wsB.ID, fwA.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = f.CatalogStandards().Get(ctx, wsB.ID, a.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	visibleGlobal, err := f.CatalogStandards().Get(ctx, wsB.ID, g.ID)
	require.NoError(t, err)
	require.Equal(t, g.ID, visibleGlobal.ID)
	foreignTree, err := f.CatalogStandards().ListByFramework(ctx, wsB.ID, fwA.ID)
	require.NoError(t, err)
	require.Empty(t, foreignTree)

	cw, err := f.Crosswalks().Create(ctx, wsA.ID, &domain.StandardCrosswalk{
		FromStandardID: a.ID, ToStandardID: g.ID,
	})
	require.NoError(t, err)
	_, err = f.Crosswalks().Get(ctx, wsB.ID, cw.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	foreignCrosswalks, err := f.Crosswalks().ListFrom(ctx, wsB.ID, a.ID)
	require.NoError(t, err)
	require.Empty(t, foreignCrosswalks)

	_, err = f.Crosswalks().Create(ctx, wsB.ID, &domain.StandardCrosswalk{
		FromStandardID: a.ID, ToStandardID: g.ID,
	})
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = f.CatalogPrereqs().Create(ctx, wsB.ID, domain.CatalogPrerequisite{
		StandardID: a.ID, PrerequisiteID: g.ID,
	})
	require.ErrorIs(t, err, repo.ErrNotFound)
	foreignPrereqs, err := f.CatalogPrereqs().ListForStandard(ctx, wsB.ID, a.ID)
	require.NoError(t, err)
	require.Empty(t, foreignPrereqs)

	// The parent framework check is database-owned and catches direct SQL too.
	sp := testutil.NewSavepointQuerier(tx)
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.catalog_standards
    (framework_id, parent_id, code, description)
VALUES ($1, $2, $3, $4)`, fwA.ID, parentB.ID, "BAD", "bad")
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)

	resource, err := f.Resources().Create(ctx, &domain.Resource{
		TenantID: tenA.ID, WorkspaceID: &wsA.ID, Kind: domain.ResourceKindBook,
		Title: "A", Metadata: json.RawMessage(`{"tags":["math"]}`),
	})
	require.NoError(t, err)
	_, err = f.Resources().Get(ctx, tenA.ID, wsB.ID, resource.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = f.Resources().Get(ctx, tenB.ID, wsB.ID, resource.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// Workspace/tenant ownership and the explicit metadata shape are enforced
	// for raw SQL callers, not merely by the repository validator.
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.resources
    (tenant_id, workspace_id, kind, title)
VALUES ($1, $2, 'book', 'wrong tenant')`, tenA.ID, wsB.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.resources
    (tenant_id, kind, title, metadata)
VALUES ($1, 'document', 'nested bytes', $2::jsonb)`, tenA.ID,
		`{"nested":{"contentBase64":"SGVsbG8="}}`)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.resources
    (tenant_id, kind, title, artifact_ref)
VALUES ($1, 'document', 'data URL', 'data:application/octet-stream;base64,SGVsbG8=')`, tenA.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
}

func TestResourcePolicyAcceptsOnlyReferencesAndMetadata(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ten := factory.Tenant(t, tx)
	r := repo.NewResourceRepo(tx)

	_, err := r.Create(ctx, &domain.Resource{
		TenantID: ten.ID, Kind: domain.ResourceKindDocument, Title: "safe",
		ArtifactRef: "obj:document-123",
		Metadata:    json.RawMessage(`{"description":"A short note","published_year":2025}`),
	})
	require.NoError(t, err)
	_, err = r.Create(ctx, &domain.Resource{
		TenantID: ten.ID, Kind: domain.ResourceKindDocument, Title: "encoded",
		ArtifactRef: "data:application/octet-stream;base64,SGVsbG8=",
	})
	require.ErrorIs(t, err, repo.ErrCheckViolation)
	_, err = r.Create(ctx, &domain.Resource{
		TenantID: ten.ID, Kind: domain.ResourceKindDocument, Title: "unknown metadata",
		Metadata: json.RawMessage(`{"contentBase64":"SGVsbG8="}`),
	})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	// Keep uuid imported in this test's policy-focused case while asserting the
	// scope-bearing getter cannot be called with an empty requested workspace.
	_, err = r.Get(ctx, ten.ID, uuid.Nil, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
}
