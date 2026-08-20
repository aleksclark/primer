package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func TestP11ProfilesCreateListAndIsolation(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/learner-profiles"

	created := doJSON(t, handler, http.MethodPost, path, map[string]any{
		"kind": "learner", "label": "Ada", "gradeBand": "6-8", "profile": map[string]any{"grade": 6},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var learner struct {
		ID, Kind, Label string
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &learner))
	assert.Equal(t, "learner", learner.Kind)
	assert.Equal(t, "Ada", learner.Label)
	assert.True(t, strings.HasPrefix(learner.ID, "lpr_"))

	class := doJSON(t, handler, http.MethodPost, path, map[string]any{"kind": "class", "label": "Math 6"}, token)
	require.Equal(t, http.StatusCreated, class.Code, class.Body.String())

	list := doJSON(t, handler, http.MethodGet, path, nil, token)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var page struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &page))
	require.Len(t, page.Items, 2)

	foreign := factory.Workspace(t, pool)
	denied := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+foreign.ID.String()+"/learner-profiles", nil, token)
	assert.Equal(t, http.StatusNotFound, denied.Code)
}

func TestP11CreateRunSnapshotFingerprintIdempotencyAndOutbox(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, true)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	revID := publishPlan(t, pool, handler, token, workspace.ID, subject)

	body := map[string]any{
		"learner": map[string]any{"label": "Ada", "grade": "6"},
		"window":  map[string]any{"availableMinutes": 40},
	}
	first := doJSONHeader(t, handler, http.MethodPost, "/studio/v1/revisions/"+revID+"/materializations", body, token, "mat-1")
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	var run struct {
		ID, ContextFingerprint, Status string
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &run))
	assert.NotEmpty(t, run.ContextFingerprint)
	assert.NotEqual(t, run.ContextFingerprint, uuid.NewString())
	assert.Equal(t, "ready", run.Status)

	second := doJSONHeader(t, handler, http.MethodPost, "/studio/v1/revisions/"+revID+"/materializations", body, token, "mat-1")
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())
	var again struct {
		ID, ContextFingerprint string
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &again))
	assert.Equal(t, run.ID, again.ID)
	assert.Equal(t, run.ContextFingerprint, again.ContextFingerprint)

	got := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID, nil, token)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())

	var count int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.materialization_runs WHERE workspace_id=$1`, workspace.ID).Scan(&count))
	assert.Equal(t, 1, count)
	var events int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.outbox_events WHERE workspace_id=$1 AND event_type='materialization.requested'`, workspace.ID).Scan(&events))
	assert.Equal(t, 1, events)
}

func TestP11LockUnlockAndLockedOverwrite(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, true)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	revID := publishPlan(t, pool, handler, token, workspace.ID, subject)
	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revID+"/materializations", map[string]any{"window": map[string]any{"availableMinutes": 20}}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var run struct{ ID string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &run))

	item, err := repo.NewMaterializedItemRepo(pool).Create(t.Context(), workspace.ID, &domain.MaterializedItem{
		RunID: decodePrefixed(t, run.ID, "mat_"), PlanRevisionID: decodePrefixed(t, revID, "prev_"),
		Kind: domain.ItemKindLesson, Title: "Fractions", Body: json.RawMessage(`{"text":"one half"}`),
	})
	require.NoError(t, err)
	itemID := "mit_" + strings.ReplaceAll(item.ID.String(), "-", "")

	locked := doJSON(t, handler, http.MethodPost, "/studio/v1/materialized-items/"+itemID+"/lock", nil, token)
	require.Equal(t, http.StatusOK, locked.Code, locked.Body.String())
	assert.Contains(t, locked.Body.String(), `"lockState":"locked"`)

	denied := doJSON(t, handler, http.MethodPatch, "/studio/v1/materialized-items/"+itemID, map[string]any{"title": "nope"}, token)
	assert.Equal(t, http.StatusConflict, denied.Code)

	unlocked := doJSON(t, handler, http.MethodPost, "/studio/v1/materialized-items/"+itemID+"/unlock", nil, token)
	require.Equal(t, http.StatusOK, unlocked.Code, unlocked.Body.String())
	assert.Contains(t, unlocked.Body.String(), `"lockState":"editable"`)

	updated := doJSON(t, handler, http.MethodPatch, "/studio/v1/materialized-items/"+itemID, map[string]any{"title": "Fractions edited"}, token)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Contains(t, updated.Body.String(), "Fractions edited")

	var audits int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.audit_events WHERE workspace_id=$1 AND action IN ('materialized_item.lock','materialized_item.unlock')`, workspace.ID).Scan(&audits))
	assert.Equal(t, 2, audits)
}

func TestP11ViewerCannotCreateRun(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, true)
	author := uuid.New()
	viewer := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(author), domain.MembershipRoleAuthor)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(viewer), domain.MembershipRoleViewer)
	authorTok := mintHuman(t, key, now, author)
	viewerTok := mintHuman(t, key, now, viewer)
	revID := publishPlan(t, pool, handler, authorTok, workspace.ID, author)
	denied := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revID+"/materializations", map[string]any{"window": map[string]any{"availableMinutes": 10}}, viewerTok)
	assert.Equal(t, http.StatusForbidden, denied.Code)
}

func TestP12E4RetryHTTPAuthorizationAndConflict(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, false)
	author, viewer := uuid.New(), uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(author), domain.MembershipRoleAuthor)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(viewer), domain.MembershipRoleViewer)
	authorToken := mintHuman(t, key, now, author)
	viewerToken := mintHuman(t, key, now, viewer)
	revisionID := decodePrefixed(t, publishPlan(t, pool, handler, authorToken, workspace.ID, author), "prev_")
	snapshot := json.RawMessage(`{"retry":true}`)
	fingerprint, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	runs := repo.NewMaterializationRunRepo(pool)
	run, err := runs.Create(ctx, &domain.MaterializationRun{WorkspaceID: workspace.ID, PlanRevisionID: revisionID, InputSnapshot: snapshot, InputFingerprint: fingerprint})
	require.NoError(t, err)
	_, err = runs.Start(ctx, workspace.ID, run.ID)
	require.NoError(t, err)
	_, err = runs.Fail(ctx, workspace.ID, run.ID)
	require.NoError(t, err)
	path := "/studio/v1/materializations/mat_" + strings.ReplaceAll(run.ID.String(), "-", "") + "/retry"

	allowed := doJSON(t, handler, http.MethodPost, path, nil, authorToken)
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
	denied := doJSON(t, handler, http.MethodPost, path, nil, viewerToken)
	assert.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	conflict := doJSON(t, handler, http.MethodPost, path, nil, authorToken)
	assert.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
}

func newStubMaterializationHandler(t *testing.T, stub bool) (http.Handler, *jwttest.Keypair, time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	validator, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{Validator: validator, Now: func() time.Time { return now }, MatStub: stub})
	return handler, key, now
}

func publishPlan(t *testing.T, pool *pgxpool.Pool, _ http.Handler, _ string, workspaceID, subject uuid.UUID) string {
	t.Helper()
	cur, err := repo.NewCurriculumRepo(pool).Create(t.Context(), &domain.Curriculum{
		WorkspaceID: workspaceID,
		Slug:        "math-" + uuid.NewString()[:8],
		Title:       "Math",
	})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(pool).Create(t.Context(), workspaceID, &domain.PlanRevision{
		CurriculumID: cur.ID, Revision: 1, Title: "Draft 1",
	})
	require.NoError(t, err)
	require.NoError(t, repo.NewPlanRevisionRepo(pool).Publish(t.Context(), workspaceID, rev.ID, domain.HumanSubjectRef(subject)))
	return "prev_" + strings.ReplaceAll(rev.ID.String(), "-", "")
}

func doJSONHeader(t *testing.T, handler http.Handler, method, path string, body any, tok, idem string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodePrefixed(t *testing.T, raw, prefix string) uuid.UUID {
	t.Helper()
	require.True(t, strings.HasPrefix(raw, prefix), raw)
	hex := strings.TrimPrefix(raw, prefix)
	require.Len(t, hex, 32)
	id, err := uuid.Parse(hex[:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:])
	require.NoError(t, err)
	return id
}
