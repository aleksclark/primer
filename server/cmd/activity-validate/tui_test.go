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

func TestActivityValidateTUI(t *testing.T) {
	trifle.SkipOnWindows(t)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	cmdDir := filepath.Dir(thisFile)
	repoRoot := filepath.Clean(filepath.Join(cmdDir, "..", "..", ".."))
	activities := filepath.Join(repoRoot, "curriculum", "activities")
	bin := filepath.Join(t.TempDir(), "activity-validate")

	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = cmdDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build activity-validate: %v\n%s", err, out)
	}

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program:     bin,
		Args:        []string{"--tui", "-dir", activities},
		Rows:        30,
		Cols:        100,
		StartupWait: 500 * time.Millisecond,
		Timeout:     20 * time.Second,
	})

	suite.Run("lists validated activities with pass status", func(t *testing.T, term *trifle.Terminal) {
		time.Sleep(400 * time.Millisecond)
		h := trifle.NewTestHelper(t, term)
		h.ExpectVisible("Activity validation", trifle.WithFull())
		h.ExpectVisible("PASS", trifle.WithFull())
		h.ExpectVisible("basic-navigation", trifle.WithFull())
		h.ExpectVisible("file-organization", trifle.WithFull())
		h.ExpectVisible("2/2 passed", trifle.WithFull())

		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		if err := term.WaitWithTimeout(5 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}
