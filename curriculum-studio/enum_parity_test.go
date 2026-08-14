package curriculumstudio_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnumParityGateGreen(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(thisFile)
	script := filepath.Join(root, "tools/contract-gates/enum_parity.py")
	cmd := exec.Command("python3", script, "--studio-root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("enum_parity failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "OK: enum parity") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestEnumParityPlantedOpenAPIDriftFails(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(thisFile)
	oa := filepath.Join(root, "contracts/openapi/v1/curriculum-studio.yaml")
	raw, err := os.ReadFile(oa)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(oa, raw, 0o644)
	})
	planted := strings.Replace(
		string(raw),
		"enum: [requested, running, ready, failed, cancelled]",
		"enum: [requested, running, ready, failed, cancelled, planted_drift]",
		1,
	)
	if planted == string(raw) {
		t.Fatal("failed to plant OpenAPI drift")
	}
	if err := os.WriteFile(oa, []byte(planted), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "tools/contract-gates/enum_parity.py")
	cmd := exec.Command("python3", script, "--studio-root", root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected planted drift to fail, got success:\n%s", out)
	}
	combined := string(out)
	if !strings.Contains(combined, "planted_drift") {
		t.Fatalf("failure message should name planted value:\n%s", combined)
	}
}

func TestNoPrimaryEnumCatalogFile(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(thisFile)
	forbidden := []string{
		"enum-catalog.yaml",
		"enum-catalog.yml",
		"enums.yaml",
		"enums.yml",
		"enum_values.yaml",
		"enum_catalog.yaml",
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		for _, f := range forbidden {
			if base == f {
				t.Errorf("forbidden primary enum catalog: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
