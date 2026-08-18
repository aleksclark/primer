package repo_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestFrameworkRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	r := repo.NewFrameworkRepo(tx)
	ws := factory.Workspace(t, tx)

	_, err := r.Create(ctx, nil)
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.StandardFramework{Code: "", Name: "n"})
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.StandardFramework{Code: "c", Name: ""})
	require.Error(t, err)
	nilWS := uuid.Nil
	_, err = r.Create(ctx, &domain.StandardFramework{Code: "c", Name: "n", WorkspaceID: &nilWS})
	require.Error(t, err)

	_, err = r.Get(ctx, ws.ID, uuid.Nil)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)

	list, err := r.ListWorkspace(ctx, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, list)
	list, err = r.ListVisible(ctx, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, list)

	var nilR *repo.FrameworkRepo
	_, err = nilR.Create(ctx, &domain.StandardFramework{Code: "c", Name: "n"})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, uuid.New(), uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListWorkspace(ctx, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListVisible(ctx, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestCatalogStandardRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	r := repo.NewCatalogStandardRepo(tx)
	ws := factory.Workspace(t, tx)

	_, err := r.Create(ctx, uuid.Nil, nil)
	require.Error(t, err)
	_, err = r.Create(ctx, ws.ID, &domain.CatalogStandard{Code: "X"})
	require.Error(t, err)
	fw := factory.Framework(t, tx)
	_, err = r.Create(ctx, ws.ID, &domain.CatalogStandard{FrameworkID: fw.ID, Code: ""})
	require.Error(t, err)
	nilParent := uuid.Nil
	_, err = r.Create(ctx, ws.ID, &domain.CatalogStandard{FrameworkID: fw.ID, Code: "X", ParentID: &nilParent})
	require.Error(t, err)

	_, err = r.Get(ctx, ws.ID, uuid.Nil)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
	list, err := r.ListByFramework(ctx, ws.ID, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, list)
	list, err = r.ListChildren(ctx, ws.ID, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, list)
	_, err = r.Import(ctx, ws.ID, uuid.Nil, nil)
	require.Error(t, err)

	var nilR *repo.CatalogStandardRepo
	_, err = nilR.Create(ctx, ws.ID, &domain.CatalogStandard{FrameworkID: fw.ID, Code: "X"})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListByFramework(ctx, ws.ID, fw.ID)
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListChildren(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Import(ctx, ws.ID, fw.ID, nil)
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestCatalogPrereqRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	r := repo.NewCatalogPrereqRepo(tx)
	ws := factory.Workspace(t, tx)

	_, err := r.Create(ctx, ws.ID, domain.CatalogPrerequisite{})
	require.Error(t, err)
	id := uuid.New()
	_, err = r.Create(ctx, ws.ID, domain.CatalogPrerequisite{StandardID: id, PrerequisiteID: id})
	require.ErrorIs(t, err, repo.ErrCheckViolation)
	list, err := r.ListForStandard(ctx, ws.ID, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, list)

	var nilR *repo.CatalogPrereqRepo
	_, err = nilR.Create(ctx, ws.ID, domain.CatalogPrerequisite{StandardID: uuid.New(), PrerequisiteID: uuid.New()})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListForStandard(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestCrosswalkRepoRoundTripAndEdges(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	scope := factory.Workspace(t, tx).ID
	from := factory.CatalogStandard(t, tx)
	to := factory.CatalogStandard(t, tx)

	cw, err := f.Crosswalks().Create(ctx, scope, &domain.StandardCrosswalk{
		FromStandardID: from.ID,
		ToStandardID:   to.ID,
		Relationship:   domain.CrosswalkEquivalent,
		Notes:          "same skill",
	})
	require.NoError(t, err)
	got, err := f.Crosswalks().Get(ctx, scope, cw.ID)
	require.NoError(t, err)
	require.Equal(t, domain.CrosswalkEquivalent, got.Relationship)

	list, err := f.Crosswalks().ListFrom(ctx, scope, from.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewCrosswalkRepo(sp).Create(ctx, scope, &domain.StandardCrosswalk{
		FromStandardID: from.ID,
		ToStandardID:   from.ID,
	})
	require.Error(t, err)

	_, err = repo.NewCrosswalkRepo(tx).Create(ctx, uuid.Nil, nil)
	require.Error(t, err)
	_, err = repo.NewCrosswalkRepo(tx).Create(ctx, scope, &domain.StandardCrosswalk{})
	require.Error(t, err)
	_, err = repo.NewCrosswalkRepo(tx).Get(ctx, scope, uuid.Nil)
	require.ErrorIs(t, err, repo.ErrNotFound)
	empty, err := repo.NewCrosswalkRepo(tx).ListFrom(ctx, scope, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	var nilR *repo.CrosswalkRepo
	_, err = nilR.Create(ctx, scope, &domain.StandardCrosswalk{FromStandardID: from.ID, ToStandardID: to.ID})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, scope, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListFrom(ctx, scope, from.ID)
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestResourceRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ten := factory.Tenant(t, tx)
	r := repo.NewResourceRepo(tx)

	_, err := r.Create(ctx, nil)
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Resource{Title: "x"})
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Resource{TenantID: ten.ID, Title: ""})
	require.Error(t, err)
	nilWS := uuid.Nil
	_, err = r.Create(ctx, &domain.Resource{TenantID: ten.ID, Title: "x", WorkspaceID: &nilWS})
	require.Error(t, err)

	_, err = r.Get(ctx, uuid.Nil, uuid.New(), uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ten.ID, uuid.New(), uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
	list, err := r.ListByWorkspace(ctx, uuid.Nil, uuid.New())
	require.NoError(t, err)
	require.Empty(t, list)

	_, err = r.Create(ctx, &domain.Resource{
		TenantID: ten.ID,
		Title:    "encoded",
		Metadata: json.RawMessage(`{"note":"ok"}`),
	})
	require.NoError(t, err)

	_, err = r.Create(ctx, &domain.Resource{
		TenantID:    ten.ID,
		Title:       "href",
		ArtifactRef: "data:application/octet-stream;base64,AAAA",
	})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	var nilR *repo.ResourceRepo
	_, err = nilR.Create(ctx, &domain.Resource{TenantID: ten.ID, Title: "x"})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, ten.ID, uuid.New(), uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.ListByWorkspace(ctx, ten.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestFactoryWiresCatalogRepos(t *testing.T) {
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	require.NotNil(t, f.Frameworks())
	require.NotNil(t, f.CatalogStandards())
	require.NotNil(t, f.CatalogPrereqs())
	require.NotNil(t, f.Crosswalks())
	require.NotNil(t, f.Resources())
}

func TestAntiCheatCatalogSources(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file)))
	needles := []string{
		"LMS_DATABASE",
		"IDENTITY_DATABASE",
		"TEST_DATABASE_URL",
		"DATABASE_URL",
		"server/internal/db",
		"dblink",
		"postgres_fdw",
		"http.Get",
		"grpc.Dial",
	}
	owned := []string{
		"framework.go",
		"catalog_standard.go",
		"catalog_prereq.go",
		"crosswalk.go",
		"resource.go",
		filepath.Join("..", "domain", "catalog.go"),
	}
	for _, name := range owned {
		p := filepath.Join(root, name)
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		body := string(b)
		for _, n := range needles {
			require.NotContainsf(t, body, n, "banned pattern %q in %s", n, p)
		}
		require.NotContains(t, body, "bytea")
	}

	// Resource policy is an explicit allowlist, not a bypassable denylist;
	// production code still refuses LMS table reads and never persists bytes.
	b, err := os.ReadFile(filepath.Join(root, "resource.go"))
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(b)), "public.standards")
	require.Contains(t, string(b), "validateResourcePolicy")
}
