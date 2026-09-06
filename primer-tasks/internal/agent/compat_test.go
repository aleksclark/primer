package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This is intentionally a source-level gate: Fantasy has a public top-level
// package, while provider internals are not part of Primer's compatibility
// promise. It also catches an accidental version drift before go mod tidy.
func TestFantasyDependencyAndImportCompatibilityAudit(t *testing.T) {
	root := filepath.Join("..", "..")
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^\s*charm\.land/fantasy v0\.41\.1(?:\s|$)`).Match(mod) {
		t.Fatal("Fantasy must remain pinned exactly at v0.41.1")
	}
	var bad []string
	err = filepath.Walk(filepath.Join(root, "internal", "agent"), func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		for _, forbidden := range []string{"charm.land/" + "fantasy/", "github.com/charmbracelet/" + "fantasy", "internal/" + "maf"} {
			if strings.Contains(string(b), forbidden) {
				bad = append(bad, path+": "+forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("forbidden non-public runtime imports: %s", strings.Join(bad, ", "))
	}
}
