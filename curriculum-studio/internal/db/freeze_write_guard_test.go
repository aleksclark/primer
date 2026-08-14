package db_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

// copyBaselineTree copies real baseline SQL into dest for write-freeze CLI trials.
func copyBaselineTree(t *testing.T, dest string) {
	t.Helper()
	src := studioMigrationsDir(t)
	require.NoError(t, os.MkdirAll(dest, 0o755))
	for _, name := range studiodb.BaselineFiles {
		b, err := os.ReadFile(filepath.Join(src, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dest, name), b, 0o644))
	}
	// Seed a committed-looking manifest so mutated rewrite attempts are meaningful.
	m, err := studiodb.BuildManifest(src)
	require.NoError(t, err)
	require.NoError(t, studiodb.WriteManifest(filepath.Join(filepath.Dir(dest), studiodb.BaselineManifestName), m))
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// internal/db -> module root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestGuardWriteFreeze_LiveEnvRefused(t *testing.T) {
	t.Parallel()
	err := studiodb.GuardWriteFreeze(true, "")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "live")
	require.Contains(t, strings.ToLower(err.Error()), "write")
}

func TestGuardWriteFreeze_MarkerRefused(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	marker := filepath.Join(tmp, studiodb.LiveMarkerFilename)
	require.NoError(t, os.WriteFile(marker, []byte("1\n"), 0o644))
	err := studiodb.GuardWriteFreeze(false, marker)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "live")
}

func TestGuardWriteFreeze_NonLiveAllowed(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Marker path that does not exist must not refuse.
	err := studiodb.GuardWriteFreeze(false, filepath.Join(tmp, studiodb.LiveMarkerFilename))
	require.NoError(t, err)
}

// TestWriteFreezeCLI_LiveEnvRefuses is adversarial: env live + mutated SQL still
// must not rewrite baseline_manifest.json.
func TestWriteFreezeCLI_LiveEnvRefuses(t *testing.T) {
	root := moduleRoot(t)
	tmp := t.TempDir()
	mig := filepath.Join(tmp, "migrations")
	copyBaselineTree(t, mig)
	// Mutate baseline bytes (operator drift that write-freeze would normalize).
	mut := filepath.Join(mig, "00002_plan_domain.sql")
	b, err := os.ReadFile(mut)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(mut, append(b, []byte("\n-- adversarial drift\n")...), 0o644))

	manifestPath := filepath.Join(tmp, studiodb.BaselineManifestName)
	before, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	cmd := exec.Command("go", "run", "./cmd/migrate", "-write-freeze", "-migrations", mig)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "STUDIO_MIGRATIONS_LIVE=true")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	require.Error(t, err, "write-freeze must fail when STUDIO_MIGRATIONS_LIVE=true; stderr=%s", stderr.String())
	require.NotZero(t, cmd.ProcessState.ExitCode())
	after, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "manifest must not be rewritten under live env")
	require.Contains(t, strings.ToLower(stderr.String()), "live")
}

// TestWriteFreezeCLI_MarkerRefuses covers the ops marker file path.
func TestWriteFreezeCLI_MarkerRefuses(t *testing.T) {
	root := moduleRoot(t)
	tmp := t.TempDir()
	mig := filepath.Join(tmp, "migrations")
	copyBaselineTree(t, mig)
	marker := filepath.Join(tmp, studiodb.LiveMarkerFilename)
	require.NoError(t, os.WriteFile(marker, []byte("1\n"), 0o644))

	mut := filepath.Join(mig, "00001_identity_and_catalogs.sql")
	b, err := os.ReadFile(mut)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(mut, append(b, []byte("\n-- marker drift\n")...), 0o644))

	manifestPath := filepath.Join(tmp, studiodb.BaselineManifestName)
	before, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	cmd := exec.Command("go", "run", "./cmd/migrate", "-write-freeze", "-migrations", mig)
	cmd.Dir = root
	// Ensure env live is off so only the marker classifies live.
	cmd.Env = append(os.Environ(), "STUDIO_MIGRATIONS_LIVE=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	require.Error(t, err, "write-freeze must fail when live marker exists; stderr=%s", stderr.String())
	after, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "manifest must not be rewritten when marker present")
}

// TestWriteFreezeCLI_CheckModeStillAvailable proves check path remains usable live.
func TestWriteFreezeCLI_CheckModeStillAvailable(t *testing.T) {
	root := moduleRoot(t)
	tmp := t.TempDir()
	mig := filepath.Join(tmp, "migrations")
	copyBaselineTree(t, mig)
	// Live marker present; check against matching files should still succeed.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, studiodb.LiveMarkerFilename), []byte("1\n"), 0o644))

	cmd := exec.Command("go", "run", "./cmd/migrate", "-check-freeze", "-migrations", mig)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "STUDIO_MIGRATIONS_LIVE=true")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "check-freeze must remain available live; out=%s", out)
	require.Contains(t, string(out), "freeze check ok")
}

// TestWriteFreezeCLI_NonLiveAllowsRewrite documents pre-live write still works.
func TestWriteFreezeCLI_NonLiveAllowsRewrite(t *testing.T) {
	root := moduleRoot(t)
	tmp := t.TempDir()
	mig := filepath.Join(tmp, "migrations")
	copyBaselineTree(t, mig)
	mut := filepath.Join(mig, "00003_materialization_and_integration.sql")
	b, err := os.ReadFile(mut)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(mut, append(b, []byte("\n-- pre-live ok\n")...), 0o644))

	cmd := exec.Command("go", "run", "./cmd/migrate", "-write-freeze", "-migrations", mig)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "STUDIO_MIGRATIONS_LIVE=")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "non-live write-freeze should succeed; out=%s", out)

	// Manifest should now match mutated tree.
	m, err := studiodb.LoadManifest(filepath.Join(tmp, studiodb.BaselineManifestName))
	require.NoError(t, err)
	require.NoError(t, studiodb.VerifyManifest(mig, m, false))
}
