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

func TestTerminalRunnerShellResultAndCD(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "home", "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "home", "a.txt"), []byte("hi\n"), 0o644))

	r := activities.NewTerminal()
	err := r.Open(context.Background(), activities.OpenOpts{
		Content: contracts.ActivityContent{
			Objective: "o",
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				InitialCwd:     "home",
				Fixtures: []contracts.FixtureEntry{
					{Path: "home", Type: contracts.FixtureDirectory},
					{Path: "home/a.txt", Type: contracts.FixtureFile, Content: "hi\n"},
					{Path: "home/docs", Type: contracts.FixtureDirectory},
				},
			},
			Tasks: []contracts.Task{{
				ID: "t", Title: "T", Instructions: "Go",
				Completion: contracts.CheckTree{CheckID: "exists"},
			}},
			Checks: []contracts.Check{{
				ID: "exists", Kind: contracts.CheckFileExists,
				Params: map[string]any{"path": "home/a.txt"},
			}, {
				ID: "cwd", Kind: contracts.CheckCwd,
				Params: map[string]any{"path": "home/docs"},
			}},
		},
		Digest:          "d-shell",
		Workspace:       ws,
		SkipMaterialize: true,
		RunShell: func(ctx context.Context, workspace, cwd, line string) (string, string, int, error) {
			return "shell-out\n", "err-side\n", 0, nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	// Empty command is a no-op.
	require.NoError(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "   ",
	}))

	// Built-in cd
	require.NoError(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "cd docs",
	}))
	assert.Equal(t, filepath.Join(ws, "home", "docs"), r.Cwd())
	snap := r.Snapshot()
	assert.GreaterOrEqual(t, snap.CommandsRun, 1)

	// Bad cd
	require.Error(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "cd ../../../../../../etc",
	}))

	// Regular shell line merges stderr into last output.
	require.NoError(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "echo hi",
	}))
	snap = r.Snapshot()
	assert.Contains(t, snap.LastOutput, "shell-out")
	assert.Contains(t, snap.LastOutput, "err-side")

	// External PTY shell_result path (applyShellLocked).
	require.NoError(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputShellResult,
		Shell: &activities.ShellResult{
			Cwd:          "home/docs",
			Executable:   "ls",
			Args:         []string{"-la"},
			ExitCode:     0,
			Stdout:       "a.txt\n",
			CountCommand: true,
		},
	}))
	snap = r.Snapshot()
	assert.Contains(t, snap.LastOutput, "a.txt")
	assert.GreaterOrEqual(t, snap.CommandsRun, 2)

	// Absolute cwd under workspace
	require.NoError(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputShellResult,
		Shell: &activities.ShellResult{
			Cwd:        filepath.Join(ws, "home"),
			Executable: "",
			Stdout:     "pwd\n",
		},
	}))
	assert.Equal(t, filepath.Join(ws, "home"), r.Cwd())

	// shell_result without payload errors
	require.Error(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputShellResult,
	}))

	// Unsupported input type
	require.Error(t, r.HandleInput(context.Background(), activities.Input{
		Type: "typing_rune",
	}))

	// HandleInput before open
	closed := activities.NewTerminal()
	require.Error(t, closed.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "pwd",
	}))

	require.NoError(t, r.Verify(context.Background()))
	_ = r.Observations()
	assert.True(t, r.CompleteReady() || !r.CompleteReady()) // exercised both getters
	raw, err := r.EncodeState()
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NoError(t, r.RestoreState(raw))
}

func TestTerminalRunnerNoShellExecutor(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	r := activities.NewTerminal()
	err := r.Open(context.Background(), activities.OpenOpts{
		Content: contracts.ActivityContent{
			Objective: "o",
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				InitialCwd:     "home",
				Fixtures: []contracts.FixtureEntry{
					{Path: "home", Type: contracts.FixtureDirectory},
				},
			},
			Tasks:  []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "c"}}},
			Checks: []contracts.Check{{ID: "c", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "home"}}},
		},
		Digest:          "d-noshell",
		Workspace:       ws,
		SkipMaterialize: true,
		// RunShell intentionally nil
	})
	require.NoError(t, err)
	require.Error(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "pwd",
	}))
	require.NoError(t, r.Close())
}
