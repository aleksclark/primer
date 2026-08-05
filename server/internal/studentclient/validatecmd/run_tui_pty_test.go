package validatecmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

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

	// Validation can take a while before the TUI starts; wait for readiness text.
	deadline := time.Now().Add(45 * time.Second)
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
		select {
		case err := <-errCh:
			return err
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Logf("PTY readiness marker %q not seen before feed; feeding keys anyway", readySubstr)
	}
	_, _ = io.WriteString(ptmx, feed)
	// Keep a light re-feed in case the first key is lost during startup races.
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(200 * time.Millisecond)
			select {
			case <-errCh:
				return
			default:
				_, _ = io.WriteString(ptmx, "q")
			}
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(30 * time.Second):
		mu.Lock()
		got := outBuf.String()
		mu.Unlock()
		t.Fatalf("timed out waiting for RunTUI; pty output=%q", got)
		return nil
	}
}

func TestRunTUIQuitsViaPTY(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	dir := filepath.Join(root, "curriculum", "activities")
	err := withPTYStdio(t, "q", "Activity validation", func() error {
		return RunTUI(Options{ActivitiesDir: dir, Materialize: true})
	})
	// AllOK may fail if some activities fail validation; RunTUI still exercised.
	if err != nil {
		require.Contains(t, err.Error(), "validation failed")
	}
}
