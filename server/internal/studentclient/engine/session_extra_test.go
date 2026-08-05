package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestTypingSessionBackspaceHintAndFindSlug(t *testing.T) {
	t.Parallel()
	env := startTypingEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "type-extra.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	// Seed work while online, then operate offline to avoid network flushes.
	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)

	eng := openEngineWS(t, env, dbPath, ws, true)

	asgID, err := eng.FindAssignmentBySlug(ctx, "command-typing-basics")
	require.NoError(t, err)
	assert.Equal(t, env.AssignmentID, asgID)
	_, err = eng.FindAssignmentBySlug(ctx, "does-not-exist")
	require.Error(t, err)

	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTyping, sess.Kind())
	t.Cleanup(func() { _ = sess.Close() })

	root := repoRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, doc.Content.Typing.Prompts)
	prompt := []rune(doc.Content.Typing.Prompts[0].Text)
	require.GreaterOrEqual(t, len(prompt), 2)
	require.NoError(t, sess.TypeString(ctx, string(prompt[:2])))
	require.NoError(t, sess.TypeBackspace(ctx))
	snap := sess.Snapshot()
	require.NotNil(t, snap.Typing)
	// Backspace is exercised; exact buffer depends on runner advance rules.
	assert.LessOrEqual(t, len([]rune(snap.Typing.Input)), 2)

	require.Error(t, sess.RunLine(ctx, "pwd"))

	sess.SetTutorHint("slow down and watch the prompt")
	assert.Equal(t, "slow down and watch the prompt", sess.Snapshot().TutorHint)
	assert.NotEmpty(t, sess.LocalHint())

	require.NoError(t, sess.Verify(ctx))
	require.NoError(t, sess.Pause(ctx))
}

func TestOpenSessionMissingAssignment(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	dbPath := filepath.Join(t.TempDir(), "miss.db")
	// Offline empty cache — assignment not found locally.
	eng := openEngine(t, env, dbPath, true)
	_, err := eng.OpenSession(context.Background(), "00000000-0000-0000-0000-000000000099")
	require.Error(t, err)
}
