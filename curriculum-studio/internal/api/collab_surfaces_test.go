package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestP17ItemCommentsOwnershipAndDurability(t *testing.T) {
	h, _, key, now := newWorkspaceSuite(t)
	q := testutil.DB(t)
	ws := factory.Workspace(t, q)
	other := factory.Workspace(t, q)
	author, reviewer, outsider := uuid.New(), uuid.New(), uuid.New()
	for _, m := range []struct {
		ws, sub    uuid.UUID
		role, name string
	}{{ws.ID, author, "author", "Ada Author"}, {ws.ID, reviewer, "reviewer", "Ruth Reviewer"}, {other.ID, outsider, "owner", "Other"}} {
		factory.Membership(t, q, func(v *domain.WorkspaceMembership) {
			v.WorkspaceID = m.ws
			v.SubjectRef = domain.HumanSubjectRef(m.sub)
			v.Role = m.role
			v.DisplayName = m.name
		})
	}
	a, r, o := mintHuman(t, key, now, author), mintHuman(t, key, now, reviewer), mintHuman(t, key, now, outsider)
	cur := collabCurriculum(t, h, a, ws.ID.String(), map[string]any{"name": "Item notes"})
	rev := collabRevision(t, h, a, cur.ID)
	snapshot := json.RawMessage(`{}`)
	fp, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	run, err := repo.NewMaterializationRunRepo(q).Create(t.Context(), &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: decodePrefixed(t, rev.ID, "prev_"), InputSnapshot: snapshot, InputFingerprint: fp})
	require.NoError(t, err)
	item, err := repo.NewMaterializedItemRepo(q).Create(t.Context(), ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: "lesson", Title: "Read the scale"})
	require.NoError(t, err)
	path := "/studio/v1/materialized-items/" + repo.ItemCommentKey(item.ID) + "/comments"
	posted := doJSON(t, h, http.MethodPost, path, map[string]any{"body": "Clarify the units."}, r)
	require.Equal(t, 201, posted.Code, posted.Body.String())
	var note api.PlanComment
	require.NoError(t, json.Unmarshal(posted.Body.Bytes(), &note))
	require.Equal(t, "Ruth Reviewer", note.AuthorName)
	require.Equal(t, repo.ItemCommentKey(item.ID), note.ItemID)
	require.Equal(t, rev.ID, note.RevisionID)
	listed := doJSON(t, h, http.MethodGet, path+"?limit=1", nil, a)
	require.Equal(t, 200, listed.Code)
	require.Contains(t, listed.Body.String(), "Clarify the units.")
	bad := doJSON(t, h, http.MethodPost, path, map[string]any{"body": strings.Repeat("界", 10001)}, r)
	require.Equal(t, 422, bad.Code)
	grant := doJSON(t, h, http.MethodPost, "/studio/v1/curricula/"+cur.ID+"/shares", map[string]any{"targetWorkspaceId": other.ID.String(), "permission": "read"}, a)
	require.Equal(t, 200, grant.Code)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		denied := doJSON(t, h, method, path, map[string]any{"body": "No"}, o)
		require.Equal(t, 404, denied.Code)
	}
	invalid := doJSON(t, h, http.MethodPost, "/studio/v1/materialized-items/item_"+uuid.NewString()+"/comments", map[string]any{"body": "No"}, a)
	require.Equal(t, 404, invalid.Code)
	_, err = q.Exec(t.Context(), `INSERT INTO curriculum_studio.plan_comments(workspace_id,plan_revision_id,node_id,item_id,author_subject_ref,body) VALUES($1,$2,'bad',$3,'identity:test','bad')`, other.ID, run.PlanRevisionID, item.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
}

func TestP17PolicyHTTPDefaultsRolesAndPublication(t *testing.T) {
	h, _, key, now := newWorkspaceSuite(t)
	q := testutil.DB(t)
	ws := factory.Workspace(t, q)
	owner, author, reviewer := uuid.New(), uuid.New(), uuid.New()
	for _, m := range []struct {
		sub  uuid.UUID
		role string
	}{{owner, "owner"}, {author, "author"}, {reviewer, "reviewer"}} {
		factory.SeedMembership(t, q, ws.ID, domain.HumanSubjectRef(m.sub), m.role)
	}
	admin, a, r := mintHuman(t, key, now, owner), mintHuman(t, key, now, author), mintHuman(t, key, now, reviewer)
	path := "/studio/v1/workspaces/" + ws.ID.String() + "/collaboration-policy"
	initial := doJSON(t, h, http.MethodGet, path, nil, a)
	require.Equal(t, 200, initial.Code)
	var p domain.CollaborationPolicy
	require.NoError(t, json.Unmarshal(initial.Body.Bytes(), &p))
	require.Equal(t, domain.DefaultCollaborationPolicy(), p)
	p.RequireApprovalForPublish = true
	denied := doJSON(t, h, http.MethodPut, path, p, a)
	require.Equal(t, 403, denied.Code)
	unknown := doJSON(t, h, http.MethodPut, path, map[string]any{"requireApprovalForPublish": false, "sharingEnabled": true, "identityAdmin": true}, admin)
	require.Equal(t, 422, unknown.Code)
	set := doJSON(t, h, http.MethodPut, path, p, admin)
	require.Equal(t, 200, set.Code, set.Body.String())
	fixture := authorProjectViaAPI(t, h, a, ws.ID.String())
	publish := "/studio/v1/revisions/" + fixture.RevisionID + "/publish"
	absent := doJSON(t, h, http.MethodPost, publish, nil, a)
	require.Equal(t, 409, absent.Code, absent.Body.String())
	approve := func() {
		fp := collabApproval(t, h, r, fixture.RevisionID).ContentFingerprint
		require.Equal(t, fp, collabGraph(t, h, r, fixture.RevisionID).ContentFingerprint)
		resp := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/approval", map[string]any{"decision": "approved", "contentFingerprint": fp}, r)
		require.Equal(t, 200, resp.Code, resp.Body.String())
	}
	approve()
	collabNode(t, h, a, fixture.RevisionID, "unit", "New unit after review", "new")
	stale := doJSON(t, h, http.MethodPost, publish, nil, a)
	require.Equal(t, 409, stale.Code)
	approve()
	_, err := q.Exec(t.Context(), `UPDATE curriculum_studio.workspace_memberships SET status='revoked' WHERE workspace_id=$1 AND subject_ref=$2`, ws.ID, domain.HumanSubjectRef(reviewer))
	require.NoError(t, err)
	revoked := doJSON(t, h, http.MethodPost, publish, nil, a)
	require.Equal(t, 409, revoked.Code)
	_, err = q.Exec(t.Context(), `UPDATE curriculum_studio.workspace_memberships SET status='active' WHERE workspace_id=$1 AND subject_ref=$2`, ws.ID, domain.HumanSubjectRef(reviewer))
	require.NoError(t, err)
	approve()
	published := doJSON(t, h, http.MethodPost, publish, nil, a)
	require.Equal(t, 200, published.Code, published.Body.String())
	other := factory.Workspace(t, q)
	cur := collabCurriculum(t, h, a, ws.ID.String(), map[string]any{"name": "Sharing policy"})
	grantPath := "/studio/v1/curricula/" + cur.ID + "/shares"
	grant := doJSON(t, h, http.MethodPost, grantPath, map[string]any{"targetWorkspaceId": other.ID.String(), "permission": "read"}, a)
	require.Equal(t, 200, grant.Code)
	p.SharingEnabled = false
	set = doJSON(t, h, http.MethodPut, path, p, admin)
	require.Equal(t, 200, set.Code)
	blocked := doJSON(t, h, http.MethodPost, grantPath, map[string]any{"targetWorkspaceId": other.ID.String(), "permission": "read"}, a)
	require.Equal(t, 403, blocked.Code)
	shares := doJSON(t, h, http.MethodGet, grantPath, nil, a)
	require.Equal(t, 200, shares.Code)
	require.Contains(t, shares.Body.String(), `"totalCount":0`)
	revoke := doJSON(t, h, http.MethodDelete, grantPath+"/"+other.ID.String(), nil, a)
	require.Equal(t, 204, revoke.Code)
}
