package engine

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

// withPTYStdio swaps os.Stdin/Stdout for a PTY slave so tea.NewProgram().Run()
// (as used by RunStatusTUI) can open a TTY, waits for readiness output, then
// feeds quit keys on the master.
func withPTYStdio(t *testing.T, feed, readySubstr string, fn func() error) error {
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

	var (
		mu     sync.Mutex
		outBuf bytes.Buffer
	)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				mu.Lock()
				_, _ = outBuf.Write(buf[:n])
				mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		s := outBuf.String()
		mu.Unlock()
		if readySubstr == "" {
			if len(s) > 0 {
				break
			}
		} else if strings.Contains(s, readySubstr) {
			break
		}
		select {
		case err := <-errCh:
			return err
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = io.WriteString(ptmx, feed)

	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second):
		mu.Lock()
		got := outBuf.String()
		mu.Unlock()
		t.Fatalf("timed out waiting for TUI to quit; pty output=%q", got)
		return nil
	}
}

func TestRunStatusTUIQuitsViaPTY(t *testing.T) {
	// Not parallel: mutates process-wide stdin/stdout.
	err := withPTYStdio(t, "q", "Phase:", func() error {
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
	err := withPTYStdio(t, "\r", "Required checks: PASS", func() error {
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
