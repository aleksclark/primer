package overseer_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/overseer"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func publishMiniCourse(t *testing.T, q repo.Querier) (*repo.PublishCourseResult, string, string) {
	t.Helper()
	ctx := context.Background()
	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	// Publish two activities and a tiny course graph: a1 -> a2.
	for _, slug := range []string{"basic-navigation", "file-organization"} {
		doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", slug, "activity.yaml"))
		require.NoError(t, err)
		_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
		require.NoError(t, err)
	}

	course := &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "mini-linux",
		Title:         "Mini Linux",
		SubjectCode:   "digital-literacy",
		Version:       "1",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation"},
			{Order: 2, Slug: "file-organization"},
		},
		Prerequisites: []contracts.CoursePrerequisite{
			{
				Activity:    "file-organization",
				Requires:    []string{"basic-navigation"},
				Requirement: contracts.PrereqCompleted,
			},
		},
		Gates: []contracts.CourseGate{
			{
				Activity:    "file-organization",
				Kind:        contracts.GateParentReview,
				Description: "Parent checks explanation before advancing",
			},
		},
	}
	res, err := repo.PublishCourseDocument(ctx, q, course, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, res.Revision)
	return res, "basic-navigation", "file-organization"
}

func TestCourseEligibilityAndAssignNextOrder(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, second := publishMiniCourse(t, q)

	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 10)
	require.NoError(t, err)
	require.Equal(t, "active", en.Status)
	require.NotNil(t, en.CurriculumRevisionID)

	prev, err := overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	require.Len(t, prev.Activities, 2)
	require.Len(t, prev.Eligible, 1)
	assert.Equal(t, first, prev.Eligible[0].Membership.ActivitySlug)
	assert.Equal(t, "blocked", prev.Activities[1].Status)
	assert.Equal(t, overseer.BlockPrerequisiteUnmet, prev.Activities[1].BlockingReasons[0].Code)

	// AssignNext should pick lesson 1, not alphabetical global library (command-typing would sort earlier if published).
	// Publish an extra activity that would win alphabetical global order.
	root := repoRoot(t)
	extra, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, extra, time.Now().UTC())
	require.NoError(t, err)

	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.NotNil(t, res.Assignment)
	assert.Contains(t, res.Reason, "course:"+first)
	assert.NotNil(t, res.Assignment.EnrollmentID)
	assert.Equal(t, en.ID, *res.Assignment.EnrollmentID)

	// Complete first activity.
	_, err = repo.StudentAssignments.Update(ctx, q, res.Assignment.ID, map[string]any{"state": "completed"})
	require.NoError(t, err)

	// Second becomes eligible.
	prev, err = overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	require.Len(t, prev.Eligible, 1)
	assert.Equal(t, second, prev.Eligible[0].Membership.ActivitySlug)

	res2, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	require.True(t, res2.Created)
	assert.Contains(t, res2.Reason, "course:"+second)
}

func TestAssignNextRespectsPauseAndPin(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, second := publishMiniCourse(t, q)

	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	// Pause blocks new assignments.
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "paused", nil, "break")
	require.NoError(t, err)
	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.NotEmpty(t, res.BlockReason)

	// Resume and pin second activity with override via pin force.
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "active", nil, "")
	require.NoError(t, err)
	_, err = repo.PinEnrollmentActivity(ctx, q, en.ID, second, "skip ahead", nil)
	require.NoError(t, err)

	res, err = overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	require.True(t, res.Created)
	assert.Contains(t, res.Reason, "pin:"+second)
	_ = first
}

func TestEnrollmentRevisionMigrationDeterministic(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()

	// Pre-phase4 style curriculum + enrollment (no explicit revision on create).
	// Migration creates revision 1 for curricula; factory curricula get slug.
	curr := factory.Curriculum(t, q)
	// Ensure a revision exists (migration does this for rows present at migrate;
	// factory-created curricula after migrate need an explicit default revision).
	rev, err := repo.CurriculumRevisions.Create(ctx, q, map[string]any{
		"curriculum_id":   curr.ID,
		"revision":        1,
		"title":           curr.Name,
		"description":     curr.Description,
		"subject_code":    curr.SubjectCode,
		"version":         "1",
		"revision_policy": "latest_published",
		"document":        map[string]any{"migrated": true},
		"published_at":    time.Now().UTC(),
	})
	require.NoError(t, err)

	student := factory.Student(t, q)
	en := factory.Enrollment(t, q, factory.Override{
		"student_id":             student.ID,
		"curriculum_id":          curr.ID,
		"curriculum_revision_id": rev.ID,
	})
	require.NotNil(t, en.CurriculumRevisionID)
	assert.Equal(t, rev.ID, *en.CurriculumRevisionID)

	// Publishing a new course revision must not mutate the active enrollment pointer.
	root := repoRoot(t)
	_, err = curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	course := &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          curr.Slug,
		Title:         curr.Name + " v2",
		SubjectCode:   "digital-literacy",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation"},
		},
	}
	pub, err := repo.PublishCourseDocument(ctx, q, course, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, 2, pub.Revision.Revision)

	en2, err := repo.Enrollments.Get(ctx, q, en.ID)
	require.NoError(t, err)
	require.NotNil(t, en2.CurriculumRevisionID)
	assert.Equal(t, rev.ID, *en2.CurriculumRevisionID, "active enrollment must stay on original revision")
}

func TestGlobalLibraryFallbackOnlyWithoutCourse(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	for _, slug := range []string{"basic-navigation", "command-typing-basics"} {
		doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", slug, "activity.yaml"))
		require.NoError(t, err)
		_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
		require.NoError(t, err)
	}

	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	require.True(t, res.Created)
	assert.Contains(t, res.Reason, "library:")
}
