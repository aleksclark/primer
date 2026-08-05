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

func TestMaterializeAndVerify(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fixtures := []contracts.FixtureEntry{
		{Path: "home", Type: contracts.FixtureDirectory},
		{Path: "home/welcome.txt", Type: contracts.FixtureFile, Content: "hello\n"},
		{Path: "home/docs", Type: contracts.FixtureDirectory},
		{Path: "home/docs/guide.txt", Type: contracts.FixtureFile, Content: "read me\n", Mode: "0644"},
		{Path: "inbox", Type: contracts.FixtureDirectory},
		{Path: "inbox/letter.txt", Type: contracts.FixtureFile, Content: "dear student\n"},
	}
	require.NoError(t, terminal.Materialize(root, fixtures))

	b, err := os.ReadFile(filepath.Join(root, "home", "welcome.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(b))

	checks := []contracts.Check{
		{ID: "welcome", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "home/welcome.txt"}},
		{ID: "no-secret", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "home/secret"}},
		{ID: "eq", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/welcome.txt", "value": "hello\n"}},
		{ID: "contains", Kind: contracts.CheckContentContains, Params: map[string]any{"path": "inbox/letter.txt", "value": "student"}},
		{ID: "match", Kind: contracts.CheckContentMatch, Params: map[string]any{"path": "home/docs/guide.txt", "pattern": `read\s+me`}},
		{ID: "type-dir", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home/docs", "type": contracts.PathTypeDirectory}},
		{ID: "type-file", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home/docs/guide.txt", "type": contracts.PathTypeFile}},
		{ID: "mode", Kind: contracts.CheckPathMode, Params: map[string]any{"path": "home/docs/guide.txt", "mode": "0644"}},
	}
	obs := terminal.VerifyAll(root, checks, nil)
	for _, o := range obs {
		assert.True(t, o.Passed, "%s: %s", o.CheckID, o.Message)
	}

	// Simulate student organizing files.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
	require.NoError(t, os.Rename(
		filepath.Join(root, "inbox", "letter.txt"),
		filepath.Join(root, "archive", "letter.txt"),
	))
	orgChecks := []contracts.Check{
		{ID: "moved", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "archive/letter.txt"}},
		{ID: "gone", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "inbox/letter.txt"}},
		{ID: "body", Kind: contracts.CheckContentContains, Params: map[string]any{"path": "archive/letter.txt", "value": "dear"}},
	}
	for _, o := range terminal.VerifyAll(root, orgChecks, nil) {
		assert.True(t, o.Passed, "%s: %s", o.CheckID, o.Message)
	}
}

func TestVerifyCwdAndCommand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, terminal.Materialize(root, []contracts.FixtureEntry{
		{Path: "home/docs", Type: contracts.FixtureDirectory},
	}))
	shell := &terminal.ShellState{
		Cwd:                       filepath.Join(root, "home", "docs"),
		Executable:                "ls",
		Args:                      []string{"-la"},
		ExitCode:                  0,
		Stdout:                    "guide.txt\n",
		StructuredCommandEvidence: true,
		Source:                    "structured",
	}
	checks := []contracts.Check{
		{ID: "cwd", Kind: contracts.CheckCwd, Params: map[string]any{"path": "home/docs"}},
		{ID: "cmd", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "ls",
			"args":       []any{"-la"},
			"exitCode":   0,
		}},
		{ID: "out", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"contains": "guide.txt"}},
	}
	for _, o := range terminal.VerifyAll(root, checks, shell) {
		assert.True(t, o.Passed, "%s: %s", o.CheckID, o.Message)
	}
}

func TestMaterializeRejectsEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := terminal.Materialize(root, []contracts.FixtureEntry{
		{Path: "../escape", Type: contracts.FixtureFile, Content: "x"},
	})
	require.Error(t, err)
}

func TestEvalTree(t *testing.T) {
	t.Parallel()
	byID := map[string]contracts.Observation{
		"a": {CheckID: "a", Passed: true},
		"b": {CheckID: "b", Passed: false, Message: "nope"},
		"c": {CheckID: "c", Passed: false, Optional: true, Message: "optional miss"},
	}
	ok, _ := terminal.EvalTree(contracts.CheckTree{
		All: []contracts.CheckTree{
			{CheckID: "a"},
			{CheckID: "c"},
		},
	}, byID)
	assert.True(t, ok)

	ok, msg := terminal.EvalTree(contracts.CheckTree{
		All: []contracts.CheckTree{
			{CheckID: "a"},
			{CheckID: "b"},
		},
	}, byID)
	assert.False(t, ok)
	assert.Contains(t, msg, "nope")

	ok, _ = terminal.EvalTree(contracts.CheckTree{
		Any: []contracts.CheckTree{
			{CheckID: "b"},
			{CheckID: "a"},
		},
	}, byID)
	assert.True(t, ok)
}
