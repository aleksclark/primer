package terminal_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func structuredObs(seq int64, exe string, argv []string, exit int, stdout, stderr string) contracts.CommandObservation {
	return contracts.CommandObservation{
		Sequence: seq, Executable: exe, Argv: argv, ArgvAvailable: true,
		ExitCode: exit, ExitAvailable: true, Structured: true,
		Source:   contracts.SourceStructured,
		Quality:  contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true, Stderr: true},
		CwdAfter: "home", CwdAvailable: true,
		Stdout:     contracts.Excerpt{Text: stdout, Trusted: true},
		Stderr:     contracts.Excerpt{Text: stderr, Trusted: true},
		RecordedAt: time.Now().UTC(),
	}
}

func TestHistoryAppendBoundsAndSinceTask(t *testing.T) {
	t.Parallel()
	h := &terminal.History{}
	// Fill past MaxHistoryEvents.
	for i := 0; i < terminal.MaxHistoryEvents+5; i++ {
		h.Append(structuredObs(int64(i+1), "echo", []string{"x"}, 0, "ok", ""))
	}
	require.Len(t, h.Events, terminal.MaxHistoryEvents)
	// Oldest dropped: first remaining sequence is 6.
	assert.Equal(t, int64(6), h.Events[0].Sequence)
	assert.Equal(t, int64(terminal.MaxHistoryEvents+5), h.Events[len(h.Events)-1].Sequence)

	var nilH *terminal.History
	assert.Nil(t, nilH.SinceTask(0))
	assert.Empty(t, (&terminal.History{}).SinceTask(0))

	h2 := &terminal.History{}
	h2.MarkTaskStart(0, 1)
	h2.MarkTaskStart(1, 3)
	h2.Append(structuredObs(1, "a", nil, 0, "", ""))
	h2.Append(structuredObs(2, "b", nil, 0, "", ""))
	h2.Append(structuredObs(3, "c", nil, 0, "", ""))
	h2.Append(structuredObs(4, "d", nil, 0, "", ""))
	assert.Len(t, h2.SinceTask(0), 4)
	assert.Len(t, h2.SinceTask(1), 2)
	assert.Equal(t, int64(3), h2.SinceTask(1)[0].Sequence)
}

func TestHistoryFindMatchFilters(t *testing.T) {
	t.Parallel()
	h := &terminal.History{}
	h.MarkTaskStart(0, 1)

	// Untrusted source must fail RequireStructured.
	h.Append(contracts.CommandObservation{
		Sequence: 1, Executable: "ls", ArgvAvailable: true, ExitCode: 0, ExitAvailable: true,
		Structured: true, Source: contracts.SourcePTYShell,
		Quality: contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true},
		Stdout:  contracts.Excerpt{Text: "a", Trusted: true},
	})
	// Good structured match.
	h.Append(structuredObs(2, "/bin/ls", []string{"-la", "home"}, 0, "file\n", ""))
	// Wrong exit.
	h.Append(structuredObs(3, "cat", []string{"x"}, 1, "", "err"))

	_, ok := h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", RequireStructured: true, RequireSuccess: true,
	})
	assert.True(t, ok) // /bin/ls matches via basename

	// ArgsSet requires exact args with path equivalence.
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", ArgsSet: true, Args: []string{"-la", "./home"},
		RequireSuccess: true,
	})
	assert.True(t, ok)

	exit1 := 1
	got, ok := h.FindMatch(0, terminal.CommandMatch{
		Executable: "cat", ExitCode: &exit1,
	})
	assert.True(t, ok)
	assert.Equal(t, int64(3), got.Sequence)

	// Stdout predicates.
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", StdoutContains: "file", RequireStdoutTrusted: true,
	})
	assert.True(t, ok)
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", StdoutEquals: "file\n",
	})
	assert.True(t, ok)
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", StdoutPattern: `^file`,
	})
	assert.True(t, ok)
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "ls", StdoutPattern: `(`, // invalid regex → no match
	})
	assert.False(t, ok)

	// Stderr predicates on cat exit 1.
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "cat", StderrContains: "err", RequireStderrTrusted: true,
	})
	assert.True(t, ok)
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "cat", StderrEquals: "err",
	})
	assert.True(t, ok)
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "cat", StderrPattern: `e.r`,
	})
	assert.True(t, ok)

	// RequireSuccess fails when exit non-zero and no ExitCode set.
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "cat", RequireSuccess: true,
	})
	assert.False(t, ok)

	// Missing argv when ArgsSet.
	h.Append(contracts.CommandObservation{
		Sequence: 4, Executable: "pwd", ArgvAvailable: false,
		ExitCode: 0, ExitAvailable: true, Structured: true,
		Source:  contracts.SourceStructured,
		Quality: contracts.EvidenceQuality{Exit: true, Cwd: true},
	})
	_, ok = h.FindMatch(0, terminal.CommandMatch{
		Executable: "pwd", ArgsSet: true, Args: []string{},
	})
	assert.False(t, ok)

	// Executable path variants (/usr prefix).
	h.Append(structuredObs(5, "/usr/bin/grep", []string{"x"}, 0, "hit", ""))
	_, ok = h.FindMatch(0, terminal.CommandMatch{Executable: "/bin/grep"})
	assert.True(t, ok)
}
