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

func TestLearningSessionCompletionsEventsAndHints(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "SessCov"})

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

	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "sess-cov")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "sess-cov-ws", time.Now().UTC())
	require.NoError(t, err)

	csid := uuid.NewString()
	now := time.Now().UTC()
	sess, err := repo.StartOrResumeSession(ctx, q, device, csid, asg.ID, now)
	require.NoError(t, err)

	// Resume same client session id returns existing.
	same, err := repo.StartOrResumeSession(ctx, q, device, csid, asg.ID, now)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, same.ID)

	// Ingest a contiguous event.
	_, err = repo.IngestSessionEvents(ctx, q, sess.ID, []contracts.SessionEvent{{
		SchemaVersion: "1",
		EventID:       uuid.NewString(),
		Type:          contracts.EventSessionStarted,
		Sequence:      0,
		ClientTime:    now,
	}}, now)
	require.NoError(t, err)

	n, err := repo.CountSessionEvents(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// Server-origin tutor events + hints.
	require.NoError(t, repo.InsertServerSessionEvent(ctx, q, sess.ID, contracts.EventTutorMessage, map[string]any{
		"reply": "try ls",
	}, now))
	require.NoError(t, repo.InsertServerSessionEvent(ctx, q, sess.ID, contracts.EventTutorMessage, map[string]any{
		"reply": "",
	}, now))
	require.NoError(t, repo.InsertServerSessionEvent(ctx, q, sess.ID, contracts.EventTutorMessage, nil, now))

	hints, err := repo.ListTutorReplyHints(ctx, q, sess.ID, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"try ls"}, hints)

	hints2, err := repo.ListTutorReplyHints(ctx, q, sess.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, hints, hints2)

	tn, err := repo.CountTutorEvents(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, tn)

	tnStu, err := repo.CountTutorEventsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, tnStu)

	// Completion insert + getters + decode.
	cid := uuid.NewString()
	result := contracts.CompletionResult{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  cid,
		Accepted:      true,
		RequestDigest: "digest-1",
		Message:       "ok",
	}
	row, err := repo.InsertCompletion(ctx, q, sess.ID, cid, "digest-1", result)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, sess.ID, row.SessionID)
	assert.Equal(t, cid, row.CompletionID)

	bySess, err := repo.GetCompletionBySession(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, row.ID, bySess.ID)

	byID, err := repo.GetCompletionByID(ctx, q, cid)
	require.NoError(t, err)
	assert.Equal(t, row.ID, byID.ID)

	_, err = repo.GetCompletionBySession(ctx, q, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = repo.GetCompletionByID(ctx, q, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	decoded, err := repo.CompletionResultFromRow(row)
	require.NoError(t, err)
	assert.True(t, decoded.Accepted)
	assert.Equal(t, cid, decoded.CompletionID)
	assert.Equal(t, "digest-1", decoded.RequestDigest)

	// Mark completed with negative duration clamp.
	past := sess
	past.StartedAt = now.Add(time.Hour)
	require.NoError(t, repo.MarkSessionCompleted(ctx, q, past, "finished early clock", now))
	got, err := repo.GetSession(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionCompleted, got.State)
	assert.Equal(t, 0, got.DurationSeconds)

	// RejectIncompatibleRevision: filesystem-only is fine without caps.
	require.NoError(t, repo.RejectIncompatibleRevision(map[string]any{
		"tasks": []any{map[string]any{
			"id": "t1", "title": "T", "instructions": "go",
			"completion": map[string]any{"checkId": "c1"},
		}},
		"checks": []any{map[string]any{
			"id": "c1", "kind": contracts.CheckFileExists,
			"params": map[string]any{"path": "a.txt"},
		}},
	}, map[string]bool{}))

	// Structured-required path without capability → bad request.
	err = repo.RejectIncompatibleRevision(map[string]any{
		"tasks": []any{map[string]any{
			"id": "t1", "title": "T", "instructions": "go",
			"completion": map[string]any{"checkId": "c1"},
		}},
		"checks": []any{map[string]any{
			"id": "c1", "kind": contracts.CheckCommandProperties,
			"params": map[string]any{"executable": "ls"},
		}},
	}, map[string]bool{})
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)
}
