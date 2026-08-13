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

func TestResponseReviewListDetailAndApply(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Reviewer"})
	ed := factory.Educator(t, q)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	doc := shortResponseDoc("resp-review-activity")
	// Add parent note block for detail path.
	doc.Content.Blocks = []contracts.InstructionBlock{{
		Kind: contracts.BlockParentNote, Text: "Parent: look for clarity.",
	}, {
		Kind: contracts.BlockProse, Text: "Student sees this.",
	}}
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "review")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "review-ws", time.Now().UTC())
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)

	subID := uuid.NewString()
	resp, created, err := repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: subID,
		TaskID:       "reflect",
		Body:         "Directories organize files by topic.",
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, domain.ResponseSubmitted, resp.Status)

	// Default queue lists submitted.
	list, err := repo.ListResponsesForReview(ctx, q, "", "", 0)
	require.NoError(t, err)
	require.NotEmpty(t, list)
	found := false
	for _, it := range list {
		if it.Response.ID == resp.ID {
			found = true
			assert.Equal(t, "resp-review-activity", it.ActivitySlug)
			assert.NotEmpty(t, it.StudentName)
			assert.True(t, it.ReviewRequired)
		}
	}
	assert.True(t, found)

	// Filtered by student + status.
	list2, err := repo.ListResponsesForReview(ctx, q, student.ID, domain.ResponseSubmitted, 10)
	require.NoError(t, err)
	require.NotEmpty(t, list2)

	// Empty filter result.
	empty, err := repo.ListResponsesForReview(ctx, q, uuid.NewString(), domain.ResponseSubmitted, 5)
	require.NoError(t, err)
	assert.Empty(t, empty)

	detail, err := repo.GetResponseDetail(ctx, q, resp.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.Task)
	assert.Equal(t, "reflect", detail.Task.ID)
	assert.Equal(t, "resp-review-activity", detail.ActivitySlug)
	assert.NotEmpty(t, detail.ParentNotes)
	assert.Empty(t, detail.Reviews)

	// Bad decisions.
	_, _, err = repo.ApplyResponseReview(ctx, q, ed.ID, resp.ID, repo.ReviewDecisionInput{Decision: "maybe"}, time.Now().UTC())
	require.Error(t, err)
	_, _, err = repo.ApplyResponseReview(ctx, q, ed.ID, resp.ID, repo.ReviewDecisionInput{Decision: domain.ReviewReturn}, time.Now().UTC())
	require.Error(t, err)

	// Accept.
	accepted, review, err := repo.ApplyResponseReview(ctx, q, ed.ID, resp.ID, repo.ReviewDecisionInput{
		Decision: domain.ReviewAccept,
		Criteria: []map[string]any{{"id": "clarity", "met": true}},
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, domain.ResponseAccepted, accepted.Status)
	require.NotNil(t, review)
	assert.Equal(t, domain.ReviewAccept, review.Decision)

	// Idempotent accept.
	accepted2, review2, err := repo.ApplyResponseReview(ctx, q, ed.ID, resp.ID, repo.ReviewDecisionInput{
		Decision: domain.ReviewAccept,
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, accepted.ID, accepted2.ID)
	require.NotNil(t, review2)

	// Cannot return after accept without resubmit.
	_, _, err = repo.ApplyResponseReview(ctx, q, ed.ID, resp.ID, repo.ReviewDecisionInput{
		Decision: domain.ReviewReturn, Reason: "too short",
	}, time.Now().UTC())
	require.Error(t, err)

	// New attempt after return path: submit second response via fresh task attempt.
	// Create a second short-response session and return it.
	asg2, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "review2")
	require.NoError(t, err)
	sess2, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg2.ID, time.Now().UTC())
	require.NoError(t, err)
	respR, _, err := repo.SubmitStudentResponse(ctx, q, device, sess2.ID, contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(), TaskID: "reflect", Body: "short",
	}, time.Now().UTC())
	require.NoError(t, err)
	returned, revRet, err := repo.ApplyResponseReview(ctx, q, ed.ID, respR.ID, repo.ReviewDecisionInput{
		Decision: domain.ReviewReturn, Reason: "please expand",
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, domain.ResponseReturned, returned.Status)
	assert.Equal(t, "please expand", returned.ReturnReason)
	require.NotNil(t, revRet)

	// Idempotent return.
	returned2, _, err := repo.ApplyResponseReview(ctx, q, ed.ID, respR.ID, repo.ReviewDecisionInput{
		Decision: domain.ReviewReturn, Reason: "please expand",
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, returned.ID, returned2.ID)

	reviews, err := repo.ListResponseReviews(ctx, q, resp.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)

	// Missing detail.
	_, err = repo.GetResponseDetail(ctx, q, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)
}
