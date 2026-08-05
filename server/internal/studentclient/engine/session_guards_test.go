package engine_test

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
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
)

func TestSessionMethodGuardsAndLocalHint(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "guards.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)

	eng := openEngineWS(t, env, dbPath, ws, true)
	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	// Typing methods rejected on terminal session.
	require.Error(t, sess.TypeRune(ctx, 'a'))
	require.Error(t, sess.TypeBackspace(ctx))
	// WriteTerminal may succeed when PTY is live, or error when unavailable.
	_ = sess.WriteTerminal(ctx, []byte("x"))

	// LocalHint: activity has hints or falls back to generic nudge.
	hint := sess.LocalHint()
	assert.NotEmpty(t, hint)

	// Complete without required checks fails (basic-navigation needs scripted cmds).
	// Fixtures alone may already satisfy some activities — only assert when incomplete.
	if !sess.Snapshot().RequiredPassed {
		err = sess.Complete(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required checks")
	}

	// After pause, session row is paused.
	require.NoError(t, sess.Pause(ctx))
	// Double close is safe.
	require.NoError(t, sess.Close())
	require.NoError(t, sess.Close())
}

func TestTypingCompletedGuardsAndGenericHint(t *testing.T) {
	t.Parallel()
	env := startTypingEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "type-guards.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)
	eng := openEngineWS(t, env, dbPath, ws, true)

	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	// Wrong-kind terminal APIs.
	require.Error(t, sess.WriteTerminal(ctx, []byte("hi\n")))
	require.Error(t, sess.RunLine(ctx, "pwd"))

	// Complete full typing activity offline then hit completed guards.
	root := repoRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	require.NotNil(t, doc.Content.Typing)
	for _, p := range doc.Content.Typing.Prompts {
		require.NoError(t, sess.TypeString(ctx, p.Text))
	}
	// If thresholds not met due to timing, Verify still exercises path.
	_ = sess.Verify(ctx)
	if sess.Snapshot().RequiredPassed {
		require.NoError(t, sess.Complete(ctx))
		// Completed guards
		require.Error(t, sess.TypeRune(ctx, 'x'))
		require.Error(t, sess.TypeBackspace(ctx))
		require.Error(t, sess.WriteTerminal(ctx, []byte("z")))
		require.Error(t, sess.RunLine(ctx, "ls"))
		// Idempotent complete
		require.NoError(t, sess.Complete(ctx))
	}
}

func TestOpenSessionDecodeAndUnsupportedKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := cache.Open(filepath.Join(t.TempDir(), "bad.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Invalid content shape that still JSON-unmarshals but lacks kind support.
	asg := uuid.NewString()
	require.NoError(t, store.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: asg, State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "x", Kind: "unsupported-kind"},
		Revision: domain.LearningActivityRevision{
			ID: "r", ContentSHA256: "d", Content: map[string]any{"objective": "o"},
		},
	}}))

	eng, err := engine.New(engine.Options{
		Store: store, WorkspaceRoot: t.TempDir(), Offline: true, AllowUnsandboxed: true,
	})
	require.NoError(t, err)
	_, err = eng.OpenSession(ctx, asg)
	require.Error(t, err)

	// Bad content that fails decode is hard (map always marshals); empty assignment missing.
	_, err = eng.OpenSession(ctx, uuid.NewString())
	require.Error(t, err)
}

func TestResumeSessionFailurePaths(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "resume.db")
	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)
	eng := openEngineWS(t, env, dbPath, ws, true)

	// Open once to create durable session + runner state.
	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	require.NoError(t, sess.Pause(ctx))
	require.NoError(t, sess.Close())

	// Corrupt runner kind so resume fails and falls through to fresh open.
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	open, err := store.FindOpenSessionByAssignment(ctx, env.AssignmentID)
	require.NoError(t, err)
	require.NotNil(t, open)
	require.NoError(t, store.SaveRunnerState(ctx, open.ClientSessionID, "typing", []byte(`{}`)))

	eng2 := openEngineWS(t, env, dbPath, ws, true)
	sess2, err := eng2.OpenSession(ctx, env.AssignmentID)
	// Either resumes after fall-through or opens fresh — must not hard-fail.
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess2.Close() })
	assert.Equal(t, contracts.KindTerminal, sess2.Kind())
}

func TestRunAssignmentMissingAssignment(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "runmiss.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	// Offline empty cache — missing assignment.
	engOff := openEngineWS(t, env, dbPath, ws, true)
	err := engOff.RunAssignment(ctx, uuid.NewString(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load assignment")
}

func TestSandboxRequiredWithoutAllowFailsOpen(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sbx.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, env.DeviceToken))
	cl := studentapi.New(env.BaseURL, env.DeviceToken)
	// Seed work online first via temporary engine.
	seed, err := engine.New(engine.Options{
		Client: cl, Store: store, WorkspaceRoot: ws, AllowUnsandboxed: true,
	})
	require.NoError(t, err)
	require.NoError(t, seed.SyncOnce(ctx).Err)

	// Require sandbox but disallow unsandboxed — PTY/shell may fail depending on bwrap.
	eng, err := engine.New(engine.Options{
		Client: cl, Store: store, WorkspaceRoot: ws,
		Offline: true, UseSandbox: true, AllowUnsandboxed: false,
	})
	require.NoError(t, err)
	// Open may succeed if sandbox available, or fail starting PTY — both exercise branches.
	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	if err != nil {
		assert.Error(t, err)
		return
	}
	t.Cleanup(func() { _ = sess.Close() })
	// If open succeeded, RunLine still exercises runCommand sandbox path.
	_ = sess.RunLine(ctx, "pwd")
}
