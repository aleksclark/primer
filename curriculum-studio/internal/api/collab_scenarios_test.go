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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collabApproval(t *testing.T, h http.Handler, token, revision string) api.PlanApproval {
	t.Helper()
	r := doJSON(t, h, http.MethodGet, "/studio/v1/revisions/"+revision+"/approval", nil, token)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var v api.PlanApproval
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &v))
	return v
}
func collabCurriculum(t *testing.T, h http.Handler, token, workspace string, body map[string]any) api.Curriculum {
	t.Helper()
	r := doJSON(t, h, http.MethodPost, "/studio/v1/workspaces/"+workspace+"/curricula", body, token)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	var v api.Curriculum
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &v))
	return v
}
func collabRevision(t *testing.T, h http.Handler, token, curriculum string) api.PlanRevision {
	t.Helper()
	r := doJSON(t, h, http.MethodPost, "/studio/v1/curricula/"+curriculum+"/revisions", map[string]any{}, token)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	var v api.PlanRevision
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &v))
	return v
}
func collabNode(t *testing.T, h http.Handler, token, rev, kind, title, code string) api.PlanNode {
	t.Helper()
	r := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+rev+"/nodes", map[string]any{"kind": kind, "title": title, "attributes": map[string]string{"code": code}}, token)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	var v api.PlanNode
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &v))
	return v
}
func collabGraph(t *testing.T, h http.Handler, token, revision string) api.PlanGraph {
	t.Helper()
	r := doJSON(t, h, http.MethodGet, "/studio/v1/revisions/"+revision+"/graph", nil, token)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var v api.PlanGraph
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &v))
	return v
}

func TestP17S2RevisionDiffNamesAndAuthorization(t *testing.T) {
	t.Parallel()
	h, _, key, now := newWorkspaceSuite(t)
	db := testutil.DB(t)
	author := uuid.New()
	ws := factory.Workspace(t, db)
	factory.SeedMembership(t, db, ws.ID, domain.HumanSubjectRef(author), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, author)
	cur := collabCurriculum(t, h, token, ws.ID.String(), map[string]any{"name": "Comparison", "template": "custom"})
	a := collabRevision(t, h, token, cur.ID)
	b := collabRevision(t, h, token, cur.ID)
	for _, v := range []struct{ rev, code, name string }{{a.ID, "same", "Unchanged"}, {b.ID, "same", "Unchanged"}, {a.ID, "rename", "Old mastery"}, {b.ID, "rename", "Refined mastery"}, {a.ID, "gone", "Removed outcome"}, {b.ID, "new", "New outcome"}} {
		collabNode(t, h, token, v.rev, "outcome", v.name, v.code)
	}
	path := "/studio/v1/revisions/" + b.ID + "/diff?fromRevisionId=" + a.ID
	response := doJSON(t, h, http.MethodGet, path, nil, token)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var diff domain.RevisionDiff
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &diff))
	assert.ElementsMatch(t, []domain.OutcomeChange{{Code: "rename", Name: "Refined mastery"}, {Code: "new", Name: "New outcome"}}, diff.AddedOutcomes)
	assert.ElementsMatch(t, []domain.OutcomeChange{{Code: "rename", Name: "Old mastery"}, {Code: "gone", Name: "Removed outcome"}}, diff.RemovedOutcomes)
	same := doJSON(t, h, http.MethodGet, "/studio/v1/revisions/"+a.ID+"/diff?fromRevisionId="+a.ID, nil, token)
	require.Equal(t, http.StatusOK, same.Code, same.Body.String())
	require.NoError(t, json.Unmarshal(same.Body.Bytes(), &diff))
	assert.Empty(t, diff.AddedOutcomes)
	assert.Empty(t, diff.RemovedOutcomes)
	other := collabCurriculum(t, h, token, ws.ID.String(), map[string]any{"name": "Unrelated"})
	otherRev := collabRevision(t, h, token, other.ID)
	mixed := doJSON(t, h, http.MethodGet, "/studio/v1/revisions/"+b.ID+"/diff?fromRevisionId="+otherRev.ID, nil, token)
	require.Equal(t, http.StatusBadRequest, mixed.Code, mixed.Body.String())
	outsider := uuid.New()
	foreign := factory.Workspace(t, db)
	factory.SeedMembership(t, db, foreign.ID, domain.HumanSubjectRef(outsider), domain.MembershipRoleAuthor)
	denied := doJSON(t, h, http.MethodGet, path, nil, mintHuman(t, key, now, outsider))
	require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
	// An authorized 'to' revision never authorizes an unshared 'from'.
	foreignToken := mintHuman(t, key, now, outsider)
	foreignCur := collabCurriculum(t, h, foreignToken, foreign.ID.String(), map[string]any{"name": "Foreign"})
	foreignRev := collabRevision(t, h, foreignToken, foreignCur.ID)
	denied = doJSON(t, h, http.MethodGet, "/studio/v1/revisions/"+b.ID+"/diff?fromRevisionId="+foreignRev.ID, nil, token)
	require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
}

func TestP17S3LibraryAndTemplatePopulateDraftAtomically(t *testing.T) {
	t.Parallel()
	h, _, key, now := newWorkspaceSuite(t)
	db := testutil.DB(t)
	author := uuid.New()
	ws := factory.Workspace(t, db)
	factory.SeedMembership(t, db, ws.ID, domain.HumanSubjectRef(author), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, author)
	source := authorProjectViaAPI(t, h, token, ws.ID.String())
	unit := collabNode(t, h, token, source.RevisionID, "unit", "Workshop geometry", "geometry")
	for _, id := range []string{source.MathID, source.ScienceID} {
		edge := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+source.RevisionID+"/edges", map[string]any{"kind": "parent_child", "fromNodeId": unit.ID, "toNodeId": id, "note": "target"}, token)
		require.Equal(t, http.StatusCreated, edge.Code, edge.Body.String())
	}
	pre := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+source.RevisionID+"/edges", map[string]any{"kind": "prerequisite", "fromNodeId": source.MathID, "toNodeId": source.ScienceID, "note": "completed"}, token)
	require.Equal(t, http.StatusCreated, pre.Code, pre.Body.String())
	saved := doJSON(t, h, http.MethodPost, "/studio/v1/workspaces/"+ws.ID.String()+"/unit-library", map[string]any{"revisionId": source.RevisionID, "unitId": unit.ID, "name": "Geometry library"}, token)
	require.Equal(t, http.StatusCreated, saved.Code, saved.Body.String())
	var entry api.LibraryEntry
	require.NoError(t, json.Unmarshal(saved.Body.Bytes(), &entry))
	assert.Equal(t, "Geometry library", entry.Name)
	listed := doJSON(t, h, http.MethodGet, "/studio/v1/workspaces/"+ws.ID.String()+"/unit-library?limit=1", nil, token)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), entry.ID)
	template := doJSON(t, h, http.MethodPost, "/studio/v1/workspaces/"+ws.ID.String()+"/templates", map[string]any{"code": "workshop", "name": "Workshop template", "briefType": "project_based_unit", "seed": map[string]any{"outcomes": []map[string]any{{"code": "craft", "title": "Craftsmanship"}}, "units": []map[string]any{{"code": "intro", "title": "Introduction to tools", "outcomeCodes": []string{"craft"}}}}}, token)
	require.Equal(t, http.StatusCreated, template.Code, template.Body.String())
	templates := doJSON(t, h, http.MethodGet, "/studio/v1/workspaces/"+ws.ID.String()+"/templates", nil, token)
	require.Equal(t, http.StatusOK, templates.Code, templates.Body.String())
	assert.Contains(t, templates.Body.String(), "Workshop template")
	cur := collabCurriculum(t, h, token, ws.ID.String(), map[string]any{"name": "Year two workshop", "templateCode": "workshop"})
	assert.Equal(t, "workshop", cur.TemplateCode)
	revisions := doJSON(t, h, http.MethodGet, "/studio/v1/curricula/"+cur.ID+"/revisions", nil, token)
	require.Equal(t, http.StatusOK, revisions.Code, revisions.Body.String())
	var page struct{ Items []api.PlanRevision }
	require.NoError(t, json.Unmarshal(revisions.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	revision := page.Items[0]
	assert.Equal(t, api.RevisionState("draft"), revision.State)
	before := collabGraph(t, h, token, revision.ID)
	require.Len(t, before.Nodes, 2)
	require.Len(t, before.Edges, 1)
	assert.Equal(t, "Craftsmanship", before.Nodes[0].Title)
	assert.Equal(t, "Introduction to tools", before.Nodes[1].Title)
	copied := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/unit-library/"+entry.ID, nil, token)
	require.Equal(t, http.StatusCreated, copied.Code, copied.Body.String())
	after := collabGraph(t, h, token, revision.ID)
	require.Len(t, after.Nodes, 5)
	names := map[string]api.PlanNode{}
	for _, n := range after.Nodes {
		names[n.Title] = n
	}
	for _, name := range []string{"Craftsmanship", "Introduction to tools", "Workshop geometry", "Scale drawings", "Load paths"} {
		require.Contains(t, names, name)
	}
	assert.NotEqual(t, unit.ID, names["Workshop geometry"].ID)
	assert.NotEqual(t, source.MathID, names["Scale drawings"].ID)
	assert.Equal(t, []string{"MATH.6.G"}, names["Scale drawings"].StandardCodes)
	assert.Equal(t, "portfolio", names["Scale drawings"].Attributes["evidenceKind"])
	assert.Equal(t, "photo essay", names["Scale drawings"].Attributes["evidenceDescription"])
	require.Len(t, after.Edges, 4)
	for _, edge := range after.Edges {
		assert.NotEqual(t, unit.ID, edge.FromNodeID)
		assert.NotEqual(t, source.MathID, edge.ToNodeID)
		assert.NotEqual(t, source.ScienceID, edge.ToNodeID)
	}
	// Repeated copies get independent node IDs, not a reference into the source.
	again := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/unit-library/"+entry.ID, nil, token)
	require.Equal(t, http.StatusCreated, again.Code, again.Body.String())
	assert.Len(t, collabGraph(t, h, token, revision.ID).Nodes, 8)
	assert.Len(t, collabGraph(t, h, token, source.RevisionID).Nodes, 5)
	// Invalid template creation cannot leave a curriculum/outbox event behind.
	failed := doJSON(t, h, http.MethodPost, "/studio/v1/workspaces/"+ws.ID.String()+"/curricula", map[string]any{"name": "Must roll back", "templateCode": "missing"}, token)
	require.Equal(t, http.StatusNotFound, failed.Code, failed.Body.String())
	var count int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.curricula WHERE workspace_id=$1 AND title='Must roll back'`, ws.ID).Scan(&count))
	assert.Zero(t, count)
	// A bad reference is rejected before a reusable template is stored.
	invalid := doJSON(t, h, http.MethodPost, "/studio/v1/workspaces/"+ws.ID.String()+"/templates", map[string]any{"code": "bad", "name": "Invalid", "briefType": "custom", "seed": map[string]any{"outcomes": []any{}, "units": []map[string]any{{"title": "Bad unit", "outcomeCodes": []string{"missing"}}}}}, token)
	require.Equal(t, http.StatusBadRequest, invalid.Code, invalid.Body.String())
	// Different principals/workspaces cannot copy private library content.
	outsider := uuid.New()
	foreign := factory.Workspace(t, db)
	factory.SeedMembership(t, db, foreign.ID, domain.HumanSubjectRef(outsider), domain.MembershipRoleAuthor)
	otherToken := mintHuman(t, key, now, outsider)
	otherCur := collabCurriculum(t, h, otherToken, foreign.ID.String(), map[string]any{"name": "Other"})
	otherRev := collabRevision(t, h, otherToken, otherCur.ID)
	denied := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+otherRev.ID+"/unit-library/"+entry.ID, nil, otherToken)
	require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
	assert.Empty(t, collabGraph(t, h, otherToken, otherRev.ID).Nodes)
	denied = doJSON(t, h, http.MethodPost, "/studio/v1/workspaces/"+foreign.ID.String()+"/curricula", map[string]any{"name": "Private template", "templateCode": "workshop"}, otherToken)
	require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
	// The saved snapshot retains its catalog reference after the author removes
	// that catalog entry. Failure partway through HTTP import must roll back the
	// newly inserted unit and outcomes, not just return an error status.
	var standardID uuid.UUID
	require.NoError(t, db.QueryRow(t.Context(), `SELECT m.standard_id FROM curriculum_studio.outcome_standard_mappings m JOIN curriculum_studio.catalog_standards s ON s.id=m.standard_id JOIN curriculum_studio.standard_frameworks f ON f.id=s.framework_id WHERE m.outcome_id=$1 AND f.workspace_id=$2`, decodePrefixed(t, source.MathID, "out_"), ws.ID).Scan(&standardID))
	_, err := db.Exec(t.Context(), `DELETE FROM curriculum_studio.outcome_standard_mappings WHERE standard_id=$1`, standardID)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), `DELETE FROM curriculum_studio.catalog_standards WHERE id=$1`, standardID)
	require.NoError(t, err)
	beforeFailure := collabGraph(t, h, token, revision.ID)
	failedCopy := doJSON(t, h, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/unit-library/"+entry.ID, nil, token)
	require.Equal(t, http.StatusNotFound, failedCopy.Code, failedCopy.Body.String())
	assert.Equal(t, beforeFailure, collabGraph(t, h, token, revision.ID))

	for _, code := range []string{"homeschool_year", "single_subject", "classroom_semester", "project_based_unit", "standards_remediation"} {
		seeded := collabCurriculum(t, h, token, ws.ID.String(), map[string]any{"name": code, "template": code})
		revs := doJSON(t, h, http.MethodGet, "/studio/v1/curricula/"+seeded.ID+"/revisions", nil, token)
		require.Equal(t, http.StatusOK, revs.Code, revs.Body.String())
		require.NoError(t, json.Unmarshal(revs.Body.Bytes(), &page))
		require.Len(t, page.Items, 1)
		graph := collabGraph(t, h, token, page.Items[0].ID)
		assert.Len(t, graph.Nodes, 2)
		assert.Len(t, graph.Edges, 1)
		get := doJSON(t, h, http.MethodGet, "/studio/v1/curricula/"+seeded.ID, nil, token)
		require.Equal(t, http.StatusOK, get.Code, get.Body.String())
		var fetched api.Curriculum
		require.NoError(t, json.Unmarshal(get.Body.Bytes(), &fetched))
		assert.Equal(t, api.CurriculumTemplate(code), fetched.Template)
	}
}

func TestP17S4ShareReadOnlyAndRevocation(t *testing.T) {
	t.Parallel()
	h, _, key, now := newWorkspaceSuite(t)
	db := testutil.DB(t)
	var ws [3]*domain.Workspace
	var token [3]string
	for i := range ws {
		ws[i] = factory.Workspace(t, db)
		sub := uuid.New()
		factory.SeedMembership(t, db, ws[i].ID, domain.HumanSubjectRef(sub), domain.MembershipRoleAuthor)
		token[i] = mintHuman(t, key, now, sub)
	}
	cur := collabCurriculum(t, h, token[0], ws[0].ID.String(), map[string]any{"name": "Shared workshop"})
	rev := collabRevision(t, h, token[0], cur.ID)
	node := collabNode(t, h, token[0], rev.ID, "outcome", "Private mastery", "private")
	cpath := "/studio/v1/curricula/" + cur.ID
	rpath := "/studio/v1/revisions/" + rev.ID
	share := doJSON(t, h, http.MethodPost, cpath+"/shares", map[string]any{"targetWorkspaceId": ws[1].ID.String(), "permission": "read"}, token[0])
	require.Equal(t, http.StatusOK, share.Code, share.Body.String())
	replay := doJSON(t, h, http.MethodPost, cpath+"/shares", map[string]any{"targetWorkspaceId": ws[1].ID.String(), "permission": "read"}, token[0])
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	for _, path := range []string{cpath, cpath + "/revisions", rpath, rpath + "/graph", rpath + "/diff?fromRevisionId=" + rev.ID} {
		read := doJSON(t, h, http.MethodGet, path, nil, token[1])
		require.Equal(t, http.StatusOK, read.Code, read.Body.String())
		denied := doJSON(t, h, http.MethodGet, path, nil, token[2])
		require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
	}
	page := doJSON(t, h, http.MethodGet, "/studio/v1/workspaces/"+ws[1].ID.String()+"/curricula?q=Shared&limit=1", nil, token[1])
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	var list struct {
		Items      []api.Curriculum
		TotalCount int
	}
	require.NoError(t, json.Unmarshal(page.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, cur.ID, list.Items[0].ID)
	assert.Equal(t, 1, list.TotalCount)
	page = doJSON(t, h, http.MethodGet, "/studio/v1/workspaces/"+ws[2].ID.String()+"/curricula", nil, token[2])
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	require.NoError(t, json.Unmarshal(page.Body.Bytes(), &list))
	assert.Empty(t, list.Items)
	// Grants do not become synthetic memberships. All mutating and private
	// adjunct routes still resolve exclusively through the source workspace.
	writes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPatch, cpath, map[string]any{"name": "Stolen"}},
		{http.MethodPost, cpath + "/revisions", map[string]any{}},
		{http.MethodPatch, rpath, map[string]any{"brief": map[string]any{"philosophy": "changed"}}},
		{http.MethodPost, rpath + "/nodes", map[string]any{"kind": "unit", "title": "No"}},
		{http.MethodDelete, rpath + "/nodes/" + node.ID, nil},
		{http.MethodPut, rpath + "/graph", map[string]any{"nodes": []any{}, "edges": []any{}}},
		{http.MethodPost, rpath + "/edges", map[string]any{"kind": "prerequisite", "fromNodeId": node.ID, "toNodeId": node.ID}},
		{http.MethodPost, rpath + "/comments", map[string]any{"nodeId": node.ID, "body": "No"}},
		{http.MethodPost, rpath + "/approval", map[string]any{"decision": "approved", "contentFingerprint": collabApproval(t, h, token[0], rev.ID).ContentFingerprint}},
		{http.MethodPost, rpath + "/publish", nil},
		{http.MethodPost, rpath + "/validate", nil},
		{http.MethodPost, rpath + "/materializations", map[string]any{"window": map[string]any{"availableMinutes": 30}}},
		{http.MethodPost, rpath + "/exports", map[string]any{"format": "markdown"}},
		{http.MethodPost, rpath + "/unit-library/lib_" + uuid.NewString(), nil},
		{http.MethodPost, cpath + "/shares", map[string]any{"targetWorkspaceId": ws[2].ID.String(), "permission": "read"}},
		{http.MethodDelete, cpath + "/shares/" + ws[1].ID.String(), nil},
	}
	for _, write := range writes {
		t.Run(write.method+write.path, func(t *testing.T) {
			response := doJSON(t, h, write.method, write.path, write.body, token[1])
			require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
		})
	}
	for _, path := range []string{rpath + "/comments", rpath + "/approval"} {
		response := doJSON(t, h, http.MethodGet, path, nil, token[1])
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	}
	unchanged := collabGraph(t, h, token[0], rev.ID)
	require.Len(t, unchanged.Nodes, 1)
	assert.Equal(t, "Private mastery", unchanged.Nodes[0].Title)
	revoke := doJSON(t, h, http.MethodDelete, cpath+"/shares/"+ws[1].ID.String(), nil, token[0])
	require.Equal(t, http.StatusNoContent, revoke.Code, revoke.Body.String())
	gone := doJSON(t, h, http.MethodGet, rpath+"/graph", nil, token[1])
	require.Equal(t, http.StatusNotFound, gone.Code, gone.Body.String())
	page = doJSON(t, h, http.MethodGet, "/studio/v1/workspaces/"+ws[1].ID.String()+"/curricula", nil, token[1])
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	require.NoError(t, json.Unmarshal(page.Body.Bytes(), &list))
	assert.Empty(t, list.Items)
	assert.Zero(t, list.TotalCount)
}

func TestP17ApprovalRequiresLocalReviewerAndDraft(t *testing.T) {
	t.Parallel()
	h, _, key, now := newWorkspaceSuite(t)
	db := testutil.DB(t)
	author, reviewer := uuid.New(), uuid.New()
	ws, other := factory.Workspace(t, db), factory.Workspace(t, db)
	factory.SeedMembership(t, db, ws.ID, domain.HumanSubjectRef(author), domain.MembershipRoleAuthor)
	factory.SeedMembership(t, db, other.ID, domain.HumanSubjectRef(author), domain.MembershipRoleReviewer)
	membership := factory.SeedMembership(t, db, ws.ID, domain.HumanSubjectRef(reviewer), domain.MembershipRoleReviewer)
	// The reviewer is an author elsewhere, which must not grant edit access here.
	factory.SeedMembership(t, db, other.ID, domain.HumanSubjectRef(reviewer), domain.MembershipRoleAuthor)
	a, r := mintHuman(t, key, now, author), mintHuman(t, key, now, reviewer)
	fixture := authorProjectViaAPI(t, h, a, ws.ID.String())
	path := "/studio/v1/revisions/" + fixture.RevisionID
	fp := collabApproval(t, h, r, fixture.RevisionID).ContentFingerprint
	denied := doJSON(t, h, http.MethodPost, path+"/approval", map[string]any{"decision": "approved", "contentFingerprint": fp}, a)
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	denied = doJSON(t, h, http.MethodPost, path+"/nodes", map[string]any{"kind": "unit", "title": "Wrong membership"}, r)
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	approved := doJSON(t, h, http.MethodPost, path+"/approval", map[string]any{"decision": "approved", "contentFingerprint": fp}, r)
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	published := doJSON(t, h, http.MethodPost, path+"/publish", nil, a)
	require.Equal(t, http.StatusOK, published.Code, published.Body.String())
	closed := doJSON(t, h, http.MethodPost, path+"/approval", map[string]any{"decision": "rejected", "contentFingerprint": fp}, r)
	require.Equal(t, http.StatusConflict, closed.Code, closed.Body.String())
	_, err := db.Exec(t.Context(), `UPDATE curriculum_studio.workspace_memberships SET status='revoked' WHERE id=$1`, membership.ID)
	require.NoError(t, err)
	denied = doJSON(t, h, http.MethodGet, path+"/approval", nil, r)
	require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
}
