package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestCacheOpenErrorsAndEdgePaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Open on a path whose parent is a file → fail.
	dir := t.TempDir()
	fileAsDir := filepath.Join(dir, "notadir")
	require.NoError(t, os.WriteFile(fileAsDir, []byte("x"), 0o644))
	_, err := cache.Open(filepath.Join(fileAsDir, "state.db"))
	require.Error(t, err)

	path := filepath.Join(t.TempDir(), "edges.db")
	s, err := cache.Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	_ = s.Close() // second close

	s2, err := cache.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s2.Close() })

	require.NoError(t, s2.SetDeviceIdentity(ctx, "d", "s", "n"))
	require.NoError(t, s2.SetDeviceToken(ctx, "tok"))
	tok, err := s2.DeviceToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "tok", tok)

	_, err = s2.GetWork(ctx, "missing-asg")
	require.Error(t, err)

	require.NoError(t, s2.SaveWork(ctx, nil))
	require.NoError(t, s2.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: "a1", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "s"},
		Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{"x": 1}},
	}}))
	got, err := s2.GetWork(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, "s", got.Activity.Slug)
	list, err := s2.ListWork(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	csid := uuid.NewString()
	require.NoError(t, s2.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, AssignmentID: "a1", ActivityRevisionID: "r",
		State: "started", LastAckedSequence: -1, NextSequence: 0, WorkspacePath: dir,
	}))
	require.NoError(t, s2.SaveRunnerState(ctx, csid, "terminal", []byte(`{"cwd":"."}`)))
	rs, err := s2.GetRunnerState(ctx, csid)
	require.NoError(t, err)
	require.NotNil(t, rs)
	assert.Equal(t, "terminal", rs.Kind)
	require.NoError(t, s2.DeleteRunnerState(ctx, csid))

	open, err := s2.FindOpenSessionByAssignment(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, open)

	req := contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: uuid.NewString(), RequestDigest: "d",
		ClientTime: time.Now().UTC(),
	}
	require.NoError(t, s2.SaveCompletionIntent(ctx, csid, "srv", req))
	pending, err := s2.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, pending.CompleteCount, 1)
	require.NoError(t, s2.MarkCompletionAcked(ctx, req.CompletionID, contracts.CompletionResult{
		SchemaVersion: "1", CompletionID: req.CompletionID, Accepted: true, RequestDigest: "d",
	}))

	artID := uuid.NewString()
	require.NoError(t, s2.SavePendingArtifact(ctx, csid, "srv", contracts.ArtifactMeta{
		SchemaVersion: "1", ArtifactID: artID, Filename: "f.txt", MediaType: "text/plain",
		ByteSize: 1, SHA256: "aa", CreatedAt: time.Now().UTC(),
	}))
	arts, err := s2.ListPendingArtifacts(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, arts)
	require.NoError(t, s2.MarkArtifactAcked(ctx, artID))

	require.NoError(t, s2.SetMeta(ctx, "k", "v"))
	v, err := s2.GetMeta(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", v)
}
