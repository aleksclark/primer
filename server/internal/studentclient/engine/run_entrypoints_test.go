package engine

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

// withPTYStdio swaps os.Stdin/Stdout for a PTY slave so tea.NewProgram().Run()
// (as used by RunStatusTUI) can open a TTY, then feeds quit keys on the master.
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

	// Give the program a moment to start, then send quit.
	time.Sleep(150 * time.Millisecond)
	_, _ = io.WriteString(ptmx, feed)

	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for TUI to quit")
		return nil
	}
}

func TestRunStatusTUIQuitsViaPTY(t *testing.T) {
	// Not parallel: mutates process-wide stdin/stdout.
	err := withPTYStdio(t, "q", func() error {
		return RunStatusTUI("coverage-status", Status{
			Phase:          "running",
			Sync:           "idle",
			WorkDownloaded: 2,
			ActivitySlug:   "basic-navigation",
			AssignmentID:   "abcdef0123456789",
			CommandsRun:    3,
			ChecksPassed:   1,
			ChecksTotal:    2,
			RequiredPassed: false,
			Message:        "hello",
			LastError:      "",
			Offline:        true,
		})
	})
	require.NoError(t, err)
}

func TestRunStatusTUIEnterQuitAndViews(t *testing.T) {
	err := withPTYStdio(t, "\r", func() error {
		return RunStatusTUI("", Status{
			Phase:           "done",
			RequiredPassed:  true,
			CompletionAcked: true,
			ChecksTotal:     1,
			ChecksPassed:    1,
			LastError:       "prior",
		})
	})
	require.NoError(t, err)
}
