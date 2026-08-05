package mastery_test

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
	"github.com/aleksclark/primer/server/internal/mastery"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestApplyCompletionIdempotentAndConflict(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()

	student := factory.Student(t, q, factory.Override{"first_name": "Mastery"})
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
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "m")
	require.NoError(t, err)

	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "m-ws", time.Now().UTC())
	require.NoError(t, err)

	csid := uuid.NewString()
	sess, err := repo.StartOrResumeSession(ctx, q, device, csid, asg.ID, time.Now().UTC())
	require.NoError(t, err)

	obs := make([]contracts.Observation, 0, len(doc.Content.Checks))
	now := time.Now().UTC()
	for _, c := range doc.Content.Checks {
		obs = append(obs, contracts.Observation{
			SchemaVersion: contracts.ObservationSchemaVersion,
			CheckID:       c.ID,
			Kind:          c.Kind,
			Passed:        true,
			Optional:      c.Optional,
			ObservedAt:    now,
		})
	}
	completionID := uuid.NewString()
	digest := "digest-a"
	req := contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  completionID,
		RequestDigest: digest,
		Observations:  obs,
		ClientTime:    now,
		Summary:       "first",
	}

	res, err := mastery.ApplyCompletion(ctx, q, device, sess.ID, req, now)
	require.NoError(t, err)
	assert.True(t, res.Accepted)
	require.NotEmpty(t, res.EvidenceIDs)
	require.NotEmpty(t, res.MasterySnapshot)

	// Idempotent same completion id + digest
	res2, err := mastery.ApplyCompletion(ctx, q, device, sess.ID, req, now.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, res.CompletionID, res2.CompletionID)
	assert.Equal(t, res.EvidenceIDs, res2.EvidenceIDs)

	// Same completion id, different digest → conflict
	reqBad := req
	reqBad.RequestDigest = "other-digest"
	_, err = mastery.ApplyCompletion(ctx, q, device, sess.ID, reqBad, now)
	require.Error(t, err)
	assert.ErrorIs(t, err, repo.ErrConflict)

	// Different completion on already-completed session → conflict
	req2 := req
	req2.CompletionID = uuid.NewString()
	req2.RequestDigest = "digest-b"
	_, err = mastery.ApplyCompletion(ctx, q, device, sess.ID, req2, now)
	require.Error(t, err)
	assert.ErrorIs(t, err, repo.ErrConflict)

	// Wrong student device → not found
	other := factory.Student(t, q, factory.Override{"first_name": "OtherM"})
	code2, _, err := repo.CreatePairingCode(ctx, q, other.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, otherDev, err := repo.ClaimStudentPairingCode(ctx, q, code2, "other", time.Now().UTC())
	require.NoError(t, err)
	_, err = mastery.ApplyCompletion(ctx, q, otherDev, sess.ID, req, now)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestApplyCompletionRejectsFailedChecksAndCancelled(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()

	student := factory.Student(t, q, factory.Override{"first_name": "Fail"})
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
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "m")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "fail-ws", time.Now().UTC())
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)

	now := time.Now().UTC()
	// Missing required observations
	_, err = mastery.ApplyCompletion(ctx, q, device, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: uuid.NewString(), RequestDigest: "d",
		Observations: []contracts.Observation{}, ClientTime: now,
	}, now)
	require.Error(t, err)

	// Cancel assignment then reject completion
	_, err = repo.CancelAssignment(ctx, q, asg.ID)
	require.NoError(t, err)
	obs := make([]contracts.Observation, 0, len(doc.Content.Checks))
	for _, c := range doc.Content.Checks {
		obs = append(obs, contracts.Observation{
			SchemaVersion: "1", CheckID: c.ID, Kind: c.Kind, Passed: true, Optional: c.Optional, ObservedAt: now,
		})
	}
	_, err = mastery.ApplyCompletion(ctx, q, device, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: uuid.NewString(), RequestDigest: "d2",
		Observations: obs, ClientTime: now,
	}, now)
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)
}

func TestApplyCompletionOnAbandonedSession(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Abd"})
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
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "m")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "abd-ws", time.Now().UTC())
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)

	// Force abandoned state
	_, err = q.Exec(ctx, `UPDATE learning_sessions SET state = $1 WHERE id = $2`, domain.SessionAbandoned, sess.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	obs := make([]contracts.Observation, 0, len(doc.Content.Checks))
	for _, c := range doc.Content.Checks {
		obs = append(obs, contracts.Observation{
			SchemaVersion: "1", CheckID: c.ID, Kind: c.Kind, Passed: true, Optional: c.Optional, ObservedAt: now,
		})
	}
	_, err = mastery.ApplyCompletion(ctx, q, device, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: uuid.NewString(), RequestDigest: "d",
		Observations: obs, ClientTime: now,
	}, now)
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)
}
