package repo_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

// P4-E1 / P4-S1: global + workspace frameworks; unique indexes.
func TestP4E1_GlobalAndWorkspaceFrameworks(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ws := factory.Workspace(t, tx)

	global, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		Code:         "TN-MATH",
		Name:         "Tennessee Math",
		Jurisdiction: "TN",
		Version:      "2016",
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, global.ID)
	require.Nil(t, global.WorkspaceID)
	require.Equal(t, "TN-MATH", global.Code)
	require.False(t, global.CreatedAt.IsZero())

	got, err := f.Frameworks().Get(ctx, global.ID)
	require.NoError(t, err)
	require.Equal(t, global.ID, got.ID)
	require.Equal(t, "Tennessee Math", got.Name)

	custom, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		WorkspaceID:  &ws.ID,
		Code:         "TN-MATH",
		Name:         "Homeschool TN Math overlay",
		Jurisdiction: "TN",
	})
	require.NoError(t, err)
	require.NotNil(t, custom.WorkspaceID)
	require.Equal(t, ws.ID, *custom.WorkspaceID)

	// Global code uniqueness (workspace_id IS NULL).
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewFrameworkRepo(sp).Create(ctx, &domain.StandardFramework{
		Code: "TN-MATH",
		Name: "Dup global",
	})
	require.ErrorIs(t, err, repo.ErrConflict)

	// Per-workspace code uniqueness.
	_, err = repo.NewFrameworkRepo(sp).Create(ctx, &domain.StandardFramework{
		WorkspaceID: &ws.ID,
		Code:        "TN-MATH",
		Name:        "Dup workspace",
	})
	require.ErrorIs(t, err, repo.ErrConflict)

	// Same code is allowed in a different workspace.
	wsB := factory.Workspace(t, tx)
	other, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		WorkspaceID: &wsB.ID,
		Code:        "TN-MATH",
		Name:        "Other overlay",
	})
	require.NoError(t, err)
	require.Equal(t, wsB.ID, *other.WorkspaceID)

	// Workspace isolation: custom frameworks of B are not listed for A.
	listA, err := f.Frameworks().ListWorkspace(ctx, ws.ID)
	require.NoError(t, err)
	ids := map[uuid.UUID]bool{}
	for _, fw := range listA {
		ids[fw.ID] = true
		if fw.WorkspaceID != nil {
			require.Equal(t, ws.ID, *fw.WorkspaceID)
		}
	}
	require.True(t, ids[custom.ID])
	require.False(t, ids[other.ID])

	// Globals are readable by all workspaces.
	vis, err := f.Frameworks().ListVisible(ctx, ws.ID)
	require.NoError(t, err)
	visIDs := map[uuid.UUID]bool{}
	for _, fw := range vis {
		visIDs[fw.ID] = true
	}
	require.True(t, visIDs[global.ID], "global frameworks must be visible to every workspace")
	require.True(t, visIDs[custom.ID])
	require.False(t, visIDs[other.ID], "foreign workspace frameworks must stay hidden")
}

// P4-E2 / P4-S2: hierarchical catalog standards; unique (framework_id, code).
func TestP4E2_HierarchicalCatalogStandards(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	fw, err := f.Frameworks().Create(ctx, &domain.StandardFramework{
		Code: "TN-MATH",
		Name: "Tennessee Math",
	})
	require.NoError(t, err)

	parent, err := f.CatalogStandards().Create(ctx, &domain.CatalogStandard{
		FrameworkID: fw.ID,
		Code:        "6.NS",
		SubjectCode: "MATH",
		GradeBand:   "6",
		Domain:      "The Number System",
		Description: "Apply and extend previous understandings of numbers.",
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, parent.ID)
	require.Nil(t, parent.ParentID)
	require.Equal(t, "6.NS", parent.Code)

	child, err := f.CatalogStandards().Create(ctx, &domain.CatalogStandard{
		FrameworkID: fw.ID,
		ParentID:    &parent.ID,
		Code:        "6.NS.A.1",
		SubjectCode: "MATH",
		GradeBand:   "6",
		Domain:      "The Number System",
		Cluster:     "A",
		Description: "Interpret and compute quotients of fractions.",
	})
	require.NoError(t, err)
	require.NotNil(t, child.ParentID)
	require.Equal(t, parent.ID, *child.ParentID)

	got, err := f.CatalogStandards().Get(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, child.ID, got.ID)
	require.Equal(t, parent.ID, *got.ParentID)

	tree, err := f.CatalogStandards().ListByFramework(ctx, fw.ID)
	require.NoError(t, err)
	require.Len(t, tree, 2)

	children, err := f.CatalogStandards().ListChildren(ctx, parent.ID)
	require.NoError(t, err)
	require.Len(t, children, 1)
	require.Equal(t, child.ID, children[0].ID)

	// unique (framework_id, code)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewCatalogStandardRepo(sp).Create(ctx, &domain.CatalogStandard{
		FrameworkID: fw.ID,
		Code:        "6.NS",
		Description: "dup",
	})
	require.ErrorIs(t, err, repo.ErrConflict)

	// same code in a different framework is allowed
	fwB, err := f.Frameworks().Create(ctx, &domain.StandardFramework{Code: "CCSS-MATH", Name: "CCSS Math"})
	require.NoError(t, err)
	other, err := f.CatalogStandards().Create(ctx, &domain.CatalogStandard{
		FrameworkID: fwB.ID,
		Code:        "6.NS",
		Description: "CCSS counterpart",
	})
	require.NoError(t, err)
	require.Equal(t, fwB.ID, other.FrameworkID)

	// Import a small set inside one WithTx.
	var imported []domain.CatalogStandard
	err = repo.WithTx(ctx, tx, func(q repo.Querier) error {
		var batchErr error
		imported, batchErr = repo.NewCatalogStandardRepo(q).Import(ctx, fw.ID, []domain.CatalogStandard{
			{Code: "6.RP", Description: "Ratios and Proportional Relationships"},
			{Code: "6.RP.A.1", Description: "Understand the concept of a ratio."},
		})
		return batchErr
	})
	require.NoError(t, err)
	require.Len(t, imported, 2)
	require.NotEqual(t, uuid.Nil, imported[0].ID)
	require.Equal(t, "6.RP", imported[0].Code)
	require.Equal(t, fw.ID, imported[0].FrameworkID)
	require.Equal(t, "6.RP.A.1", imported[1].Code)
	tree, err = f.CatalogStandards().ListByFramework(ctx, fw.ID)
	require.NoError(t, err)
	require.Len(t, tree, 4)
}

func seedTwoStandards(t *testing.T, q repo.Querier) (a, b *domain.CatalogStandard) {
	t.Helper()
	ctx := context.Background()
	fw, err := repo.NewFrameworkRepo(q).Create(ctx, &domain.StandardFramework{
		Code: "FW-" + uuid.NewString()[:8],
		Name: "Cycle FW",
	})
	require.NoError(t, err)
	a, err = repo.NewCatalogStandardRepo(q).Create(ctx, &domain.CatalogStandard{
		FrameworkID: fw.ID,
		Code:        "A",
		Description: "A",
	})
	require.NoError(t, err)
	b, err = repo.NewCatalogStandardRepo(q).Create(ctx, &domain.CatalogStandard{
		FrameworkID: fw.ID,
		Code:        "B",
		Description: "B",
	})
	require.NoError(t, err)
	return a, b
}

// P4-E3 / P4-S3: catalog prerequisite cycle rejected by the DB trigger.
func TestP4E3_CatalogPrerequisiteCycleRejected(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	a, b := seedTwoStandards(t, tx)

	edge, err := f.CatalogPrereqs().Create(ctx, domain.CatalogPrerequisite{
		StandardID:     b.ID,
		PrerequisiteID: a.ID,
	})
	require.NoError(t, err)
	require.Equal(t, b.ID, edge.StandardID)
	require.Equal(t, a.ID, edge.PrerequisiteID)

	listed, err := f.CatalogPrereqs().ListForStandard(ctx, b.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, a.ID, listed[0].PrerequisiteID)

	// Reverse edge is a cycle; DB trigger must reject even via raw SQL.
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewCatalogPrereqRepo(sp).Create(ctx, domain.CatalogPrerequisite{
		StandardID:     a.ID,
		PrerequisiteID: b.ID,
	})
	require.ErrorIs(t, err, repo.ErrPrerequisiteCycle)

	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.catalog_standard_prerequisites (standard_id, prerequisite_id)
VALUES ($1, $2)`, a.ID, b.ID)
	require.Error(t, err)
	require.ErrorIs(t, repo.MapError(err), repo.ErrPrerequisiteCycle)

	// Self-edge CHECK.
	_, err = repo.NewCatalogPrereqRepo(sp).Create(ctx, domain.CatalogPrerequisite{
		StandardID:     a.ID,
		PrerequisiteID: a.ID,
	})
	require.Error(t, err)
}

// Concurrent conflicting inserts cannot leave a cycle committed.
func TestP4E3_ConcurrentCycleRejected(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	a, b := seedTwoStandards(t, pool)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	pairs := [][2]uuid.UUID{{b.ID, a.ID}, {a.ID, b.ID}}
	for _, pair := range pairs {
		pair := pair
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.WithTx(ctx, pool, func(q repo.Querier) error {
				_, err := repo.NewCatalogPrereqRepo(q).Create(ctx, domain.CatalogPrerequisite{
					StandardID:     pair[0],
					PrerequisiteID: pair[1],
				})
				return err
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var ok, cycle int
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		mapped := repo.MapError(err)
		if errors.Is(err, repo.ErrPrerequisiteCycle) || errors.Is(mapped, repo.ErrPrerequisiteCycle) {
			cycle++
			continue
		}
		t.Fatalf("unexpected error: %v", err)
	}
	require.Equal(t, 1, ok, "exactly one directed edge may commit")
	require.Equal(t, 1, cycle, "the reverse edge must be rejected as a cycle")

	var n int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*) FROM curriculum_studio.catalog_standard_prerequisites
WHERE (standard_id = $1 AND prerequisite_id = $2)
   OR (standard_id = $2 AND prerequisite_id = $1)`, a.ID, b.ID).Scan(&n))
	require.Equal(t, 1, n, "committed graph must remain acyclic")
}

// P4-E4 / P4-S4: resource metadata without bytes; kind CHECK.
func TestP4E4_ResourceMetadataWithoutBytes(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ten := factory.Tenant(t, tx)
	ws := factory.Workspace(t, tx, func(w *domain.Workspace) { w.TenantID = ten.ID })
	wsB := factory.Workspace(t, tx, func(w *domain.Workspace) { w.TenantID = ten.ID })

	book := factory.Resource(t, tx, func(r *domain.Resource) {
		r.TenantID = ten.ID
		r.WorkspaceID = &ws.ID
		r.Kind = domain.ResourceKindBook
		r.Title = "Euclid's Elements"
		r.Authors = "Euclid"
		r.ArtifactRef = "obj:euclid-elements"
	})
	require.NotEqual(t, uuid.Nil, book.ID)
	require.Equal(t, domain.ResourceKindBook, book.Kind)
	require.Equal(t, "obj:euclid-elements", book.ArtifactRef)
	require.NotContains(t, string(book.Metadata), "body")

	got, err := f.Resources().Get(ctx, ten.ID, book.ID)
	require.NoError(t, err)
	require.Equal(t, book.Title, got.Title)
	require.Equal(t, ws.ID, *got.WorkspaceID)

	// Foreign workspace isolation.
	listA, err := f.Resources().ListByWorkspace(ctx, ten.ID, ws.ID)
	require.NoError(t, err)
	require.Len(t, listA, 1)
	require.Equal(t, book.ID, listA[0].ID)

	other, err := f.Resources().Create(ctx, &domain.Resource{
		TenantID:    ten.ID,
		WorkspaceID: &wsB.ID,
		Kind:        domain.ResourceKindURL,
		Title:       "TN Math standards",
		URL:         "https://example.test/tn-math",
	})
	require.NoError(t, err)
	listA, err = f.Resources().ListByWorkspace(ctx, ten.ID, ws.ID)
	require.NoError(t, err)
	require.Len(t, listA, 1)
	require.NotEqual(t, other.ID, listA[0].ID)

	// IDOR: wrong tenant cannot see the row.
	tenB := factory.Tenant(t, tx)
	_, err = f.Resources().Get(ctx, tenB.ID, book.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// kind CHECK
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewResourceRepo(sp).Create(ctx, &domain.Resource{
		TenantID: ten.ID,
		Kind:     "blob",
		Title:    "Nope",
	})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	// Giant / encoded-byte payloads are refused before SQL.
	_, err = f.Resources().Create(ctx, &domain.Resource{
		TenantID: ten.ID,
		Kind:     domain.ResourceKindDocument,
		Title:    "Too big",
		Metadata: []byte(`{"content_b64":"` + strings.Repeat("A", 200_000) + `"}`),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, repo.ErrPayloadTooLarge)

	// Schema has no body/file byte columns.
	var n int
	require.NoError(t, tx.QueryRow(ctx, `
SELECT count(*)
FROM information_schema.columns
WHERE table_schema = 'curriculum_studio'
  AND table_name = 'resources'
  AND column_name IN ('body', 'bytes', 'content', 'file', 'data', 'blob')`).Scan(&n))
	require.Zero(t, n)
}
