package repo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

	// The database rejects cross-workspace endpoint pairs even when callers
	// bypass the repository's visibility predicates.
	sp := testutil.NewSavepointQuerier(tx)
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.standard_crosswalks
    (from_standard_id, to_standard_id)
VALUES ($1, $2)`, a.ID, parentB.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.catalog_standard_prerequisites
    (standard_id, prerequisite_id)
VALUES ($1, $2)`, a.ID, parentB.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)

	// The parent framework check is database-owned and catches direct SQL too.
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.catalog_standards
    (framework_id, parent_id, code, description)
VALUES ($1, $2, $3, $4)`, fwA.ID, parentB.ID, "BAD", "bad")
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)

	// Ownership reassignment is rejected even through raw SQL. No-op updates
	// remain valid and demonstrate that the triggers test changed values.
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.standard_frameworks
SET workspace_id = workspace_id
WHERE id = $1`, fwA.ID)
	require.NoError(t, err)
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.standard_frameworks
SET workspace_id = $1
WHERE id = $2`, wsB.ID, fwA.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.catalog_standards
SET framework_id = framework_id
WHERE id = $1`, a.ID)
	require.NoError(t, err)
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.catalog_standards
SET framework_id = $1
WHERE id = $2`, fwB.ID, a.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)

	resource, err := f.Resources().Create(ctx, &domain.Resource{
		TenantID: tenA.ID, WorkspaceID: &wsA.ID, Kind: domain.ResourceKindBook,
		Title: "A", Metadata: json.RawMessage(`{"tags":["math"]}`),
	})
	require.NoError(t, err)
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.workspaces
SET tenant_id = tenant_id
WHERE id = $1`, wsA.ID)
	require.NoError(t, err)
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.workspaces
SET tenant_id = $1
WHERE id = $2`, tenB.ID, wsA.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
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

	globalResource, err := f.Resources().Create(ctx, &domain.Resource{
		TenantID: tenA.ID, Kind: domain.ResourceKindBook, Title: "tenant global",
	})
	require.NoError(t, err)
	_, err = f.Resources().Get(ctx, tenA.ID, wsA.ID, globalResource.ID)
	require.NoError(t, err)
	_, err = f.Resources().Get(ctx, tenA.ID, wsB.ID, globalResource.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = f.Resources().Get(ctx, tenA.ID, uuid.New(), globalResource.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
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

func TestResourcePolicyDirectSQLUpdatesAndOctetLimit(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ten := factory.Tenant(t, tx)
	r := repo.NewResourceRepo(tx)
	resource, err := r.Create(ctx, &domain.Resource{
		TenantID: ten.ID, Kind: domain.ResourceKindDocument, Title: "mutable",
		URL: "https://example.test/document",
	})
	require.NoError(t, err)
	sp := testutil.NewSavepointQuerier(tx)

	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.resources
SET artifact_ref = 'data:application/octet-stream;base64,SGVsbG8='
WHERE id = $1`, resource.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
	_, err = sp.Exec(ctx, `
UPDATE curriculum_studio.resources
SET url = 'data:text/plain;base64,SGVsbG8='
WHERE id = $1`, resource.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)

	// Every individual field is valid, but the canonical JSONB representation
	// exceeds the 16KiB stored-octet invariant.
	large := fmt.Sprintf(`{"description":%q,"publisher":%q,"language":%q,"license":%q,"note":%q}`,
		strings.Repeat("x", 3500), strings.Repeat("x", 3500), strings.Repeat("x", 3500),
		strings.Repeat("x", 3500), strings.Repeat("x", 3500))
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.resources (tenant_id, kind, title, metadata)
VALUES ($1, 'document', 'too much metadata', $2::jsonb)`, ten.ID, large)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
}
