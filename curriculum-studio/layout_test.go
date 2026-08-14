package curriculumstudio_test

import (
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
	return filepath.Dir(thisFile)
}

func TestReservedPackageLayout(t *testing.T) {
	t.Parallel()
	root := studioRoot(t)
	required := []string{
		"contracts/OWNERS.md",
		"cmd/openapi-gen/README.md",
		"cmd/studio-api/README.md",
		"internal/api/README.md",
		"internal/grpcapi/README.md",
		"internal/boundary/README.md",
		"internal/authn/README.md",
		"clients/ts-rest/README.md",
		"clients/go-rest/README.md",
		"clients/go-grpc/README.md",
		"tools/contract-gates/README.md",
		"tools/contract-gates/ownership_scan.py",
		"tools/contract-gates/check_no_tracked_generated.sh",
		"tools/contract-gates/requirements.txt",
		"Makefile",
	}
	for _, rel := range required {
		p := filepath.Join(root, rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing reserved path %s: %v", rel, err)
		}
	}
}

func TestServerPackagesDoNotImportClients(t *testing.T) {
	t.Parallel()
	root := studioRoot(t)
	// C1: no Go under internal/cmd yet; still enforce when files appear.
	var offenders []string
	scan := func(dir string) {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(raw), "curriculum-studio/clients/") ||
				strings.Contains(string(raw), "/clients/ts-rest") ||
				strings.Contains(string(raw), "/clients/go-rest") ||
				strings.Contains(string(raw), "/clients/go-grpc") {
				// Allow comments that mention the ban.
				for _, line := range strings.Split(string(raw), "\n") {
					trim := strings.TrimSpace(line)
					if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "*") {
						continue
					}
					if strings.Contains(line, "curriculum-studio/clients/") ||
						strings.Contains(line, "/clients/ts-rest") ||
						strings.Contains(line, "/clients/go-rest") ||
						strings.Contains(line, "/clients/go-grpc") {
						offenders = append(offenders, path)
						break
					}
				}
			}
			return nil
		})
	}
	scan(filepath.Join(root, "internal"))
	scan(filepath.Join(root, "cmd"))
	if len(offenders) > 0 {
		t.Fatalf("server packages must not import clients/*: %v", offenders)
	}
}

func TestOwnershipScanScriptGreen(t *testing.T) {
	root := studioRoot(t)
	script := filepath.Join(root, "tools/contract-gates/ownership_scan.py")
	cmd := exec.Command("python3", script, "--studio-root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ownership_scan failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "OK: ownership scan") {
		t.Fatalf("unexpected ownership_scan output:\n%s", out)
	}
}

func TestGeneratedPathsGitignored(t *testing.T) {
	root := studioRoot(t)
	// repo root is parent of curriculum-studio
	repoRoot := filepath.Dir(root)
	samples := []string{
		"curriculum-studio/contracts/gen/go/foo.pb.go",
		"curriculum-studio/contracts/.tmp/x.binpb",
		"curriculum-studio/clients/ts-rest/generated/x.ts",
		"curriculum-studio/openapi.emitted.yaml",
	}
	for _, s := range samples {
		cmd := exec.Command("git", "check-ignore", "-q", s)
		cmd.Dir = repoRoot
		if err := cmd.Run(); err != nil {
			t.Errorf("expected gitignore for %s (exit=%v)", s, err)
		}
	}
}
