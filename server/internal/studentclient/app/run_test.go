package app

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

// withPTYStdio swaps process stdio for a PTY, waits until the program writes
// output that matches readySubstr (or any output if empty), then feeds keys.
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
	// Drain master so the program can write without blocking; capture for readiness.
	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
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
	ready := false
	for time.Now().Before(deadline) {
		mu.Lock()
		s := outBuf.String()
		mu.Unlock()
		if readySubstr == "" {
			if len(s) > 0 {
				ready = true
				break
			}
		} else if strings.Contains(s, readySubstr) {
			ready = true
			break
		}
		// Also proceed if the program already exited.
		select {
		case err := <-errCh:
			return err
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		// Feed anyway after timeout so we don't hang forever if render is delayed;
		// still fail clearly if quit never happens.
		t.Logf("PTY readiness marker %q not seen before feed; feeding keys anyway", readySubstr)
	}
	_, _ = io.WriteString(ptmx, feed)

	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second):
		mu.Lock()
		got := outBuf.String()
		mu.Unlock()
		t.Fatalf("timed out waiting for app.Run to quit; pty output=%q", got)
		return nil
	}
}

func TestRunQuitsOnCtrlC(t *testing.T) {
	// Boot without store lands on pairing; ctrl+c should quit.
	err := withPTYStdio(t, "\x03", "pair", func() error {
		return Run(Options{})
	})
	require.NoError(t, err)
}
