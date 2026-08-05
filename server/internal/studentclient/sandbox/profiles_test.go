package sandbox_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
)

func TestLookupProfileKnown(t *testing.T) {
	t.Parallel()
	p, err := sandbox.LookupProfile("coreutils-basic")
	require.NoError(t, err)
	assert.Equal(t, "coreutils-basic", p.Name)
	assert.Contains(t, p.Binaries, "ls")
}

func TestLookupProfileUnknown(t *testing.T) {
	t.Parallel()
	_, err := sandbox.LookupProfile("not-a-real-profile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown runtime profile")
}

func TestResolveProfileDirEmptyWithoutEnv(t *testing.T) {
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	dir, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.NoError(t, err)
	assert.Empty(t, dir)
}

func TestResolveProfileDirFromProfileDirEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sandbox.EnvRuntimeProfileDir, root)
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	dir, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.NoError(t, err)
	assert.Equal(t, root, dir)
}

func TestResolveProfileDirFromProfilesDir(t *testing.T) {
	parent := t.TempDir()
	want := filepath.Join(parent, "coreutils-basic")
	require.NoError(t, os.MkdirAll(want, 0o755))
	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	dir, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.NoError(t, err)
	assert.Equal(t, want, dir)
}

func TestResolveProfileDirMissingNamedFails(t *testing.T) {
	parent := t.TempDir()
	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	_, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.Error(t, err)
}

func TestApplyProfileHostFallback(t *testing.T) {
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	cfg := sandbox.Config{Workspace: t.TempDir()}
	require.NoError(t, sandbox.ApplyProfile(&cfg, "coreutils-basic"))
	assert.True(t, cfg.UseHostToolBinds())
	assert.Equal(t, "coreutils-basic", cfg.RuntimeProfile)
}

func TestApplyProfileNixTree(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("bwrap not installed")
	}
	profile := t.TempDir()
	bin := filepath.Join(profile, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	// Minimal fake sh is enough for bind path assertions.
	require.NoError(t, os.WriteFile(filepath.Join(bin, "sh"), []byte("#!/bin/sh\n"), 0o755))

	t.Setenv(sandbox.EnvRuntimeProfileDir, profile)
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")

	ws := t.TempDir()
	cfg := sandbox.Config{Workspace: ws}
	require.NoError(t, sandbox.ApplyProfile(&cfg, "coreutils-basic"))
	assert.False(t, cfg.UseHostToolBinds())
	assert.Equal(t, profile, cfg.ProfileDir)
	assert.Equal(t, "/runtime", cfg.ReadOnlyBinds[profile])
	assert.Equal(t, "/bin", cfg.ReadOnlyBinds[bin])

	args, err := sandbox.BuildArgs(cfg, []string{"/bin/sh", "-c", "true"})
	require.NoError(t, err)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, profile)
	assert.Contains(t, joined, "/runtime")
	// Must not fall back to broad host /usr when profile is set.
	// (Host /usr may still appear only if UseHostToolBinds — which is false.)
	assert.NotContains(t, joined, "--ro-bind /usr /usr")
}

func TestApplyProfileUnknown(t *testing.T) {
	cfg := sandbox.Config{}
	err := sandbox.ApplyProfile(&cfg, "nope")
	require.Error(t, err)
}
