package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
)

func TestInstalledProfileNamesAndLookupEmpty(t *testing.T) {
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	assert.Empty(t, sandbox.InstalledProfileNames())

	_, err := sandbox.LookupProfile("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	_, err = sandbox.LookupProfile("   ")
	require.Error(t, err)
}

func TestInstalledProfileNamesFromProfilesDir(t *testing.T) {
	parent := t.TempDir()
	// Only one known profile installed.
	want := filepath.Join(parent, "coreutils-basic")
	require.NoError(t, os.MkdirAll(want, 0o755))
	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")

	got := sandbox.InstalledProfileNames()
	assert.Equal(t, []string{"coreutils-basic"}, got)
}

func TestInstalledProfileNamesFromSingularFallback(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, root)

	got := sandbox.InstalledProfileNames()
	// Singular PROFILE_DIR maps every known name to the same dir.
	assert.ElementsMatch(t, sandbox.KnownProfileNames(), got)
	assert.True(t, sandbox.UsingSingularProfileFallback())
}

func TestResolveProfileDirNotDirectoryErrors(t *testing.T) {
	parent := t.TempDir()
	// Named path is a file, not a directory.
	filePath := filepath.Join(parent, "coreutils-basic")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")

	_, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestResolveProfileDirSingularNotDirectory(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "profile-file")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, filePath)

	_, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestResolveProfileDirSingularMissing(t *testing.T) {
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, filepath.Join(t.TempDir(), "nope"))
	_, err := sandbox.ResolveProfileDir("coreutils-basic")
	require.Error(t, err)
}

func TestResolveProfileDirUnknownName(t *testing.T) {
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	_, err := sandbox.ResolveProfileDir("not-real")
	require.Error(t, err)
}

func TestVerifyProfileBinariesEdges(t *testing.T) {
	_, err := sandbox.VerifyProfileBinaries("unknown-profile", t.TempDir())
	require.Error(t, err)

	// Empty profileDir → no missing list.
	missing, err := sandbox.VerifyProfileBinaries("coreutils-basic", "")
	require.NoError(t, err)
	assert.Nil(t, missing)

	// Profile dir without bin/ and without binaries at root → all missing.
	root := t.TempDir()
	missing, err = sandbox.VerifyProfileBinaries("coreutils-basic", root)
	require.NoError(t, err)
	require.NotEmpty(t, missing)
	assert.Contains(t, missing, "ls")

	// bin/ present with one binary.
	bin := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "ls"), []byte("#!/bin/sh\n"), 0o755))
	missing, err = sandbox.VerifyProfileBinaries("coreutils-basic", root)
	require.NoError(t, err)
	assert.NotContains(t, missing, "ls")
	assert.Contains(t, missing, "cat")
}

func TestApplyProfileNilAndEmptyName(t *testing.T) {
	require.Error(t, sandbox.ApplyProfile(nil, "coreutils-basic"))

	cfg := sandbox.Config{Workspace: t.TempDir()}
	require.NoError(t, sandbox.ApplyProfile(&cfg, ""))
	assert.Empty(t, cfg.RuntimeProfile)

	// Resolve error when PROFILES_DIR set but named dir missing.
	t.Setenv(sandbox.EnvRuntimeProfilesDir, t.TempDir())
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")
	err := sandbox.ApplyProfile(&cfg, "coreutils-basic")
	require.Error(t, err)
}
