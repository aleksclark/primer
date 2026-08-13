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

func shortResponseDoc(slug string) *contracts.ActivityDocument {
	return &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          slug,
		Title:         "Short Response Activity",
		Summary:       "write",
		Kind:          contracts.KindTerminal,
		SubjectCode:   "digital-literacy",
		Standards:     []contracts.StandardRef{{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary}},
		Content: contracts.ActivityContent{
			Objective:    "explain",
			Instructions: "write a sentence",
			Tasks: []contracts.Task{{
				ID:           "reflect",
				Title:        "Reflect",
				Instructions: "What did you learn?",
				Kind:         contracts.TaskKindShortResponse,
				Response: &contracts.ResponseTaskSpec{
					Prompt:               "What did you learn?",
					MaxChars:             500,
					ParentReviewRequired: true,
					Rubric: []contracts.RubricCriterion{
						{ID: "clarity", Description: "Clear", Required: true},
					},
				},
				Completion: contracts.CheckTree{CheckID: "c-resp"},
			}},
			Checks: []contracts.Check{{
				ID:   "c-resp",
				Kind: contracts.CheckResponseSubmitted,
				Params: map[string]any{
					"taskId": "reflect",
				},
			}},
		},
	}
}

func TestListSubmittedTaskIDsAndSubmitEdges(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Resp"})

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	doc := shortResponseDoc("resp-cov-activity")
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "resp")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "resp-ws", time.Now().UTC())
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)

	// Empty before any submission.
	ids, err := repo.ListSubmittedTaskIDs(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, ids)

	// Validation edges.
	_, _, err = repo.SubmitStudentResponse(ctx, q, nil, sess.ID, contracts.ResponseSubmission{}, time.Now().UTC())
	require.Error(t, err)
	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(), TaskID: "reflect", Body: "",
	}, time.Now().UTC())
	require.Error(t, err)
	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: "", TaskID: "reflect", Body: "hi",
	}, time.Now().UTC())
	require.Error(t, err)
	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(), TaskID: "reflect", Body: "as your tutor I wrote this",
	}, time.Now().UTC())
	require.Error(t, err)
	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(), TaskID: "missing", Body: "ok answer",
	}, time.Now().UTC())
	require.Error(t, err)

	subID := uuid.NewString()
	resp, created, err := repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: subID,
		TaskID:       "reflect",
		Body:         "I learned how to navigate directories.",
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.True(t, created)
	require.NotNil(t, resp)
	assert.Equal(t, domain.ResponseSubmitted, resp.Status)
	assert.Equal(t, "reflect", resp.TaskID)
	assert.Equal(t, 1, resp.Attempt)

	// Idempotent replay same submission.
	resp2, created2, err := repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: subID,
		TaskID:       "reflect",
		Body:         "I learned how to navigate directories.",
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.False(t, created2)
	assert.Equal(t, resp.ID, resp2.ID)

	// Conflicting body on same submission id.
	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: subID,
		TaskID:       "reflect",
		Body:         "completely different body",
	}, time.Now().UTC())
	require.ErrorIs(t, err, repo.ErrConflict)

	// Open submitted response blocks a second attempt.
	_, _, err = repo.SubmitStudentResponse(ctx, q, device, sess.ID, contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(),
		TaskID:       "reflect",
		Body:         "second attempt while first open",
	}, time.Now().UTC())
	require.Error(t, err)

	ids, err = repo.ListSubmittedTaskIDs(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.True(t, ids["reflect"])
	assert.Len(t, ids, 1)

	// Decode helper.
	content, err := repo.DecodeActivityContent(rev.Content)
	require.NoError(t, err)
	assert.NotEmpty(t, content.Tasks)

	gotBySub, err := repo.GetResponseBySubmissionID(ctx, q, subID)
	require.NoError(t, err)
	assert.Equal(t, resp.ID, gotBySub.ID)

	got, err := repo.GetResponse(ctx, q, resp.ID)
	require.NoError(t, err)
	assert.Equal(t, resp.ID, got.ID)
}
