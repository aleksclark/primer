package db_test

import (
	"bytes"
	"fmt"
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

// liveTruthySpellings must all refuse write-freeze (shared with package truthy).
var liveTruthySpellings = []string{
	"1", "true", "TRUE", "True", "yes", "YES", "on", "ON", "Yes",
	" true", "true ", " true ", " True", "YES ", "\ton\t",
}

// nonLiveTruthySpellings must not classify as live by themselves.
var nonLiveTruthySpellings = []string{
	"", "0", "false", "FALSE", "no", "off", "t", "y", "enabled",
}

func TestTruthy_LiveSpellings(t *testing.T) {
	t.Parallel()
	for _, sp := range liveTruthySpellings {
		sp := sp
		t.Run(fmt.Sprintf("%q", sp), func(t *testing.T) {
			t.Parallel()
			require.True(t, studiodb.Truthy(sp), "expected live for %q", sp)
		})
	}
}

func TestTruthy_NonLiveSpellings(t *testing.T) {
	t.Parallel()
	for _, sp := range nonLiveTruthySpellings {
		sp := sp
		t.Run(fmt.Sprintf("%q", sp), func(t *testing.T) {
			t.Parallel()
			require.False(t, studiodb.Truthy(sp), "expected non-live for %q", sp)
		})
	}
}

// TestWriteFreezeCLI_TruthySpellingMatrix is adversarial: every live spelling
// must refuse -write-freeze with mutated SQL and leave the manifest byte-identical.
func TestWriteFreezeCLI_TruthySpellingMatrix(t *testing.T) {
	root := moduleRoot(t)
	for _, sp := range liveTruthySpellings {
		sp := sp
		t.Run(fmt.Sprintf("%q", sp), func(t *testing.T) {
			tmp := t.TempDir()
			mig := filepath.Join(tmp, "migrations")
			copyBaselineTree(t, mig)
			mut := filepath.Join(mig, "00002_plan_domain.sql")
			b, err := os.ReadFile(mut)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(mut, append(b, []byte("\n-- spelling drift\n")...), 0o644))

			manifestPath := filepath.Join(tmp, studiodb.BaselineManifestName)
			before, err := os.ReadFile(manifestPath)
			require.NoError(t, err)

			cmd := exec.Command("go", "run", "./cmd/migrate", "-write-freeze", "-migrations", mig)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "STUDIO_MIGRATIONS_LIVE="+sp)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err = cmd.Run()
			require.Error(t, err, "write-freeze must refuse live spelling %q; stderr=%s", sp, stderr.String())
			after, err := os.ReadFile(manifestPath)
			require.NoError(t, err)
			require.Equal(t, before, after, "manifest must stay byte-identical for live spelling %q", sp)
			require.Contains(t, strings.ToLower(stderr.String()), "live")
		})
	}
}

// markerShapes returns (name, setup) pairs. setup creates the marker path and
// returns it. Any existing path shape must classify live (fail-closed).
func markerShapeSetups(t *testing.T, tmp string) []struct {
	name  string
	setup func() string
} {
	t.Helper()
	return []struct {
		name  string
		setup func() string
	}{
		{
			name: "regular-file",
			setup: func() string {
				p := filepath.Join(tmp, "regular", studiodb.LiveMarkerFilename)
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte("1\n"), 0o644))
				return p
			},
		},
		{
			name: "directory",
			setup: func() string {
				p := filepath.Join(tmp, "dirmarker", studiodb.LiveMarkerFilename)
				require.NoError(t, os.MkdirAll(p, 0o755))
				return p
			},
		},
		{
			name: "symlink-to-file",
			setup: func() string {
				base := filepath.Join(tmp, "symfile")
				require.NoError(t, os.MkdirAll(base, 0o755))
				target := filepath.Join(base, "target")
				require.NoError(t, os.WriteFile(target, []byte("1\n"), 0o644))
				p := filepath.Join(base, studiodb.LiveMarkerFilename)
				require.NoError(t, os.Symlink(target, p))
				return p
			},
		},
		{
			name: "symlink-to-dir",
			setup: func() string {
				base := filepath.Join(tmp, "symdir")
				require.NoError(t, os.MkdirAll(base, 0o755))
				target := filepath.Join(base, "targetdir")
				require.NoError(t, os.MkdirAll(target, 0o755))
				p := filepath.Join(base, studiodb.LiveMarkerFilename)
				require.NoError(t, os.Symlink(target, p))
				return p
			},
		},
	}
}

func TestLiveMarkerExists_ShapeMatrix(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	for _, tc := range markerShapeSetups(t, tmp) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := tc.setup()
			require.True(t, studiodb.LiveMarkerExists(p), "shape %s must be live", tc.name)
			require.True(t, studiodb.IsLiveEnv(false, p))
			require.Error(t, studiodb.GuardWriteFreeze(false, p))
		})
	}
	// Broken symlink: path does not resolve — treat as non-live (err != nil from Stat).
	// Documented: only existing resolvable paths classify live.
	broken := filepath.Join(tmp, "broken", studiodb.LiveMarkerFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(broken), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(tmp, "does-not-exist"), broken))
	require.False(t, studiodb.LiveMarkerExists(broken), "broken symlink must not classify live")
	require.False(t, studiodb.IsLiveEnv(false, broken))
	require.NoError(t, studiodb.GuardWriteFreeze(false, broken))
}

// TestWriteFreezeCLI_MarkerShapeMatrix: dir/symlink markers must refuse write.
func TestWriteFreezeCLI_MarkerShapeMatrix(t *testing.T) {
	root := moduleRoot(t)
	shapes := []string{"regular-file", "directory", "symlink-to-file", "symlink-to-dir"}
	for _, shape := range shapes {
		shape := shape
		t.Run(shape, func(t *testing.T) {
			tmp := t.TempDir()
			mig := filepath.Join(tmp, "migrations")
			copyBaselineTree(t, mig)
			// Place marker beside manifest (db root = tmp).
			switch shape {
			case "regular-file":
				require.NoError(t, os.WriteFile(filepath.Join(tmp, studiodb.LiveMarkerFilename), []byte("1\n"), 0o644))
			case "directory":
				require.NoError(t, os.MkdirAll(filepath.Join(tmp, studiodb.LiveMarkerFilename), 0o755))
			case "symlink-to-file":
				tgt := filepath.Join(tmp, "marker-target")
				require.NoError(t, os.WriteFile(tgt, []byte("1\n"), 0o644))
				require.NoError(t, os.Symlink(tgt, filepath.Join(tmp, studiodb.LiveMarkerFilename)))
			case "symlink-to-dir":
				tgt := filepath.Join(tmp, "marker-target-dir")
				require.NoError(t, os.MkdirAll(tgt, 0o755))
				require.NoError(t, os.Symlink(tgt, filepath.Join(tmp, studiodb.LiveMarkerFilename)))
			}
			mut := filepath.Join(mig, "00001_identity_and_catalogs.sql")
			b, err := os.ReadFile(mut)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(mut, append(b, []byte("\n-- shape drift\n")...), 0o644))
			manifestPath := filepath.Join(tmp, studiodb.BaselineManifestName)
			before, err := os.ReadFile(manifestPath)
			require.NoError(t, err)

			cmd := exec.Command("go", "run", "./cmd/migrate", "-write-freeze", "-migrations", mig)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "STUDIO_MIGRATIONS_LIVE=")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err = cmd.Run()
			require.Error(t, err, "write-freeze must refuse marker shape %s; stderr=%s", shape, stderr.String())
			after, err := os.ReadFile(manifestPath)
			require.NoError(t, err)
			require.Equal(t, before, after, "manifest unchanged for marker shape %s", shape)
		})
	}
}

// TestLoadConfig_MarkerOnlyLive folds migration-root marker into MigrationsLive
// so DownWithPolicy refuses marker-only live the same way as env-live.
func TestLoadConfig_MarkerOnlyLive(t *testing.T) {
	tmp := t.TempDir()
	mig := filepath.Join(tmp, "migrations")
	require.NoError(t, os.MkdirAll(mig, 0o755))
	// Directory marker still live (fail-closed path-shape parity).
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, studiodb.LiveMarkerFilename), 0o755))

	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@127.0.0.1:5432/curriculum_studio?sslmode=disable")
	t.Setenv("STUDIO_MIGRATIONS_LIVE", "")
	t.Setenv("STUDIO_MIGRATE_BREAK_GLASS_DOWN", "")
	t.Setenv("STUDIO_MIGRATIONS_DIR", mig)

	cfg, err := studiodb.LoadConfig()
	require.NoError(t, err)
	require.True(t, cfg.MigrationsLive, "marker-only must set MigrationsLive")
	require.False(t, cfg.AllowDown(), "marker-only live must refuse down without break-glass")

	// ApplyLiveMarker (CLI down path) agrees with LoadConfig for the same root.
	cfgCLI := studiodb.Config{DatabaseURL: cfg.DatabaseURL}
	studiodb.ApplyLiveMarker(&cfgCLI, mig)
	require.True(t, cfgCLI.MigrationsLive)
	require.False(t, cfgCLI.AllowDown())

	// Break-glass remains separate and does not require clearing marker.
	t.Setenv("STUDIO_MIGRATE_BREAK_GLASS_DOWN", "true")
	cfg2, err := studiodb.LoadConfig()
	require.NoError(t, err)
	require.True(t, cfg2.MigrationsLive)
	require.True(t, cfg2.BreakGlassDown)
	require.True(t, cfg2.AllowDown())
}

// TestApplyLiveMarker_NoFalsePositive when marker missing and env empty.
func TestApplyLiveMarker_NoFalsePositive(t *testing.T) {
	tmp := t.TempDir()
	mig := filepath.Join(tmp, "migrations")
	require.NoError(t, os.MkdirAll(mig, 0o755))
	t.Setenv("STUDIO_MIGRATIONS_LIVE", "")
	cfg := studiodb.Config{}
	studiodb.ApplyLiveMarker(&cfg, mig)
	require.False(t, cfg.MigrationsLive)
	require.True(t, cfg.AllowDown())
}

// TestClassifyLive_EnvAndMarkerParity is the shared cross-path classification.
func TestClassifyLive_EnvAndMarkerParity(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	missing := filepath.Join(tmp, studiodb.LiveMarkerFilename)
	require.False(t, studiodb.ClassifyLive("", missing))

	require.True(t, studiodb.ClassifyLive("True", missing))
	require.True(t, studiodb.ClassifyLive(" YES ", missing))
	require.True(t, studiodb.ClassifyLive("on", missing))
	require.False(t, studiodb.ClassifyLive("false", missing))
	require.False(t, studiodb.ClassifyLive("0", missing))

	require.NoError(t, os.WriteFile(missing, []byte("x"), 0o644))
	require.True(t, studiodb.ClassifyLive("", missing))
	require.True(t, studiodb.ClassifyLive("false", missing), "marker wins over non-truthy env")
}
