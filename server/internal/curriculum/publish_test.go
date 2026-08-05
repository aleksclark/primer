package curriculum_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func TestPublishStandardsAndActivities(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	root := repoRoot(t)

	// Standards only first.
	res, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Greater(t, res.StandardsUpserted, 0)

	// Idempotent re-publish updates existing standards.
	res2, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	assert.Equal(t, res.StandardsUpserted, res2.StandardsUpserted)

	// Activities from a temp dir with one document.
	actDir := t.TempDir()
	src := filepath.Join(root, "curriculum", "activities", "basic-navigation")
	// symlink or copy activity.yaml
	data, err := os.ReadFile(filepath.Join(src, "activity.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(actDir, "basic-navigation"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(actDir, "basic-navigation", "activity.yaml"), data, 0o644))

	res3, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir:  filepath.Join(root, "curriculum", "standards"),
		ActivitiesDir: actDir,
		Now:           time.Now().UTC(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res3.Activities)
	assert.Equal(t, 1, res3.Revisions)

	// Empty options still works (now defaulted).
	res4, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res4.Activities)
}

func TestPublishDocumentCreatesStubStandards(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "unit-test-activity",
		Title:         "Unit Test Activity",
		Kind:          contracts.KindTerminal,
		SubjectCode:   "math",
		Content: contracts.ActivityContent{
			Objective: "test",
			Tasks:     []contracts.Task{{ID: "t1", Title: "T", Instructions: "Do it", Completion: contracts.CheckTree{CheckID: "c1"}}},
			Checks:    []contracts.Check{{ID: "c1", Kind: "command", Params: map[string]any{"command": "true"}}},
		},
		Standards: []contracts.StandardRef{{Code: "CUSTOM.TEST.1", Role: contracts.StandardRolePrimary, Weight: 1}},
	}
	act, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, act)
	require.NotNil(t, rev)
	assert.Equal(t, "unit-test-activity", act.Slug)
	assert.NotEmpty(t, rev.ID)

	// Second publish same content is idempotent (same digest).
	_, rev2, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, rev.ID, rev2.ID)

	// CreateDraft + PublishDraftRevision path.
	draft, err := repo.CreateDraftActivity(ctx, q, "draft-slug", "Draft", "sum", contracts.KindTerminal, act.SubjectID)
	require.NoError(t, err)
	// Look up standard created as stub.
	page, err := repo.Standards.List(ctx, q, repo.ListParams{Limit: 200})
	require.NoError(t, err)
	var stdID string
	for _, s := range page.Items {
		if s.Code == "CUSTOM.TEST.1" {
			stdID = s.ID
			break
		}
	}
	require.NotEmpty(t, stdID)
	ids := map[string]string{"CUSTOM.TEST.1": stdID}
	drev, err := repo.PublishDraftRevision(ctx, q, draft.ID, doc.Content, "", doc.Standards, ids, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, drev)

	// Idempotent republish of same draft content.
	drev2, err := repo.PublishDraftRevision(ctx, q, draft.ID, doc.Content, "", doc.Standards, ids, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, drev.ID, drev2.ID)

	// ListAssignmentsForStudent empty.
	list, err := repo.ListAssignmentsForStudent(ctx, q, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Empty(t, list)
}
