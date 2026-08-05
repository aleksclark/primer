package app

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

func withPTYStdio(t *testing.T, feed string, fn func() error) error {
	t.Helper()
	ptmx, tty, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin = tty
	os.Stdout = tty
	defer func() {
		os.Stdin = oldIn
		os.Stdout = oldOut
	}()
	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()
	time.Sleep(200 * time.Millisecond)
	_, _ = io.WriteString(ptmx, feed)
	select {
	case err := <-errCh:
		return err
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for app.Run to quit")
		return nil
	}
}

func TestRunQuitsOnCtrlC(t *testing.T) {
	// Boot without store lands on pairing; ctrl+c should quit.
	err := withPTYStdio(t, "\x03", func() error {
		return Run(Options{})
	})
	require.NoError(t, err)
}
