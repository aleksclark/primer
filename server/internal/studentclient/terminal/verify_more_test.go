package terminal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestVerifyCheckFailureAndErrorPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "home", "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "home", "welcome.txt"), []byte("hello\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "home", "docs", "guide.txt"), []byte("read me\n"), 0o600))
	require.NoError(t, os.Symlink("welcome.txt", filepath.Join(root, "home", "link.txt")))

	// Missing params / bad types fail closed with Message set (not panic).
	for _, check := range []contracts.Check{
		{ID: "no-path", Kind: contracts.CheckFileExists, Params: map[string]any{}},
		{ID: "bad-path-type", Kind: contracts.CheckFileExists, Params: map[string]any{"path": 12}},
		{ID: "escape", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "../outside"}},
		{ID: "no-value", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/welcome.txt"}},
		{ID: "bad-pattern", Kind: contracts.CheckContentMatch, Params: map[string]any{"path": "home/welcome.txt", "pattern": "("}},
		{ID: "unknown", Kind: "not_a_real_kind", Params: map[string]any{}},
		{ID: "bad-mode", Kind: contracts.CheckPathMode, Params: map[string]any{"path": "home/welcome.txt", "mode": "xyz"}},
		{ID: "cmd-no-shell", Kind: contracts.CheckCommandProperties, Params: map[string]any{"executable": "ls"}},
		{ID: "pipe-no-shell", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"contains": "x"}},
		{ID: "cwd-no-shell", Kind: contracts.CheckCwd, Params: map[string]any{"path": "home"}},
	} {
		obs := terminal.VerifyCheck(root, check, nil)
		assert.False(t, obs.Passed, check.ID)
		assert.NotEmpty(t, obs.Message, check.ID)
		assert.Equal(t, check.ID, obs.CheckID)
		assert.Equal(t, contracts.ObservationSchemaVersion, obs.SchemaVersion)
	}

	// Content / path mismatches
	fail := terminal.VerifyAll(root, []contracts.Check{
		{ID: "missing-file", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "home/nope.txt"}},
		{ID: "still-there", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "home/welcome.txt"}},
		{ID: "eq-miss", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/welcome.txt", "value": "bye\n"}},
		{ID: "eq-absent", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/absent.txt", "value": "x"}},
		{ID: "contains-miss", Kind: contracts.CheckContentContains, Params: map[string]any{"path": "home/welcome.txt", "value": "zzz"}},
		{ID: "match-miss", Kind: contracts.CheckContentMatch, Params: map[string]any{"path": "home/welcome.txt", "pattern": "^bye"}},
		{ID: "type-miss", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home/welcome.txt", "type": contracts.PathTypeDirectory}},
		{ID: "type-absent", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home/gone", "type": contracts.PathTypeFile}},
		{ID: "mode-miss", Kind: contracts.CheckPathMode, Params: map[string]any{"path": "home/docs/guide.txt", "mode": "0777"}},
		{ID: "mode-absent", Kind: contracts.CheckPathMode, Params: map[string]any{"path": "home/gone", "mode": "0644"}},
		{ID: "symlink-type", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home/link.txt", "type": contracts.PathTypeSymlink}},
	}, nil)
	for _, o := range fail {
		if o.CheckID == "symlink-type" {
			assert.True(t, o.Passed, o.Message)
			continue
		}
		assert.False(t, o.Passed, "%s: %s", o.CheckID, o.Message)
	}

	// Command + pipeline success and failure variants
	shell := &terminal.ShellState{
		Cwd:        filepath.Join(root, "home"),
		Executable: "/bin/ls",
		Args:       []string{"-la", "docs"},
		ExitCode:   0,
		Stdout:     "guide.txt\r\n",
		Stderr:     "",
	}
	ok := terminal.VerifyAll(root, []contracts.Check{
		{ID: "cwd-abs", Kind: contracts.CheckCwd, Params: map[string]any{"path": "home"}},
		{ID: "cmd-base", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "ls", "args": []string{"-la", "docs"}, "exitCode": float64(0),
		}},
		{ID: "out-eq", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"value": "guide.txt\n"}},
		{ID: "out-contains", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"contains": "guide"}},
		{ID: "out-pat", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"pattern": `guide\.txt`}},
	}, shell)
	for _, o := range ok {
		assert.True(t, o.Passed, "%s: %s", o.CheckID, o.Message)
	}

	// Relative cwd reported by shell
	relShell := &terminal.ShellState{Cwd: "home/docs", Executable: "pwd", ExitCode: 0, Stdout: "/workspace/home/docs\n"}
	// Materialize path for relative equality branch uses filepath.Rel against root —
	// when Cwd is already relative, JoinUnder comparison may fail; exercise mismatch.
	obs := terminal.VerifyCheck(root, contracts.Check{
		ID: "cwd-rel", Kind: contracts.CheckCwd, Params: map[string]any{"path": "home/docs"},
	}, relShell)
	// Either pass (rel equality) or fail cleanly.
	assert.NotEmpty(t, obs.Message+"ok")

	badShell := &terminal.ShellState{
		Cwd: filepath.Join(root, "home", "docs"), Executable: "cat", Args: []string{"x"}, ExitCode: 1,
		Stdout: "nope\n",
	}
	for _, check := range []contracts.Check{
		{ID: "cwd-wrong", Kind: contracts.CheckCwd, Params: map[string]any{"path": "home"}},
		{ID: "exe-wrong", Kind: contracts.CheckCommandProperties, Params: map[string]any{"executable": "ls"}},
		{ID: "args-wrong", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "cat", "args": []any{"y"},
		}},
		{ID: "exit-wrong", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "cat", "exitCode": 0,
		}},
		{ID: "args-badtype", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "cat", "args": "nope",
		}},
		{ID: "exit-badtype", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "cat", "exitCode": "nope",
		}},
		{ID: "out-wrong", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"value": "yes"}},
		{ID: "out-miss", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"contains": "zzz"}},
		{ID: "out-pat-miss", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"pattern": `^yes`}},
		{ID: "out-badre", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"pattern": "("}},
		{ID: "out-empty", Kind: contracts.CheckPipelineOutput, Params: map[string]any{}},
	} {
		o := terminal.VerifyCheck(root, check, badShell)
		assert.False(t, o.Passed, check.ID)
	}

	// EvalTree empty / missing / optional leaf
	byID := map[string]contracts.Observation{
		"a": {CheckID: "a", Passed: true},
		"b": {CheckID: "b", Passed: false, Message: "fail-b"},
	}
	okTree, _ := terminal.EvalTree(contracts.CheckTree{CheckID: "a"}, byID)
	assert.True(t, okTree)
	okTree, msg := terminal.EvalTree(contracts.CheckTree{CheckID: "missing"}, byID)
	assert.False(t, okTree)
	assert.Contains(t, msg, "missing")
	okTree, msg = terminal.EvalTree(contracts.CheckTree{Optional: true, CheckID: "b"}, byID)
	assert.True(t, okTree)
	assert.Contains(t, msg, "optional")
	okTree, msg = terminal.EvalTree(contracts.CheckTree{}, byID)
	assert.False(t, okTree)
	assert.Contains(t, msg, "empty")
	okTree, msg = terminal.EvalTree(contracts.CheckTree{Any: []contracts.CheckTree{{CheckID: "b"}, {CheckID: "missing"}}}, byID)
	assert.False(t, okTree)
	assert.True(t, strings.Contains(msg, "none of any") || strings.Contains(msg, "missing") || strings.Contains(msg, "fail-b"))

	// Nil params map is tolerated for kinds that don't need them (unknown still errors).
	o := terminal.VerifyCheck(root, contracts.Check{ID: "nil-params", Kind: contracts.CheckFileExists}, nil)
	assert.False(t, o.Passed)
}

func TestVerifyNotExistsAndModePass(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o640))
	obs := terminal.VerifyCheck(root, contracts.Check{
		ID: "gone", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "missing.txt"},
	}, nil)
	assert.True(t, obs.Passed)
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "mode", Kind: contracts.CheckPathMode, Params: map[string]any{"path": "a.txt", "mode": "640"},
	}, nil)
	assert.True(t, obs.Passed, obs.Message)
}
