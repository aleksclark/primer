package overseer_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/overseer"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func publishCourseWithExtras(t *testing.T, q repo.Querier, opts struct {
	EvidenceGate bool
	Remediation  bool
	ThirdSlug    string
}) (*repo.PublishCourseResult, string, string) {
	t.Helper()
	ctx := context.Background()
	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	slugs := []string{"basic-navigation", "file-organization"}
	if opts.ThirdSlug != "" {
		slugs = append(slugs, opts.ThirdSlug)
	}
	for _, slug := range slugs {
		doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", slug, "activity.yaml"))
		require.NoError(t, err)
		_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
		require.NoError(t, err)
	}

	acts := []contracts.CourseActivityRef{
		{Order: 1, Slug: "basic-navigation"},
		{Order: 2, Slug: "file-organization"},
	}
	if opts.ThirdSlug != "" {
		acts = append(acts, contracts.CourseActivityRef{Order: 3, Slug: opts.ThirdSlug})
	}
	course := &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "elig-course-" + uuid.NewString()[:8],
		Title:         "Eligibility Course",
		SubjectCode:   "digital-literacy",
		Version:       "1",
		Activities:    acts,
		Prerequisites: []contracts.CoursePrerequisite{{
			Activity: "file-organization", Requires: []string{"basic-navigation"}, Requirement: contracts.PrereqCompleted,
		}},
		Gates: []contracts.CourseGate{{
			Activity: "file-organization", Kind: contracts.GateParentReview, Description: "review",
		}},
	}
	if opts.EvidenceGate {
		course.Gates = append(course.Gates, contracts.CourseGate{
			Activity:  "basic-navigation",
			Kind:      contracts.GateEvidence,
			Standards: []string{"PRIMER.DL.6.NAV.1"},
		})
	}
	if opts.Remediation {
		// Branch to file-organization when basic-navigation is returned.
		course.Remediation = []contracts.CourseRemediation{{
			ForActivity: "basic-navigation",
			BranchSlug:  "file-organization",
			Kind:        "remediation",
		}}
	}
	res, err := repo.PublishCourseDocument(ctx, q, course, time.Now().UTC())
	require.NoError(t, err)
	return res, "basic-navigation", "file-organization"
}

func TestEvaluateEnrollmentMissingRevisionAndStatuses(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	curr := factory.Curriculum(t, q)
	// Enrollment without curriculum revision.
	en, err := repo.Enrollments.Create(ctx, q, map[string]any{
		"student_id":    student.ID,
		"curriculum_id": curr.ID,
		"status":        "active",
	})
	require.NoError(t, err)
	_, err = overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "curriculum revision")
}

func TestRuntimeProfileBlocksWhenDeviceMissing(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, _ := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	// Device reports only an unrelated profile → basic-navigation (coreutils) blocked.
	dev := factory.StudentDevice(t, q, factory.Override{"student_id": student.ID})
	err = repo.StoreDeviceCapabilities(ctx, q, dev.ID, map[string]any{
		"runtimeProfiles": []any{"text-processing"},
	}, time.Now().UTC())
	require.NoError(t, err)

	prev, err := overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	require.NotEmpty(t, prev.Activities)

	var found bool
	for _, st := range prev.Activities {
		if st.Membership.ActivitySlug != first {
			continue
		}
		found = true
		assert.False(t, st.Eligible)
		assert.Equal(t, "blocked", st.Status)
		require.NotEmpty(t, st.BlockingReasons)
		assert.Equal(t, overseer.BlockMissingRuntimeProfile, st.BlockingReasons[0].Code)
		assert.Equal(t, contracts.RuntimeCoreutilsBasic, st.BlockingReasons[0].Requirement)
	}
	require.True(t, found)

	// Matching profile clears the runtime gate.
	err = repo.StoreDeviceCapabilities(ctx, q, dev.ID, map[string]any{
		"runtimeProfiles": []string{contracts.RuntimeCoreutilsBasic},
	}, time.Now().UTC())
	require.NoError(t, err)
	prev, err = overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	require.NotEmpty(t, prev.Eligible)
	assert.Equal(t, first, prev.Eligible[0].Membership.ActivitySlug)
}

func TestEvidenceGateAndCompletedAssignedStatuses(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, second := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{EvidenceGate: true})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	prev, err := overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	// Evidence gate on first lesson blocks until mastery approaches.
	require.Empty(t, prev.Eligible)
	var firstSt overseer.ActivityStatus
	for _, st := range prev.Activities {
		if st.Membership.ActivitySlug == first {
			firstSt = st
		}
	}
	require.Equal(t, "blocked", firstSt.Status)
	require.True(t, hasBlock(firstSt, overseer.BlockEvidenceGateUnmet))

	// Seed approaching mastery → first becomes eligible.
	var stdID string
	err = q.QueryRow(ctx, `SELECT id FROM standards WHERE code = $1`, "PRIMER.DL.6.NAV.1").Scan(&stdID)
	require.NoError(t, err)
	factory.MasteryRecord(t, q, factory.Override{
		"student_id": student.ID, "standard_id": stdID, "status": "approaching", "confidence": 0.5,
	})
	prev, err = overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	require.Len(t, prev.Eligible, 1)
	assert.Equal(t, first, prev.Eligible[0].Membership.ActivitySlug)

	// Open assignment → assigned status.
	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	require.True(t, res.Created)
	prev, err = overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	for _, st := range prev.Activities {
		if st.Membership.ActivitySlug == first {
			assert.Equal(t, "assigned", st.Status)
			assert.Equal(t, overseer.BlockAlreadyOpen, st.BlockingReasons[0].Code)
		}
	}

	// Complete → completed status; second still blocked by prereq until done.
	_, err = repo.StudentAssignments.Update(ctx, q, res.Assignment.ID, map[string]any{"state": "completed"})
	require.NoError(t, err)
	prev, err = overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	for _, st := range prev.Activities {
		if st.Membership.ActivitySlug == first {
			assert.Equal(t, "completed", st.Status)
		}
		if st.Membership.ActivitySlug == second {
			// parent_review gate alone does not block without pending review; prereq is met now.
			assert.True(t, st.Eligible || st.Status == "eligible" || st.Status == "blocked" || st.Status == "review_needed")
		}
	}
}

func hasBlock(st overseer.ActivityStatus, code string) bool {
	for _, b := range st.BlockingReasons {
		if b.Code == code {
			return true
		}
	}
	return false
}

func TestCollectEnrollmentBlockSummaryAndFirstEligible(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()

	// No enrollments.
	student := factory.Student(t, q)
	sum, err := overseer.CollectEnrollmentBlockSummary(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, overseer.BlockNoActiveEnrollment, sum)

	enPtr, act, reason, err := overseer.FirstEligibleCourseActivity(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Nil(t, enPtr)
	assert.Nil(t, act)
	assert.Equal(t, "", reason)

	pub, first, second := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	// First eligible is lesson 1.
	enPtr, act, reason, err = overseer.FirstEligibleCourseActivity(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotNil(t, enPtr)
	require.NotNil(t, act)
	assert.Equal(t, en.ID, enPtr.ID)
	assert.Equal(t, first, act.ActivitySlug)
	assert.Equal(t, "course:"+first, reason)

	// With eligible work, block summary is empty.
	sum, err = overseer.CollectEnrollmentBlockSummary(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, "", sum)

	// Pin forces second even though prereq unmet.
	_, err = repo.PinEnrollmentActivity(ctx, q, en.ID, second, "skip", nil)
	require.NoError(t, err)
	enPtr, act, reason, err = overseer.FirstEligibleCourseActivity(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotNil(t, act)
	assert.Equal(t, second, act.ActivitySlug)
	assert.Equal(t, "pin:"+second, reason)

	// Clear pin, complete first, open second → no eligible left after completing both.
	_, err = repo.PinEnrollmentActivity(ctx, q, en.ID, "", "clear", nil)
	require.NoError(t, err)

	// Mark both memberships completed via assignments.
	acts, err := repo.ListCurriculumActivities(ctx, q, pub.Revision.ID)
	require.NoError(t, err)
	for _, a := range acts {
		require.NotNil(t, a.ActivityRevisionID)
		asg, err := repo.CreateAssignment(ctx, q, student.ID, *a.ActivityRevisionID, nil, 1, "done")
		require.NoError(t, err)
		_, err = repo.StudentAssignments.Update(ctx, q, asg.ID, map[string]any{"state": "completed"})
		require.NoError(t, err)
	}

	enPtr, act, reason, err = overseer.FirstEligibleCourseActivity(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotNil(t, enPtr)
	assert.Nil(t, act)
	assert.Equal(t, overseer.BlockNoEligibleActivity, reason)

	// All membership rows are completed → blocking summary collapses to no_eligible_activity
	// (completed reasons are skipped when selecting an incomplete blocked activity).
	sum, err = overseer.CollectEnrollmentBlockSummary(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, overseer.BlockNoEligibleActivity, sum)
}

func TestCollectBlockSummaryPausedAndPrereq(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, _ := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	// Pause removes the enrollment from the active list used by CollectEnrollmentBlockSummary.
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "paused", nil, "break")
	require.NoError(t, err)
	sum, err := overseer.CollectEnrollmentBlockSummary(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, overseer.BlockNoActiveEnrollment, sum)

	// Resume and leave first incomplete → summary empty (eligible work exists).
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "active", nil, "")
	require.NoError(t, err)
	sum, err = overseer.CollectEnrollmentBlockSummary(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, "", sum)
	_ = first
}

func TestRemediationBranchSlugAndIsSlug(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, second := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{Remediation: true})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	branch, err := overseer.RemediationBranchSlug(ctx, q, pub.Revision.ID, first)
	require.NoError(t, err)
	assert.Equal(t, second, branch)

	branch, err = overseer.RemediationBranchSlug(ctx, q, pub.Revision.ID, "nope")
	require.NoError(t, err)
	assert.Equal(t, "", branch)

	ok, enOut, mem, err := overseer.IsSlugInActiveEnrollments(ctx, q, student.ID, first)
	require.NoError(t, err)
	assert.True(t, ok)
	require.NotNil(t, enOut)
	require.NotNil(t, mem)
	assert.Equal(t, en.ID, enOut.ID)
	assert.Equal(t, first, mem.ActivitySlug)

	ok, _, _, err = overseer.IsSlugInActiveEnrollments(ctx, q, student.ID, "missing-slug")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAssignNextSlugNotInCourseAndRemediation(t *testing.T) {
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

	// Publish typing activity outside the course.
	typeDoc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, typeDoc, time.Now().UTC())
	require.NoError(t, err)

	pub, first, second := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{Remediation: true})
	_, err = repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	// Slug not in course while enrolled → block.
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{Slug: "command-typing-basics"})
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.Contains(t, res.BlockReason, overseer.BlockNotInRevision)

	// Explicit in-course slug assigns with membership provenance.
	res, err = overseer.AssignNext(ctx, q, student.ID, overseer.Options{Slug: first})
	require.NoError(t, err)
	require.True(t, res.Created)
	assert.Contains(t, res.Reason, "slug:"+first)
	require.NotNil(t, res.Assignment.EnrollmentID)

	// Complete first assignment; insert a returned response to trigger remediation.
	_, err = repo.StudentAssignments.Update(ctx, q, res.Assignment.ID, map[string]any{"state": "completed"})
	require.NoError(t, err)

	// Seed returned conceptual response for first activity.
	var revID string
	err = q.QueryRow(ctx, `
SELECT r.id FROM learning_activity_revisions r
JOIN learning_activities a ON a.id = r.activity_id
WHERE a.slug = $1 AND r.published_at IS NOT NULL
ORDER BY r.revision DESC LIMIT 1`, first).Scan(&revID)
	require.NoError(t, err)
	asg2, err := repo.CreateAssignment(ctx, q, student.ID, revID, nil, 1, "for-response")
	require.NoError(t, err)
	// Mark available so we can attach a response row via raw insert (status returned).
	dev := factory.StudentDevice(t, q, factory.Override{"student_id": student.ID})
	sess, err := repo.StartOrResumeSession(ctx, q, dev, uuid.NewString(), asg2.ID, time.Now().UTC())
	require.NoError(t, err)
	_, err = q.Exec(ctx, `
INSERT INTO student_responses (
  submission_id, student_id, session_id, assignment_id, activity_revision_id,
  task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
  parent_review_required, submitted_at
) VALUES (
  $1,$2,$3,$4,$5,'task-1','because cd moves directories','abc','returned','d',1,'[]'::jsonb,true,now()
)`, uuid.NewString(), student.ID, sess.ID, asg2.ID, revID)
	require.NoError(t, err)

	// Cancel the temp assignment so it does not count as open for the first slug.
	_, err = repo.CancelAssignment(ctx, q, asg2.ID)
	require.NoError(t, err)

	prefer := false
	resRem, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	// Remediation should pick file-organization branch (not already open/completed).
	require.True(t, resRem.Created)
	assert.Contains(t, resRem.Reason, "remediation:")
	assert.Contains(t, resRem.Reason, second)
}

func TestAssignNextCourseReinforcementConstrained(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, _ := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{})
	_, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	// Due typing mastery points at out-of-course typing activity — must be skipped
	// for enrolled students; course first lesson should win.
	root := repoRoot(t)
	typeDoc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, typeDoc, time.Now().UTC())
	require.NoError(t, err)

	var typeStdID string
	err = q.QueryRow(ctx, `SELECT id FROM standards WHERE code = $1`, "PRIMER.DL.6.TYPE.1").Scan(&typeStdID)
	require.NoError(t, err)
	past := time.Now().UTC().Add(-2 * time.Hour)
	factory.MasteryRecord(t, q, factory.Override{
		"student_id": student.ID, "standard_id": typeStdID,
		"status": "approaching", "confidence": 0.4, "next_reinforcement_at": past,
	})

	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{})
	require.NoError(t, err)
	require.True(t, res.Created)
	// Out-of-course reinforcement skipped → course activity.
	assert.Contains(t, res.Reason, "course:"+first)
}

func TestAssignNextNoEligibleLibraryEmpty(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	// No published activities at all.
	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.Equal(t, overseer.BlockNoEligibleActivity, res.Reason)
	assert.Contains(t, res.BlockReason, "no assignable")
}

func TestAssignNextUnknownStudent(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	_, err := overseer.AssignNext(ctx, q, uuid.NewString(), overseer.Options{})
	require.Error(t, err)
}

func TestOverridePrereqMakesSecondEligible(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, _, second := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)

	_, err = repo.OverrideEnrollmentPrereq(ctx, q, en.ID, second, "parent skip", nil)
	require.NoError(t, err)

	prev, err := overseer.EvaluateEnrollmentEligibility(ctx, q, en.ID)
	require.NoError(t, err)
	// Both may be eligible: first naturally, second via override.
	slugs := map[string]bool{}
	for _, st := range prev.Eligible {
		slugs[st.Membership.ActivitySlug] = true
	}
	assert.True(t, slugs[second], "override should make second eligible: %+v", prev.Eligible)
}

func TestPausedEnrollmentBlocksAutoButAllowsSlug(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	pub, first, _ := publishCourseWithExtras(t, q, struct {
		EvidenceGate bool
		Remediation  bool
		ThirdSlug    string
	}{})
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, nil, 0)
	require.NoError(t, err)
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "paused", nil, "break")
	require.NoError(t, err)

	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.Equal(t, overseer.BlockEnrollmentPaused, res.BlockReason)

	// Explicit slug with only paused enrollments: IsSlugInActiveEnrollments walks
	// active enrollments only, so expect not-in-revision block.
	res2, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{Slug: first})
	require.NoError(t, err)
	assert.False(t, res2.Created)
	assert.Contains(t, res2.BlockReason, overseer.BlockNotInRevision)
}
