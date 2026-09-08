package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveChildMergeRaisesNumeratorWithoutInflatingDenominator(t *testing.T) {
	mod := filepath.Join(t.TempDir(), "mod")
	writeLiveModule(t, mod)
	rawParent := filepath.Join(t.TempDir(), "raw-parent.out")
	cmd := exec.Command("go", "test", "./internal/...", "-count=1", "-covermode=atomic", "-coverprofile="+rawParent, "-coverpkg=./internal/...")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("parent test: %v (%s)", err, output)
	}
	parentPath := rewriteCovermodPrefix(t, rawParent)
	parentProf, err := parseProfile(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	parentTotal, parentCovered := profileTotals(parentProf)
	if parentTotal != 5 || parentCovered != 1 {
		t.Fatalf("parent totals %d/%d, want 1/5", parentCovered, parentTotal)
	}

	bin := filepath.Join(t.TempDir(), "srv")
	build := exec.Command("go", "build", "-cover", "-covermode=atomic", "-coverpkg=covermod/internal/alpha,covermod/internal/beta,covermod/cmd/srv", "-o", bin, "./cmd/srv")
	build.Dir = mod
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("child build: %v (%s)", err, output)
	}

	covDir := t.TempDir()
	runCmd := exec.Command(bin)
	runCmd.Env = append(os.Environ(), "GOCOVERDIR="+covDir)
	if output, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("child run: %v (%s)", err, output)
	}
	rawChild := filepath.Join(t.TempDir(), "raw-child.out")
	convert := exec.Command("go", "tool", "covdata", "textfmt", "-hw", "-i="+covDir, "-o="+rawChild, "-pkg=covermod/internal/...")
	if output, err := convert.CombinedOutput(); err != nil {
		t.Fatalf("covdata textfmt: %v (%s)", err, output)
	}
	childPath := rewriteCovermodPrefix(t, rawChild)
	childProf, err := parseProfile(childPath)
	if err != nil {
		t.Fatal(err)
	}
	merged, added, err := mergeChildIntoParent(parentProf, childProf, "primer-tasks/internal/")
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("child-only branch should add 2 statements, got %d", added)
	}
	total, covered := profileTotals(merged)
	if total != parentTotal {
		t.Fatalf("denominator changed from %d to %d", parentTotal, total)
	}
	if covered != 3 {
		t.Fatalf("child-only branch should raise coverage to 3/5, got %d/%d", covered, total)
	}
	beta := merged.Blocks[blockKey{File: "primer-tasks/internal/beta/b.go", Start: "2.22,2.34"}]
	if beta.Count != 0 {
		t.Fatal("untouched package received coverage")
	}
}

func rewriteCovermodPrefix(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), filepath.Base(path)+".rewritten")
	if err := os.WriteFile(out, []byte(strings.ReplaceAll(string(raw), "covermod/", "primer-tasks/")), 0o600); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeLiveModule(t *testing.T, mod string) {
	t.Helper()
	for _, dir := range []string{
		filepath.Join(mod, "internal", "alpha"),
		filepath.Join(mod, "internal", "beta"),
		filepath.Join(mod, "cmd", "srv"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(mod, "go.mod"), "module covermod\n\ngo 1.26.6\n")
	mustWrite(t, filepath.Join(mod, "internal", "alpha", "a.go"), `package alpha
func ParentHit() int { return 1 }
func ChildOnly(x bool) int {
	if x {
		return 2
	}
	return 3
}
`)
	mustWrite(t, filepath.Join(mod, "internal", "alpha", "a_test.go"), `package alpha
import "testing"
func TestParentHit(t *testing.T) {
	if ParentHit() != 1 {
		t.Fatal("parent")
	}
}
`)
	mustWrite(t, filepath.Join(mod, "internal", "beta", "b.go"), "package beta\nfunc Untouched() int { return 9 }\n")
	mustWrite(t, filepath.Join(mod, "cmd", "srv", "main.go"), `package main
import (
	"fmt"
	"covermod/internal/alpha"
)
func main() { fmt.Println(alpha.ChildOnly(true)) }
`)
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCovermergeUnknownInternalBlockFailsClosed(t *testing.T) {
	parent := writeFile(t, t.TempDir(), "parent.out", `mode: atomic
primer-tasks/internal/alpha/a.go:2.22,2.34 1 1
`)
	root := t.TempDir()
	writeManifest(t, root, runManifest{RunID: "run-1", CoverMode: "atomic"})
	dir := filepath.Join(root, "launches", "unknown")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "meta.json"), map[string]any{
		"id": "unknown", "exitClass": "normal", "cover": true, "runId": "run-1", "coverMode": "atomic",
	})
	mustWrite(t, filepath.Join(dir, "covmeta.x"), "meta")
	mustWrite(t, filepath.Join(dir, "covcounters.x"), "counters")
	if err := runCovermerge(buildCovermerge(t), root, parent, filepath.Join(t.TempDir(), "out.out")); err == nil {
		t.Fatal("unknown/corrupt child data accepted")
	} else if !strings.Contains(err.Error(), "conversion failed") && !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}
