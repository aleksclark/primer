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

func TestStartOrResumeSessionEdgesAndEventConflict(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Sess"})
	other := factory.Student(t, q, factory.Override{"first_name": "Other"})

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

	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "edge")
	require.NoError(t, err)
	asgOther, err := repo.CreateAssignment(ctx, q, other.ID, rev.ID, nil, 1, "other")
	require.NoError(t, err)

	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "edge-ws", time.Now().UTC())
	require.NoError(t, err)

	// Wrong student assignment → not found
	_, err = repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asgOther.ID, time.Now().UTC())
	require.Error(t, err)

	// Missing assignment
	_, err = repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), uuid.NewString(), time.Now().UTC())
	require.Error(t, err)

	// Cancelled assignment
	cancelled, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "cancel-me")
	require.NoError(t, err)
	_, err = repo.CancelAssignment(ctx, q, cancelled.ID)
	require.NoError(t, err)
	_, err = repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), cancelled.ID, time.Now().UTC())
	require.Error(t, err)

	// Happy path
	csid := uuid.NewString()
	sess, err := repo.StartOrResumeSession(ctx, q, device, csid, asg.ID, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Second device resumes open session for same assignment.
	code2, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device2, err := repo.ClaimStudentPairingCode(ctx, q, code2, "edge-ws-2", time.Now().UTC())
	require.NoError(t, err)
	sess2, err := repo.StartOrResumeSession(ctx, q, device2, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, sess.ID, sess2.ID)

	// Empty event id rejected
	_, err = repo.IngestSessionEvents(ctx, q, sess.ID, []contracts.SessionEvent{{
		SchemaVersion: "1", EventID: "", Type: contracts.EventSessionStarted, Sequence: 10, ClientTime: time.Now().UTC(),
	}}, time.Now().UTC())
	require.Error(t, err)

	// Sequence conflict: same sequence, different event ids
	e1 := uuid.NewString()
	e2 := uuid.NewString()
	_, err = repo.IngestSessionEvents(ctx, q, sess.ID, []contracts.SessionEvent{{
		SchemaVersion: "1", EventID: e1, Type: contracts.EventSessionStarted, Sequence: 5, ClientTime: time.Now().UTC(),
	}}, time.Now().UTC())
	require.NoError(t, err)
	_, err = repo.IngestSessionEvents(ctx, q, sess.ID, []contracts.SessionEvent{{
		SchemaVersion: "1", EventID: e2, Type: contracts.EventCommandFinished, Sequence: 5, ClientTime: time.Now().UTC(),
	}}, time.Now().UTC())
	require.Error(t, err)
	assert.ErrorIs(t, err, repo.ErrConflict)

	// Nil payload + empty schema version defaults
	_, err = repo.IngestSessionEvents(ctx, q, sess.ID, []contracts.SessionEvent{{
		EventID: uuid.NewString(), Type: contracts.EventCheckEvaluated, Sequence: 6, ClientTime: time.Now().UTC(), Payload: nil,
	}}, time.Now().UTC())
	require.NoError(t, err)

	// GetSession helper
	got, err := repo.GetSession(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)

	// Mark completed then start on completed assignment fails
	require.NoError(t, repo.MarkSessionCompleted(ctx, q, sess, "done", time.Now().UTC()))
	// Complete the assignment too
	_, err = repo.StudentAssignments.Update(ctx, q, asg.ID, map[string]any{"state": domain.AssignmentCompleted})
	require.NoError(t, err)
	_, err = repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.Error(t, err)
}
