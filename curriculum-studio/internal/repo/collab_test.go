package repo_test

import (
	"encoding/json"
	"testing"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestP17LibraryCopiesProjectRelationshipsAndRollsBack(t *testing.T) {
	// A real pool is important here: failure must roll back all inserts, rather
	// than relying on the test harness's cleanup transaction to hide partial work.
	t.Parallel()
	q := testutil.DB(t)
	ctx := t.Context()
	ws, cur, rev := planFixture(t, q)
	g := repo.NewPlanGraphRepo(q)
	arc, err := g.CreateArc(ctx, ws.ID, &domain.LearningArc{PlanRevisionID: rev.ID, Code: "arc", Title: "Workshop arc"})
	require.NoError(t, err)
	objective, err := g.CreateObjective(ctx, ws.ID, &domain.Objective{PlanRevisionID: rev.ID, Code: "objective", Title: "Build precisely"})
	require.NoError(t, err)
	outcome, err := g.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, ObjectiveID: &objective.ID, Code: "scale", Title: "Scale drawings"})
	require.NoError(t, err)
	unit, err := g.CreateUnit(ctx, ws.ID, &domain.Unit{PlanRevisionID: rev.ID, LearningArcID: &arc.ID, Code: "unit", Title: "Geometry workshop", EssentialQuestions: []string{"How does scale work?"}})
	require.NoError(t, err)
	project, err := g.CreateProject(ctx, ws.ID, &domain.Project{PlanRevisionID: rev.ID, UnitID: &unit.ID, Code: "project", Title: "Chicken coop", Phases: json.RawMessage(`[{"id":"build","name":"Build","offScreen":true}]`)})
	require.NoError(t, err)
	_, err = g.CreateProjectOutcome(ctx, ws.ID, &domain.ProjectOutcome{ProjectID: project.ID, OutcomeID: outcome.ID, Role: "stretch"})
	require.NoError(t, err)
	resource := factory.Resource(t, q, func(r *domain.Resource) { r.WorkspaceID = &ws.ID; r.TenantID = ws.TenantID; r.Title = "Ruler" })
	_, err = g.CreatePlanResource(ctx, ws.ID, &domain.PlanResource{PlanRevisionID: rev.ID, UnitID: &unit.ID, ProjectID: &project.ID, ResourceID: resource.ID, Role: "required"})
	require.NoError(t, err)
	library := repo.NewUnitLibraryRepo(q)
	saved, err := library.SaveUnit(ctx, ws.ID, rev.ID, unit.ID, "")
	require.NoError(t, err)
	destination, err := repo.NewPlanRevisionRepo(q).Create(ctx, ws.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 2, Title: "New draft"})
	require.NoError(t, err)
	copied, err := library.CopyIntoRevision(ctx, ws.ID, destination.ID, saved.ID)
	require.NoError(t, err)
	graph, err := g.Load(ctx, ws.ID, destination.ID)
	require.NoError(t, err)
	require.Len(t, graph.Units, 1)
	require.Len(t, graph.Projects, 1)
	require.Len(t, graph.Outcomes, 1)
	require.Len(t, graph.Objectives, 1)
	require.Len(t, graph.LearningArcs, 1)
	assert.Equal(t, "Geometry workshop", copied.Title)
	assert.Equal(t, unit.EssentialQuestions, copied.EssentialQuestions)
	assert.Equal(t, "Chicken coop", graph.Projects[0].Title)
	assert.Equal(t, copied.ID, *graph.Projects[0].UnitID)
	assert.Equal(t, "Workshop arc", graph.LearningArcs[0].Title)
	assert.Equal(t, graph.LearningArcs[0].ID, *copied.LearningArcID)
	assert.Equal(t, "Build precisely", graph.Objectives[0].Title)
	assert.Equal(t, graph.Objectives[0].ID, *graph.Outcomes[0].ObjectiveID)
	require.Len(t, graph.ProjectOutcomes, 1)
	assert.Equal(t, graph.Outcomes[0].ID, graph.ProjectOutcomes[0].OutcomeID)
	assert.Equal(t, "stretch", graph.ProjectOutcomes[0].Role)
	require.Len(t, graph.PlanResources, 1)
	assert.Equal(t, resource.ID, graph.PlanResources[0].ResourceID)
	assert.Equal(t, graph.Projects[0].ID, *graph.PlanResources[0].ProjectID)
	assert.NotEqual(t, project.ID, graph.Projects[0].ID)
	// Retiring a resource after saving a library entry is a real missing-dependency
	// failure. The failed import must not leave newly inserted graph rows behind.
	_, err = q.Exec(ctx, `DELETE FROM curriculum_studio.plan_resources WHERE resource_id=$1`, resource.ID)
	require.NoError(t, err)
	_, err = q.Exec(ctx, `DELETE FROM curriculum_studio.resources WHERE id=$1`, resource.ID)
	require.NoError(t, err)
	before, err := g.Load(ctx, ws.ID, destination.ID)
	require.NoError(t, err)
	_, err = library.CopyIntoRevision(ctx, ws.ID, destination.ID, saved.ID)
	require.Error(t, err)
	after, err := g.Load(ctx, ws.ID, destination.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestP17ApprovalRepoRejectsForgedStaleAndRevokedDecisions(t *testing.T) {
	t.Parallel()
	q := testutil.DB(t)
	ctx := t.Context()
	ws, _, rev := planFixture(t, q)
	a := repo.NewApprovalRepo(q)
	subject := domain.HumanSubjectRef(uuid.New())
	membership := factory.SeedMembership(t, q, ws.ID, subject, domain.MembershipRoleAuthor)
	fp, err := a.Fingerprint(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	_, err = a.Decide(ctx, ws.ID, rev.ID, subject, "approved", fp)
	require.ErrorIs(t, err, repo.ErrConflict)
	_, err = q.Exec(ctx, `UPDATE curriculum_studio.workspace_memberships SET role='reviewer',display_name='Actual reviewer' WHERE id=$1`, membership.ID)
	require.NoError(t, err)
	_, err = a.Create(ctx, &domain.PlanApproval{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, ReviewerSubjectRef: subject, ReviewerDisplayName: "Forged name", Status: "approved", ContentFingerprint: fp})
	require.NoError(t, err)
	current, err := a.Current(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, "Actual reviewer", current.ReviewerDisplayName)
	_, err = repo.NewPlanGraphRepo(q).CreateUnit(ctx, ws.ID, &domain.Unit{PlanRevisionID: rev.ID, Code: "new", Title: "Changed content"})
	require.NoError(t, err)
	current, err = a.Current(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	assert.Nil(t, current)
	_, err = a.Decide(ctx, ws.ID, rev.ID, subject, "approved", fp)
	require.ErrorIs(t, err, repo.ErrConflict)
	fp, err = a.Fingerprint(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	_, err = q.Exec(ctx, `UPDATE curriculum_studio.workspace_memberships SET status='revoked' WHERE id=$1`, membership.ID)
	require.NoError(t, err)
	_, err = a.Decide(ctx, ws.ID, rev.ID, subject, "approved", fp)
	require.ErrorIs(t, err, repo.ErrConflict)
	foreign := factory.Workspace(t, q)
	_, err = a.Fingerprint(ctx, foreign.ID, rev.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}
