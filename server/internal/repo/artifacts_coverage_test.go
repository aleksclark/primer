package repo_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func seedSession(t *testing.T, q repo.Querier) (*domain.Student, *domain.StudentDevice, *domain.LearningSession, *domain.LearningActivityRevision) {
	t.Helper()
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Art"})
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
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "art")
	require.NoError(t, err)
	code, _, err := repo.CreatePairingCode(ctx, q, student.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device, err := repo.ClaimStudentPairingCode(ctx, q, code, "art-ws", time.Now().UTC())
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, device, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)
	return student, device, sess, rev
}

func TestSessionArtifactsReserveListUpsertAndConflict(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student, _, sess, _ := seedSession(t, q)

	empty, err := repo.ListSessionArtifacts(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Empty(t, empty)

	sum := sha256.Sum256([]byte("payload-a"))
	digest := hex.EncodeToString(sum[:])
	artClientID := uuid.NewString()
	meta := contracts.ArtifactMeta{
		ArtifactID: artClientID,
		Filename:   "notes.txt",
		MediaType:  "text/plain",
		ByteSize:   int64(len("payload-a")),
		SHA256:     digest,
	}

	art, err := repo.ReserveSessionArtifact(ctx, q, sess, meta, "")
	require.NoError(t, err)
	require.NotNil(t, art)
	assert.Equal(t, domain.ArtifactStatusReserved, art.Status)
	assert.Equal(t, sess.ID, art.SessionID)
	assert.Equal(t, meta.Filename, art.Filename)

	// Idempotent same meta.
	art2, err := repo.ReserveSessionArtifact(ctx, q, sess, meta, domain.ArtifactStatusReserved)
	require.NoError(t, err)
	assert.Equal(t, art.ID, art2.ID)

	// Conflicting meta on same artifact_id.
	bad := meta
	bad.Filename = "other.txt"
	_, err = repo.ReserveSessionArtifact(ctx, q, sess, bad, "")
	require.ErrorIs(t, err, repo.ErrConflict)

	// Second session cannot claim same artifact_id.
	otherStudent := factory.Student(t, q, factory.Override{"first_name": "OtherArt"})
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)
	asg2, err := repo.CreateAssignment(ctx, q, otherStudent.ID, rev.ID, nil, 1, "art2")
	require.NoError(t, err)
	code2, _, err := repo.CreatePairingCode(ctx, q, otherStudent.ID, nil, time.Now().UTC())
	require.NoError(t, err)
	_, device2, err := repo.ClaimStudentPairingCode(ctx, q, code2, "art-ws-2", time.Now().UTC())
	require.NoError(t, err)
	sess2, err := repo.StartOrResumeSession(ctx, q, device2, uuid.NewString(), asg2.ID, time.Now().UTC())
	require.NoError(t, err)
	_, err = repo.ReserveSessionArtifact(ctx, q, sess2, meta, "")
	require.ErrorIs(t, err, repo.ErrConflict)

	// Upsert metadata_only for a new id.
	sumB := sha256.Sum256([]byte("meta-only"))
	meta2ID := uuid.NewString()
	meta2 := contracts.ArtifactMeta{
		ArtifactID: meta2ID,
		Filename:   "meta.txt",
		MediaType:  "text/plain",
		ByteSize:   9,
		SHA256:     hex.EncodeToString(sumB[:]),
	}
	up, err := repo.UpsertSessionArtifact(ctx, q, sess.ID, meta2)
	require.NoError(t, err)
	assert.Equal(t, domain.ArtifactStatusMetadataOnly, up.Status)

	// Upsert of reserved artifact leaves richer status.
	upReserved, err := repo.UpsertSessionArtifact(ctx, q, sess.ID, meta)
	require.NoError(t, err)
	assert.Equal(t, art.ID, upReserved.ID)
	assert.Equal(t, domain.ArtifactStatusReserved, upReserved.Status)

	// Cross-session upsert conflict.
	_, err = repo.UpsertSessionArtifact(ctx, q, sess2.ID, meta2)
	require.ErrorIs(t, err, repo.ErrConflict)

	listed, err := repo.ListSessionArtifacts(ctx, q, sess.ID)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	files, total, err := repo.SessionArtifactUsage(ctx, q, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, files)
	assert.Equal(t, meta.ByteSize+meta2.ByteSize, total)

	// Mark uploaded then promote to portfolio + fixture bundle.
	path := "objects/sha256/aa/" + digest
	until := time.Now().UTC().Add(24 * time.Hour)
	uploaded, err := repo.MarkArtifactUploaded(ctx, q, meta.ArtifactID, path, &until)
	require.NoError(t, err)
	assert.True(t, uploaded.BytesStored)
	assert.Equal(t, domain.ArtifactStatusUploaded, uploaded.Status)

	item, bundle, err := repo.PromoteArtifactToPortfolio(ctx, q, meta.ArtifactID, "", "My Notes", domain.PortfolioDestinationFixtureBundle, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, item)
	require.NotNil(t, bundle)
	assert.Equal(t, student.ID, item.StudentID)
	assert.Equal(t, domain.PortfolioDestinationFixtureBundle, item.Destination)
	assert.Equal(t, "approved", bundle.Status)
	assert.Equal(t, meta.ArtifactID, bundle.SourceArtifactID)

	// Idempotent promote reuses rows.
	item2, bundle2, err := repo.PromoteArtifactToPortfolio(ctx, q, meta.ArtifactID, "", "My Notes", domain.PortfolioDestinationFixtureBundle, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, item.ID, item2.ID)
	assert.Equal(t, bundle.ID, bundle2.ID)

	// Also promote to plain portfolio destination.
	port, noBundle, err := repo.PromoteArtifactToPortfolio(ctx, q, meta.ArtifactID, "", "", domain.PortfolioDestinationPortfolio, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, port)
	assert.Nil(t, noBundle)
	assert.Equal(t, "notes.txt", port.Title) // empty title → filename

	items, err := repo.ListPortfolioItems(ctx, q, student.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)

	emptyPort, err := repo.ListPortfolioItems(ctx, q, otherStudent.ID)
	require.NoError(t, err)
	assert.Empty(t, emptyPort)

	// Continuity: default fresh with no binding.
	asgID := sess.AssignmentID
	res, err := repo.ResolveContinuityForAssignment(ctx, q, asgID)
	require.NoError(t, err)
	assert.Equal(t, contracts.ContinuityFresh, res.Mode)
	assert.Nil(t, res.Bundle)

	// Bind optional previous with bundle.
	bid := bundle.ID
	bind, err := repo.BindAssignmentContinuity(ctx, q, asgID, student.ID, contracts.ContinuityOptionalPrevious, &bid, "", "carry", time.Time{})
	require.NoError(t, err)
	require.NotNil(t, bind.BundleID)
	assert.Equal(t, bundle.ID, *bind.BundleID)

	res2, err := repo.ResolveContinuityForAssignment(ctx, q, asgID)
	require.NoError(t, err)
	assert.Equal(t, contracts.ContinuityOptionalPrevious, res2.Mode)
	require.NotNil(t, res2.Bundle)
	assert.Equal(t, bundle.ID, res2.Bundle.ID)

	// required_project without bundle rejected at bind time.
	_, err = repo.BindAssignmentContinuity(ctx, q, asgID, student.ID, contracts.ContinuityRequiredProject, nil, "", "", time.Now().UTC())
	require.Error(t, err)

	// required_project with bundle resolves.
	_, err = repo.BindAssignmentContinuity(ctx, q, asgID, student.ID, contracts.ContinuityRequiredProject, &bid, "", "need", time.Now().UTC())
	require.NoError(t, err)
	res3, err := repo.ResolveContinuityForAssignment(ctx, q, asgID)
	require.NoError(t, err)
	assert.Equal(t, contracts.ContinuityRequiredProject, res3.Mode)
	require.NotNil(t, res3.Bundle)

	// Fresh mode clears bundle id.
	_, err = repo.BindAssignmentContinuity(ctx, q, asgID, student.ID, contracts.ContinuityFresh, &bid, "", "", time.Now().UTC())
	require.NoError(t, err)
	res4, err := repo.ResolveContinuityForAssignment(ctx, q, asgID)
	require.NoError(t, err)
	assert.Equal(t, contracts.ContinuityFresh, res4.Mode)
	assert.Nil(t, res4.Bundle)

	// Missing assignment continuity still fresh.
	res5, err := repo.ResolveContinuityForAssignment(ctx, q, uuid.NewString())
	require.NoError(t, err)
	assert.Equal(t, contracts.ContinuityFresh, res5.Mode)

	// Policy extraction from revision.
	gotRev, err := repo.GetRevision(ctx, q, sess.ActivityRevisionID)
	require.NoError(t, err)
	pol, err := repo.ArtifactPolicyFromRevision(gotRev)
	require.NoError(t, err)
	_ = pol // may be nil depending on activity yaml

	nilPol, err := repo.ArtifactPolicyFromRevision(nil)
	require.NoError(t, err)
	assert.Nil(t, nilPol)

	// Bad destination.
	_, _, err = repo.PromoteArtifactToPortfolio(ctx, q, meta.ArtifactID, "", "x", "cloud", time.Now().UTC())
	require.Error(t, err)

	// Not uploaded yet cannot promote metadata-only.
	_, _, err = repo.PromoteArtifactToPortfolio(ctx, q, meta2.ArtifactID, "", "x", domain.PortfolioDestinationPortfolio, time.Now().UTC())
	require.Error(t, err)

	n, err := repo.StudentArtifactBytes(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Equal(t, meta.ByteSize, n)
}
