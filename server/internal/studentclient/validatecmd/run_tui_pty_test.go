package validatecmd

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	// Validation can take a moment before the TUI starts.
	time.Sleep(800 * time.Millisecond)
	_, _ = io.WriteString(ptmx, feed)
	// Keep feeding q in case validation finishes after first write.
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(300 * time.Millisecond)
			_, _ = io.WriteString(ptmx, "q")
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(60 * time.Second):
		t.Fatal("timed out waiting for RunTUI")
		return nil
	}
}

func TestRunTUIQuitsViaPTY(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	// Validate a single known-good activity directory parent.
	dir := filepath.Join(root, "curriculum", "activities")
	err := withPTYStdio(t, "q", func() error {
		return RunTUI(Options{ActivitiesDir: dir, Materialize: true})
	})
	// AllOK may fail if some activities fail validation; RunTUI still exercised.
	// Accept either nil or validation-failed error after TUI quit.
	if err != nil {
		require.Contains(t, err.Error(), "validation failed")
	}
}
