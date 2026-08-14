package repo_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

// P3-E1: create tenant+workspace round-trip fields.
func TestP3E1_CreateTenantAndWorkspace(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)

	ten, err := f.Tenants().Create(ctx, &domain.Tenant{Slug: "acme-" + uuid.NewString()[:8], Name: "Acme"})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, ten.ID)
	require.Equal(t, domain.TenantStatusActive, ten.Status)
	require.False(t, ten.CreatedAt.IsZero())
	require.False(t, ten.UpdatedAt.IsZero())

	got, err := f.Tenants().Get(ctx, ten.ID)
	require.NoError(t, err)
	require.Equal(t, ten.Slug, got.Slug)
	require.Equal(t, "Acme", got.Name)

	ws, err := f.Workspaces().Create(ctx, &domain.Workspace{
		TenantID: ten.ID,
		Slug:     "hs-g6",
		Name:     "Homeschool G6",
		Kind:     domain.WorkspaceKindFamily,
	})
	require.NoError(t, err)
	require.Equal(t, ten.ID, ws.TenantID)
	require.Equal(t, "hs-g6", ws.Slug)
	require.Equal(t, domain.WorkspaceKindFamily, ws.Kind)
	require.Equal(t, domain.WorkspaceStatusActive, ws.Status)

	gotWS, err := f.Workspaces().Get(ctx, ten.ID, ws.ID)
	require.NoError(t, err)
	require.Equal(t, ws.ID, gotWS.ID)
	require.Equal(t, "Homeschool G6", gotWS.Name)

	// unique (tenant_id, slug)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewWorkspaceRepo(sp).Create(ctx, &domain.Workspace{
		TenantID: ten.ID,
		Slug:     "hs-g6",
		Name:     "Dup",
		Kind:     domain.WorkspaceKindFamily,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, repo.ErrConflict)
}

// P3-E2: two tenants — list by tenant isolates.
func TestP3E2_WorkspaceListTenantScoped(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)

	a := factory.Tenant(t, tx, func(ten *domain.Tenant) { ten.Slug = "a-" + uuid.NewString()[:8]; ten.Name = "A" })
	b := factory.Tenant(t, tx, func(ten *domain.Tenant) { ten.Slug = "b-" + uuid.NewString()[:8]; ten.Name = "B" })

	wa1 := factory.Workspace(t, tx, func(w *domain.Workspace) {
		w.TenantID = a.ID
		w.Slug = "a1"
	})
	wa2 := factory.Workspace(t, tx, func(w *domain.Workspace) {
		w.TenantID = a.ID
		w.Slug = "a2"
	})
	_ = factory.Workspace(t, tx, func(w *domain.Workspace) {
		w.TenantID = b.ID
		w.Slug = "b1"
	})

	listA, err := f.Workspaces().List(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, listA, 2)
	ids := map[uuid.UUID]bool{listA[0].ID: true, listA[1].ID: true}
	require.True(t, ids[wa1.ID])
	require.True(t, ids[wa2.ID])
	for _, w := range listA {
		require.Equal(t, a.ID, w.TenantID)
	}

	listB, err := f.Workspaces().List(ctx, b.ID)
	require.NoError(t, err)
	require.Len(t, listB, 1)
	require.Equal(t, b.ID, listB[0].TenantID)
}

// P3-E3: human membership + unique conflict + active filter.
func TestP3E3_HumanMembership(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ws := factory.Workspace(t, tx)
	sub := domain.HumanSubjectRef(uuid.New())

	m, err := f.Memberships().Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  sub,
		SubjectKind: domain.SubjectKindHuman,
		Role:        domain.MembershipRoleAuthor,
		Status:      domain.MembershipStatusActive,
		DisplayName: "Ada",
	})
	require.NoError(t, err)
	require.Equal(t, domain.MembershipRoleAuthor, m.Role)

	active, err := f.Memberships().GetActiveMembership(ctx, ws.ID, sub)
	require.NoError(t, err)
	require.Equal(t, m.ID, active.ID)
	require.Equal(t, domain.MembershipRoleAuthor, active.Role)

	// duplicate (workspace_id, subject_ref)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewMembershipRepo(sp).Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  sub,
		SubjectKind: domain.SubjectKindHuman,
		Role:        domain.MembershipRoleViewer,
	})
	require.ErrorIs(t, err, repo.ErrConflict)

	// revoke → GetActiveMembership not found
	_, err = f.Memberships().UpdateStatus(ctx, ws.ID, m.ID, domain.MembershipStatusRevoked)
	require.NoError(t, err)
	_, err = f.Memberships().GetActiveMembership(ctx, ws.ID, sub)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

// Case-variant subject_ref must not create duplicate active memberships; lookup
// via any case form resolves the same canonical row.
func TestMembershipSubjectRefCaseCanonical(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ws := factory.Workspace(t, tx)

	id := uuid.MustParse("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	lower := "identity:" + id.String()
	upper := "IDENTITY:AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE"
	mixed := "identity:AaAaAaAa-BbBb-4CcC-8DdD-EeEeEeEeEeEe"

	m, err := f.Memberships().Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  upper,
		SubjectKind: domain.SubjectKindHuman,
		Role:        domain.MembershipRoleAuthor,
		Status:      domain.MembershipStatusActive,
	})
	require.NoError(t, err)
	require.Equal(t, lower, m.SubjectRef, "persisted subject_ref must be canonical lowercase")

	// Lookup via case variants hits the same membership.
	for _, ref := range []string{lower, upper, mixed} {
		got, err := f.Memberships().GetActiveMembership(ctx, ws.ID, ref)
		require.NoError(t, err, "lookup via %q", ref)
		require.Equal(t, m.ID, got.ID)
		require.Equal(t, lower, got.SubjectRef)
	}

	// Case-variant create conflicts on unique (workspace_id, subject_ref).
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewMembershipRepo(sp).Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  mixed,
		SubjectKind: domain.SubjectKindHuman,
		Role:        domain.MembershipRoleViewer,
	})
	require.ErrorIs(t, err, repo.ErrConflict)

	// Service prefix case variants canonicalize deterministically.
	svcUpper := "IDENTITY:SVC:primer-lms"
	sm, err := f.Memberships().Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  svcUpper,
		SubjectKind: domain.SubjectKindService,
		Role:        domain.MembershipRoleViewer,
	})
	require.NoError(t, err)
	require.Equal(t, "identity:svc:primer-lms", sm.SubjectRef)
	gotSvc, err := f.Memberships().GetActiveMembership(ctx, ws.ID, "identity:svc:primer-lms")
	require.NoError(t, err)
	require.Equal(t, sm.ID, gotSvc.ID)
	_, err = repo.NewMembershipRepo(sp).Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  "identity:svc:primer-lms",
		SubjectKind: domain.SubjectKindService,
		Role:        domain.MembershipRoleViewer,
	})
	require.ErrorIs(t, err, repo.ErrConflict)
}

// P3-E4: service principal membership + CHECK violations.
func TestP3E4_ServiceMembershipAndChecks(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ws := factory.Workspace(t, tx)

	svcRef, err := domain.ServiceSubjectRef("primer-lms")
	require.NoError(t, err)

	m, err := f.Memberships().Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  svcRef,
		SubjectKind: domain.SubjectKindService,
		Role:        domain.MembershipRoleViewer,
		Status:      domain.MembershipStatusActive,
	})
	require.NoError(t, err)
	require.Equal(t, domain.SubjectKindService, m.SubjectKind)
	require.Equal(t, svcRef, m.SubjectRef)
	// No password/token fields on domain type — compile-time + anti-cheat grep.

	// empty subject_ref rejected at app layer
	_, err = f.Memberships().Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  "",
		SubjectKind: domain.SubjectKindService,
	})
	require.ErrorIs(t, err, domain.ErrInvalidSubject)

	// kind mismatch
	_, err = f.Memberships().Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  svcRef,
		SubjectKind: domain.SubjectKindHuman,
	})
	require.ErrorIs(t, err, domain.ErrInvalidSubject)

	// bad role CHECK via savepoint
	sp := testutil.NewSavepointQuerier(tx)
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.workspace_memberships
    (workspace_id, subject_ref, subject_kind, role)
VALUES ($1, $2, 'human', 'superuser')`, ws.ID, domain.HumanSubjectRef(uuid.New()))
	require.Error(t, err)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)

	// bad subject_kind CHECK
	_, err = sp.Exec(ctx, `
INSERT INTO curriculum_studio.workspace_memberships
    (workspace_id, subject_ref, subject_kind, role)
VALUES ($1, $2, 'robot', 'viewer')`, ws.ID, domain.HumanSubjectRef(uuid.New()))
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
}

// P3-E5: integration identity upsert + uniqueness + no LMS DSN usage.
func TestP3E5_IntegrationIdentityUpsert(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ws := factory.Workspace(t, tx)

	snap1 := json.RawMessage(`{"display":"Kid A","grade":6}`)
	first, err := f.IntegrationIdentities().Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "learner_456",
		DisplayLabel: "Kid A",
		Snapshot:     snap1,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, first.ID)
	require.Equal(t, "learner_456", first.ExternalRef)
	require.JSONEq(t, string(snap1), string(first.Snapshot))

	snap2 := json.RawMessage(`{"display":"Kid A","grade":7,"note":"promoted"}`)
	second, err := f.IntegrationIdentities().Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "learner_456",
		DisplayLabel: "Kid A+",
		Snapshot:     snap2,
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "upsert must keep single row")
	require.Equal(t, "Kid A+", second.DisplayLabel)
	require.JSONEq(t, string(snap2), string(second.Snapshot))
	require.True(t, !second.LastSeenAt.Before(first.LastSeenAt))

	list, err := f.IntegrationIdentities().List(ctx, ws.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// email not required — upsert without email fields
	_, err = f.IntegrationIdentities().Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemOIDC,
		ExternalKind: domain.ExternalKindAuthSubject,
		ExternalRef:  "oidc-sub-1",
		Snapshot:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// bad system CHECK
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewIntegrationIdentityRepo(sp).Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       "evil_system",
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "x",
	})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	// Anti-cheat: repo production sources must not reference LMS/Identity DSN or network.
	assertNoCrossDBInRepo(t)
}

// P3-E6: wrong-scope get returns not-found (IDOR).
func TestP3E6_CrossTenantIDOR(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)

	tenA := factory.Tenant(t, tx)
	tenB := factory.Tenant(t, tx)
	wsA := factory.Workspace(t, tx, func(w *domain.Workspace) { w.TenantID = tenA.ID; w.Slug = "a" })
	wsB := factory.Workspace(t, tx, func(w *domain.Workspace) { w.TenantID = tenB.ID; w.Slug = "b" })

	// Get workspace with wrong tenant
	_, err := f.Workspaces().Get(ctx, tenA.ID, wsB.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = f.Workspaces().Get(ctx, tenB.ID, wsA.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// UpdateStatus wrong tenant
	_, err = f.Workspaces().UpdateStatus(ctx, tenA.ID, wsB.ID, domain.WorkspaceStatusArchived)
	require.ErrorIs(t, err, repo.ErrNotFound)

	memB := factory.Membership(t, tx, func(m *domain.WorkspaceMembership) {
		m.WorkspaceID = wsB.ID
	})
	_, err = f.Memberships().Get(ctx, wsA.ID, memB.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// GetActiveMembership wrong workspace
	_, err = f.Memberships().GetActiveMembership(ctx, wsA.ID, memB.SubjectRef)
	require.ErrorIs(t, err, repo.ErrNotFound)

	iiB := factory.IntegrationIdentity(t, tx, func(ii *domain.IntegrationIdentity) {
		ii.WorkspaceID = wsB.ID
		ii.ExternalRef = "idor-ref"
	})
	_, err = f.IntegrationIdentities().Get(ctx, wsA.ID, iiB.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = f.IntegrationIdentities().GetByExternalRef(ctx, wsA.ID, iiB.System, iiB.ExternalKind, iiB.ExternalRef)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// List memberships scoped
	listA, err := f.Memberships().List(ctx, wsA.ID)
	require.NoError(t, err)
	for _, m := range listA {
		require.Equal(t, wsA.ID, m.WorkspaceID)
	}
}

func TestTenantStatusUpdateAndList(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ten := factory.Tenant(t, tx)
	updated, err := f.Tenants().UpdateStatus(ctx, ten.ID, domain.TenantStatusSuspended)
	require.NoError(t, err)
	require.Equal(t, domain.TenantStatusSuspended, updated.Status)
	bySlug, err := f.Tenants().GetBySlug(ctx, ten.Slug)
	require.NoError(t, err)
	require.Equal(t, ten.ID, bySlug.ID)
	list, err := f.Tenants().List(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, list)
}

func TestWorkspaceGetBySlugAndStatus(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	ten := factory.Tenant(t, tx)
	ws := factory.Workspace(t, tx, func(w *domain.Workspace) {
		w.TenantID = ten.ID
		w.Slug = "school-1"
		w.Kind = domain.WorkspaceKindSchool
	})
	got, err := f.Workspaces().GetBySlug(ctx, ten.ID, "school-1")
	require.NoError(t, err)
	require.Equal(t, ws.ID, got.ID)
	arch, err := f.Workspaces().UpdateStatus(ctx, ten.ID, ws.ID, domain.WorkspaceStatusArchived)
	require.NoError(t, err)
	require.Equal(t, domain.WorkspaceStatusArchived, arch.Status)
}

func TestMembershipUpdateRole(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	m := factory.Membership(t, tx)
	updated, err := f.Memberships().UpdateRole(ctx, m.WorkspaceID, m.ID, domain.MembershipRoleAdmin)
	require.NoError(t, err)
	require.Equal(t, domain.MembershipRoleAdmin, updated.Role)
}

func TestConcurrentDuplicateMembership(t *testing.T) {
	// Use pool + WithTx so concurrent inserts hit real unique constraint.
	ctx := context.Background()
	pool := testutil.DB(t)
	// Seed workspace on pool (committed) then clean up via unique slug isolation.
	ten, err := repo.NewTenantRepo(pool).Create(ctx, &domain.Tenant{
		Slug: "conc-" + uuid.NewString(),
		Name: "Conc",
	})
	require.NoError(t, err)
	ws, err := repo.NewWorkspaceRepo(pool).Create(ctx, &domain.Workspace{
		TenantID: ten.ID,
		Slug:     "w",
		Name:     "W",
		Kind:     domain.WorkspaceKindTeacher,
	})
	require.NoError(t, err)
	sub := domain.HumanSubjectRef(uuid.New())

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.WithTx(ctx, pool, func(q repo.Querier) error {
				_, err := repo.NewMembershipRepo(q).Create(ctx, &domain.WorkspaceMembership{
					WorkspaceID: ws.ID,
					SubjectRef:  sub,
					SubjectKind: domain.SubjectKindHuman,
					Role:        domain.MembershipRoleAuthor,
				})
				return err
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var ok, conflict int
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		if errors.Is(err, repo.ErrConflict) {
			conflict++
			continue
		}
		// Map wrapped unique from commit path
		if errors.Is(repo.MapError(err), repo.ErrConflict) {
			conflict++
			continue
		}
		t.Fatalf("unexpected error: %v", err)
	}
	require.Equal(t, 1, ok)
	require.Equal(t, 7, conflict)
}

func TestConcurrentIntegrationUpsert(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	ten, err := repo.NewTenantRepo(pool).Create(ctx, &domain.Tenant{
		Slug: "ups-" + uuid.NewString(),
		Name: "Ups",
	})
	require.NoError(t, err)
	ws, err := repo.NewWorkspaceRepo(pool).Create(ctx, &domain.Workspace{
		TenantID: ten.ID,
		Slug:     "w",
		Name:     "W",
		Kind:     domain.WorkspaceKindTeacher,
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			out, err := repo.NewIntegrationIdentityRepo(pool).Upsert(ctx, &domain.IntegrationIdentity{
				WorkspaceID:  ws.ID,
				System:       domain.IntegrationSystemPrimerLMS,
				ExternalKind: domain.ExternalKindLearner,
				ExternalRef:  "same-ref",
				DisplayLabel: "L",
				Snapshot:     json.RawMessage([]byte(`{"n":` + string(rune('0'+i%10)) + `}`)),
			})
			require.NoError(t, err)
			ids <- out.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[uuid.UUID]struct{}{}
	for id := range ids {
		seen[id] = struct{}{}
	}
	require.Len(t, seen, 1, "all upserts must converge on one row id")
}

func TestFactoryWiresAuthzRepos(t *testing.T) {
	tx := testutil.Tx(t)
	f := repo.NewFactory(tx)
	require.NotNil(t, f.Tenants())
	require.NotNil(t, f.Workspaces())
	require.NotNil(t, f.Memberships())
	require.NotNil(t, f.IntegrationIdentities())
}

func TestAntiCheatNoCredentialColumnsInAuthz(t *testing.T) {
	t.Parallel()
	assertNoCredentialsInAuthzSources(t)
	assertNoCrossDBInRepo(t)
}

func assertNoCrossDBInRepo(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	needles := []string{
		"LMS_DATABASE",
		"IDENTITY_DATABASE",
		"TEST_DATABASE_URL",
		"DATABASE_URL",
		"primer-identity",
		"server/internal/db",
		"password_hash",
		"bcrypt",
		"refresh_token",
		"oauth_token",
		"dblink",
		"postgres_fdw",
	}
	err := filepath.WalkDir(filepath.Join(root, "repo"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(b)
		for _, n := range needles {
			require.NotContainsf(t, body, n, "banned pattern %q in %s", n, path)
		}
		return nil
	})
	require.NoError(t, err)

	// domain package likewise — ban cross-DB and credential *storage* patterns.
	// integration_identity.go is allowed to mention denylist *fragments* only via
	// joined parts; still ban contiguous credential column names and LMS DSN.
	err = filepath.WalkDir(filepath.Join(root, "domain"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(b)
		low := strings.ToLower(body)
		for _, n := range []string{"bcrypt", "oauth_token", "lms_database", "password_hash", "token_hash"} {
			require.NotContainsf(t, low, n, "banned %q in %s", n, path)
		}
		// Contiguous credential field names banned except in the sanitizer denylist builder.
		base := filepath.Base(path)
		if base != "integration_identity.go" {
			for _, n := range []string{"password", "refresh_token", "access_token", "client_secret"} {
				require.NotContainsf(t, low, n, "banned %q in %s", n, path)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func assertNoCredentialsInAuthzSources(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// Scan authz domain + membership/integration repo for credential-ish columns.
	paths := []string{
		filepath.Join(filepath.Dir(file), "..", "domain", "authz.go"),
		filepath.Join(filepath.Dir(file), "membership.go"),
		filepath.Join(filepath.Dir(file), "integration_identity.go"),
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		low := strings.ToLower(string(b))
		// Column-ish credential storage patterns only (sanitizer denylist is separate).
		for _, n := range []string{"bcrypt", "token_hash", "secret_hash", "password_hash"} {
			require.NotContainsf(t, low, n, "%s must not mention %s", p, n)
		}
	}
}
