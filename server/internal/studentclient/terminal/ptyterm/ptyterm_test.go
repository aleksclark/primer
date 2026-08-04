package ptyterm_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/terminal/ptyterm"
)

func TestStartShellEcho(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi\n"), 0o644))

	term, err := ptyterm.StartShell(dir, "sh", 24, 80)
	require.NoError(t, err)
	t.Cleanup(func() { _ = term.Close() })

	// Give the shell a moment to print a prompt.
	time.Sleep(150 * time.Millisecond)

	_, err = term.WriteString("echo hello-pty\n")
	require.NoError(t, err)
	_, err = term.WriteString("ls\n")
	require.NoError(t, err)

	deadline := time.Now().Add(3 * time.Second)
	var plain string
	for time.Now().Before(deadline) {
		plain = term.ScreenPlain()
		if strings.Contains(plain, "hello-pty") && strings.Contains(plain, "hello.txt") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected echo + ls output in screen, got:\n%s", plain)
}

func TestResizeAndClose(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("sh", "-i")
	cmd.Dir = dir
	term, err := ptyterm.Start(ptyterm.Options{Cmd: cmd, Rows: 20, Cols: 60})
	require.NoError(t, err)

	rows, cols := term.Size()
	require.Equal(t, uint16(20), rows)
	require.Equal(t, uint16(60), cols)

	require.NoError(t, term.Resize(30, 100))
	rows, cols = term.Size()
	require.Equal(t, uint16(30), rows)
	require.Equal(t, uint16(100), cols)

	require.NoError(t, term.Close())
	require.False(t, term.Alive())
	_, err = term.WriteString("x")
	require.Error(t, err)
}

func TestScrollbackBound(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("sh", "-c", "while true; do echo AAAAAAAAAA; done")
	cmd.Dir = dir
	term, err := ptyterm.Start(ptyterm.Options{Cmd: cmd, Rows: 10, Cols: 40, Scrollback: 256})
	require.NoError(t, err)
	t.Cleanup(func() { _ = term.Close() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(term.ScreenContent()) >= 200 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	content := term.ScreenContent()
	require.LessOrEqual(t, len(content), 256+64) // small overshoot window while appending
	require.NotEmpty(t, content)
}
