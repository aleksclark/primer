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

func TestTerminalRunnerHistoryPredicates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "readme.txt"), []byte("hi\n"), 0o644))

	content := contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			Fixtures: []contracts.FixtureEntry{
				{Path: "readme.txt", Type: contracts.FixtureFile, Content: "hi\n"},
			},
		},
		Tasks: []contracts.Task{{
			ID: "t1", Title: "list", Instructions: "ls",
			Completion: contracts.CheckTree{CheckID: "did-ls"},
		}},
		Checks: []contracts.Check{
			{ID: "did-ls", Kind: contracts.CheckCommandProperties, Params: map[string]any{
				"executable": "ls", "exitCode": 0,
			}},
			{ID: "has-readme", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "readme.txt"}},
		},
	}

	runShell := func(ctx context.Context, workspace, cwd, line string) (string, string, int, error) {
		if line == "ls" {
			return "readme.txt\n", "", 0, nil
		}
		if line == "pwd" {
			return cwd + "\n", "", 0, nil
		}
		return "", "unknown", 1, nil
	}

	r := activities.NewTerminal()
	require.NoError(t, r.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: content, RunShell: runShell, SkipMaterialize: true,
	}))

	// pwd first — should not satisfy ls check.
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "pwd"}))
	assert.False(t, r.CompleteReady())

	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "ls"}))
	// History should include ls even if a later command runs.
	require.NoError(t, r.HandleInput(ctx, activities.Input{Type: activities.InputCommand, Line: "pwd"}))
	assert.True(t, r.CompleteReady(), "ls in history should satisfy command_properties")

	hist := r.History()
	require.GreaterOrEqual(t, len(hist.Events), 2)
	assert.True(t, hist.Events[0].Structured)

	// Encode/restore preserves history.
	raw, err := r.EncodeState()
	require.NoError(t, err)
	require.NoError(t, r.Close())

	r2 := activities.NewTerminal()
	require.NoError(t, r2.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: content, RunShell: runShell, SkipMaterialize: true,
	}))
	require.NoError(t, r2.RestoreState(raw))
	assert.True(t, r2.CompleteReady())
	assert.GreaterOrEqual(t, len(r2.History().Events), 2)
}

func TestTerminalRunnerRejectsPTYShellAsStructured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()
	content := contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			Fixtures:       []contracts.FixtureEntry{{Path: "f", Type: contracts.FixtureDirectory}},
		},
		Checks: []contracts.Check{{
			ID: "cmd", Kind: contracts.CheckCommandProperties,
			Params: map[string]any{"executable": "ls", "exitCode": 0},
		}},
	}
	r := activities.NewTerminal()
	require.NoError(t, r.Open(ctx, activities.OpenOpts{
		Workspace: ws, Content: content, RunShell: func(context.Context, string, string, string) (string, string, int, error) {
			return "", "", 0, nil
		},
		SkipMaterialize: true,
	}))
	require.NoError(t, r.HandleInput(ctx, activities.Input{
		Type: activities.InputShellResult,
		Shell: &activities.ShellResult{
			Executable: "pty-shell", ExitCode: 0, Stdout: "screen dump",
			CountCommand: true, Structured: false, Source: "pty-shell",
		},
	}))
	assert.False(t, r.CompleteReady())
	for _, o := range r.Observations() {
		if o.CheckID == "cmd" {
			assert.False(t, o.Passed)
			assert.Contains(t, o.Message, "structured command evidence unavailable")
		}
	}
}
