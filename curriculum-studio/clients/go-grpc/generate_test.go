package grpcclient_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func studioRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(studioRoot(t))
}

func contractsRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(studioRoot(t), "contracts")
}

func runIn(t *testing.T, dir string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TMPDIR="+os.TempDir())
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestP4S1ValidateAndBufBuildSucceed(t *testing.T) {
	root := contractsRoot(t)
	out, err := runIn(t, root, "./scripts/validate.sh")
	if err != nil {
		t.Fatalf("validate.sh failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK: contracts validated") {
		t.Fatalf("validate.sh missing success marker:\n%s", out)
	}

	image := filepath.Join(root, ".tmp", "curriculumstudio.v1.binpb")
	desc := filepath.Join(root, ".tmp", "curriculumstudio.v1.desc")
	for _, p := range []string{image, desc} {
		st, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("expected gitignored image %s: %v", p, statErr)
		}
		if st.Size() < 200 {
			t.Fatalf("%s too small: %d bytes", p, st.Size())
		}
	}

	out, err = runIn(t, root, "buf", "build")
	if err != nil {
		t.Fatalf("buf build failed: %v\n%s", err, out)
	}
}

func TestP4S1GenerateScriptUsesLocalPinnedPlugins(t *testing.T) {
	script := filepath.Join(contractsRoot(t), "scripts", "generate.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("missing generate.sh: %v", err)
	}
	raw, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "buf generate --template") ||
		strings.Contains(body, "remote: buf.build") {
		t.Fatal("generate.sh must not invoke BSR remote plugins")
	}
	if !strings.Contains(body, "bootstrap_local_plugins.sh") {
		t.Fatal("generate.sh must bootstrap local pinned plugins")
	}
	if !strings.Contains(body, "buf generate") {
		t.Fatal("generate.sh must run buf generate")
	}
}

func TestP4S1GenerateRejectsUnpinnedBuf(t *testing.T) {
	tmp := t.TempDir()
	fakeBuf := filepath.Join(tmp, "buf")
	if err := os.WriteFile(fakeBuf, []byte("#!/bin/sh\necho 1.71.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", filepath.Join(contractsRoot(t), "scripts", "generate.sh"))
	cmd.Dir = contractsRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+tmp+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("generate.sh accepted an unpinned Buf CLI:\n%s", out)
	}
	if !strings.Contains(string(out), "not in 1.72.x") {
		t.Fatalf("missing pinned Buf rejection:\n%s", out)
	}
}

func TestP4S3GenerateTwiceDeterministic(t *testing.T) {
	root := contractsRoot(t)
	out, err := runIn(t, root, "./scripts/generate.sh", "--twice")
	if err != nil {
		t.Fatalf("generate.sh --twice failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK: generate twice deterministic") {
		t.Fatalf("missing determinism marker:\n%s", out)
	}

	gen := filepath.Join(root, "gen", "go")
	entries, err := os.ReadDir(gen)
	if err != nil {
		t.Fatalf("expected gen/go after generate: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("gen/go is empty after generate")
	}
}

func TestP4S4GeneratedSourcesRemainUntracked(t *testing.T) {
	repo := repoRoot(t)
	cmd := exec.Command("git", "ls-files", "--", "curriculum-studio")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files: %v\n%s", err, out)
	}
	var tracked []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		base := filepath.Base(line)
		if strings.HasSuffix(base, ".pb.go") ||
			strings.HasSuffix(base, "_grpc.pb.go") ||
			strings.Contains(line, "/contracts/gen/") ||
			strings.Contains(line, "/clients/") && strings.Contains(line, "/generated/") {
			tracked = append(tracked, line)
		}
	}
	if len(tracked) > 0 {
		t.Fatalf("generated sources are tracked: %v", tracked)
	}

	samples := []string{
		"curriculum-studio/contracts/gen/go/curriculumstudio/v1/integration.pb.go",
		"curriculum-studio/contracts/gen/go/curriculumstudio/v1/integration_grpc.pb.go",
		"curriculum-studio/contracts/.tmp/curriculumstudio.v1.binpb",
	}
	for _, s := range samples {
		ignore := exec.Command("git", "check-ignore", "-q", s)
		ignore.Dir = repo
		if err := ignore.Run(); err != nil {
			t.Errorf("expected gitignore for %s", s)
		}
	}

	gate := filepath.Join(studioRoot(t), "tools/contract-gates/check_no_tracked_generated.sh")
	out2, err := runIn(t, repo, "bash", gate)
	if err != nil {
		t.Fatalf("check_no_tracked_generated.sh failed: %v\n%s", err, out2)
	}
}

func TestMakefileWiresGenerateAndClientBuild(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(studioRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if bytes.Contains(raw, []byte("TODO(C4)")) {
		t.Fatal("Makefile still has TODO(C4) stubs")
	}
	for _, needle := range []string{
		"contracts-buf-generate:",
		"clients-go-grpc-build:",
		"./scripts/generate.sh",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("Makefile missing %q", needle)
		}
	}
}
