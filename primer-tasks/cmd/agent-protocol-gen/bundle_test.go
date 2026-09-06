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
	student, err := api.StudentSocketSchema()
	if err != nil {
		t.Fatal(err)
	}
	config, err := api.DialogueConfigSchema()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"openapi.yaml": api.OpenAPI() + "\n", "openapi.json": api.OpenAPIJSON() + "\n", "agent-protocol.schema.json": string(schema) + "\n", "agent-protocol.ts": api.AgentSocketTypeScript(), "student-dialogue.schema.json": string(student) + "\n", "student-dialogue.ts": api.StudentSocketTypeScript(), "dialogue-config.schema.json": string(config) + "\n", "dialogue-config.ts": api.DialogueConfigTypeScript()}
	data, err := os.ReadFile(filepath.Join(first, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest bundleManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Format != bundleFormat || manifest.Normalization != contractNormalization || len(manifest.ContractDigest) != 64 || len(manifest.Files) != len(expected) {
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
	files := map[string][]byte{}
	for name, data := range expected {
		files[name] = []byte(data)
	}
	normalized, err := normalizedContracts(files)
	if err != nil || digest(normalized) != manifest.ContractDigest {
		t.Fatal("normalized contract digest is not reproducible")
	}
	names := []string{"manifest.json", "manifest.sha256"}
	for name := range expected {
		names = append(names, name)
	}
	for _, name := range names {
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
	if err = os.Remove(filepath.Join(first, "student-dialogue.ts")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(first, "student-dialogue.ts"), 0755); err != nil {
		t.Fatal(err)
	}
	if emitBundle(first) == nil {
		t.Fatal("expected output write failure")
	}
	if _, err = os.Stat(filepath.Join(first, "manifest.sha256")); !os.IsNotExist(err) {
		t.Fatal("failed emission left a complete marker")
	}
}
