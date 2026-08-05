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

func TestTerminalRunnerWorkspaceSetCwdAndShell(t *testing.T) {
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
			}},
		},
		Digest:          "d1",
		Workspace:       ws,
		SkipMaterialize: true,
		RunShell: func(ctx context.Context, workspace, cwd, line string) (string, string, int, error) {
			return "ok\n", "", 0, nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, ws, r.Workspace())
	assert.Equal(t, filepath.Join(ws, "home"), r.Cwd())

	// Valid SetCwd into docs
	require.NoError(t, r.SetCwd(filepath.Join(ws, "home", "docs")))
	assert.Equal(t, filepath.Join(ws, "home", "docs"), r.Cwd())
	// Outside workspace rejected
	require.Error(t, r.SetCwd("/tmp"))
	// File rejected
	require.Error(t, r.SetCwd(filepath.Join(ws, "home", "a.txt")))

	// Command input exercises shell path
	require.NoError(t, r.HandleInput(context.Background(), activities.Input{
		Type: activities.InputCommand, Line: "echo ok",
	}))
	snap := r.Snapshot()
	assert.GreaterOrEqual(t, snap.CommandsRun, 1)
	assert.Contains(t, snap.LastOutput, "ok")

	require.NoError(t, r.Verify(context.Background()))
	// Fixture home/a.txt satisfies the exists check → required checks ready.
	assert.True(t, r.CompleteReady())
	assert.GreaterOrEqual(t, snap.ChecksTotal, 1)
	require.NoError(t, r.Close())
}
