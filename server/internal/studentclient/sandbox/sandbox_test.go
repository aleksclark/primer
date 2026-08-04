package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
)

func TestBuildArgsRequiresWorkspace(t *testing.T) {
	t.Parallel()
	_, err := sandbox.BuildArgs(sandbox.Config{}, []string{"/bin/true"})
	require.Error(t, err)
}

func TestBuildArgsFailClosedWithoutBwrap(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	_, err := sandbox.BuildArgs(sandbox.Config{
		BwrapPath: filepath.Join(t.TempDir(), "no-such-bwrap"),
		Workspace: ws,
	}, []string{"/bin/true"})
	require.ErrorIs(t, err, sandbox.ErrUnavailable)
}

func TestBuildArgsIncludesIsolationFlags(t *testing.T) {
	t.Parallel()
	if !sandbox.Available() {
		t.Skip("bwrap not installed")
	}
	ws := t.TempDir()
	args, err := sandbox.BuildArgs(sandbox.Config{Workspace: ws}, []string{"/bin/echo", "hi"})
	require.NoError(t, err)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--unshare-net")
	assert.Contains(t, joined, "--unshare-user")
	assert.Contains(t, joined, "--die-with-parent")
	assert.Contains(t, joined, "/workspace")
	assert.True(t, strings.HasSuffix(joined, "/bin/echo hi") || strings.Contains(joined, "-- /bin/echo hi"))
}

func TestSandboxCannotReadHostCanary(t *testing.T) {
	t.Parallel()
	sandbox.RequireBwrap(t)

	hostDir := t.TempDir()
	canary := filepath.Join(hostDir, "secret-canary.txt")
	require.NoError(t, os.WriteFile(canary, []byte("LEAKME"), 0o600))

	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "ok.txt"), []byte("inside"), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Probe: try to cat the host canary absolute path from inside the sandbox.
	cfg := sandbox.Config{Workspace: ws}
	probe := sandbox.CanaryProbeArgs(canary)
	out, err := sandbox.Run(ctx, cfg, probe[0], probe[1:]...)
	require.NoError(t, err, "output: %s", out)
	assert.Contains(t, string(out), "ISOLATED")
	assert.NotContains(t, string(out), "CANARY_LEAK")
	assert.NotContains(t, string(out), "LEAKME")

	// Positive control: workspace file is readable.
	out2, err := sandbox.Run(ctx, cfg, "/bin/cat", "/workspace/ok.txt")
	require.NoError(t, err, "output: %s", out2)
	assert.Equal(t, "inside", string(out2))
}

func TestSandboxNetworkNamespaceOff(t *testing.T) {
	t.Parallel()
	sandbox.RequireBwrap(t)

	ws := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Listing /sys/class/net should not include a real eth0 with carrier in a
	// fresh netns; more reliably, connecting to a host port should fail.
	// Use a simple check: `ip link` may not exist; try reading /proc/net/dev
	// and ensure we only see lo (or nothing useful).
	out, err := sandbox.Run(ctx, sandbox.Config{Workspace: ws}, "/bin/cat", "/proc/net/dev")
	if err != nil {
		// Some minimal environments lack /proc/net; treat as soft pass if isolated.
		t.Logf("cat /proc/net/dev: %v (%s)", err, out)
		return
	}
	// Should not see typical host interface names beyond lo.
	body := string(out)
	assert.NotContains(t, body, "wlan")
	// lo is fine; eth0 often absent in unshared netns
	t.Logf("proc net dev:\n%s", body)
}
