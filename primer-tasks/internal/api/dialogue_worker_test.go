package api

// Subprocess harness qualification. These tests prove instrumentation and
// fail-closed exit classification, not Tasks/Fantasy product acceptance.
import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type dialogueChildBuild struct {
	Race      bool   `json:"race"`
	Cover     bool   `json:"cover"`
	CoverMode string `json:"coverMode,omitempty"`
	SHA256    string `json:"sha256"`
	Revision  string `json:"revision"`
	Modified  bool   `json:"modified"`
}

func inspectDialogueChild(path string, parentRace bool) (proof dialogueChildBuild, err error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return proof, errors.New("child build information unavailable")
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "-race":
			proof.Race = setting.Value == "true"
		case "-cover":
			proof.Cover = setting.Value == "true"
		case "-covermode":
			proof.CoverMode = setting.Value
		case "vcs.revision":
			proof.Revision = setting.Value
		case "vcs.modified":
			proof.Modified = setting.Value == "true"
		}
	}
	if !proof.Cover {
		proof.Cover, proof.CoverMode = inspectDialogueChildCoverFallback(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return proof, err
	}
	defer f.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, f); err != nil {
		return proof, err
	}
	proof.SHA256 = fmt.Sprintf("%x", digest.Sum(nil))
	if parentRace && !proof.Race {
		return proof, errors.New("race-instrumented parent received an ordinary child")
	}
	return proof, nil
}
func buildDialogueChild(t *testing.T, binary, target string, race bool) dialogueChildBuild {
	t.Helper()
	return buildDialogueChildInDir(t, binary, target, race, "")
}
func buildDialogueChildInDir(t *testing.T, binary, target string, race bool, dir string) dialogueChildBuild {
	t.Helper()
	args := []string{"build"}
	if race {
		args = append(args, "-race")
	}
	coverRequested := childCoverageRequested() && strings.Contains(target, "cmd/tasks-server")
	if coverRequested {
		args = append(args, "-cover", "-covermode", childCoverageMode(), "-coverpkg", childCoveragePackages())
	}
	args = append(args, "-o", binary, target)
	command := exec.Command("go", args...)
	command.Dir = dir
	command.Env = dialogueBuildEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child build failed: %v (%d diagnostic bytes)", err, len(output))
	}
	proof, err := inspectDialogueChild(binary, race)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Race != race {
		t.Fatal("effective child race setting differs from requested build mode")
	}
	if coverRequested && !proof.Cover {
		t.Fatal("coverage gate received an uninstrumented Tasks server child")
	}
	return proof
}

func childCoverageRequested() bool {
	return os.Getenv("PRIMER_TASKS_CHILD_COVER_ROOT") != ""
}

func childCoverageMode() string {
	if mode := os.Getenv("PRIMER_TASKS_CHILD_COVER_MODE"); mode != "" {
		return mode
	}
	return "atomic"
}

func childCoveragePackages() string {
	if pkgs := os.Getenv("PRIMER_TASKS_CHILD_COVERPKG"); pkgs != "" {
		return pkgs + ",primer-tasks/cmd/tasks-server"
	}
	return "./internal/...,./cmd/tasks-server"
}

func inspectDialogueChildCoverFallback(path string) (bool, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	covered := bytes.Contains(data, []byte("runtime/coverage")) && bytes.Contains(data, []byte("GOCOVERDIR"))
	if !covered {
		return false, ""
	}
	mode := ""
	switch {
	case bytes.Contains(data, []byte("atomic")):
		mode = "atomic"
	case bytes.Contains(data, []byte("count")):
		mode = "count"
	case bytes.Contains(data, []byte("set")):
		mode = "set"
	}
	return true, mode
}

type dialogueChildExit struct {
	BeforeSignal   bool
	SignalSent     bool
	CrashRequested bool
	TimedOut       bool
	RaceReport     bool
	WaitError      error
}

func validateDialogueChildExit(exit dialogueChildExit) error {
	if exit.RaceReport {
		return errors.New("child race detector report observed")
	}
	var failure *exec.ExitError
	if errors.As(exit.WaitError, &failure) && failure.ExitCode() == 66 {
		return errors.New("child race detector exit observed")
	}
	if exit.BeforeSignal {
		return errors.New("child exited before requested termination")
	}
	if exit.TimedOut {
		return errors.New("child termination exceeded its bound")
	}
	if !exit.SignalSent {
		return errors.New("requested child termination was not delivered")
	}
	if exit.CrashRequested {
		if !errors.As(exit.WaitError, &failure) {
			return errors.New("intentional crash did not produce a signal exit")
		}
		status, ok := failure.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
			return errors.New("intentional crash observed a different exit cause")
		}
		return nil
	}
	if exit.WaitError != nil {
		return errors.New("child failed during ordinary teardown")
	}
	return nil
}
func dialogueRaceReport(data []byte) bool {
	return bytes.Contains(data, []byte("WARNING: DATA RACE")) || bytes.Contains(data, []byte("ThreadSanitizer")) || bytes.Contains(data, []byte("data race(s)"))
}
func (h *publicDialogueHarness) childDiagnostics() (bool, error) {
	f, err := os.Open(h.log.Name())
	if err != nil {
		return false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16*1024*1024+1))
	if err != nil {
		return false, err
	}
	if len(data) > 16*1024*1024 {
		return true, errors.New("child diagnostic bound exceeded")
	}
	// Scan ALL earlier process segments, not only the latest restart. A later
	// requested SIGKILL/provider retry must never forgive a prior detector report.
	return dialogueRaceReport(data), nil
}
func (h *publicDialogueHarness) recordChild(phase string, observation *dialogueChildExit) {
	h.t.Helper()
	proof := struct {
		Test                    string                `json:"test"`
		Phase                   string                `json:"phase"`
		ParentRace              bool                  `json:"parentRace"`
		Build                   dialogueChildBuild    `json:"build"`
		Source                  dialogueSourceBinding `json:"source"`
		RunningExecutableSHA256 string                `json:"runningExecutableSha256"`
		PID                     int                   `json:"pid"`
		ExitObserved            bool                  `json:"exitObserved"`
		CrashRequested          bool                  `json:"crashRequested"`
		SignalSent              bool                  `json:"signalSent"`
		BeforeSignal            bool                  `json:"beforeSignal"`
		TimedOut                bool                  `json:"timedOut"`
		RaceReport              bool                  `json:"raceReport"`
		ExitCode                int                   `json:"exitCode"`
		ExitSignal              string                `json:"exitSignal"`
	}{Test: h.t.Name(), Phase: phase, ParentRace: dialogueParentRace, Build: h.build, Source: h.binding, RunningExecutableSHA256: h.runningSHA, PID: h.pid}
	if observation != nil {
		proof.ExitObserved, proof.CrashRequested, proof.SignalSent, proof.BeforeSignal, proof.TimedOut, proof.RaceReport = true, observation.CrashRequested, observation.SignalSent, observation.BeforeSignal, observation.TimedOut, observation.RaceReport
		var failure *exec.ExitError
		if errors.As(observation.WaitError, &failure) {
			proof.ExitCode = failure.ExitCode()
			if status, ok := failure.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				proof.ExitSignal = status.Signal().String()
			}
		}
	}
	// Ignored product-local artifacts survive t.TempDir cleanup. Explicit fields
	// contain only build/exit booleans, hashes and causes, never environment,
	// credentials, command output, provider payloads or student content.
	dir := filepath.Join("..", "..", "tmp", "p4-child-proof")
	if err := os.MkdirAll(dir, 0700); err != nil {
		h.t.Fatal(err)
	}
	f, err := os.CreateTemp(dir, "child-*.json")
	if err != nil {
		h.t.Fatal(err)
	}
	if err = json.NewEncoder(f).Encode(proof); err != nil {
		f.Close()
		h.t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		h.t.Fatal(err)
	}
	h.t.Logf("P4 child runtime parentRace=%t effectiveChildRace=%t phase=%s proof=%s", dialogueParentRace, h.build.Race, phase, f.Name())
}

func TestDialogueChildInstrumentationAndExitClassification(t *testing.T) {
	folder := t.TempDir()
	source := filepath.Join(folder, "instrumentation.go")
	// A deliberately racy standalone qualification fixture. It never replaces
	// a first-party server/model/PG boundary in the public Tasks scenarios.
	program := `package main
import("os";"sync";"time")
func main(){if len(os.Args)>1&&os.Args[1]=="wait"{for{time.Sleep(time.Second)}};var group sync.WaitGroup;var value int;group.Add(2);for i:=0;i<2;i++{go func(){defer group.Done();for j:=0;j<100000;j++{value++}}()};group.Wait();_ = value}
`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	ordinary := filepath.Join(folder, "ordinary")
	ordinaryProof := buildDialogueChild(t, ordinary, source, false)
	if ordinaryProof.Race {
		t.Fatal("ordinary qualification child unexpectedly instrumented")
	}
	if _, err := inspectDialogueChild(ordinary, true); err == nil {
		t.Fatal("race parent accepted a real ordinary binary")
	}
	instrumented := filepath.Join(folder, "instrumented")
	proof := buildDialogueChild(t, instrumented, source, true)
	if !proof.Race {
		t.Fatal("effective -race setting absent")
	}
	command := exec.Command(instrumented)
	command.Env = append(os.Environ(), "GORACE=halt_on_error=1 exitcode=66 log_path=stderr")
	output, waitError := command.CombinedOutput()
	if !dialogueRaceReport(output) {
		t.Fatal("real race fixture did not emit a detector report")
	}
	for _, crash := range []bool{false, true} {
		if err := validateDialogueChildExit(dialogueChildExit{SignalSent: true, CrashRequested: crash, RaceReport: true, WaitError: waitError}); err == nil {
			t.Fatal("real race report/exit was forgiven by shutdown/crash classification")
		}
	}
	if err := validateDialogueChildExit(dialogueChildExit{SignalSent: true, CrashRequested: true, WaitError: waitError}); err == nil {
		t.Fatal("detector exit66 forgiven even without report bytes")
	}
	killed := exec.Command(instrumented, "wait")
	killed.Env = append(os.Environ(), "GORACE=halt_on_error=1 exitcode=66 log_path=stderr")
	if err := killed.Start(); err != nil {
		t.Fatal(err)
	}
	sent := killed.Process.Kill() == nil
	killError := killed.Wait()
	if err := validateDialogueChildExit(dialogueChildExit{SignalSent: sent, CrashRequested: true, WaitError: killError}); err != nil {
		t.Fatal("observed requested SIGKILL rejected")
	}
	for _, bad := range []dialogueChildExit{
		{SignalSent: true, WaitError: killError},
		{BeforeSignal: true},
		{TimedOut: true, SignalSent: true, CrashRequested: true, WaitError: killError},
		{SignalSent: false, CrashRequested: true, WaitError: killError},
		{SignalSent: true, CrashRequested: true, WaitError: nil},
	} {
		if validateDialogueChildExit(bad) == nil {
			t.Fatal("unexpected child exit/hang was forgiven")
		}
	}
	if err := validateDialogueChildExit(dialogueChildExit{SignalSent: true}); err != nil {
		t.Fatal(err)
	}
	if ordinaryProof.Cover {
		t.Fatal("standalone qualification fixture unexpectedly coverage-instrumented")
	}
	t.Logf("effective instrumentation verified: ordinary=%t instrumented=%t; real detector report/exit66 rejected; only observed requested SIGKILL accepted", ordinaryProof.Race, proof.Race)
}
