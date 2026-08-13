package terminal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestVerifyHistoryScopedCommandMatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	hist := &terminal.History{}
	hist.MarkTaskStart(0, 1)
	hist.MarkTaskStart(1, 10)
	hist.Append(contracts.CommandObservation{
		SchemaVersion: "1",
		Sequence:      2,
		TaskIndex:     0,
		Executable:    "ls",
		Argv:          []string{"-la"},
		ArgvAvailable: true,
		ExitCode:      0,
		ExitAvailable: true,
		CwdAfter:      "/ws",
		CwdAvailable:  true,
		Structured:    true,
		Source:        contracts.SourceStructured,
		Quality:       contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true},
		Stdout:        contracts.Excerpt{Text: "a.txt\n", Trusted: true, ByteLen: 6},
	})
	hist.Append(contracts.CommandObservation{
		SchemaVersion: "1",
		Sequence:      11,
		TaskIndex:     1,
		Executable:    "cat",
		Argv:          []string{"a.txt"},
		ArgvAvailable: true,
		ExitCode:      0,
		ExitAvailable: true,
		CwdAvailable:  true,
		Structured:    true,
		Source:        contracts.SourceStructured,
		Quality:       contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true},
		Stdout:        contracts.Excerpt{Text: "hello", Trusted: true, ByteLen: 5},
	})

	// Shell on task 1 should find cat in active window.
	shell := &terminal.ShellState{
		History:   hist,
		TaskIndex: 1,
	}
	obs := terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-hist", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{
			"executable":     "cat",
			"args":           []any{"a.txt"},
			"exitCode":       float64(0),
			"stdoutContains": "hello",
		},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)

	// Fall back to earlier task when current window misses.
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-prior", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "args": []any{"-la"}},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)

	// Task index past end still searches recorded starts.
	shell.TaskIndex = 5
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-past", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "cat"},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)

	// No history → fail.
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-none", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls"},
	}, &terminal.ShellState{TaskIndex: 0})
	assert.False(t, obs.Passed)

	// Missing executable in history.
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-empty-hist", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "zzz"},
	}, shell)
	assert.False(t, obs.Passed)

	// Stream predicates.
	shell.TaskIndex = 1
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-out-eq", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{
			"executable":   "cat",
			"stdoutEquals": "hello",
		},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)

	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-out-pat", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{
			"executable":    "cat",
			"stdoutPattern": `^he`,
		},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)

	// Require success default (exitCode omitted) with exit 0 in history.
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd-success", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "cat"},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)

	// Pipeline with shell stdout (and history structured check path).
	shell2 := &terminal.ShellState{
		History:                   hist,
		TaskIndex:                 0,
		Executable:                "ls",
		Args:                      []string{"-la"},
		ExitCode:                  0,
		Stdout:                    "a.txt\n",
		StructuredCommandEvidence: true,
		Source:                    "structured",
	}
	obs = terminal.VerifyCheck(root, contracts.Check{
		ID: "pipe-ok", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{"contains": "a.txt"},
	}, shell2)
	assert.True(t, obs.Passed, obs.Message)
}
