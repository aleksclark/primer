package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestB17InvalidApprovedGraphReportsActionableFindings(t *testing.T) {
	h, _, key, now := newWorkspaceSuite(t)
	q := testutil.DB(t)
	ws := factory.Workspace(t, q)
	author, reviewer, owner := uuid.New(), uuid.New(), uuid.New()
	for _, m := range []struct {
		sub  uuid.UUID
		role string
	}{{author, "author"}, {reviewer, "reviewer"}, {owner, "owner"}} {
		factory.SeedMembership(t, q, ws.ID, domain.HumanSubjectRef(m.sub), m.role)
	}
	a, r, o := mintHuman(t, key, now, author), mintHuman(t, key, now, reviewer), mintHuman(t, key, now, owner)
	configured := doJSON(t, h, http.MethodPut, "/studio/v1/workspaces/"+ws.ID.String()+"/collaboration-policy", domain.CollaborationPolicy{RequireApprovalForPublish: true, SharingEnabled: true}, o)
	require.Equal(t, 200, configured.Code)
	f := authorProjectViaAPI(t, h, a, ws.ID.String())
	path := "/studio/v1/revisions/" + f.RevisionID
	for _, name := range []string{"Stale-review mutation outcome", "Policy-gated publication outcome"} {
		added := doJSON(t, h, http.MethodPost, path+"/nodes", map[string]any{"kind": "outcome", "title": name, "attributes": map[string]string{"evidenceKind": "portfolio", "evidenceDescription": "Demonstrate mastery"}}, a)
		require.Equal(t, 201, added.Code, added.Body.String())
	}
	fp := collabApproval(t, h, r, f.RevisionID).ContentFingerprint
	approved := doJSON(t, h, http.MethodPost, path+"/approval", map[string]any{"decision": "approved", "contentFingerprint": fp}, r)
	require.Equal(t, 200, approved.Code)
	denied := doJSON(t, h, http.MethodPost, path+"/publish", nil, a)
	require.Equal(t, 409, denied.Code, denied.Body.String())
	var problem struct {
		Detail string
		Errors []struct{ Message, Location string }
	}
	require.NoError(t, json.Unmarshal(denied.Body.Bytes(), &problem))
	require.Contains(t, problem.Detail, "Revision validation failed")
	require.NotContains(t, problem.Detail, "not editable")
	require.Len(t, problem.Errors, 2)
	messages := []string{}
	for _, f := range problem.Errors {
		require.Contains(t, f.Message, "OUTCOME_UNMAPPED")
		require.Contains(t, f.Location, "graph.outcome.")
		messages = append(messages, f.Message)
	}
	require.ElementsMatch(t, []string{`OUTCOME_UNMAPPED: outcome "Stale-review mutation outcome" has no standards mapping`, `OUTCOME_UNMAPPED: outcome "Policy-gated publication outcome" has no standards mapping`}, messages)
	got := doJSON(t, h, http.MethodGet, path, nil, a)
	var revision api.PlanRevision
	require.NoError(t, json.Unmarshal(got.Body.Bytes(), &revision))
	require.Equal(t, api.RevisionState("draft"), revision.State)
	require.Equal(t, "approved", collabApproval(t, h, a, f.RevisionID).State)
	var publications int
	require.NoError(t, q.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.outbox_events WHERE aggregate_id=$1 AND event_type='plan_revision.published'`, decodePrefixed(t, f.RevisionID, "prev_")).Scan(&publications))
	require.Zero(t, publications)
}

func TestB17MappedOutcomeEditRequiresFreshReviewThenPublishes(t *testing.T) {
	h, _, key, now := newWorkspaceSuite(t)
	q := testutil.DB(t)
	ws := factory.Workspace(t, q)
	author, reviewer, owner := uuid.New(), uuid.New(), uuid.New()
	for _, m := range []struct {
		sub  uuid.UUID
		role string
	}{{author, "author"}, {reviewer, "reviewer"}, {owner, "owner"}} {
		factory.SeedMembership(t, q, ws.ID, domain.HumanSubjectRef(m.sub), m.role)
	}
	a, r, o := mintHuman(t, key, now, author), mintHuman(t, key, now, reviewer), mintHuman(t, key, now, owner)
	f := authorProjectViaAPI(t, h, a, ws.ID.String())
	path := "/studio/v1/revisions/" + f.RevisionID
	configured := doJSON(t, h, http.MethodPut, "/studio/v1/workspaces/"+ws.ID.String()+"/collaboration-policy", domain.CollaborationPolicy{RequireApprovalForPublish: true, SharingEnabled: true}, o)
	require.Equal(t, 200, configured.Code)
	old := collabApproval(t, h, r, f.RevisionID).ContentFingerprint
	// Same valid shape as the UI, referencing the real workspace catalog created
	// by authorProjectViaAPI. No standard is invented or validation rule waived.
	added := doJSON(t, h, http.MethodPost, path+"/nodes", map[string]any{"kind": "outcome", "title": "Mapped edit for review", "standardCodes": []string{"MATH.6.G"}, "attributes": map[string]string{"evidenceKind": "portfolio", "evidenceDescription": "Demonstrate mastery"}}, a)
	require.Equal(t, 201, added.Code, added.Body.String())
	var node api.PlanNode
	require.NoError(t, json.Unmarshal(added.Body.Bytes(), &node))
	require.Equal(t, []string{"MATH.6.G"}, node.StandardCodes)
	stale := doJSON(t, h, http.MethodPost, path+"/approval", map[string]any{"decision": "approved", "contentFingerprint": old}, r)
	require.Equal(t, 409, stale.Code)
	before := doJSON(t, h, http.MethodPost, path+"/publish", nil, a)
	require.Equal(t, 409, before.Code)
	require.Contains(t, before.Body.String(), "current reviewer approval required")
	require.NotContains(t, before.Body.String(), "Revision validation failed")
	current := collabApproval(t, h, r, f.RevisionID).ContentFingerprint
	require.NotEqual(t, old, current)
	approved := doJSON(t, h, http.MethodPost, path+"/approval", map[string]any{"decision": "approved", "contentFingerprint": current}, r)
	require.Equal(t, 200, approved.Code)
	published := doJSON(t, h, http.MethodPost, path+"/publish", nil, a)
	require.Equal(t, 200, published.Code, published.Body.String())
	var revision api.PlanRevision
	require.NoError(t, json.Unmarshal(published.Body.Bytes(), &revision))
	require.Equal(t, api.RevisionState("published"), revision.State)
	again := doJSON(t, h, http.MethodPost, path+"/publish", nil, a)
	require.Equal(t, 409, again.Code)
	require.Contains(t, again.Body.String(), "revision is not editable")
}

func TestB17W2ValidMutationDeniedWithoutPersistence(t *testing.T) {
	h, _, key, now := newWorkspaceSuite(t)
	q := testutil.DB(t)
	w1, w2 := factory.Workspace(t, q), factory.Workspace(t, q)
	author, reader := uuid.New(), uuid.New()
	factory.SeedMembership(t, q, w1.ID, domain.HumanSubjectRef(author), "author")
	factory.SeedMembership(t, q, w2.ID, domain.HumanSubjectRef(reader), "author")
	a, b := mintHuman(t, key, now, author), mintHuman(t, key, now, reader)
	cur := collabCurriculum(t, h, a, w1.ID.String(), map[string]any{"name": "Protected probe"})
	revision := collabRevision(t, h, a, cur.ID)
	payload := map[string]any{"kind": "unit", "title": "W2 authorization probe", "position": 0}
	path := "/studio/v1/revisions/" + revision.ID + "/nodes"
	control := doJSON(t, h, http.MethodPost, path, payload, a)
	require.Equal(t, 201, control.Code, control.Body.String())
	shared := doJSON(t, h, http.MethodPost, "/studio/v1/curricula/"+cur.ID+"/shares", map[string]any{"targetWorkspaceId": w2.ID.String(), "permission": "read"}, a)
	require.Equal(t, 200, shared.Code)
	before := collabGraph(t, h, a, revision.ID)
	denied := doJSON(t, h, http.MethodPost, path, payload, b)
	require.Equal(t, 404, denied.Code, denied.Body.String())
	require.NotEqual(t, 422, denied.Code)
	require.Equal(t, before, collabGraph(t, h, a, revision.ID))
}
