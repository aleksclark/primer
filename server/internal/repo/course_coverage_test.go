package repo_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func publishTwoActivitiesCourse(t *testing.T, q repo.Querier, slug string) *repo.PublishCourseResult {
	t.Helper()
	ctx := context.Background()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	for _, actSlug := range []string{"basic-navigation", "file-organization"} {
		doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", actSlug, "activity.yaml"))
		require.NoError(t, err)
		_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
		require.NoError(t, err)
	}
	pub, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion:     "1",
		Slug:              slug,
		Title:             "Coverage Course",
		SubjectCode:       "digital-literacy",
		ParentDescription: "desc",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation", Metadata: map[string]string{"unit": "a"}},
			{Order: 2, Slug: "file-organization", Capstone: true, Continuity: &contracts.ContinuityPolicy{Mode: contracts.ContinuityFresh}},
		},
		Prerequisites: []contracts.CoursePrerequisite{{
			Activity: "file-organization", Requires: []string{"basic-navigation"}, Requirement: "completed",
		}},
		Gates: []contracts.CourseGate{{
			Activity: "file-organization", Kind: "parent_review", Description: "review",
		}},
		Remediation: []contracts.CourseRemediation{{
			ForActivity: "basic-navigation", BranchSlug: "file-organization", Kind: "remediation", Description: "retry",
		}},
	}, time.Now().UTC())
	require.NoError(t, err)
	return pub
}

func TestCurriculumLookupRemediationAndOpenAssignment(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	pub := publishTwoActivitiesCourse(t, q, "cov-lookup-course")

	got, err := repo.GetCurriculumBySlug(ctx, q, pub.Curriculum.Slug)
	require.NoError(t, err)
	assert.Equal(t, pub.Curriculum.ID, got.ID)

	_, err = repo.GetCurriculumBySlug(ctx, q, "no-such-curriculum-slug")
	require.ErrorIs(t, err, repo.ErrNotFound)

	latest, err := repo.LatestCurriculumRevision(ctx, q, pub.Curriculum.ID)
	require.NoError(t, err)
	assert.Equal(t, pub.Revision.ID, latest.ID)
	assert.Equal(t, pub.Revision.Revision, latest.Revision)

	// Second publish bumps revision; latest must follow.
	pub2, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          pub.Curriculum.Slug,
		Title:         "Coverage Course v2",
		SubjectCode:   "digital-literacy",
		Activities:    []contracts.CourseActivityRef{{Order: 1, Slug: "basic-navigation"}},
	}, time.Now().UTC())
	require.NoError(t, err)
	latest2, err := repo.LatestCurriculumRevision(ctx, q, pub.Curriculum.ID)
	require.NoError(t, err)
	assert.Equal(t, pub2.Revision.ID, latest2.ID)
	assert.Greater(t, latest2.Revision, latest.Revision)

	_, err = repo.LatestCurriculumRevision(ctx, q, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	// Remediations from first revision only.
	rems, err := repo.ListCurriculumRemediations(ctx, q, pub.Revision.ID)
	require.NoError(t, err)
	require.Len(t, rems, 1)
	assert.Equal(t, "basic-navigation", rems[0].ForActivitySlug)
	assert.Equal(t, "file-organization", rems[0].BranchSlug)

	remsEmpty, err := repo.ListCurriculumRemediations(ctx, q, pub2.Revision.ID)
	require.NoError(t, err)
	assert.Empty(t, remsEmpty)

	gates, err := repo.ListCurriculumGates(ctx, q, pub.Revision.ID)
	require.NoError(t, err)
	require.Len(t, gates, 1)

	student := factory.Student(t, q)
	// Resolve activity revision for open-assignment check.
	page, err := repo.LearningActivities.List(ctx, q, repo.ListParams{Limit: 1, Filters: map[string]any{"slug": "basic-navigation"}})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalCount)
	revPage, err := repo.LearningActivityRevisions.List(ctx, q, repo.ListParams{
		Limit: 1, Sort: "revision", Dir: repo.SortDesc,
		Filters: map[string]any{"activity_id": page.Items[0].ID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, revPage.Items)
	revID := revPage.Items[0].ID

	open, err := repo.HasOpenAssignmentForRevision(ctx, q, student.ID, revID)
	require.NoError(t, err)
	assert.False(t, open)

	asg, err := repo.CreateAssignment(ctx, q, student.ID, revID, nil, 1, "open-check")
	require.NoError(t, err)
	open, err = repo.HasOpenAssignmentForRevision(ctx, q, student.ID, revID)
	require.NoError(t, err)
	assert.True(t, open)

	// Completed assignment no longer counts as open.
	_, err = repo.StudentAssignments.Update(ctx, q, asg.ID, map[string]any{"state": domain.AssignmentCompleted})
	require.NoError(t, err)
	open, err = repo.HasOpenAssignmentForRevision(ctx, q, student.ID, revID)
	require.NoError(t, err)
	assert.False(t, open)
}

func TestEnrollStudentResumeAndValidationEdges(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	pub := publishTwoActivitiesCourse(t, q, "cov-enroll-course")
	student := factory.Student(t, q)
	ed := factory.Educator(t, q)
	edID := ed.ID

	// Missing student.
	_, err := repo.EnrollStudentInCurriculumRevision(ctx, q, uuid.NewString(), pub.Curriculum.ID, pub.Revision.ID, &edID, 1)
	require.Error(t, err)

	// Missing curriculum.
	_, err = repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, uuid.NewString(), pub.Revision.ID, &edID, 1)
	require.Error(t, err)

	// Revision/curriculum mismatch.
	other := publishTwoActivitiesCourse(t, q, "cov-enroll-other")
	_, err = repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, other.Revision.ID, &edID, 1)
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)

	en1, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, &edID, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, en1.Priority)
	assert.Equal(t, "active", en1.Status)

	// Pause then re-enroll resumes same row with new priority/revision.
	_, err = repo.SetEnrollmentStatus(ctx, q, en1.ID, "paused", &edID, "break")
	require.NoError(t, err)

	pub2, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          pub.Curriculum.Slug,
		Title:         "Coverage Course resume",
		SubjectCode:   "digital-literacy",
		Activities:    []contracts.CourseActivityRef{{Order: 1, Slug: "basic-navigation"}},
	}, time.Now().UTC())
	require.NoError(t, err)

	en2, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub2.Revision.ID, &edID, 9)
	require.NoError(t, err)
	assert.Equal(t, en1.ID, en2.ID)
	assert.Equal(t, "active", en2.Status)
	assert.Equal(t, 9, en2.Priority)
	require.NotNil(t, en2.CurriculumRevisionID)
	assert.Equal(t, pub2.Revision.ID, *en2.CurriculumRevisionID)

	// Still a single enrollment row for student+curriculum.
	page, err := repo.Enrollments.List(ctx, q, repo.ListParams{
		Limit: 10,
		Filters: map[string]any{
			"student_id":    student.ID,
			"curriculum_id": pub.Curriculum.ID,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, page.TotalCount)

	events, err := repo.ListEnrollmentAudit(ctx, q, en1.ID, 50)
	require.NoError(t, err)
	actions := map[string]int{}
	for _, e := range events {
		actions[e.Action]++
	}
	assert.GreaterOrEqual(t, actions["enroll"], 2)
	assert.GreaterOrEqual(t, actions["pause"], 1)
}

func TestRevisionRequiresStructuredCommandWrapper(t *testing.T) {
	t.Parallel()
	// Filesystem-only completion path → false.
	fsOnly := contracts.ActivityContent{
		Tasks: []contracts.Task{{
			ID: "t1", Title: "T", Instructions: "go",
			Completion: contracts.CheckTree{CheckID: "c1"},
		}},
		Checks: []contracts.Check{{
			ID: "c1", Kind: contracts.CheckFileExists,
			Params: map[string]any{"path": "a.txt"},
		}},
	}
	assert.False(t, repo.RevisionRequiresStructuredCommand(fsOnly))

	// Required command check → true.
	cmd := contracts.ActivityContent{
		Tasks: []contracts.Task{{
			ID: "t1", Title: "T", Instructions: "go",
			Completion: contracts.CheckTree{CheckID: "c1"},
		}},
		Checks: []contracts.Check{{
			ID: "c1", Kind: contracts.CheckCommandProperties,
			Params: map[string]any{"command": "true"},
		}},
	}
	assert.True(t, repo.RevisionRequiresStructuredCommand(cmd))
}
