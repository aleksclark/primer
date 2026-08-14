package identity_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestModulePathFrozen(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	modFile := filepath.Join(filepath.Dir(thisFile), "go.mod")
	raw, err := os.ReadFile(modFile)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	first := strings.SplitN(string(raw), "\n", 2)[0]
	want := "module github.com/aleksclark/primer/identity"
	if first != want {
		t.Fatalf("module path: got %q want %q", first, want)
	}
}
