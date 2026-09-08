package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeChildOnlyBranchKeepsUntouchedPackage(t *testing.T) {
	parent := mustProfile(t, `mode: atomic
example/internal/alpha/a.go:2.22,2.34 1 1
example/internal/alpha/a.go:3.28,4.8 1 0
example/internal/alpha/a.go:4.8,6.4 1 0
example/internal/alpha/a.go:7.3,7.11 1 0
example/internal/beta/b.go:2.22,2.34 1 0
`)
	child := mustProfile(t, `mode: atomic
example/cmd/srv/main.go:9.13,11.31 2 1
example/internal/alpha/a.go:2.22,2.34 1 0
example/internal/alpha/a.go:3.28,4.8 1 1
example/internal/alpha/a.go:4.8,6.4 1 1
example/internal/alpha/a.go:7.3,7.11 1 0
`)
	merged, added, err := mergeChildIntoParent(parent, child, "example/internal/")
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("child-only branch should add 2 statements, got %d", added)
	}
	total, covered := profileTotals(merged)
	if total != 5 || covered != 3 {
		t.Fatalf("merged totals %d/%d, want 3/5", covered, total)
	}
	beta := merged.Blocks[blockKey{File: "example/internal/beta/b.go", Start: "2.22,2.34"}]
	if beta.Count != 0 {
		t.Fatal("untouched internal package received coverage credit")
	}
}

func TestMergeOverlapDoesNotInflateTotals(t *testing.T) {
	parent := mustProfile(t, `mode: atomic
example/internal/alpha/a.go:2.22,2.34 1 4
example/internal/alpha/a.go:3.28,4.8 1 1
`)
	child := mustProfile(t, `mode: atomic
example/internal/alpha/a.go:2.22,2.34 1 9
example/internal/alpha/a.go:3.28,4.8 1 2
`)
	merged, added, err := mergeChildIntoParent(parent, child, "example/internal/")
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("overlap added %d statements", added)
	}
	total, covered := profileTotals(merged)
	if total != 2 || covered != 2 {
		t.Fatalf("overlap changed totals to %d/%d", covered, total)
	}
	if merged.Blocks[blockKey{File: "example/internal/alpha/a.go", Start: "2.22,2.34"}].Count != 13 {
		t.Fatal("overlapping hits should sum counts without changing the statement set")
	}
}

func TestMergeRejectsUnknownModeMismatchAndOverflow(t *testing.T) {
	parent := mustProfile(t, `mode: atomic
example/internal/alpha/a.go:2.22,2.34 1 0
`)
	if _, _, err := mergeChildIntoParent(parent, mustProfile(t, "mode: atomic\nexample/internal/alpha/missing.go:1.1,1.2 1 1\n"), "example/internal/"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown child block accepted: %v", err)
	}
	if _, _, err := mergeChildIntoParent(parent, mustProfile(t, "mode: set\nexample/internal/alpha/a.go:2.22,2.34 1 1\n"), "example/internal/"); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("mode mismatch accepted: %v", err)
	}
	parent.Blocks[blockKey{File: "example/internal/alpha/a.go", Start: "2.22,2.34"}] = block{File: "example/internal/alpha/a.go", Start: "2.22,2.34", NStmt: 1, Count: ^uint64(0)}
	if _, _, err := mergeChildIntoParent(parent, mustProfile(t, "mode: atomic\nexample/internal/alpha/a.go:2.22,2.34 1 1\n"), "example/internal/"); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("count overflow accepted: %v", err)
	}
	parent = mustProfile(t, `mode: atomic
example/internal/alpha/a.go:2.22,2.34 1 0
`)
	if _, _, err := mergeChildIntoParent(parent, mustProfile(t, "mode: atomic\nexample/internal/alpha/a.go:2.22,2.34 3 1\n"), "example/internal/"); err == nil || !strings.Contains(err.Error(), "statement count") {
		t.Fatalf("statement mismatch accepted: %v", err)
	}
}

func TestParseProfileRejectsMalformedEmptyAndDuplicatesMismatch(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, body, needle string
	}{
		{"empty", "", "empty or malformed"},
		{"no-mode", "example/internal/a.go:1.1,1.2 1 1\n", "missing coverage mode"},
		{"no-blocks", "mode: atomic\n", "no blocks"},
		{"bad-count", "mode: atomic\nexample/internal/a.go:1.1,1.2 1 -3\n", "invalid execution count"},
		{"neg-stmt", "mode: atomic\nexample/internal/a.go:1.1,1.2 -1 1\n", "invalid statement count"},
		{"stmt-mismatch", "mode: atomic\nexample/internal/a.go:1.1,1.2 1 1\nexample/internal/a.go:1.1,1.2 2 0\n", "statement count mismatch"},
	} {
		path := filepath.Join(dir, tc.name+".out")
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := parseProfile(path); err == nil || !strings.Contains(err.Error(), tc.needle) {
			t.Fatalf("%s: want %q, got %v", tc.name, tc.needle, err)
		}
	}
}

func mustProfile(t *testing.T, body string) profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.out")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := parseProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
