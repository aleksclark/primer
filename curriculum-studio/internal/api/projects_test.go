package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestP16E1ProjectBlueprintPersistNamesAndRoles(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)

	cur, err := repo.NewCurriculumRepo(pool).Create(t.Context(), &domain.Curriculum{WorkspaceID: workspace.ID, Slug: "shop-" + uuid.NewString()[:8], Title: "Grade 6 integrated shop"})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(pool).Create(t.Context(), workspace.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 1, Title: "Draft"})
	require.NoError(t, err)
	revisionID := "prev_" + strings.ReplaceAll(rev.ID.String(), "-", "")

	project := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revisionID+"/nodes", map[string]any{
		"kind":  "project",
		"title": "Chicken coop",
		"body":  "Design and build a backyard coop spanning math and science.",
		"attributes": map[string]string{
			"code":       "P-COOP",
			"phases":     "design,build,present",
			"phaseNames": "Design|Build|Present",
		},
	}, token)
	require.Equal(t, http.StatusCreated, project.Code, project.Body.String())
	var projectNode struct {
		ID, Title  string
		Attributes map[string]string
	}
	require.NoError(t, json.Unmarshal(project.Body.Bytes(), &projectNode))
	assert.Equal(t, "Chicken coop", projectNode.Title)
	assert.Equal(t, "design,build,present", projectNode.Attributes["phases"])
	assert.Equal(t, "Design|Build|Present", projectNode.Attributes["phaseNames"])

	graphPut := doJSON(t, handler, http.MethodPut, "/studio/v1/revisions/"+revisionID+"/graph", map[string]any{
		"nodes": []map[string]any{
			{"kind": "outcome", "title": "Scale drawings", "attributes": map[string]string{"code": "MATH.6.G"}},
			{"kind": "outcome", "title": "Load paths", "attributes": map[string]string{"code": "SCI.6.PS"}},
			{"kind": "outcome", "title": "Cost estimate", "attributes": map[string]string{"code": "MATH.6.RP"}},
			{"kind": "project", "title": "Chicken coop", "body": "Design and build a backyard coop spanning math and science.", "attributes": map[string]string{
				"code": "P-COOP", "phases": "design,build,present", "phaseNames": "Design|Build|Present",
			}},
		},
		"edges": []map[string]any{
			{"kind": "parent_child", "fromNodeId": "3", "toNodeId": "0", "note": "target"},
			{"kind": "parent_child", "fromNodeId": "3", "toNodeId": "1", "note": "prior"},
			{"kind": "parent_child", "fromNodeId": "3", "toNodeId": "2", "note": "stretch"},
		},
	}, token)
	require.Equal(t, http.StatusOK, graphPut.Code, graphPut.Body.String())
	var graph struct {
		Nodes []struct {
			ID, Kind, Title string
			Attributes      map[string]string
		}
		Edges []struct {
			Kind, FromNodeID, ToNodeID, Note string
		}
	}
	require.NoError(t, json.Unmarshal(graphPut.Body.Bytes(), &graph))
	require.Len(t, graph.Nodes, 4)
	var persistedProject string
	for _, node := range graph.Nodes {
		if node.Kind == "project" {
			assert.Equal(t, "Chicken coop", node.Title)
			assert.Equal(t, "design,build,present", node.Attributes["phases"])
			persistedProject = node.ID
		}
	}
	require.NotEmpty(t, persistedProject)
	roles := map[string]string{}
	for _, edge := range graph.Edges {
		if edge.Kind == "parent_child" && edge.FromNodeID == persistedProject {
			roles[edge.Note] = edge.ToNodeID
		}
	}
	assert.NotEmpty(t, roles["target"])
	assert.NotEmpty(t, roles["prior"])
	assert.NotEmpty(t, roles["stretch"])

	projectUUID := decodePrefixed(t, persistedProject, "proj_")
	var title, phasesJSON string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT title, phases::text FROM curriculum_studio.projects WHERE id=$1`, projectUUID).Scan(&title, &phasesJSON))
	assert.Equal(t, "Chicken coop", title)
	assert.Contains(t, phasesJSON, `"id": "design"`)
	assert.Contains(t, phasesJSON, `"name": "Design"`)
	var roleCount int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.project_outcomes WHERE project_id=$1`, projectUUID).Scan(&roleCount))
	assert.Equal(t, 3, roleCount)
}

func TestP16E2PhaseMaterializationSnapshotAndItems(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, true)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	revisionID, projectID := seedPublishedProject(t, pool, workspace.ID, subject)

	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revisionID+"/materializations", map[string]any{
		"window":     map[string]any{"availableMinutes": 90},
		"attributes": map[string]string{"projectId": projectID, "projectPhaseId": "build"},
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var run struct{ ID string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &run))

	var snapshot []byte
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT input_snapshot FROM curriculum_studio.materialization_runs WHERE id=$1`, decodePrefixed(t, run.ID, "mat_")).Scan(&snapshot))
	var snap map[string]any
	require.NoError(t, json.Unmarshal(snapshot, &snap))
	assert.Equal(t, "build", snap["projectPhaseId"])
	assert.NotEmpty(t, snap["projectId"])

	items := doJSON(t, handler, http.MethodGet, "/studio/v1/materializations/"+run.ID+"/items", nil, token)
	require.Equal(t, http.StatusOK, items.Code, items.Body.String())
	var page struct {
		Items []struct {
			Kind, Title, Body string
		}
	}
	require.NoError(t, json.Unmarshal(items.Body.Bytes(), &page))
	require.NotEmpty(t, page.Items)
	foundTask, foundTool := false, false
	for _, item := range page.Items {
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(item.Body), &body))
		assert.Equal(t, "build", body["phaseId"])
		if item.Kind == "project_task" {
			foundTask = true
		}
		tools, _ := body["toolRequirements"].([]any)
		for _, tool := range tools {
			if tool == "tool: Circular saw" {
				foundTool = true
			}
		}
		assert.Equal(t, false, body["writesLmsMastery"])
	}
	assert.True(t, foundTask)
	assert.True(t, foundTool)
}

func TestP16E3OffScreenToolsAndPortfolioEvidence(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, key, now := newStubMaterializationHandler(t, true)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	revisionID, projectID := seedPublishedProject(t, pool, workspace.ID, subject)

	created := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revisionID+"/materializations", map[string]any{
		"window":     map[string]any{"availableMinutes": 40},
		"attributes": map[string]string{"projectId": projectID, "projectPhaseId": "build"},
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

func seedPublishedProject(t *testing.T, pool repo.Querier, workspaceID, subject uuid.UUID) (revisionID, projectID string) {
	t.Helper()
	ctx := t.Context()
	cur, err := repo.NewCurriculumRepo(pool).Create(ctx, &domain.Curriculum{WorkspaceID: workspaceID, Slug: "shop-" + uuid.NewString()[:8], Title: "Shop"})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(pool).Create(ctx, workspaceID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 1, Title: "Draft"})
	require.NoError(t, err)
	graphs := repo.NewPlanGraphRepo(pool)
	math, err := graphs.CreateOutcome(ctx, workspaceID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "MATH.6.G", Title: "Scale drawings"})
	require.NoError(t, err)
	science, err := graphs.CreateOutcome(ctx, workspaceID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "SCI.6.PS", Title: "Load paths"})
	require.NoError(t, err)
	phases, err := domain.EncodeProjectPhases([]domain.ProjectPhase{
		{ID: "design", Name: "Design", Position: 1},
		{ID: "build", Name: "Build", Position: 2, OffScreen: true, Activities: []domain.ProjectActivity{{Kind: domain.ActivityKindOffScreen, Title: "Cut the lumber"}}},
	})
	require.NoError(t, err)
	project, err := graphs.CreateProject(ctx, workspaceID, &domain.Project{PlanRevisionID: rev.ID, Code: "P-COOP", Title: "Chicken coop", Phases: phases})
	require.NoError(t, err)
	_, err = graphs.CreateProjectOutcome(ctx, workspaceID, &domain.ProjectOutcome{ProjectID: project.ID, OutcomeID: math.ID, Role: domain.ProjectOutcomeRoleTarget})
	require.NoError(t, err)
	_, err = graphs.CreateProjectOutcome(ctx, workspaceID, &domain.ProjectOutcome{ProjectID: project.ID, OutcomeID: science.ID, Role: domain.ProjectOutcomeRolePrior})
	require.NoError(t, err)
	_, err = graphs.CreateEvidenceRequirement(ctx, workspaceID, &domain.EvidenceRequirement{PlanRevisionID: rev.ID, OutcomeID: math.ID, Kind: "portfolio", Description: "photo essay"})
	require.NoError(t, err)
	ws, err := repo.NewWorkspaceRepo(pool).GetByID(ctx, workspaceID)
	require.NoError(t, err)
	tool := factory.Resource(t, pool, func(r *domain.Resource) {
		r.TenantID = ws.TenantID
		r.WorkspaceID = &workspaceID
		r.Kind = domain.ResourceKindTool
		r.Title = "Circular saw"
	})
	_, err = graphs.CreatePlanResource(ctx, workspaceID, &domain.PlanResource{PlanRevisionID: rev.ID, ResourceID: tool.ID, ProjectID: &project.ID, Role: "required"})
	require.NoError(t, err)
	require.NoError(t, repo.NewPlanRevisionRepo(pool).Publish(ctx, workspaceID, rev.ID, domain.HumanSubjectRef(subject)))
	return "prev_" + strings.ReplaceAll(rev.ID.String(), "-", ""), "proj_" + strings.ReplaceAll(project.ID.String(), "-", "")
}
