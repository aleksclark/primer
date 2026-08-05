package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
)

func TestTerminalSessionPTYWriteResizeIdleVerify(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "pty.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)

	eng := openEngineWS(t, env, dbPath, ws, true)
	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })
	assert.Equal(t, contracts.KindTerminal, sess.Kind())

	// Wait for PTY to come up. Fail clearly if unavailable (no silent skip).
	deadline := time.Now().Add(5 * time.Second)
	var hasTerm bool
	for time.Now().Before(deadline) {
		if sess.Snapshot().HasTerminal {
			hasTerm = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	require.True(t, hasTerm, "expected live PTY for terminal session (HasTerminal never became true)")

	require.NoError(t, sess.ResizeTerminal(30, 100))
	// Non-newline write should not panic and updates last input time.
	require.NoError(t, sess.WriteTerminal(ctx, []byte("echo hello")))
	// Newline triggers idleVerify goroutine.
	require.NoError(t, sess.WriteTerminal(ctx, []byte("\n")))

	// Wait for command output / idle verify to land on the screen (event-driven).
	screenDeadline := time.Now().Add(3 * time.Second)
	var screen string
	for time.Now().Before(screenDeadline) {
		screen = sess.TerminalScreen()
		if screen != "" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	assert.NotEmpty(t, screen)
	snap := sess.Snapshot()
	assert.True(t, snap.HasTerminal)
	assert.NotEmpty(t, snap.TerminalScreen)

	// Resize/write after close should error.
	require.NoError(t, sess.Close())
	require.Error(t, sess.WriteTerminal(ctx, []byte("x")))
	require.Error(t, sess.ResizeTerminal(10, 10))
}

func TestShellCommandAndRunCommandPaths(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cmd.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)
	eng := openEngineWS(t, env, dbPath, ws, true)

	// RunAssignment exercises runCommand unsandboxed shell/argv paths.
	require.NoError(t, eng.RunAssignment(ctx, env.AssignmentID, []engine.ScriptedCommand{
		{Argv: []string{"pwd"}},
		{Argv: []string{"echo hi"}, Shell: true},
		{Argv: []string{"cd", "docs"}},
		{Argv: []string{"false"}}, // non-zero exit still OK for harness
		{Argv: []string{"ls", "-la"}},
	}))
}
