package overseer_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/overseer"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestAssignNextBySlug(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{Slug: "basic-navigation"})
	require.NoError(t, err)
	require.True(t, res.Created)
	assert.Equal(t, rev.ID, res.Assignment.ActivityRevisionID)

	res2, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{Slug: "basic-navigation"})
	require.NoError(t, err)
	assert.False(t, res2.Created)
	assert.Equal(t, res.Assignment.ID, res2.Assignment.ID)
}

func TestAssignNextCurriculumOrder(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	for _, slug := range []string{"basic-navigation", "command-typing-basics", "file-organization"} {
		doc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", slug, "activity.yaml"))
		require.NoError(t, err)
		_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
		require.NoError(t, err)
	}

	prefer := false
	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{PreferReinforcement: &prefer})
	require.NoError(t, err)
	require.True(t, res.Created)
	// alphabetically first unassigned published slug
	assert.Contains(t, res.Reason, "curriculum:basic-navigation")
}

func TestAssignNextReinforcementTyping(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	navDoc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, navDoc, time.Now().UTC())
	require.NoError(t, err)

	typeDoc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, typeRev, err := curriculum.PublishDocument(ctx, q, typeDoc, time.Now().UTC())
	require.NoError(t, err)

	var typeStdID string
	err = q.QueryRow(ctx, `SELECT id FROM standards WHERE code = $1`, "PRIMER.DL.6.TYPE.1").Scan(&typeStdID)
	require.NoError(t, err)

	past := time.Now().UTC().Add(-2 * time.Hour)
	factory.MasteryRecord(t, q, factory.Override{
		"student_id":            student.ID,
		"standard_id":           typeStdID,
		"status":                "approaching",
		"confidence":            0.4,
		"next_reinforcement_at": past,
	})

	res, err := overseer.AssignNext(ctx, q, student.ID, overseer.Options{})
	require.NoError(t, err)
	require.True(t, res.Created)
	assert.Equal(t, typeRev.ID, res.Assignment.ActivityRevisionID)
	assert.Contains(t, res.Reason, "TYPE")
}
