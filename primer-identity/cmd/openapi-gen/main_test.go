package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTwiceIsByteEqual(t *testing.T) {
	first, stderr := runGen(t)
	require.Empty(t, stderr)
	second, _ := runGen(t)
	assert.Equal(t, first, second)
	assert.NotEmpty(t, first)
}

func TestGeneratedSpecMatchesCommittedBaseline(t *testing.T) {
	got, _ := runGen(t)
	want, err := os.ReadFile(filepath.Join(moduleRoot(t), "openapi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOutFileModeIs0644(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	_, stderr := runGen(t, "-out", path)
	require.Empty(t, string(stderr))
	st, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), st.Mode().Perm())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	stdout, _ := runGen(t)
	assert.Equal(t, stdout, data)
}

func TestOrdinaryGenerationDoesNotDirtyBaseline(t *testing.T) {
	root := moduleRoot(t)
	baseline := filepath.Join(root, "openapi.yaml")
	before, err := os.ReadFile(baseline)
	require.NoError(t, err)
	beforeStatus := gitPorcelain(t, root, "openapi.yaml")

	out := filepath.Join(t.TempDir(), "check.yaml")
	runGen(t, "-out", out)
	_, _ = runGen(t)

	after, err := os.ReadFile(baseline)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, beforeStatus, gitPorcelain(t, root, "openapi.yaml"))
}

func TestUsageMentionsBaselineUpdate(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/openapi-gen", "-h")
	cmd.Dir = moduleRoot(t)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	help := stdout.String() + stderr.String()
	assert.Contains(t, help, "-out")
	assert.Contains(t, help, "openapi.yaml")
}

func runGen(t *testing.T, args ...string) ([]byte, []byte) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/openapi-gen"}, args...)...)
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), "DATABASE_URL=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, err, "openapi-gen failed: %s", stderr.String())
	return stdout.Bytes(), bytes.TrimSpace(stderr.Bytes())
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func gitPorcelain(t *testing.T, root, rel string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--", rel)
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
