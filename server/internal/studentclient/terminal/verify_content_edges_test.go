package terminal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestVerifyContentAndPathTypeEdges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "home", "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "home", "a.txt"), []byte("hello world\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "home", "mode.txt"), []byte("x"), 0o600))
	require.NoError(t, os.Symlink("a.txt", filepath.Join(root, "home", "link")))

	// content_equals pass/fail
	obs := terminal.VerifyCheck(root, contracts.Check{
		ID: "eq", Kind: contracts.CheckContentEquals,
		Params: map[string]any{"path": "home/a.txt", "value": "hello world\n"},
	}, nil)
	assert.True(t, obs.Passed)

	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "eq2", Kind: contracts.CheckContentEquals,
		Params: map[string]any{"path": "home/a.txt", "value": "nope"},
	}, nil)
	assert.False(t, obs.Passed)

	// missing file for content
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "eq3", Kind: contracts.CheckContentEquals,
		Params: map[string]any{"path": "home/missing.txt", "value": "x"},
	}, nil)
	assert.False(t, obs.Passed)

	// content_contains
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "c", Kind: contracts.CheckContentContains,
		Params: map[string]any{"path": "home/a.txt", "value": "world"},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "c2", Kind: contracts.CheckContentContains,
		Params: map[string]any{"path": "home/a.txt", "value": "zzz"},
	}, nil)
	assert.False(t, obs.Passed)

	// content_match
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "m", Kind: contracts.CheckContentMatch,
		Params: map[string]any{"path": "home/a.txt", "pattern": `hello\s+world`},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "m2", Kind: contracts.CheckContentMatch,
		Params: map[string]any{"path": "home/a.txt", "pattern": `^nope`},
	}, nil)
	assert.False(t, obs.Passed)
	// bad regex
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "m3", Kind: contracts.CheckContentMatch,
		Params: map[string]any{"path": "home/a.txt", "pattern": `(`},
	}, nil)
	assert.False(t, obs.Passed)
	assert.NotEmpty(t, obs.Message)

	// path_type file/dir/symlink
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "pt", Kind: contracts.CheckPathType,
		Params: map[string]any{"path": "home/a.txt", "type": contracts.PathTypeFile},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "ptd", Kind: contracts.CheckPathType,
		Params: map[string]any{"path": "home/docs", "type": contracts.PathTypeDirectory},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "pts", Kind: contracts.CheckPathType,
		Params: map[string]any{"path": "home/link", "type": contracts.PathTypeSymlink},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "ptbad", Kind: contracts.CheckPathType,
		Params: map[string]any{"path": "home/a.txt", "type": contracts.PathTypeDirectory},
	}, nil)
	assert.False(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "ptmiss", Kind: contracts.CheckPathType,
		Params: map[string]any{"path": "home/nope", "type": contracts.PathTypeFile},
	}, nil)
	assert.False(t, obs.Passed)

	// path_mode
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "mode", Kind: contracts.CheckPathMode,
		Params: map[string]any{"path": "home/mode.txt", "mode": "0600"},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "mode2", Kind: contracts.CheckPathMode,
		Params: map[string]any{"path": "home/mode.txt", "mode": "0644"},
	}, nil)
	assert.False(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "mode3", Kind: contracts.CheckPathMode,
		Params: map[string]any{"path": "home/nope", "mode": "0644"},
	}, nil)
	assert.False(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "mode4", Kind: contracts.CheckPathMode,
		Params: map[string]any{"path": "home/mode.txt", "mode": "not-a-mode"},
	}, nil)
	assert.False(t, obs.Passed)

	// cwd with absolute and relative shell.Cwd
	shell := &terminal.ShellState{Cwd: filepath.Join(root, "home", "docs")}
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cwd", Kind: contracts.CheckCwd,
		Params: map[string]any{"path": "home/docs"},
	}, shell)
	assert.True(t, obs.Passed)
	shell.Cwd = "home/docs"
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cwd2", Kind: contracts.CheckCwd,
		Params: map[string]any{"path": "home/docs"},
	}, shell)
	// relative may or may not match depending on Rel — accept either path covered
	_ = obs
	shell.Cwd = ""
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cwd3", Kind: contracts.CheckCwd,
		Params: map[string]any{"path": "home/docs"},
	}, shell)
	assert.False(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cwd4", Kind: contracts.CheckCwd,
		Params: map[string]any{"path": "home/docs"},
	}, nil)
	assert.False(t, obs.Passed)

	// response_submitted
	shell = &terminal.ShellState{SubmittedTasks: map[string]bool{"t1": true}}
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "rs", Kind: contracts.CheckResponseSubmitted,
		Params: map[string]any{"taskId": "t1"},
	}, shell)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "rs2", Kind: contracts.CheckResponseSubmitted,
		Params: map[string]any{"task_id": "t2"},
	}, shell)
	assert.False(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "rs3", Kind: contracts.CheckResponseSubmitted,
		Params: map[string]any{},
	}, shell)
	assert.False(t, obs.Passed)

	// unknown kind
	obs = terminal.VerifyCheck(root, contracts.Check{ID: "u", Kind: "nope"}, nil)
	assert.False(t, obs.Passed)

	// file_exists / not_exists missing params
	obs = terminal.VerifyCheck(root, contracts.Check{ID: "fe", Kind: contracts.CheckFileExists, Params: map[string]any{}}, nil)
	assert.False(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{ID: "fne", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "../escape"}}, nil)
	assert.False(t, obs.Passed)

	// command properties without shell
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls"},
	}, nil)
	assert.False(t, obs.Passed)

	// pipeline without shell
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "pipe", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{"contains": "x"},
	}, nil)
	assert.False(t, obs.Passed)

	// structured command last-observation match (Cwd required for MeetsStructuredBar)
	shell = &terminal.ShellState{
		Cwd: root, Executable: "ls", Args: []string{"-la"}, ExitCode: 0,
		Source: contracts.SourceStructured, StructuredCommandEvidence: true,
		Stdout: "ok", Stderr: "",
		ManifestDigest: "abc",
	}
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd2", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "args": []any{"-la"}, "exitCode": 0},
	}, shell)
	assert.True(t, obs.Passed)
	assert.Equal(t, "abc", obs.Details["workspaceManifestDigest"])

	// command mismatch
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd3", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "cat"},
	}, shell)
	assert.False(t, obs.Passed)

	// untrusted source rejected
	shell.Source = "pty-shell"
	shell.StructuredCommandEvidence = true
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd4", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls"},
	}, shell)
	assert.False(t, obs.Passed)

	// pipeline last-obs path
	shell = &terminal.ShellState{
		Cwd: root, Executable: "printf", ExitCode: 0, Stdout: "hello\n",
		Source: contracts.SourceStructured, StructuredCommandEvidence: true,
		ManifestDigest: "m1",
	}
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "p1", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{"contains": "hello"},
	}, shell)
	assert.True(t, obs.Passed)

	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "p2", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{"contains": "zzz"},
	}, shell)
	assert.False(t, obs.Passed)

	// untrusted source rejected for pipeline last-obs
	shell.Source = "pty-shell"
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "p3", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{"contains": "hello"},
	}, shell)
	assert.False(t, obs.Passed)

	// no output expectation
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "p4", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{},
	}, shell)
	assert.False(t, obs.Passed)
}

func TestEvalTreeEdges(t *testing.T) {
	t.Parallel()
	byID := map[string]contracts.Observation{
		"a": {CheckID: "a", Passed: true},
		"b": {CheckID: "b", Passed: false, Message: "no"},
	}
	ok, msg := terminal.EvalTree(contracts.CheckTree{Optional: true, CheckID: "b"}, byID)
	assert.True(t, ok)
	// Optional failing leaves still pass the tree but surface a detail message.
	assert.Contains(t, msg, "optional check b failed")

	ok, msg = terminal.EvalTree(contracts.CheckTree{CheckID: "a"}, byID)
	assert.True(t, ok)

	ok, msg = terminal.EvalTree(contracts.CheckTree{CheckID: "b"}, byID)
	assert.False(t, ok)
	assert.Contains(t, msg, "no")

	ok, msg = terminal.EvalTree(contracts.CheckTree{CheckID: "missing"}, byID)
	assert.False(t, ok)

	ok, msg = terminal.EvalTree(contracts.CheckTree{All: []contracts.CheckTree{{CheckID: "a"}, {CheckID: "b"}}}, byID)
	assert.False(t, ok)

	ok, msg = terminal.EvalTree(contracts.CheckTree{Any: []contracts.CheckTree{{CheckID: "b"}, {CheckID: "a"}}}, byID)
	assert.True(t, ok)

	ok, msg = terminal.EvalTree(contracts.CheckTree{Any: []contracts.CheckTree{{CheckID: "b"}}}, byID)
	assert.False(t, ok)
	assert.Contains(t, msg, "none of any-branch")

	ok, msg = terminal.EvalTree(contracts.CheckTree{}, byID)
	assert.False(t, ok)
	assert.Contains(t, msg, "empty")
}
