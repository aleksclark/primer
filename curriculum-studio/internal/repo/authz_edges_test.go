package repo_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestTenantRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	r := repo.NewTenantRepo(tx)

	_, err := r.Create(ctx, nil)
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Tenant{Slug: "", Name: "x"})
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Tenant{Slug: "x", Name: ""})
	require.Error(t, err)

	// bad status CHECK
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewTenantRepo(sp).Create(ctx, &domain.Tenant{Slug: "bad-st-" + uuid.NewString()[:8], Name: "N", Status: "nope"})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	_, err = r.UpdateStatus(ctx, uuid.New(), "")
	require.Error(t, err)
	_, err = r.Get(ctx, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.GetBySlug(ctx, "missing-"+uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	var nilR *repo.TenantRepo
	_, err = nilR.Create(ctx, &domain.Tenant{Slug: "a", Name: "b"})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.GetBySlug(ctx, "x")
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.List(ctx)
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.UpdateStatus(ctx, uuid.New(), domain.TenantStatusActive)
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestWorkspaceRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ten := factory.Tenant(t, tx)
	r := repo.NewWorkspaceRepo(tx)

	_, err := r.Create(ctx, nil)
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Workspace{Slug: "s", Name: "n"})
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Workspace{TenantID: ten.ID, Slug: "", Name: "n"})
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.Workspace{TenantID: ten.ID, Slug: "s", Name: ""})
	require.Error(t, err)

	// bad kind CHECK
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewWorkspaceRepo(sp).Create(ctx, &domain.Workspace{
		TenantID: ten.ID, Slug: "k", Name: "N", Kind: "galaxy",
	})
	require.ErrorIs(t, err, repo.ErrCheckViolation)

	// nil ids → not found
	_, err = r.Get(ctx, uuid.Nil, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ten.ID, uuid.Nil)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.GetBySlug(ctx, uuid.Nil, "x")
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.GetBySlug(ctx, ten.ID, "nope")
	require.ErrorIs(t, err, repo.ErrNotFound)
	list, err := r.List(ctx, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, list)
	_, err = r.UpdateStatus(ctx, ten.ID, uuid.New(), "")
	require.Error(t, err)

	var nilR *repo.WorkspaceRepo
	_, err = nilR.Create(ctx, &domain.Workspace{TenantID: ten.ID, Slug: "a", Name: "b"})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, ten.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.GetBySlug(ctx, ten.ID, "x")
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.List(ctx, ten.ID)
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.UpdateStatus(ctx, ten.ID, uuid.New(), domain.WorkspaceStatusActive)
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestMembershipRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	r := repo.NewMembershipRepo(tx)

	_, err := r.Create(ctx, nil)
	require.Error(t, err)
	_, err = r.Create(ctx, &domain.WorkspaceMembership{SubjectRef: domain.HumanSubjectRef(uuid.New())})
	require.Error(t, err)

	// defaults: empty kind/role/status
	m, err := r.Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  domain.HumanSubjectRef(uuid.New()),
	})
	require.NoError(t, err)
	require.Equal(t, domain.SubjectKindHuman, m.SubjectKind)
	require.Equal(t, domain.MembershipRoleViewer, m.Role)
	require.Equal(t, domain.MembershipStatusActive, m.Status)

	list, err := r.List(ctx, ws.ID)
	require.NoError(t, err)
	require.NotEmpty(t, list)
	empty, err := r.List(ctx, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	_, err = r.Get(ctx, uuid.Nil, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ws.ID, uuid.Nil)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrNotFound)

	_, err = r.GetActiveMembership(ctx, uuid.Nil, domain.HumanSubjectRef(uuid.New()))
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.GetActiveMembership(ctx, ws.ID, "")
	require.ErrorIs(t, err, domain.ErrInvalidSubject)
	_, err = r.GetActiveMembership(ctx, ws.ID, "not-a-subject")
	require.ErrorIs(t, err, domain.ErrInvalidSubject)

	_, err = r.UpdateStatus(ctx, ws.ID, m.ID, "")
	require.Error(t, err)
	_, err = r.UpdateRole(ctx, ws.ID, m.ID, "")
	require.Error(t, err)
	_, err = r.UpdateStatus(ctx, ws.ID, uuid.New(), domain.MembershipStatusInvited)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.UpdateRole(ctx, ws.ID, uuid.New(), domain.MembershipRoleOwner)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// invited membership is not active
	inv, err := r.Create(ctx, &domain.WorkspaceMembership{
		WorkspaceID: ws.ID,
		SubjectRef:  domain.HumanSubjectRef(uuid.New()),
		Status:      domain.MembershipStatusInvited,
		Role:        domain.MembershipRoleReviewer,
	})
	require.NoError(t, err)
	_, err = r.GetActiveMembership(ctx, ws.ID, inv.SubjectRef)
	require.ErrorIs(t, err, repo.ErrNotFound)

	var nilR *repo.MembershipRepo
	_, err = nilR.Create(ctx, &domain.WorkspaceMembership{WorkspaceID: ws.ID, SubjectRef: domain.HumanSubjectRef(uuid.New())})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.GetActiveMembership(ctx, ws.ID, domain.HumanSubjectRef(uuid.New()))
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.List(ctx, ws.ID)
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.UpdateStatus(ctx, ws.ID, uuid.New(), domain.MembershipStatusActive)
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.UpdateRole(ctx, ws.ID, uuid.New(), domain.MembershipRoleAdmin)
	require.ErrorIs(t, err, repo.ErrClosed)
}

func TestIntegrationIdentityRepoValidationAndNil(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	r := repo.NewIntegrationIdentityRepo(tx)

	_, err := r.Upsert(ctx, nil)
	require.Error(t, err)
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{System: "x", ExternalKind: "y", ExternalRef: "z"})
	require.Error(t, err)
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{WorkspaceID: ws.ID, System: "", ExternalKind: "learner", ExternalRef: "a"})
	require.Error(t, err)
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{WorkspaceID: ws.ID, System: "primer_lms", ExternalKind: "", ExternalRef: "a"})
	require.Error(t, err)
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{WorkspaceID: ws.ID, System: "primer_lms", ExternalKind: "learner", ExternalRef: ""})
	require.Error(t, err)
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID: ws.ID, System: "primer_lms", ExternalKind: "learner", ExternalRef: "badjson",
		Snapshot: json.RawMessage(`not-json`),
	})
	require.Error(t, err)

	// empty snapshot defaults to {}
	out, err := r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerIdentity,
		ExternalKind: domain.ExternalKindService,
		ExternalRef:  "svc-1",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(out.Snapshot))

	got, err := r.Get(ctx, ws.ID, out.ID)
	require.NoError(t, err)
	require.Equal(t, out.ID, got.ID)
	got2, err := r.GetByExternalRef(ctx, ws.ID, out.System, out.ExternalKind, out.ExternalRef)
	require.NoError(t, err)
	require.Equal(t, out.ID, got2.ID)

	list, err := r.List(ctx, ws.ID)
	require.NoError(t, err)
	require.NotEmpty(t, list)
	empty, err := r.List(ctx, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	_, err = r.Get(ctx, uuid.Nil, out.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.Get(ctx, ws.ID, uuid.Nil)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = r.GetByExternalRef(ctx, uuid.Nil, out.System, out.ExternalKind, out.ExternalRef)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// snapshot must be object (array fails CHECK) — rejected at app validation now
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewIntegrationIdentityRepo(sp).Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemOther,
		ExternalKind: domain.ExternalKindClass,
		ExternalRef:  "c1",
		Snapshot:     json.RawMessage(`[]`),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)

	var nilR *repo.IntegrationIdentityRepo
	_, err = nilR.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID: ws.ID, System: "primer_lms", ExternalKind: "learner", ExternalRef: "z",
	})
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.Get(ctx, ws.ID, uuid.New())
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.GetByExternalRef(ctx, ws.ID, "a", "b", "c")
	require.ErrorIs(t, err, repo.ErrClosed)
	_, err = nilR.List(ctx, ws.ID)
	require.ErrorIs(t, err, repo.ErrClosed)
}

// Bound/sanitize IntegrationIdentity inputs: length, UTF-8/controls, snapshot
// object size, nested secret-bearing keys. Rejects must leave raw DB unchanged.
func TestIntegrationIdentityBoundSanitize(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws := factory.Workspace(t, tx)
	r := repo.NewIntegrationIdentityRepo(tx)

	// Seed a known-good row to prove rejects do not mutate existing data.
	seedSnap := json.RawMessage(`{"display":"safe","grade":6}`)
	seed, err := r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "seed-ref",
		DisplayLabel: "Seed",
		Snapshot:     seedSnap,
	})
	require.NoError(t, err)
	require.JSONEq(t, string(seedSnap), string(seed.Snapshot))

	countRows := func() int {
		t.Helper()
		var n int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.integration_identities WHERE workspace_id = $1`, ws.ID).Scan(&n)
		require.NoError(t, err)
		return n
	}
	rawSnap := func(id uuid.UUID) string {
		t.Helper()
		var b []byte
		err := tx.QueryRow(ctx, `SELECT snapshot::text FROM curriculum_studio.integration_identities WHERE id = $1 AND workspace_id = $2`, id, ws.ID).Scan(&b)
		require.NoError(t, err)
		return string(b)
	}
	beforeCount := countRows()
	beforeSnap := rawSnap(seed.ID)

	// Accepted safe object.
	safe, err := r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemOIDC,
		ExternalKind: domain.ExternalKindAuthSubject,
		ExternalRef:  "safe-sub",
		DisplayLabel: "Safe Label",
		Snapshot:     json.RawMessage(`{"display_name":"Ada","attrs":{"grade":"6","nested":{"ok":true}}}`),
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, safe.ID)
	require.Equal(t, "safe-sub", safe.ExternalRef)

	// Oversize external_ref rejected; DB unchanged.
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  strings.Repeat("x", domain.MaxExternalRefBytes+1),
		Snapshot:     json.RawMessage(`{}`),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)
	require.Equal(t, beforeCount+1, countRows())
	require.JSONEq(t, beforeSnap, rawSnap(seed.ID))

	// Oversize display_label rejected.
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "label-big",
		DisplayLabel: strings.Repeat("y", domain.MaxDisplayLabelBytes+1),
		Snapshot:     json.RawMessage(`{}`),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)

	// Control / NUL in external_ref rejected.
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "bad\x00ref",
		Snapshot:     json.RawMessage(`{}`),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)

	// Invalid UTF-8 in display_label rejected.
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "utf8-bad",
		DisplayLabel: "bad\xfflabel",
		Snapshot:     json.RawMessage(`{}`),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)

	// Nested secret-bearing keys rejected (case-insensitive).
	secretCases := []string{
		`{"Access_Token":"leak"}`,
		`{"nested":{"refresh_token":"x"}}`,
		`{"id_token":"y"}`,
		`{"meta":{"Password":"z"}}`,
		`{"client_secret":"s"}`,
		`{"headers":{"Authorization":"Bearer x"}}`,
		`{"api_secret":"nope"}`,
		`{"secret":"nope"}`,
	}
	for i, body := range secretCases {
		_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
			WorkspaceID:  ws.ID,
			System:       domain.IntegrationSystemPrimerLMS,
			ExternalKind: domain.ExternalKindLearner,
			ExternalRef:  "secret-" + string(rune('a'+i)),
			Snapshot:     json.RawMessage(body),
		})
		require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity, "body=%s", body)
	}
	// Seed row untouched after secret rejects.
	require.JSONEq(t, beforeSnap, rawSnap(seed.ID))

	// Oversize snapshot rejected ( > MaxSnapshotBytes ).
	big := make([]byte, 0, domain.MaxSnapshotBytes+64)
	big = append(big, `{"pad":"`...)
	for len(big) < domain.MaxSnapshotBytes+8 {
		big = append(big, 'z')
	}
	big = append(big, `"}`...)
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "snap-big",
		Snapshot:     json.RawMessage(big),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)

	// Non-object snapshot rejected at validation (not only DB CHECK).
	_, err = r.Upsert(ctx, &domain.IntegrationIdentity{
		WorkspaceID:  ws.ID,
		System:       domain.IntegrationSystemPrimerLMS,
		ExternalKind: domain.ExternalKindLearner,
		ExternalRef:  "snap-arr",
		Snapshot:     json.RawMessage(`["x"]`),
	})
	require.ErrorIs(t, err, domain.ErrInvalidIntegrationIdentity)

	// Final: seed still original; only the accepted safe row was added (+seed).
	require.JSONEq(t, beforeSnap, rawSnap(seed.ID))
	gotSeed, err := r.Get(ctx, ws.ID, seed.ID)
	require.NoError(t, err)
	require.Equal(t, "Seed", gotSeed.DisplayLabel)
	require.JSONEq(t, string(seedSnap), string(gotSeed.Snapshot))
}

func TestWorkspaceDefaultsKindStatus(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ten := factory.Tenant(t, tx)
	ws, err := repo.NewWorkspaceRepo(tx).Create(ctx, &domain.Workspace{
		TenantID: ten.ID,
		Slug:     "def-" + uuid.NewString()[:8],
		Name:     "Defaults",
	})
	require.NoError(t, err)
	require.Equal(t, domain.WorkspaceKindTeacher, ws.Kind)
	require.Equal(t, domain.WorkspaceStatusActive, ws.Status)
}
