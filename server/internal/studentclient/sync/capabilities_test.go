package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestCollectDeviceCapabilitiesProfilesDirWithDigest(t *testing.T) {
	// Env-sensitive; do not parallelize.
	parent := t.TempDir()
	for _, name := range sandbox.KnownProfileNames() {
		dir := filepath.Join(parent, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		// Only one profile gets a PROFILE_DIGEST file; others fall back to basename.
		if name == "coreutils-basic" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "PROFILE_DIGEST"), []byte("  dig-core  \n"), 0o644))
		}
	}
	// Non-directory entry under parent is skipped.
	require.NoError(t, os.WriteFile(filepath.Join(parent, "text-processing-file"), []byte("x"), 0o644))
	// Unknown profile name subdirectory is ignored (KnownProfileNames only).
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "not-a-known-profile"), 0o755))

	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")

	caps := sync.CollectDeviceCapabilities()
	require.NotNil(t, caps)
	assert.NotEmpty(t, caps.RunnerVersion)
	require.ElementsMatch(t, sandbox.KnownProfileNames(), caps.RuntimeProfiles)
	assert.Equal(t, "dig-core", caps.ProfileDigests["coreutils-basic"])
	// Basename fallback when no digest file.
	assert.Equal(t, "text-processing", caps.ProfileDigests["text-processing"])
	// Sorted.
	assert.IsIncreasing(t, caps.RuntimeProfiles)
}

func TestCollectDeviceCapabilitiesSingularProfileDir(t *testing.T) {
	profile := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(profile, "DIGEST"), []byte("shared-digest"), 0o644))

	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, profile)

	caps := sync.CollectDeviceCapabilities()
	require.NotNil(t, caps)
	require.NotEmpty(t, caps.RuntimeProfiles)
	for _, name := range caps.RuntimeProfiles {
		assert.Equal(t, "shared-digest", caps.ProfileDigests[name], "name=%s", name)
	}
	// bash heuristic: when bash is on PATH, structured evidence capability is present.
	if _, err := os.Stat("/bin/bash"); err == nil || pathExists("bash") {
		assert.Contains(t, caps.Capabilities, contracts.CapStructuredCommandEvidence)
	}
}

func TestCollectDeviceCapabilitiesSingularMissingDir(t *testing.T) {
	t.Setenv(sandbox.EnvRuntimeProfilesDir, "")
	t.Setenv(sandbox.EnvRuntimeProfileDir, filepath.Join(t.TempDir(), "missing-profile"))

	caps := sync.CollectDeviceCapabilities()
	// Still may be non-nil via runner version / bash capability.
	if caps != nil {
		assert.Empty(t, caps.RuntimeProfiles)
	}
}

func TestCollectDeviceCapabilitiesEmptyDigestFiles(t *testing.T) {
	parent := t.TempDir()
	name := "coreutils-basic"
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// Empty digest files are ignored; falls through to basename.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "PROFILE_DIGEST"), []byte("   \n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "DIGEST"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".primer-profile-digest"), []byte("\t"), 0o644))

	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")

	caps := sync.CollectDeviceCapabilities()
	require.NotNil(t, caps)
	assert.Equal(t, name, caps.ProfileDigests[name])
}

func TestCollectDeviceCapabilitiesDotPrimerDigest(t *testing.T) {
	parent := t.TempDir()
	name := "text-processing"
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".primer-profile-digest"), []byte("dot-digest\n"), 0o644))

	t.Setenv(sandbox.EnvRuntimeProfilesDir, parent)
	t.Setenv(sandbox.EnvRuntimeProfileDir, "")

	caps := sync.CollectDeviceCapabilities()
	require.NotNil(t, caps)
	assert.Equal(t, "dot-digest", caps.ProfileDigests[name])
}

func pathExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}
