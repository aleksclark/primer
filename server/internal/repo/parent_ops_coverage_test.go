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

func publishNavActivity(t *testing.T, q repo.Querier) *domain.LearningActivityRevision {
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
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)
	return rev
}

func TestParentOpsCancelRetryTutorAndOverview(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "ParentOps", "notes": "hello"})
	ed := factory.Educator(t, q)
	rev := publishNavActivity(t, q)

	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, &ed.ID, 3, "parent-assign")
	require.NoError(t, err)

	// Cancel open assignment.
	cancelled, err := repo.CancelAssignment(ctx, q, asg.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AssignmentCancelled, cancelled.State)

	// Idempotent cancel.
	again, err := repo.CancelAssignment(ctx, q, asg.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AssignmentCancelled, again.State)

	// Completed cannot be cancelled.
	done, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "done-asg")
	require.NoError(t, err)
	_, err = repo.StudentAssignments.Update(ctx, q, done.ID, map[string]any{"state": domain.AssignmentCompleted})
	require.NoError(t, err)
	_, err = repo.CancelAssignment(ctx, q, done.ID)
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)

	// Retry creates a new available assignment.
	retry, err := repo.RetryAssignment(ctx, q, asg.ID, &ed.ID, "")
	require.NoError(t, err)
	assert.Equal(t, domain.AssignmentAvailable, retry.State)
	assert.Equal(t, asg.ActivityRevisionID, retry.ActivityRevisionID)
	assert.Equal(t, student.ID, retry.StudentID)
	assert.Equal(t, "parent-retry", retry.Reason)

	retry2, err := repo.RetryAssignment(ctx, q, asg.ID, nil, "custom-reason")
	require.NoError(t, err)
	assert.Equal(t, "custom-reason", retry2.Reason)

	// Tutor enable/disable via notes marker.
	stOff, err := repo.SetStudentTutorEnabled(ctx, q, student.ID, false)
	require.NoError(t, err)
	assert.Contains(t, stOff.Notes, repo.TutorOffMarker)
	// Idempotent disable.
	stOff2, err := repo.SetStudentTutorEnabled(ctx, q, student.ID, false)
	require.NoError(t, err)
	assert.Equal(t, stOff.Notes, stOff2.Notes)

	stOn, err := repo.SetStudentTutorEnabled(ctx, q, student.ID, true)
	require.NoError(t, err)
	assert.NotContains(t, stOn.Notes, repo.TutorOffMarker)
	// Idempotent enable.
	stOn2, err := repo.SetStudentTutorEnabled(ctx, q, student.ID, true)
	require.NoError(t, err)
	assert.Equal(t, stOn.Notes, stOn2.Notes)

	// Empty notes then disable.
	bare := factory.Student(t, q, factory.Override{"first_name": "Bare", "notes": ""})
	stBare, err := repo.SetStudentTutorEnabled(ctx, q, bare.ID, false)
	require.NoError(t, err)
	assert.Equal(t, repo.TutorOffMarker, stBare.Notes)

	// Device + session for overview lists.
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "parent-ops-ws", time.Now().UTC())
	require.NoError(t, err)
	openAsg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 5, "open-now")
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), openAsg.ID, time.Now().UTC())
	require.NoError(t, err)

	std := factory.Standard(t, q, factory.Override{"code": "PRIMER.OPS.UNIQUE." + uuid.NewString()[:8]})
	mr := factory.MasteryRecord(t, q, factory.Override{
		"student_id":  student.ID,
		"standard_id": std.ID,
		"status":      "in_progress",
		"confidence":  0.4,
	})
	factory.MasteryEvidence(t, q, factory.Override{
		"mastery_record_id": mr.ID,
		"evidence_class":    domain.EvidenceProceduralContinuous,
		"context":           "practice set",
	})

	// Overview with default session limit clamp.
	ov, err := repo.GetStudentLearningOverview(ctx, q, student.ID, 0)
	require.NoError(t, err)
	require.NotNil(t, ov)
	assert.Equal(t, student.ID, ov.Student.ID)
	assert.NotEmpty(t, ov.Devices)
	assert.NotEmpty(t, ov.OpenAssignments)
	assert.NotEmpty(t, ov.RecentSessions)
	assert.NotEmpty(t, ov.MasterySummary)
	assert.NotEmpty(t, ov.EvidenceStatuses)
	assert.False(t, ov.TutorNotesDisable)

	// Evidence statuses include procedural accepted (may also flag additional evidence).
	found := false
	for _, es := range ov.EvidenceStatuses {
		if es.StandardID == std.ID {
			found = true
			assert.True(t, es.ProceduralAccepted)
			assert.Contains(t, []string{
				repo.EvidenceStatusProceduralAccepted,
				repo.EvidenceStatusAdditionalEvidenceReq,
			}, es.EvidenceStatus)
		}
	}
	assert.True(t, found)

	open, err := repo.ListOpenAssignmentsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, open)

	recent, err := repo.ListRecentSessionsForStudent(ctx, q, student.ID, 5)
	require.NoError(t, err)
	require.NotEmpty(t, recent)
	assert.Equal(t, sess.ID, recent[0].ID)

	mastery, err := repo.ListMasteryForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, mastery)

	metrics, err := repo.GetStudentClientMetrics(ctx, q, time.Now().UTC())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, metrics.DevicesActive, 1)
	assert.GreaterOrEqual(t, metrics.AssignmentsOpen, 1)
	assert.GreaterOrEqual(t, metrics.SessionsActive, 1)
}

func TestSupersedeMasteryEvidenceAndEvidenceStatuses(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q)
	std := factory.Standard(t, q, factory.Override{"code": "PRIMER.TYPE.6.HOME.1"})
	mr := factory.MasteryRecord(t, q, factory.Override{
		"student_id":  student.ID,
		"standard_id": std.ID,
		"status":      "approaching",
		"confidence":  0.7,
	})
	ev := factory.MasteryEvidence(t, q, factory.Override{
		"mastery_record_id": mr.ID,
		"evidence_class":    domain.EvidenceProceduralContinuous,
		"context":           "",
		"kind":              "continuous",
	})
	ed := factory.Educator(t, q)

	orig, rep, err := repo.SupersedeMasteryEvidence(ctx, q, ev.ID, "wrong score", ed.ID, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, orig)
	require.NotNil(t, rep)
	assert.Contains(t, orig.Context, "superseded by parent")
	assert.Contains(t, rep.Context, "wrong score")
	assert.Equal(t, mr.ID, rep.MasteryRecordID)

	// Second supersede of same row rejected.
	_, _, err = repo.SupersedeMasteryEvidence(ctx, q, ev.ID, "again", ed.ID, time.Now().UTC())
	require.Error(t, err)

	// Typing code path uses typing default policy.
	statuses, err := repo.ListEvidenceStatusesForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotEmpty(t, statuses)
	// Superseded original is excluded from accepted classes.
	for _, st := range statuses {
		if st.StandardID == std.ID {
			// Replacement still counts as accepted.
			assert.Contains(t, st.AcceptedEvidenceClasses, domain.EvidenceProceduralContinuous)
		}
	}

	// Supersede with empty educator and note.
	ev2 := factory.MasteryEvidence(t, q, factory.Override{
		"mastery_record_id": mr.ID,
		"evidence_class":    domain.EvidenceProceduralContinuous,
		"context":           "keep",
	})
	o2, r2, err := repo.SupersedeMasteryEvidence(ctx, q, ev2.ID, "", "", time.Now().UTC())
	require.NoError(t, err)
	assert.Contains(t, o2.Context, "superseded by parent")
	assert.Contains(t, r2.Context, "parent correction superseding")
}
