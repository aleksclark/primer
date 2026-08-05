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

func TestCountTutorEventsAndSessionHelpers(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Tutor"})

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
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "test")
	require.NoError(t, err)

	// Pair a device for the student.
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	tok, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "tutor-ws", time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, tok)
	require.NotNil(t, device)

	csid := uuid.NewString()
	sess, err := repo.StartOrResumeSession(ctx, q, device, csid, asg.ID, time.Now().UTC())
	require.NoError(t, err)

	events := []contracts.SessionEvent{
		{SchemaVersion: "1", EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: 0, ClientTime: time.Now().UTC()},
		{SchemaVersion: "1", EventID: uuid.NewString(), Type: contracts.EventTutorMessage, Sequence: 1, ClientTime: time.Now().UTC(), Payload: map[string]any{"text": "hi"}},
		{SchemaVersion: "1", EventID: uuid.NewString(), Type: contracts.EventTutorMessage, Sequence: 2, ClientTime: time.Now().UTC(), Payload: map[string]any{"text": "there"}},
	}
	acked, err := repo.IngestSessionEvents(ctx, q, sess.ID, events, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, int64(2), acked)

	// Idempotent re-ingest
	acked2, err := repo.IngestSessionEvents(ctx, q, sess.ID, events, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, int64(2), acked2)

	n, err := repo.CountTutorEvents(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = repo.CountTutorEventsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	list, err := repo.ListAssignmentsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	items, cursor, err := repo.ListStudentWork(ctx, q, student.ID, "", 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotEmpty(t, cursor)
	more, _, err := repo.ListStudentWork(ctx, q, student.ID, cursor, 10)
	require.NoError(t, err)
	assert.Empty(t, more)

	require.NoError(t, repo.MarkSessionCompleted(ctx, q, sess, "done", time.Now().UTC()))
	got, err := repo.LearningSessions.Get(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionCompleted, got.State)

	// Resume same client session id returns same session
	sess2, err := repo.StartOrResumeSession(ctx, q, device, csid, asg.ID, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, sess.ID, sess2.ID)
}
