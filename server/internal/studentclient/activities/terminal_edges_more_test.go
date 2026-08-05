package activities_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestTerminalRunnerOpenGuardsAndCDEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := activities.NewTerminal()
	require.Error(t, r.Open(ctx, activities.OpenOpts{Content: contracts.ActivityContent{}}))
	require.Error(t, r.Open(ctx, activities.OpenOpts{
		Content: contracts.ActivityContent{Terminal: &contracts.TerminalContent{RuntimeProfile: contracts.RuntimeCoreutilsBasic}},
	}))

	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "home", "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "home", "a.txt"), []byte("x\n"), 0o644))

	content := contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			InitialCwd:     "home",
			Fixtures: []contracts.FixtureEntry{
				{Path: "home", Type: contracts.FixtureDirectory},
				{Path: "home/a.txt", Type: contracts.FixtureFile, Content: "x\n"},
				{Path: "home/docs", Type: contracts.FixtureDirectory},
			},
		},
		Tasks: []contracts.Task{
			{ID: "t1", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "ex"}},
			{ID: "t2", Title: "O", Instructions: "G", Optional: true, Completion: contracts.CheckTree{CheckID: "ex"}},
		},
		Checks: []contracts.Check{
			{ID: "ex", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "home/a.txt"}},
			{ID: "opt", Kind: contracts.CheckFileExists, Optional: true, Params: map[string]any{"path": "home/missing.txt"}},
		},
	}

	// Bad initial cwd escapes workspace.
	require.Error(t, r.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: contracts.ActivityContent{
			Terminal: &contracts.TerminalContent{RuntimeProfile: contracts.RuntimeCoreutilsBasic, InitialCwd: "../outside"},
		},
		SkipMaterialize: true,
	}))

	r = activities.NewTerminal()
	require.NoError(t, r.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: content, SkipMaterialize: true,
		RunShell: func(ctx context.Context, workspace, cwd, line string) (string, string, int, error) {
			return "out\n", "", 0, nil
		},
	}))

	// Built-in cd paths.
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd"}))
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd docs"}))
	assert.Equal(t, filepath.Join(ws, "home", "docs"), r.Cwd())
	// Absolute cd inside workspace.
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd " + filepath.Join(ws, "home")}))
	// Absolute outside rejected (returns error after stamping lastError).
	err := r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd /tmp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside")
	// cd to file.
	err = r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd a.txt"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
	// Missing path.
	err = r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "cd nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cd:")

	// Shell result input path.
	require.NoError(t, r.HandleInput(ctx, activities.Input{
		Type:  activities.InputShellResult,
		Shell: &activities.ShellResult{Cwd: "home", Executable: "ls", Args: []string{"-l"}, ExitCode: 0, Stdout: "a.txt\n"},
	}))
	drained := r.DrainEvents()
	assert.True(t, drained.EmitCommand || drained.LastShell != nil || drained.CommandsRun >= 1)

	// Unsupported input.
	require.Error(t, r.HandleInput(ctx, activities.Input{Type: activities.InputKey, Rune: 'x'}))

	// Encode/restore with shell state + empty/invalid restore.
	raw, err := r.EncodeState()
	require.NoError(t, err)
	require.NoError(t, r.RestoreState(nil))
	require.NoError(t, r.RestoreState([]byte{}))
	require.Error(t, r.RestoreState([]byte("{")))
	require.Error(t, r.RestoreState([]byte(`{"v":9}`)))
	require.NoError(t, r.RestoreState(raw))

	// Restore with relative cwd.
	var st map[string]any
	require.NoError(t, json.Unmarshal(raw, &st))
	st["relCwd"] = "home/docs"
	st["v"] = 1
	raw2, err := json.Marshal(st)
	require.NoError(t, err)
	require.NoError(t, r.RestoreState(raw2))
	assert.Equal(t, filepath.Join(ws, "home", "docs"), r.Cwd())

	require.NoError(t, r.Verify(ctx))
	assert.True(t, r.CompleteReady())
	assert.NotEmpty(t, r.Observations())

	// SetCwd missing path error.
	require.Error(t, r.SetCwd(filepath.Join(ws, "missing-dir")))

	require.NoError(t, r.Close())
	// Restore after close / not open.
	r2 := activities.NewTerminal()
	require.Error(t, r2.RestoreState(raw))
}

func TestTerminalRunnerMaterializeFailure(t *testing.T) {
	t.Parallel()
	// Workspace path that cannot be written (file as parent).
	dir := t.TempDir()
	block := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(block, []byte("x"), 0o644))
	r := activities.NewTerminal()
	err := r.Open(context.Background(), activities.OpenOpts{
		Workspace: block, // not a dir
		Content: contracts.ActivityContent{
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				Fixtures:       []contracts.FixtureEntry{{Path: "a", Type: contracts.FixtureFile, Content: "x"}},
			},
		},
	})
	require.Error(t, err)
}
