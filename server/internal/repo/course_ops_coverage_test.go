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

func TestEnrollmentPinOverrideListsAndSlugs(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	pub := publishTwoActivitiesCourse(t, q, "ops-course-"+uuid.NewString()[:8])
	student := factory.Student(t, q)
	ed := factory.Educator(t, q)
	edID := ed.ID

	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, &edID, 4)
	require.NoError(t, err)

	// Invalid status.
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "nope", &edID, "x")
	require.Error(t, err)

	// Complete + withdraw set ended_on; resume clears it.
	completed, err := repo.SetEnrollmentStatus(ctx, q, en.ID, "completed", &edID, "done")
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
	require.NotNil(t, completed.EndedOn)

	withdrawn, err := repo.SetEnrollmentStatus(ctx, q, en.ID, "withdrawn", &edID, "bye")
	require.NoError(t, err)
	assert.Equal(t, "withdrawn", withdrawn.Status)

	active, err := repo.SetEnrollmentStatus(ctx, q, en.ID, "active", &edID, "back")
	require.NoError(t, err)
	assert.Equal(t, "active", active.Status)
	assert.Nil(t, active.EndedOn)

	// Pin valid slug.
	pinned, err := repo.PinEnrollmentActivity(ctx, q, en.ID, "basic-navigation", "focus", &edID)
	require.NoError(t, err)
	assert.Equal(t, "basic-navigation", pinned.PinnedActivitySlug)

	// Pin unknown slug.
	_, err = repo.PinEnrollmentActivity(ctx, q, en.ID, "no-such-slug", "x", &edID)
	require.Error(t, err)

	// Unpin.
	unpinned, err := repo.PinEnrollmentActivity(ctx, q, en.ID, "", "", &edID)
	require.NoError(t, err)
	assert.Empty(t, unpinned.PinnedActivitySlug)

	// Override prereq validation.
	_, err = repo.OverrideEnrollmentPrereq(ctx, q, en.ID, "", "r", &edID)
	require.Error(t, err)
	_, err = repo.OverrideEnrollmentPrereq(ctx, q, en.ID, "basic-navigation", "", &edID)
	require.Error(t, err)
	_, err = repo.OverrideEnrollmentPrereq(ctx, q, en.ID, "missing-slug", "ok", &edID)
	require.Error(t, err)

	over, err := repo.OverrideEnrollmentPrereq(ctx, q, en.ID, "file-organization", "skip nav", &edID)
	require.NoError(t, err)
	assert.Contains(t, over.OverrideSlugs, "file-organization")
	// Second override is idempotent on slug list.
	over2, err := repo.OverrideEnrollmentPrereq(ctx, q, en.ID, "file-organization", "again", &edID)
	require.NoError(t, err)
	count := 0
	for _, s := range over2.OverrideSlugs {
		if s == "file-organization" {
			count++
		}
	}
	assert.Equal(t, 1, count)

	require.NoError(t, repo.UpdateEnrollmentBlockingReasons(ctx, q, en.ID, []any{
		map[string]any{"slug": "file-organization", "reason": "prereq"},
	}))
	require.NoError(t, repo.UpdateEnrollmentBlockingReasons(ctx, q, en.ID, nil))

	// Active enrollments list.
	activeList, err := repo.ListActiveEnrollmentsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotEmpty(t, activeList)
	assert.Equal(t, en.ID, activeList[0].ID)

	// Pause still included in course enrollments list.
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "paused", &edID, "break")
	require.NoError(t, err)
	courseList, err := repo.ListCourseEnrollmentsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotEmpty(t, courseList)

	// Non-course enrollment (no revision) excluded.
	factory.Enrollment(t, q, factory.Override{"student_id": student.ID, "status": "active"})
	courseList2, err := repo.ListCourseEnrollmentsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	for _, e := range courseList2 {
		require.NotNil(t, e.CurriculumRevisionID)
		assert.NotEmpty(t, *e.CurriculumRevisionID)
	}

	// Assignment slug maps.
	page, err := repo.LearningActivities.List(ctx, q, repo.ListParams{Limit: 1, Filters: map[string]any{"slug": "basic-navigation"}})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalCount)
	revPage, err := repo.LearningActivityRevisions.List(ctx, q, repo.ListParams{
		Limit: 1, Sort: "revision", Dir: repo.SortDesc,
		Filters: map[string]any{"activity_id": page.Items[0].ID},
	})
	require.NoError(t, err)
	revID := revPage.Items[0].ID

	openAsg, err := repo.CreateAssignment(ctx, q, student.ID, revID, nil, 1, "open-slug")
	require.NoError(t, err)
	openSlugs, err := repo.OpenAssignmentSlugsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.True(t, openSlugs["basic-navigation"])

	_, err = repo.StudentAssignments.Update(ctx, q, openAsg.ID, map[string]any{"state": domain.AssignmentCompleted})
	require.NoError(t, err)
	doneSlugs, err := repo.CompletedActivitySlugsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.True(t, doneSlugs["basic-navigation"])

	// Mastery by code.
	std := factory.Standard(t, q, factory.Override{"code": "PRIMER.OPS.TEST.1"})
	factory.MasteryRecord(t, q, factory.Override{
		"student_id":  student.ID,
		"standard_id": std.ID,
		"status":      "mastered",
	})
	byCode, err := repo.MasteryStatusByStandardCode(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, "mastered", byCode["PRIMER.OPS.TEST.1"])

	// Empty pending parent review.
	pending, err := repo.PendingParentReviewSlugs(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Empty(t, pending)

	// Audit with limit clamp.
	events, err := repo.ListEnrollmentAudit(ctx, q, en.ID, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	events2, err := repo.ListEnrollmentAudit(ctx, q, en.ID, 500)
	require.NoError(t, err)
	assert.NotEmpty(t, events2)

	// Missing enrollment pin/override.
	_, err = repo.PinEnrollmentActivity(ctx, q, uuid.NewString(), "basic-navigation", "x", &edID)
	require.Error(t, err)
	_, err = repo.OverrideEnrollmentPrereq(ctx, q, uuid.NewString(), "basic-navigation", "x", &edID)
	require.Error(t, err)
}

func TestPendingParentReviewSlugsWithResponse(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "ReviewSlug"})

	// Reuse short-response publisher from responses coverage via local publish.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	_ = thisFile
	// Publish standards + short response doc.
	pubRoot := func() string {
		_, f, _, _ := runtime.Caller(0)
		return filepath.Clean(filepath.Join(filepath.Dir(f), "..", "..", ".."))
	}()
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(pubRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc := shortResponseDoc("pending-review-slug-act")
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "rev-slug")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "rev-slug-ws", time.Now().UTC())
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)

	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(),
		TaskID:       "reflect",
		Body:         "I learned about parent review gates.",
	}, time.Now().UTC())
	require.NoError(t, err)

	pending, err := repo.PendingParentReviewSlugs(ctx, q, student.ID)
	require.NoError(t, err)
	assert.True(t, pending["pending-review-slug-act"])
}
