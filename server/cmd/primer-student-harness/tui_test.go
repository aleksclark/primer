package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/aleksclark/trifle"
)

func TestHarnessStatusTUI(t *testing.T) {
	trifle.SkipOnWindows(t)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	cmdDir := filepath.Dir(thisFile)
	bin := filepath.Join(t.TempDir(), "primer-student-harness")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = cmdDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build harness: %v\n%s", err, out)
	}

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program:     bin,
		Args:        []string{"-status-only", "-tui"},
		Rows:        24,
		Cols:        80,
		StartupWait: 400 * time.Millisecond,
		Timeout:     15 * time.Second,
	})

	suite.Run("shows harness status chrome", func(t *testing.T, term *trifle.Terminal) {
		time.Sleep(300 * time.Millisecond)
		h := trifle.NewTestHelper(t, term)
		h.ExpectVisible("Primer student harness", trifle.WithFull())
		h.ExpectVisible("Phase:", trifle.WithFull())
		h.ExpectVisible("Sync:", trifle.WithFull())
		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		if err := term.WaitWithTimeout(5 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}
