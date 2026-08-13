package observe_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal/observe"
)

func TestPrepareAndNilSpoolEdges(t *testing.T) {
	t.Parallel()
	_, err := observe.Prepare("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "baseDir")

	// baseDir parent is a file → MkdirAll fails.
	base := t.TempDir()
	fileAsDir := filepath.Join(base, "notadir")
	require.NoError(t, os.WriteFile(fileAsDir, []byte("x"), 0o644))
	_, err = observe.Prepare(filepath.Join(fileAsDir, "child"))
	require.Error(t, err)

	spool, err := observe.Prepare(t.TempDir())
	require.NoError(t, err)
	assert.NotEmpty(t, spool.Dir)
	assert.FileExists(t, spool.EventsPath())
	assert.NotEmpty(t, spool.RCPath())

	var nilSpool *observe.Spool
	assert.Empty(t, nilSpool.EventsPath())
	assert.Empty(t, nilSpool.RCPath())
	require.NoError(t, nilSpool.Close())
	require.NoError(t, spool.Close())

	// WriteBashRC requires paths.
	require.Error(t, observe.WriteBashRC("", "e", "w"))
	require.Error(t, observe.WriteBashRC("rc", "", "w"))
	rc := filepath.Join(t.TempDir(), "rc.bash")
	require.NoError(t, observe.WriteBashRC(rc, "/tmp/events.ndjson", "/workspace"))
	body, err := os.ReadFile(rc)
	require.NoError(t, err)
	assert.Contains(t, string(body), "PROMPT_COMMAND")
	assert.Contains(t, string(body), observe.InstrumentationVersion)
}

func TestDrainSkipsMalformedAndTruncatesLongCmd(t *testing.T) {
	t.Parallel()
	spool, err := observe.Prepare(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close() })

	longCmd := strings.Repeat("a", observe.MaxSubmittedLine+50)
	good, err := json.Marshal(map[string]any{
		"v": 1, "cmd": longCmd, "cwd_before": ".", "cwd_after": ".", "exit": 0, "ts": time.Now().Unix(),
	})
	require.NoError(t, err)
	payload := strings.Join([]string{
		"", // empty line skipped
		"{not-json",
		`{"v":2,"cmd":"old","exit":0}`, // wrong version
		`{"v":1,"cmd":"","exit":0}`,    // empty cmd
		string(good),
		`{"v":1,"cmd":"ls","cwd_after":"/workspace/docs","exit":1,"ts":0}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(spool.EventsPath(), []byte(payload), 0o644))

	r := observe.NewReader(spool)
	r.SessionID = "sess"
	r.RunnerVersion = "r1"
	r.VerifierVersion = "v1"
	r.SandboxWorkspace = "/workspace"
	events, err := r.Drain()
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.LessOrEqual(t, len(events[0].SubmittedLine), observe.MaxSubmittedLine)
	assert.Equal(t, "sess", events[0].SessionID)
	assert.Equal(t, "r1", events[0].RunnerVersion)
	assert.Equal(t, contracts.SourceObserveBash, events[0].Source)
	assert.False(t, events[0].Stdout.Trusted)
	assert.Equal(t, "docs", events[1].CwdAfter)
	assert.Equal(t, 1, events[1].ExitCode)

	// Second drain is empty (offset advanced).
	more, err := r.Drain()
	require.NoError(t, err)
	assert.Empty(t, more)

	// Nil / empty-path readers.
	evs, err := (*observe.Reader)(nil).Drain()
	require.NoError(t, err)
	assert.Nil(t, evs)
	evs, err = observe.NewReader(nil).Drain()
	require.NoError(t, err)
	assert.Nil(t, evs)
}

func TestBoundExcerptAndParseCommandLineRedirs(t *testing.T) {
	t.Parallel()
	ex := observe.BoundExcerpt("hi", true)
	assert.Equal(t, "hi", ex.Text)
	assert.True(t, ex.Trusted)
	assert.False(t, ex.Truncated)
	assert.Equal(t, 2, ex.ByteLen)
	assert.NotEmpty(t, ex.SHA256)

	big := strings.Repeat("x", observe.MaxExcerptBytes+10)
	ex = observe.BoundExcerpt(big, false)
	assert.Equal(t, observe.MaxExcerptBytes, len(ex.Text))
	assert.False(t, ex.Trusted)
	assert.True(t, ex.Truncated)
	assert.Equal(t, observe.MaxExcerptBytes+10, ex.ByteLen)

	exe, args, ok, pipe := observe.ParseCommandLine(`echo hi > out.txt`)
	require.True(t, ok)
	assert.Equal(t, "echo", exe)
	assert.Equal(t, []string{"hi"}, args)
	require.NotNil(t, pipe)
	assert.True(t, pipe.HasRedirectOut)
}
