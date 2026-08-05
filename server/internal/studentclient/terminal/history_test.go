package terminal_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestHistoryTaskScopedMatch(t *testing.T) {
	t.Parallel()
	h := &terminal.History{TaskStartSeq: map[int]int64{0: 1, 1: 3}}
	h.Append(contracts.CommandObservation{
		Sequence: 1, TaskIndex: 0, Executable: "pwd", ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source: contracts.SourceStructured,
		Quality: contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true},
		CwdAfter: ".", CwdAvailable: true,
		RecordedAt: time.Now().UTC(),
	})
	h.Append(contracts.CommandObservation{
		Sequence: 2, TaskIndex: 0, Executable: "ls", Argv: []string{"-la"}, ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source: contracts.SourceObserveBash,
		Quality: contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true},
		CwdAfter: "docs", CwdAvailable: true,
		Stdout: contracts.Excerpt{Text: "guide.txt\n", Trusted: false},
		RecordedAt: time.Now().UTC(),
	})
	h.Append(contracts.CommandObservation{
		Sequence: 3, TaskIndex: 1, Executable: "cat", Argv: []string{"guide.txt"}, ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source: contracts.SourceStructured,
		Quality: contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true},
		Stdout: contracts.Excerpt{Text: "hello\n", Trusted: true},
		RecordedAt: time.Now().UTC(),
	})

	// Since task start: task 0 (seq>=1) sees ls; task 1 (seq>=3) does not see earlier ls.
	zero := 0
	hit, ok := h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", Args: []string{"-la"}, ArgsSet: true,
		ExitCode: &zero, RequireStructured: true,
	})
	require.True(t, ok)
	assert.Equal(t, int64(2), hit.Sequence)

	_, ok = h.FindMatch(1, terminal.CommandMatch{
		Executable: "ls", RequireStructured: true, RequireSuccess: true,
	})
	assert.False(t, ok, "ls was before task 1 start")

	hit, ok = h.FindMatch(1, terminal.CommandMatch{
		Executable: "cat", RequireStructured: true, RequireSuccess: true,
		StdoutContains: "hello", RequireStdoutTrusted: true,
	})
	require.True(t, ok)
	assert.Equal(t, "cat", hit.Executable)
}

func TestHistoryRejectsScreenScrape(t *testing.T) {
	t.Parallel()
	h := &terminal.History{}
	h.Append(contracts.CommandObservation{
		Sequence: 1, Executable: "ls", ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true,
		Structured: false, Source: contracts.SourcePTYShell,
		Stdout: contracts.Excerpt{Text: "fake from screen", Trusted: false},
		Quality: contracts.EvidenceQuality{},
	})
	_, ok := h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", RequireStructured: true, RequireSuccess: true,
	})
	assert.False(t, ok)
}

func TestVerifyCommandPropertiesUsesHistory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	h := &terminal.History{TaskStartSeq: map[int]int64{0: 1}}
	h.Append(contracts.CommandObservation{
		Sequence: 1, Executable: "ls", Argv: []string{"-la"}, ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source: contracts.SourceStructured,
		Quality: contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true},
		Stdout: contracts.Excerpt{Text: "a\n", Trusted: true},
		CwdAfter: ".", CwdAvailable: true,
	})
	// Latest shell is something else; history still has ls.
	shell := &terminal.ShellState{
		Cwd: ".", Executable: "pwd", ExitCode: 0,
		StructuredCommandEvidence: true, Source: contracts.SourceStructured,
		History: h, TaskIndex: 0,
	}
	obs := terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "args": []any{"-la"}, "exitCode": 0},
	}, shell)
	assert.True(t, obs.Passed, obs.Message)
	assert.Equal(t, contracts.CapStructuredCommandEvidence, obs.Details["capability"])
}

func TestVerifyRejectsScreenAsStdout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	shell := &terminal.ShellState{
		Cwd: ".", Executable: "pty-shell", ExitCode: 0,
		Stdout: "welcome.txt\n$ ",
		StructuredCommandEvidence: false, Source: contracts.SourcePTYShell,
	}
	obs := terminal.VerifyCheck(root, contracts.Check{
		ID: "cmd", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "exitCode": 0},
	}, shell)
	assert.False(t, obs.Passed)
	assert.Contains(t, obs.Message, "structured command evidence unavailable")

	obs2 := terminal.VerifyCheck(root, contracts.Check{
		ID: "out", Kind: contracts.CheckPipelineOutput,
		Params: map[string]any{"contains": "welcome"},
	}, shell)
	assert.False(t, obs2.Passed)
}

func TestManifestWriteSet(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("one"), 0o644))
	before, err := terminal.CaptureManifest(root)
	require.NoError(t, err)
	require.NotEmpty(t, before.Digest)

	require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("two"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("changed"), 0o644))
	after, err := terminal.CaptureManifest(root)
	require.NoError(t, err)

	ws := terminal.WriteSetDiff(before, after)
	assert.Contains(t, ws, "a.txt")
	assert.Contains(t, ws, "b.txt")
}

func TestBuildStructuredEventFromShC(t *testing.T) {
	t.Parallel()
	ev := terminal.BuildStructuredEvent(
		1, "s", ".", ".",
		"/bin/sh", []string{"-c", "ls -la"}, "ls -la",
		0, "x\n", "",
		"1", "2",
		contracts.WorkspaceManifest{Digest: "aaa"},
		contracts.WorkspaceManifest{Digest: "bbb"},
	)
	assert.True(t, ev.Structured)
	assert.Equal(t, "ls", ev.Executable)
	assert.Equal(t, []string{"-la"}, ev.Argv)
	assert.True(t, ev.Stdout.Trusted)
	assert.Equal(t, contracts.SourceStructured, ev.Source)
	assert.Equal(t, "aaa", ev.ManifestBefore)
	assert.Equal(t, "bbb", ev.ManifestAfter)
}
