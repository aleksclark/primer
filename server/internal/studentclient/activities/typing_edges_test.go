package activities_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func typingContentWithTasks() contracts.ActivityContent {
	return contracts.ActivityContent{
		Typing: &contracts.TypingContent{
			PromptSetID:     "set",
			SuccessWPM:      1,
			SuccessAccuracy: 0.5,
			Prompts: []contracts.TypingPrompt{
				{ID: "p1", Text: "hi"},
				{ID: "p2", Text: "yo"},
			},
		},
		Checks: []contracts.Check{
			{ID: "m", Kind: contracts.CheckTypingMetrics, Params: map[string]any{"min_wpm": 1.0, "min_accuracy": 0.5}},
			{ID: "opt", Kind: contracts.CheckTypingMetrics, Optional: true, Params: map[string]any{"min_wpm": 999.0, "min_accuracy": 1.0}},
		},
		Tasks: []contracts.Task{
			{ID: "t1", Title: "T1", Instructions: "type", Completion: contracts.CheckTree{CheckID: "m"}},
			{ID: "t2", Title: "T2", Instructions: "opt", Optional: true, Completion: contracts.CheckTree{CheckID: "opt"}},
			{ID: "t3", Title: "T3", Instructions: "empty-tree", Completion: contracts.CheckTree{}}, // empty completion skipped
		},
	}
}

func TestTypingRunnerOpenAndInputGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := activities.NewTyping()
	require.Error(t, r.Open(ctx, activities.OpenOpts{Content: contracts.ActivityContent{}}))
	assert.Contains(t, r.Open(ctx, activities.OpenOpts{Content: contracts.ActivityContent{}}).Error(), "typing content")

	// Not open.
	require.Error(t, r.HandleInput(ctx, activities.Input{Type: activities.InputKey, Rune: 'a'}))
	require.Error(t, r.Verify(ctx))
	assert.Nil(t, r.Session())
	raw, err := r.EncodeState()
	require.NoError(t, err)
	var bare map[string]any
	require.NoError(t, json.Unmarshal(raw, &bare))
	assert.Equal(t, contracts.KindTyping, bare["kind"])

	require.NoError(t, r.Open(ctx, activities.OpenOpts{Workspace: t.TempDir(), Content: typingContentWithTasks(), Digest: "d1"}))
	require.NotNil(t, r.Session())
	assert.False(t, r.CompleteReady())

	// Unsupported input type.
	require.Error(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "pwd"}))

	// Key / backspace / string paths.
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputKey, Rune: 'h'}))
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputBackspace}))
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputString, Text: "hi"}))
	drained := r.DrainEvents()
	assert.True(t, drained.EmitSample)
	assert.NotNil(t, drained.Typing)
	assert.Equal(t, 1, drained.Typing.PromptIndex)

	require.NoError(t, r.Verify(ctx))
	obs := r.Observations()
	require.NotEmpty(t, obs)

	// Finish second prompt → complete ready via required metrics + task tree.
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputString, Text: "yo"}))
	assert.True(t, r.CompleteReady())
	snap := r.Snapshot()
	assert.True(t, snap.RequiredPassed)
	// Empty completion tree on t3 keeps current task index at that task (not past end).
	assert.Equal(t, 2, snap.CurrentTaskIdx)
}

func TestTypingRunnerRestoreWrappedAndBareState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := typingContentWithTasks()
	r := activities.NewTyping()
	require.NoError(t, r.Open(ctx, activities.OpenOpts{Workspace: t.TempDir(), Content: content}))

	// Empty restore is no-op.
	require.NoError(t, r.RestoreState(nil))
	require.NoError(t, r.RestoreState([]byte{}))

	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputString, Text: "h"}))
	wrapped, err := r.EncodeState()
	require.NoError(t, err)

	// Bare durable state from underlying session.
	bare, err := r.Session().EncodeState()
	require.NoError(t, err)

	r2 := activities.NewTyping()
	require.NoError(t, r2.Open(ctx, activities.OpenOpts{Workspace: t.TempDir(), Content: content}))
	require.NoError(t, r2.RestoreState(wrapped))
	assert.Equal(t, "h", r2.Snapshot().Typing.Input)
	drained := r2.DrainEvents()
	assert.False(t, drained.EmitSample)

	r3 := activities.NewTyping()
	require.NoError(t, r3.Open(ctx, activities.OpenOpts{Workspace: t.TempDir(), Content: content}))
	require.NoError(t, r3.RestoreState(bare))
	assert.Equal(t, "h", r3.Snapshot().Typing.Input)

	// Bad restore while open.
	require.Error(t, r3.RestoreState([]byte(`{"v":1,"state":"not-json"`)))

	// Restore after close fails.
	require.NoError(t, r3.Close())
	require.Error(t, r3.RestoreState(bare))
	assert.Nil(t, r3.Session())
}

func TestTypingRunnerTaskTreeBlocksComplete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := contracts.ActivityContent{
		Typing: &contracts.TypingContent{
			PromptSetID: "s", SuccessWPM: 1, SuccessAccuracy: 0.5,
			Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "a"}},
		},
		Checks: []contracts.Check{
			{ID: "m", Kind: contracts.CheckTypingMetrics, Params: map[string]any{"min_wpm": 1.0, "min_accuracy": 0.5}},
			{ID: "hard", Kind: contracts.CheckTypingMetrics, Params: map[string]any{"min_wpm": 1e9, "min_accuracy": 1.0}},
		},
		Tasks: []contracts.Task{
			{ID: "need-hard", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "hard"}},
		},
	}
	r := activities.NewTyping()
	require.NoError(t, r.Open(ctx, activities.OpenOpts{Workspace: t.TempDir(), Content: content}))
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputKey, Rune: 'a'}))
	// Metrics may pass content defaults but task requires impossible hard check.
	assert.False(t, r.CompleteReady())
	assert.Equal(t, 0, r.Snapshot().CurrentTaskIdx)
}
