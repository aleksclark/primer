package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCovermergeRejectsMissingCorruptAndMismatchedArtifacts(t *testing.T) {
	bin := buildCovermerge(t)
	parent := writeFile(t, t.TempDir(), "parent.out", `mode: atomic
primer-tasks/internal/alpha/a.go:2.22,2.34 1 1
primer-tasks/internal/beta/b.go:2.22,2.34 1 0
`)
	root := t.TempDir()
	if err := runCovermerge(bin, root, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("missing manifest accepted: %v", err)
	}

	writeManifest(t, root, runManifest{RunID: "run-1", CoverMode: "atomic", BinarySHA256: "abc"})
	if err := runCovermerge(bin, root, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "no child coverage launches") {
		t.Fatalf("empty launches accepted: %v", err)
	}

	normalDir := filepath.Join(root, "launches", "normal-1")
	if err := os.MkdirAll(normalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(normalDir, "meta.json"), map[string]any{
		"id": "normal-1", "exitClass": "normal", "cover": true, "runId": "run-1", "coverMode": "atomic", "binarySha256": "abc",
	})
	if err := runCovermerge(bin, root, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "missing coverage counters") {
		t.Fatalf("metadata-only normal exit accepted: %v", err)
	}

	if err := os.WriteFile(filepath.Join(normalDir, "covmeta.x"), []byte("meta"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(normalDir, "covcounters.x"), []byte("not-a-counter"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runCovermerge(bin, root, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "conversion failed") {
		t.Fatalf("corrupt counters accepted: %v", err)
	}

	stale := t.TempDir()
	writeManifest(t, stale, runManifest{RunID: "run-1", CoverMode: "atomic"})
	staleLaunch := filepath.Join(stale, "launches", "stale")
	if err := os.MkdirAll(staleLaunch, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(staleLaunch, "meta.json"), map[string]any{
		"id": "stale", "exitClass": "normal", "cover": true, "runId": "other-run", "coverMode": "atomic",
	})
	if err := runCovermerge(bin, stale, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "run id") {
		t.Fatalf("stale run artifact accepted: %v", err)
	}

	wrong := t.TempDir()
	writeManifest(t, wrong, runManifest{RunID: "run-1", CoverMode: "atomic", BinarySHA256: "abc", SourceSHA: "src-a"})
	wrongLaunch := filepath.Join(wrong, "launches", "wrong")
	if err := os.MkdirAll(wrongLaunch, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(wrongLaunch, "meta.json"), map[string]any{
		"id": "wrong", "exitClass": "normal", "cover": true, "runId": "run-1", "coverMode": "set", "sourceSha": "src-b", "binarySha256": "zzz",
	})
	if err := runCovermerge(bin, wrong, parent, filepath.Join(t.TempDir(), "out.out")); err == nil {
		t.Fatal("mismatched source/mode/binary accepted")
	}

	uninstrumented := t.TempDir()
	writeManifest(t, uninstrumented, runManifest{RunID: "run-1", CoverMode: "atomic"})
	plain := filepath.Join(uninstrumented, "launches", "plain")
	if err := os.MkdirAll(plain, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(plain, "meta.json"), map[string]any{
		"id": "plain", "exitClass": "normal", "cover": false, "runId": "run-1", "coverMode": "atomic",
	})
	if err := os.WriteFile(filepath.Join(plain, "covmeta.x"), []byte("m"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, "covcounters.x"), []byte("c"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runCovermerge(bin, uninstrumented, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "not instrumented") {
		t.Fatalf("uninstrumented child accepted: %v", err)
	}
}

func TestCovermergeSkipsObservedSigkillWithoutCredit(t *testing.T) {
	bin := buildCovermerge(t)
	root := t.TempDir()
	parent := writeFile(t, t.TempDir(), "parent.out", `mode: atomic
primer-tasks/internal/alpha/a.go:2.22,2.34 1 0
`)
	writeManifest(t, root, runManifest{RunID: "run-1", CoverMode: "atomic"})
	killed := filepath.Join(root, "launches", "killed")
	if err := os.MkdirAll(killed, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(killed, "meta.json"), map[string]any{
		"id": "killed", "exitClass": "sigkill", "cover": true, "runId": "run-1",
	})
	if err := runCovermerge(bin, root, parent, filepath.Join(t.TempDir(), "out.out")); err == nil || !strings.Contains(err.Error(), "no normal-exit") {
		t.Fatalf("sigkill-only run must not succeed: %v", err)
	}
}

func buildCovermerge(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "covermerge")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Dir(pathOfCovermergeTest(t))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build covermerge: %v (%s)", err, output)
	}
	return bin
}

func pathOfCovermergeTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("covermerge test path unavailable")
	}
	return file
}

func runCovermerge(bin, root, parent, out string) error {
	cmd := exec.Command(bin, root, parent, out)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return wrapOutput(err, output)
	}
	return nil
}

type outputError struct {
	err error
	out string
}

func (e outputError) Error() string { return e.err.Error() + ": " + e.out }

func wrapOutput(err error, output []byte) error {
	return outputError{err: err, out: string(output)}
}

func writeManifest(t *testing.T, root string, manifest runManifest) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, "manifest.json"), manifest)
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
