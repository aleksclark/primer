package repo_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func planFixture(t *testing.T, q repo.Querier) (*domain.Workspace, *domain.Curriculum, *domain.PlanRevision) {
	t.Helper()
	ws := factory.Workspace(t, q)
	cur, err := repo.NewCurriculumRepo(q).Create(context.Background(), &domain.Curriculum{WorkspaceID: ws.ID, Slug: "math-" + uuid.NewString()[:8], Title: "Math plan"})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(q).Create(context.Background(), ws.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 1, Title: "Draft"})
	require.NoError(t, err)
	return ws, cur, rev
}

func TestP5E1DraftGraphRoundTrip(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	graphs := repo.NewPlanGraphRepo(tx)
	obj, err := graphs.CreateObjective(ctx, ws.ID, &domain.Objective{PlanRevisionID: rev.ID, Code: "O1", Title: "Numbers"})
	require.NoError(t, err)
	out, err := graphs.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, ObjectiveID: &obj.ID, Code: "O1.1", Title: "Add fractions", MasteryCriteria: "Accurate"})
	require.NoError(t, err)
	other, err := graphs.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "O1.2", Title: "Compare fractions"})
	require.NoError(t, err)
	_, err = graphs.CreateOutcomePrerequisite(ctx, ws.ID, &domain.OutcomePrerequisite{PlanRevisionID: rev.ID, OutcomeID: other.ID, PrerequisiteID: out.ID, Requirement: "completed"})
	require.NoError(t, err)
	fw := factory.Framework(t, tx)
	standard := factory.CatalogStandard(t, tx, func(s *domain.CatalogStandard) { s.FrameworkID = fw.ID; s.Code = "6.NS" })
	_, err = graphs.CreateOutcomeStandardMapping(ctx, ws.ID, &domain.OutcomeStandardMapping{OutcomeID: out.ID, StandardID: standard.ID, Alignment: "addresses"})
	require.NoError(t, err)
	_, err = graphs.CreateEvidenceRequirement(ctx, ws.ID, &domain.EvidenceRequirement{PlanRevisionID: rev.ID, OutcomeID: out.ID, Kind: "formal", Description: "show work"})
	require.NoError(t, err)
	_, err = graphs.CreateSchedulingConstraint(ctx, ws.ID, &domain.SchedulingConstraint{PlanRevisionID: rev.ID, Kind: "available_minutes"})
	require.NoError(t, err)
	arc, err := graphs.CreateLearningArc(ctx, ws.ID, &domain.LearningArc{PlanRevisionID: rev.ID, Code: "A1", Title: "Foundations"})
	require.NoError(t, err)
	unit, err := graphs.CreateUnit(ctx, ws.ID, &domain.Unit{PlanRevisionID: rev.ID, LearningArcID: &arc.ID, Code: "U1", Title: "Fractions"})
	require.NoError(t, err)
	project, err := graphs.CreateProject(ctx, ws.ID, &domain.Project{PlanRevisionID: rev.ID, UnitID: &unit.ID, Code: "P1", Title: "Fraction kitchen"})
	require.NoError(t, err)
	_, err = graphs.CreateUnitOutcome(ctx, ws.ID, &domain.UnitOutcome{UnitID: unit.ID, OutcomeID: out.ID, Role: "target"})
	require.NoError(t, err)
	_, err = graphs.CreateProjectOutcome(ctx, ws.ID, &domain.ProjectOutcome{ProjectID: project.ID, OutcomeID: other.ID, Role: "target"})
	require.NoError(t, err)
	resource := factory.Resource(t, tx, func(r *domain.Resource) { r.TenantID = ws.TenantID; r.WorkspaceID = &ws.ID; r.Title = "Fraction book" })
	_, err = graphs.CreatePlanResource(ctx, ws.ID, &domain.PlanResource{PlanRevisionID: rev.ID, ResourceID: resource.ID, UnitID: &unit.ID, ProjectID: &project.ID, Role: "required"})
	require.NoError(t, err)
	got, err := graphs.Load(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	require.Equal(t, rev.ID, got.Revision.ID)
	require.Len(t, got.Objectives, 1)
	require.Len(t, got.Outcomes, 2)
	require.Len(t, got.OutcomePrerequisites, 1)
	require.Len(t, got.LearningArcs, 1)
	require.Len(t, got.Units, 1)
}

func TestP5E2PublishLocksRevisionAndChildren(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	_, err := repo.NewPlanGraphRepo(tx).CreateObjective(ctx, ws.ID, &domain.Objective{PlanRevisionID: rev.ID, Code: "O1", Title: "Objective"})
	require.NoError(t, err)
	r := repo.NewPlanRevisionRepo(tx)
	require.NoError(t, r.Publish(ctx, ws.ID, rev.ID, "identity:"+uuid.NewString()))
	got, err := r.Get(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	require.Equal(t, "published", got.Status)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = sp.Exec(ctx, `UPDATE curriculum_studio.plan_revisions SET title='changed' WHERE id=$1`, rev.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrImmutable)
	_, err = sp.Exec(ctx, `INSERT INTO curriculum_studio.objectives(plan_revision_id,code,title) VALUES($1,'O2','blocked')`, rev.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrImmutable)
}

func TestP5E3SupersedeOnlyPublishedAndPointer(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, cur, first := planFixture(t, tx)
	r := repo.NewPlanRevisionRepo(tx)
	require.NoError(t, r.Publish(ctx, ws.ID, first.ID, "identity:"+uuid.NewString()))
	second, err := r.Create(ctx, ws.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 2, Title: "Second"})
	require.NoError(t, err)
	require.NoError(t, r.Publish(ctx, ws.ID, second.ID, "identity:"+uuid.NewString()))
	old, err := r.Get(ctx, ws.ID, first.ID)
	require.NoError(t, err)
	require.Equal(t, "superseded", old.Status)
	current, err := repo.NewCurriculumRepo(tx).Get(ctx, ws.ID, cur.ID)
	require.NoError(t, err)
	require.Equal(t, second.ID, *current.PublishedRevisionID)
	require.NoError(t, r.Supersede(ctx, ws.ID, second.ID))
	require.ErrorIs(t, r.Supersede(ctx, ws.ID, second.ID), repo.ErrInvalidTransition)
}

func TestP5E4ConcurrentOutcomeCycleRejected(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	ws, _, rev := planFixture(t, pool)
	graphs := repo.NewPlanGraphRepo(pool)
	a, err := graphs.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "A", Title: "A"})
	require.NoError(t, err)
	b, err := graphs.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "B", Title: "B"})
	require.NoError(t, err)
	pairs := [][2]uuid.UUID{{a.ID, b.ID}, {b.ID, a.ID}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, pair := range pairs {
		pair := pair
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.WithTx(ctx, pool, func(q repo.Querier) error {
				_, err := q.Exec(ctx, `INSERT INTO curriculum_studio.outcome_prerequisites(plan_revision_id,outcome_id,prerequisite_id) VALUES($1,$2,$3)`, rev.ID, pair[0], pair[1])
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	var ok, cycles int
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		require.True(t, errors.Is(repo.MapError(err), repo.ErrPrerequisiteCycle), "unexpected error: %v", err)
		cycles++
	}
	require.Equal(t, 1, ok)
	require.Equal(t, 1, cycles)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.outcome_prerequisites WHERE plan_revision_id=$1`, rev.ID).Scan(&count))
	require.Equal(t, 1, count)
}

func TestP5E5CrossRevisionPrerequisiteRejected(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, cur, rev := planFixture(t, tx)
	r := repo.NewPlanRevisionRepo(tx)
	otherRev, err := r.Create(ctx, ws.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 2, Title: "Other"})
	require.NoError(t, err)
	g := repo.NewPlanGraphRepo(tx)
	a, err := g.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "A", Title: "A"})
	require.NoError(t, err)
	b, err := g.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: otherRev.ID, Code: "B", Title: "B"})
	require.NoError(t, err)
	_, err = g.CreateOutcomePrerequisite(ctx, ws.ID, &domain.OutcomePrerequisite{PlanRevisionID: rev.ID, OutcomeID: a.ID, PrerequisiteID: b.ID})
	require.ErrorIs(t, err, repo.ErrNotFound)
}
