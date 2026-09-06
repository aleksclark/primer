package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"primer-tasks/internal/api"
)

func TestBundleUsesActualContractsAndIsDeterministic(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	if err := emitBundle(first); err != nil {
		t.Fatal(err)
	}
	if err := emitBundle(second); err != nil {
		t.Fatal(err)
	}
	schema, err := api.AgentSocketSchema()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"openapi.yaml": api.OpenAPI() + "\n", "agent-protocol.schema.json": string(schema) + "\n", "agent-protocol.ts": api.AgentSocketTypeScript()}
	data, err := os.ReadFile(filepath.Join(first, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest bundleManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Format != bundleFormat || len(manifest.Files) != len(expected) {
		t.Fatal("incomplete bundle manifest")
	}
	for name, want := range expected {
		got, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want || manifest.Files[name] != digest(got) {
			t.Fatalf("bundle drift from actual source: %s", name)
		}
	}
	for _, name := range []string{"openapi.yaml", "agent-protocol.schema.json", "agent-protocol.ts", "manifest.json", "manifest.sha256"} {
		a, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(second, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(a) != string(b) {
			t.Fatalf("nondeterministic bundle member %s", name)
		}
	}
	marker, err := os.ReadFile(filepath.Join(first, "manifest.sha256"))
	if err != nil || string(marker) != digest(data)+"\n" {
		t.Fatal("manifest integrity marker missing")
	}
	// Interrupted emission cannot leave a previous success marker usable.
	if err = os.Remove(filepath.Join(first, "agent-protocol.ts")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(first, "agent-protocol.ts"), 0755); err != nil {
		t.Fatal(err)
	}
	if emitBundle(first) == nil {
		t.Fatal("expected output write failure")
	}
	if _, err = os.Stat(filepath.Join(first, "manifest.sha256")); !os.IsNotExist(err) {
		t.Fatal("failed emission left a complete marker")
	}
}
