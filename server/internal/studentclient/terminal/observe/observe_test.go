package observe_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal/observe"
)

func TestParseCommandLineSimple(t *testing.T) {
	t.Parallel()
	exe, argv, ok, pipe := observe.ParseCommandLine(`ls -la docs`)
	require.True(t, ok)
	assert.Equal(t, "ls", exe)
	assert.Equal(t, []string{"-la", "docs"}, argv)
	assert.False(t, pipe.HasPipe)
}

func TestParseCommandLinePipeline(t *testing.T) {
	t.Parallel()
	exe, argv, ok, pipe := observe.ParseCommandLine(`cat a.txt | grep foo`)
	require.True(t, ok)
	assert.Equal(t, "cat", exe)
	assert.Equal(t, []string{"a.txt"}, argv)
	assert.True(t, pipe.HasPipe)
	assert.Equal(t, []string{"cat", "grep"}, pipe.Stages)
}

func TestParseCommandLineRejectsExpansion(t *testing.T) {
	t.Parallel()
	_, _, ok, _ := observe.ParseCommandLine(`echo $(whoami)`)
	assert.False(t, ok)
}

func TestParseCommandLineRedirect(t *testing.T) {
	t.Parallel()
	exe, argv, ok, pipe := observe.ParseCommandLine(`echo hi > out.txt`)
	require.True(t, ok)
	assert.Equal(t, "echo", exe)
	assert.Equal(t, []string{"hi"}, argv)
	assert.True(t, pipe.HasRedirectOut)
}

func TestSpoolDrainEvents(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	spool, err := observe.Prepare(base)
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close() })

	require.NoError(t, observe.WriteBashRC(spool.RCPath(), spool.EventsPath(), "/workspace"))
	rc, err := os.ReadFile(spool.RCPath())
	require.NoError(t, err)
	assert.Contains(t, string(rc), "PROMPT_COMMAND")
	assert.Contains(t, string(rc), "_primer_debug")

	// Simulate bash hook output.
	line, _ := json.Marshal(map[string]any{
		"v": 1, "cmd": "ls -la", "cwd_before": "/workspace", "cwd_after": "/workspace/docs",
		"exit": 0, "ts": time.Now().Unix(),
	})
	require.NoError(t, os.WriteFile(spool.EventsPath(), append(line, '\n'), 0o644))

	r := observe.NewReader(spool)
	r.SandboxWorkspace = "/workspace"
	r.SessionID = "sess-1"
	events, err := r.Drain()
	require.NoError(t, err)
	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, contracts.SourceObserveBash, ev.Source)
	assert.True(t, ev.Structured)
	assert.True(t, ev.Quality.MeetsStructuredBar())
	assert.Equal(t, "ls", ev.Executable)
	assert.Equal(t, []string{"-la"}, ev.Argv)
	assert.Equal(t, 0, ev.ExitCode)
	assert.Equal(t, "docs", ev.CwdAfter)
	assert.False(t, ev.Stdout.Trusted, "interactive observe must not claim trusted stdout")

	// Second drain is empty.
	more, err := r.Drain()
	require.NoError(t, err)
	assert.Empty(t, more)
}

func TestBoundExcerpt(t *testing.T) {
	t.Parallel()
	ex := observe.BoundExcerpt("hello", true)
	assert.Equal(t, "hello", ex.Text)
	assert.True(t, ex.Trusted)
	assert.NotEmpty(t, ex.SHA256)

	big := string(make([]byte, observe.MaxExcerptBytes+10))
	ex2 := observe.BoundExcerpt(big, false)
	assert.True(t, ex2.Truncated)
	assert.Equal(t, observe.MaxExcerptBytes, len(ex2.Text))
}

func TestEventsPathOutsideWorkspace(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	base := filepath.Join(t.TempDir(), "obs")
	spool, err := observe.Prepare(base)
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close() })
	// Spool must not live inside the student workspace.
	assert.NotContains(t, spool.Dir, ws)
}
