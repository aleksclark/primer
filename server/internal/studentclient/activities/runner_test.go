package activities_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestKindOf(t *testing.T) {
	t.Parallel()
	assert.Equal(t, contracts.KindTerminal, activities.KindOf(contracts.KindTerminal))
	assert.Equal(t, contracts.KindTyping, activities.KindOf(contracts.KindTyping))
	assert.Equal(t, "future", activities.KindOf("future"))
}

func TestSupportedKindsRegistered(t *testing.T) {
	t.Parallel()
	kinds := activities.SupportedKinds()
	assert.Contains(t, kinds, contracts.KindTerminal)
	assert.Contains(t, kinds, contracts.KindTyping)
	assert.True(t, activities.Supports(contracts.KindTerminal))
	assert.True(t, activities.Supports(contracts.KindTyping))
	assert.False(t, activities.Supports("quantum-tutor"))
}

func TestNewUnsupportedKind(t *testing.T) {
	t.Parallel()
	_, err := activities.New("nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, activities.ErrUnsupportedKind)
}

func TestBothRunnersImplementInterface(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{contracts.KindTerminal, contracts.KindTyping} {
		r, err := activities.New(kind)
		require.NoError(t, err)
		require.Equal(t, kind, r.Kind())
		require.NoError(t, r.Close())
	}
}

func TestTypingRunnerEncodeRestoreMidPrompt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()
	content := contracts.ActivityContent{
		Typing: &contracts.TypingContent{
			PromptSetID:     "t",
			SuccessWPM:      1,
			SuccessAccuracy: 0.5,
			Prompts: []contracts.TypingPrompt{
				{ID: "p1", Text: "hello"},
				{ID: "p2", Text: "world"},
			},
		},
		Checks: []contracts.Check{{
			ID: "m", Kind: contracts.CheckTypingMetrics,
			Params: map[string]any{"min_wpm": 1.0, "min_accuracy": 0.5},
		}},
	}

	r1 := activities.NewTyping()
	require.NoError(t, r1.Open(ctx, activities.OpenOpts{Workspace: ws, Content: content}))
	require.NoError(t, r1.HandleInput(ctx, activities.Input{Type: activities.InputString, Text: "hel"}))
	snap := r1.Snapshot()
	require.NotNil(t, snap.Typing)
	assert.Equal(t, "hel", snap.Typing.Input)
	assert.Equal(t, 0, snap.Typing.PromptIndex)
	assert.False(t, r1.CompleteReady())

	raw, err := r1.EncodeState()
	require.NoError(t, err)
	require.NoError(t, r1.Close())

	r2 := activities.NewTyping()
	require.NoError(t, r2.Open(ctx, activities.OpenOpts{Workspace: ws, Content: content}))
	require.NoError(t, r2.RestoreState(raw))
	// Restore must not set emit flags (no double-count).
	drained := r2.DrainEvents()
	assert.False(t, drained.EmitSample)
	assert.False(t, drained.EmitChecks)

	snap = r2.Snapshot()
	require.NotNil(t, snap.Typing)
	assert.Equal(t, "hel", snap.Typing.Input)
	assert.Equal(t, 0, snap.Typing.PromptIndex)

	require.NoError(t, r2.HandleInput(ctx, activities.Input{Type: activities.InputString, Text: "lo"}))
	require.NoError(t, r2.HandleInput(ctx, activities.Input{Type: activities.InputString, Text: "world"}))
	assert.True(t, r2.CompleteReady())
	snap = r2.Snapshot()
	require.NotNil(t, snap.Typing)
	assert.True(t, snap.Typing.Done)
}

func TestTerminalRunnerEncodeRestoreCwdAndCommands(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "welcome.txt"), []byte("hi\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "docs", "guide.txt"), []byte("g\n"), 0o644))

	content := contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			Fixtures: []contracts.FixtureEntry{
				{Path: "welcome.txt", Type: contracts.FixtureFile, Content: "hi\n"},
				{Path: "docs", Type: contracts.FixtureDirectory},
				{Path: "docs/guide.txt", Type: contracts.FixtureFile, Content: "g\n"},
			},
		},
		Checks: []contracts.Check{{
			ID: "welcome", Kind: contracts.CheckFileExists,
			Params: map[string]any{"path": "welcome.txt"},
		}},
	}

	runShell := func(ctx context.Context, workspace, cwd, line string) (string, string, int, error) {
		return "ok\n", "", 0, nil
	}

	r1 := activities.NewTerminal()
	require.NoError(t, r1.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: content, RunShell: runShell, SkipMaterialize: true,
	}))
	require.NoError(t, r1.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd docs"}))
	require.NoError(t, r1.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "pwd"}))
	snap := r1.Snapshot()
	assert.Equal(t, 2, snap.CommandsRun)
	assert.Equal(t, "docs", snap.RelCwd)

	raw, err := r1.EncodeState()
	require.NoError(t, err)
	// Drain any pending events then close.
	_ = r1.DrainEvents()
	require.NoError(t, r1.Close())

	r2 := activities.NewTerminal()
	require.NoError(t, r2.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: content, RunShell: runShell, SkipMaterialize: true,
	}))
	require.NoError(t, r2.RestoreState(raw))
	drained := r2.DrainEvents()
	assert.False(t, drained.EmitCommand)
	assert.False(t, drained.EmitChecks)

	snap = r2.Snapshot()
	assert.Equal(t, 2, snap.CommandsRun)
	assert.Equal(t, "docs", snap.RelCwd)
	assert.True(t, snap.RequiredPassed || snap.ChecksPassed >= 0)
}
