package terminal_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestHistoryLastAndShellStateFromObservation(t *testing.T) {
	t.Parallel()
	var nilH *terminal.History
	assert.Nil(t, nilH.Last())

	h := &terminal.History{}
	assert.Nil(t, h.Last())

	h.Append(contracts.CommandObservation{
		Sequence: 1, Executable: "pwd", Argv: nil, ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source:   contracts.SourceStructured,
		Quality:  contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true},
		CwdAfter: ".", CwdAvailable: true,
		Stdout:     contracts.Excerpt{Text: "/ws\n", Trusted: true},
		RecordedAt: time.Now().UTC(),
	})
	h.Append(contracts.CommandObservation{
		Sequence: 2, Executable: "ls", Argv: []string{"-la"}, ArgvAvailable: true,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source:   contracts.SourceObserveBash,
		Quality:  contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true},
		CwdAfter: "docs", CwdAvailable: true,
		Stdout:     contracts.Excerpt{Text: "a\n", Trusted: false},
		Stderr:     contracts.Excerpt{Text: "", Trusted: true},
		RecordedAt: time.Now().UTC(),
	})

	last := h.Last()
	require.NotNil(t, last)
	assert.Equal(t, int64(2), last.Sequence)
	assert.Equal(t, "ls", last.Executable)
	assert.Equal(t, "docs", last.CwdAfter)

	// Last returns a copy — mutating the pointer must not alter history.
	last.Executable = "mutated"
	assert.Equal(t, "ls", h.Last().Executable)

	ss := terminal.ShellStateFromObservation(*h.Last())
	require.NotNil(t, ss)
	assert.Equal(t, "docs", ss.Cwd)
	assert.Equal(t, "ls", ss.Executable)
	assert.Equal(t, []string{"-la"}, ss.Args)
	assert.Equal(t, 0, ss.ExitCode)
	assert.Equal(t, "a\n", ss.Stdout)
	assert.Equal(t, contracts.SourceObserveBash, ss.Source)
	// Observe-bash quality without stdout trust still meets structured bar when Exit/Cwd/Argv set.
	assert.True(t, ss.StructuredCommandEvidence)

	// Non-structured observation maps StructuredCommandEvidence false.
	weak := contracts.CommandObservation{
		Sequence: 9, Executable: "x", ExitCode: 1, Source: contracts.SourcePTYShell,
		CwdAfter: "tmp", Stdout: contracts.Excerpt{Text: "scr"}, Stderr: contracts.Excerpt{Text: "e"},
		Argv: []string{"1"}, Quality: contracts.EvidenceQuality{},
	}
	ss2 := terminal.ShellStateFromObservation(weak)
	require.NotNil(t, ss2)
	assert.Equal(t, "tmp", ss2.Cwd)
	assert.Equal(t, 1, ss2.ExitCode)
	assert.Equal(t, "scr", ss2.Stdout)
	assert.Equal(t, "e", ss2.Stderr)
	assert.False(t, ss2.StructuredCommandEvidence)
	assert.Equal(t, contracts.SourcePTYShell, ss2.Source)
}

func TestHistoryMarkTaskStartIdempotent(t *testing.T) {
	t.Parallel()
	h := &terminal.History{}
	h.MarkTaskStart(0, 1)
	h.MarkTaskStart(0, 99) // first write wins
	h.MarkTaskStart(1, 5)
	assert.Equal(t, int64(1), h.TaskStartSeq[0])
	assert.Equal(t, int64(5), h.TaskStartSeq[1])

	h.Append(contracts.CommandObservation{Sequence: 1, Executable: "a"})
	h.Append(contracts.CommandObservation{Sequence: 5, Executable: "b"})
	h.Append(contracts.CommandObservation{Sequence: 6, Executable: "c"})
	since1 := h.SinceTask(1)
	require.Len(t, since1, 2)
	assert.Equal(t, "b", since1[0].Executable)
}
