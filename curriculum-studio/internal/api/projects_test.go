package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/workflow"
)

func TestP16E1ProjectBlueprintPersistNamesAndRoles(t *testing.T) {
	t.Parallel()
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, testutil.DB(t))
	factory.SeedMembership(t, testutil.DB(t), workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	fixture := authorProjectViaAPI(t, handler, token, workspace.ID.String())

	got := doJSON(t, handler, http.MethodGet, "/studio/v1/revisions/"+fixture.RevisionID+"/graph", nil, token)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	graph := decodeGraph(t, got.Body.Bytes())
	project := graph.project()
	require.NotNil(t, project)
	assert.Equal(t, "Chicken coop", project.Title)
	phases := decodePhasesJSON(t, project.Attributes["phasesJSON"])
	require.Len(t, phases, 2)
	assert.Equal(t, "design", phases[0].ID)
	assert.Equal(t, "build", phases[1].ID)
	assert.True(t, phases[1].OffScreen)
	require.Len(t, phases[1].Activities, 1)
	assert.Equal(t, "Cut the lumber", phases[1].Activities[0].Title)
	roles := graph.rolesFor(project.ID)
	assert.Equal(t, fixture.MathID, roles["target"])
	assert.Equal(t, fixture.ScienceID, roles["prior"])
	assert.Equal(t, fixture.StretchID, roles["stretch"])
}

func TestP16GraphPhasesJSONRoundTripPreservesOffScreenActivity(t *testing.T) {
	t.Parallel()
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, testutil.DB(t))
	factory.SeedMembership(t, testutil.DB(t), workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	fixture := authorProjectViaAPI(t, handler, token, workspace.ID.String())

	got := doJSON(t, handler, http.MethodGet, "/studio/v1/revisions/"+fixture.RevisionID+"/graph", nil, token)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	var graph map[string]any
	require.NoError(t, json.Unmarshal(got.Body.Bytes(), &graph))
	before := decodePhasesJSON(t, graphProjectAttrs(t, graph)["phasesJSON"])

	replaced := doJSON(t, handler, http.MethodPut, "/studio/v1/revisions/"+fixture.RevisionID+"/graph", writeGraph(t, graph), token)
	require.Equal(t, http.StatusOK, replaced.Code, replaced.Body.String())
	require.NoError(t, json.Unmarshal(replaced.Body.Bytes(), &graph))
	after := decodePhasesJSON(t, graphProjectAttrs(t, graph)["phasesJSON"])
	require.Equal(t, before, after)
	require.True(t, after[1].OffScreen)
	require.Equal(t, "Cut the lumber", after[1].Activities[0].Title)
}

func TestP16E2PhaseMaterializationSnapshotAndItems(t *testing.T) {
	t.Parallel()
	handler, key, now := newStubMaterializationHandler(t, true)
	subject := uuid.New()
	workspace := factory.Workspace(t, testutil.DB(t))
	factory.SeedMembership(t, testutil.DB(t), workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	fixture := authorProjectViaAPI(t, handler, token, workspace.ID.String())
	publish := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/publish", nil, token)
	require.Equal(t, http.StatusOK, publish.Code, publish.Body.String())

	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/materializations", map[string]any{
		"window":     map[string]any{"availableMinutes": 90},
		"attributes": map[string]string{"projectId": fixture.ProjectID, "projectPhaseId": "build"},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var run struct{ ID, Status string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &run))
	assert.Equal(t, "ready", run.Status)

	var snapshot []byte
	require.NoError(t, testutil.DB(t).QueryRow(t.Context(), `SELECT input_snapshot FROM curriculum_studio.materialization_runs WHERE id=$1`, decodePrefixed(t, run.ID, "mat_")).Scan(&snapshot))
	var snap map[string]any
	require.NoError(t, json.Unmarshal(snapshot, &snap))
	assert.Equal(t, "build", snap["projectPhaseId"])
	assert.NotEmpty(t, snap["projectId"])

	items := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID+"/items", nil, token)
	require.Equal(t, http.StatusOK, items.Code, items.Body.String())
	page := decodeItems(t, items.Body.Bytes())
	require.NotEmpty(t, page)
	foundTask, foundTool := false, false
	for _, item := range page {
		body := decodeBody(t, item.Body)
		assert.Equal(t, "build", body["phaseId"])
		if item.Kind == "project_task" {
			foundTask = true
		}
		for _, tool := range asStrings(body["toolRequirements"]) {
			if tool == "tool: Circular saw" {
				foundTool = true
			}
		}
		assert.Equal(t, false, body["writesLmsMastery"])
	}
	assert.True(t, foundTask)
	assert.True(t, foundTool)
}

func TestP16E2NonStubPhaseRunStaysPhaseScoped(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, false)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	fixture := authorProjectViaAPI(t, handler, token, workspace.ID.String())
	publish := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/publish", nil, token)
	require.Equal(t, http.StatusOK, publish.Code, publish.Body.String())

	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/materializations", map[string]any{
		"window":     map[string]any{"availableMinutes": 45},
		"attributes": map[string]string{"projectId": fixture.ProjectID, "projectPhaseId": "build"},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var run struct{ ID, Status string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &run))
	assert.Equal(t, "ready", run.Status)

	ctx, cancel := contextWithTimeout(t, 2*time.Second)
	defer cancel()
	runner := workflow.NewRunner(pool, &workflow.Scripted{Fixture: "default-v1"})
	_, err := runner.RunOnce(ctx)
	require.NoError(t, err)

	got := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID, nil, token)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	var after struct{ Status string }
	require.NoError(t, json.Unmarshal(got.Body.Bytes(), &after))
	assert.Equal(t, "ready", after.Status)

	items := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID+"/items", nil, token)
	require.Equal(t, http.StatusOK, items.Code, items.Body.String())
	page := decodeItems(t, items.Body.Bytes())
	require.NotEmpty(t, page)
	for _, item := range page {
		body := decodeBody(t, item.Body)
		assert.Equal(t, "build", body["phaseId"])
		assert.NotEqual(t, "lesson", item.Kind)
		assert.NotEqual(t, "Scripted lesson", item.Title)
		assert.NotEqual(t, "assessment", item.Kind)
	}
	var requested int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.outbox_events WHERE aggregate_id=$1 AND event_type='materialization.requested'`, decodePrefixed(t, run.ID, "mat_")).Scan(&requested))
	assert.Equal(t, 0, requested)
}

func TestP16E3OffScreenToolsAndPortfolioEvidence(t *testing.T) {
	t.Parallel()
	handler, key, now := newStubMaterializationHandler(t, false)
	subject := uuid.New()
	workspace := factory.Workspace(t, testutil.DB(t))
	factory.SeedMembership(t, testutil.DB(t), workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	fixture := authorProjectViaAPI(t, handler, token, workspace.ID.String())
	publish := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/publish", nil, token)
	require.Equal(t, http.StatusOK, publish.Code, publish.Body.String())

	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/materializations", map[string]any{
		"window":     map[string]any{"availableMinutes": 40},
		"attributes": map[string]string{"projectId": fixture.ProjectID, "projectPhaseId": "build"},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var run struct{ ID string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &run))
	items := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID+"/items?q=kind:project_task", nil, token)
	require.Equal(t, http.StatusOK, items.Code, items.Body.String())
	assert.Contains(t, items.Body.String(), `"kind":"project_task"`)
	assert.Contains(t, items.Body.String(), "tool: Circular saw")
	assert.Contains(t, items.Body.String(), "portfolio: photo essay")
	assert.NotContains(t, items.Body.String(), "mastery_records")
}

func TestP16AuthoringJourneyCreatePublishMaterializePhase(t *testing.T) {
	t.Parallel()
	handler, key, now := newStubMaterializationHandler(t, false)
	subject := uuid.New()
	workspace := factory.Workspace(t, testutil.DB(t))
	factory.SeedMembership(t, testutil.DB(t), workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	fixture := authorProjectViaAPI(t, handler, token, workspace.ID.String())

	publish := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/publish", nil, token)
	require.Equal(t, http.StatusOK, publish.Code, publish.Body.String())
	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+fixture.RevisionID+"/materializations", map[string]any{
		"window":     map[string]any{"availableMinutes": 30},
		"attributes": map[string]string{"projectId": fixture.ProjectID, "projectPhaseId": "build"},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var run struct{ ID, Status string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &run))
	assert.Equal(t, "ready", run.Status)
	items := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID+"/items", nil, token)
	require.Equal(t, http.StatusOK, items.Code, items.Body.String())
	page := decodeItems(t, items.Body.Bytes())
	require.NotEmpty(t, page)
	foundTask := false
	for _, item := range page {
		body := decodeBody(t, item.Body)
		assert.Equal(t, "build", body["phaseId"])
		if item.Kind == "project_task" {
			foundTask = true
		}
	}
	assert.True(t, foundTask)
}

type authoredProject struct {
	RevisionID, ProjectID, MathID, ScienceID, StretchID string
}

func authorProjectViaAPI(t *testing.T, handler http.Handler, token, workspaceID string) authoredProject {
	t.Helper()
	created := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces/"+workspaceID+"/curricula", map[string]any{
		"name": "Grade 6 integrated shop", "template": "custom",
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var curriculum struct{ ID string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &curriculum))

	rev := doJSON(t, handler, http.MethodPost, "/studio/v1/curricula/"+curriculum.ID+"/revisions", map[string]any{}, token)
	require.Equal(t, http.StatusCreated, rev.Code, rev.Body.String())
	var revision struct{ ID string }
	require.NoError(t, json.Unmarshal(rev.Body.Bytes(), &revision))

	catalog := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces/"+workspaceID+"/standards-catalogs", map[string]any{
		"source": "custom", "title": "Shop standards",
		"standards": []map[string]any{
			{"code": "MATH.6.G", "source": "custom", "description": "Scale drawings"},
			{"code": "SCI.6.PS", "source": "custom", "description": "Load paths"},
			{"code": "MATH.6.RP", "source": "custom", "description": "Ratios"},
		},
	}, token)
	require.Equal(t, http.StatusCreated, catalog.Code, catalog.Body.String())

	math := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/nodes", map[string]any{
		"kind": "outcome", "title": "Scale drawings", "standardCodes": []string{"MATH.6.G"}, "attributes": map[string]string{"code": "MATH.6.G", "evidenceKind": "portfolio", "evidenceDescription": "photo essay"},
	}, token)
	require.Equal(t, http.StatusCreated, math.Code, math.Body.String())
	science := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/nodes", map[string]any{
		"kind": "outcome", "title": "Load paths", "standardCodes": []string{"SCI.6.PS"}, "attributes": map[string]string{"code": "SCI.6.PS"},
	}, token)
	require.Equal(t, http.StatusCreated, science.Code, science.Body.String())
	stretch := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/nodes", map[string]any{
		"kind": "outcome", "title": "Cost estimate", "standardCodes": []string{"MATH.6.RP"}, "attributes": map[string]string{"code": "MATH.6.RP"},
	}, token)
	require.Equal(t, http.StatusCreated, stretch.Code, stretch.Body.String())

	phasesJSON, err := json.Marshal([]map[string]any{
		{"id": "design", "name": "Design", "position": 1},
		{"id": "build", "name": "Build", "position": 2, "offScreen": true, "activities": []map[string]any{{"kind": "off_screen", "title": "Cut the lumber"}}},
	})
	require.NoError(t, err)
	project := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/nodes", map[string]any{
		"kind": "project", "title": "Chicken coop", "body": "Design and build a backyard coop spanning math and science.",
		"attributes": map[string]string{"code": "P-COOP", "phasesJSON": string(phasesJSON)},
	}, token)
	require.Equal(t, http.StatusCreated, project.Code, project.Body.String())

	var mathNode, scienceNode, stretchNode, projectNode struct{ ID string }
	require.NoError(t, json.Unmarshal(math.Body.Bytes(), &mathNode))
	require.NoError(t, json.Unmarshal(science.Body.Bytes(), &scienceNode))
	require.NoError(t, json.Unmarshal(stretch.Body.Bytes(), &stretchNode))
	require.NoError(t, json.Unmarshal(project.Body.Bytes(), &projectNode))

	for _, edge := range []map[string]any{
		{"kind": "parent_child", "fromNodeId": projectNode.ID, "toNodeId": mathNode.ID, "note": "target"},
		{"kind": "parent_child", "fromNodeId": projectNode.ID, "toNodeId": scienceNode.ID, "note": "prior"},
		{"kind": "parent_child", "fromNodeId": projectNode.ID, "toNodeId": stretchNode.ID, "note": "stretch"},
	} {
		createdEdge := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/edges", edge, token)
		require.Equal(t, http.StatusCreated, createdEdge.Code, createdEdge.Body.String())
	}

	tool := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces/"+workspaceID+"/resources", map[string]any{
		"kind": "tool", "title": "Circular saw",
	}, token)
	require.Equal(t, http.StatusCreated, tool.Code, tool.Body.String())
	var toolNode struct{ ID string }
	require.NoError(t, json.Unmarshal(tool.Body.Bytes(), &toolNode))
	attached := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/edges", map[string]any{
		"kind": "uses_resource", "fromNodeId": projectNode.ID, "toNodeId": toolNode.ID, "note": "required",
	}, token)
	require.Equal(t, http.StatusCreated, attached.Code, attached.Body.String())

	return authoredProject{RevisionID: revision.ID, ProjectID: projectNode.ID, MathID: mathNode.ID, ScienceID: scienceNode.ID, StretchID: stretchNode.ID}
}

type graphView struct {
	Nodes []struct {
		ID, Kind, Title string
		Attributes      map[string]string
	}
	Edges []struct {
		Kind, FromNodeID, ToNodeID, Note string
	}
}

func decodeGraph(t *testing.T, raw []byte) graphView {
	t.Helper()
	var g graphView
	require.NoError(t, json.Unmarshal(raw, &g))
	return g
}

func (g graphView) project() *struct {
	ID, Kind, Title string
	Attributes      map[string]string
} {
	for i := range g.Nodes {
		if g.Nodes[i].Kind == "project" {
			return &g.Nodes[i]
		}
	}
	return nil
}

func (g graphView) rolesFor(projectID string) map[string]string {
	out := map[string]string{}
	for _, edge := range g.Edges {
		if edge.Kind == "parent_child" && edge.FromNodeID == projectID {
			out[edge.Note] = edge.ToNodeID
		}
	}
	return out
}

func decodePhasesJSON(t *testing.T, raw string) []domain.ProjectPhase {
	t.Helper()
	phases, err := domain.ParseProjectPhases(json.RawMessage(raw))
	require.NoError(t, err)
	return phases
}

func writeGraph(t *testing.T, graph map[string]any) map[string]any {
	t.Helper()
	nodes := []map[string]any{}
	for _, raw := range graph["nodes"].([]any) {
		node := raw.(map[string]any)
		item := map[string]any{"kind": node["kind"], "title": node["title"]}
		if body, ok := node["body"]; ok {
			item["body"] = body
		}
		if attrs, ok := node["attributes"]; ok {
			item["attributes"] = attrs
		}
		nodes = append(nodes, item)
	}
	index := map[string]string{}
	for i, raw := range graph["nodes"].([]any) {
		node := raw.(map[string]any)
		index[node["id"].(string)] = strconv.Itoa(i)
	}
	edges := []map[string]any{}
	for _, raw := range graph["edges"].([]any) {
		edge := raw.(map[string]any)
		if edge["kind"] == "uses_resource" {
			continue
		}
		edges = append(edges, map[string]any{
			"kind":       edge["kind"],
			"fromNodeId": index[edge["fromNodeId"].(string)],
			"toNodeId":   index[edge["toNodeId"].(string)],
			"note":       edge["note"],
		})
	}
	return map[string]any{"nodes": nodes, "edges": edges}
}

func graphProjectAttrs(t *testing.T, graph map[string]any) map[string]string {
	t.Helper()
	nodes, _ := graph["nodes"].([]any)
	for _, raw := range nodes {
		node, _ := raw.(map[string]any)
		if node["kind"] == "project" {
			attrs, _ := node["attributes"].(map[string]any)
			out := map[string]string{}
			for k, v := range attrs {
				out[k] = v.(string)
			}
			return out
		}
	}
	t.Fatal("project node missing")
	return nil
}

type itemView struct {
	Kind, Title, Body string
}

func decodeItems(t *testing.T, raw []byte) []itemView {
	t.Helper()
	var page struct{ Items []itemView }
	require.NoError(t, json.Unmarshal(raw, &page))
	return page.Items
}

func decodeBody(t *testing.T, raw string) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	return body
}

func asStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func contextWithTimeout(t *testing.T, d time.Duration) (context.Context, func()) {
	t.Helper()
	return context.WithTimeout(t.Context(), d)
}
