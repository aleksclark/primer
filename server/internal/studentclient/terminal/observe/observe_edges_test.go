package observe_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/terminal/observe"
)

func TestParseExit(t *testing.T) {
	t.Parallel()
	n, err := observe.ParseExit("0")
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = observe.ParseExit("  42\n")
	require.NoError(t, err)
	assert.Equal(t, 42, n)

	_, err = observe.ParseExit("")
	require.Error(t, err)
	_, err = observe.ParseExit("nope")
	require.Error(t, err)
}

func TestRelCwdViaDrain(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	ws := t.TempDir()
	spool, err := observe.Prepare(base)
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close() })

	// Events with sandbox mapping, host workspace mapping, relative cwd, and abs outside.
	lines := []map[string]any{
		{"v": 1, "cmd": "pwd", "cwd_before": "/sandbox", "cwd_after": "/sandbox", "exit": 0, "ts": time.Now().Unix()},
		{"v": 1, "cmd": "ls", "cwd_before": "/sandbox/docs", "cwd_after": "/sandbox/docs/nested", "exit": 0, "ts": time.Now().Unix()},
		{"v": 1, "cmd": "echo", "cwd_before": filepath.Join(ws, "home"), "cwd_after": filepath.Join(ws, "home", "work"), "exit": 0, "ts": time.Now().Unix()},
		{"v": 1, "cmd": "cd", "cwd_before": "rel/path", "cwd_after": "./rel/path", "exit": 0, "ts": time.Now().Unix()},
		{"v": 1, "cmd": "true", "cwd_before": "/elsewhere/abs", "cwd_after": "/elsewhere/abs", "exit": 0, "ts": time.Now().Unix()},
		{"v": 1, "cmd": "x", "cwd_before": "  ", "cwd_after": "", "exit": 1, "ts": time.Now().Unix()},
	}
	var buf []byte
	for _, l := range lines {
		raw, err := json.Marshal(l)
		require.NoError(t, err)
		buf = append(buf, raw...)
		buf = append(buf, '\n')
	}
	require.NoError(t, os.WriteFile(spool.EventsPath(), buf, 0o644))

	r := observe.NewReader(spool)
	r.SandboxWorkspace = "/sandbox"
	r.Workspace = ws
	events, err := r.Drain()
	require.NoError(t, err)
	require.Len(t, events, 6)

	assert.Equal(t, ".", events[0].CwdAfter)
	assert.Equal(t, "docs/nested", events[1].CwdAfter)
	assert.Equal(t, "home/work", events[2].CwdAfter)
	assert.Equal(t, "rel/path", events[3].CwdAfter)
	assert.Equal(t, "", events[4].CwdAfter, "absolute path outside workspace maps empty")
	assert.Equal(t, "", events[5].CwdAfter)
	assert.Equal(t, "", events[5].CwdBefore)
}

func TestParseCommandLineMoreEdges(t *testing.T) {
	t.Parallel()
	_, _, ok, _ := observe.ParseCommandLine("")
	assert.False(t, ok)
	_, _, ok, _ = observe.ParseCommandLine("   ")
	assert.False(t, ok)
	_, _, ok, _ = observe.ParseCommandLine("echo `date`")
	assert.False(t, ok)
	_, _, ok, _ = observe.ParseCommandLine("echo ${HOME}")
	assert.False(t, ok)

	exe, argv, ok, pipe := observe.ParseCommandLine(`cat < in.txt`)
	require.True(t, ok)
	assert.Equal(t, "cat", exe)
	assert.Empty(t, argv)
	assert.True(t, pipe.HasRedirectIn)

	exe, argv, ok, pipe = observe.ParseCommandLine(`printf 'a|b' | wc -l`)
	require.True(t, ok)
	assert.Equal(t, "printf", exe)
	assert.Equal(t, []string{"a|b"}, argv)
	assert.True(t, pipe.HasPipe)
	assert.Equal(t, []string{"printf", "wc"}, pipe.Stages)

	// Unclosed quote → argv unavailable.
	_, _, ok, _ = observe.ParseCommandLine(`echo "hi`)
	assert.False(t, ok)
}
