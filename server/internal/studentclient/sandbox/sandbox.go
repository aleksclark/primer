// Package sandbox builds bubblewrap launch commands for exercise workspaces.
//
// Fail closed: if bwrap is missing, Launch returns ErrUnavailable. Tests that
// need isolation should call RequireBwrap or skip via t.Skip.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrUnavailable is returned when bubblewrap cannot be used.
var ErrUnavailable = errors.New("bubblewrap (bwrap) is not available")

// DefaultBwrap is the binary name looked up on PATH.
const DefaultBwrap = "bwrap"

// Bind is an ordered host→sandbox path mount.
type Bind struct {
	Host     string
	Dest     string
	ReadOnly bool
}

// Config controls sandbox launch.
type Config struct {
	// BwrapPath is the bubblewrap binary; empty looks up DefaultBwrap on PATH.
	BwrapPath string
	// Workspace is the exercise directory bind-mounted read-write at /workspace.
	Workspace string
	// WorkDir is the cwd inside the sandbox (default /workspace).
	WorkDir string
	// ReadOnlyBinds maps host path -> sandbox path for optional tool closures.
	ReadOnlyBinds map[string]string
	// ExtraBinds are ordered binds applied after ReadOnlyBinds (needed when the
	// same host path must appear at two destinations).
	ExtraBinds []Bind
	// ExtraArgs are appended before the command argv.
	ExtraArgs []string
	// Network enables network namespace connectivity (default false = off).
	Network bool
	// Env is the environment inside the sandbox (default PATH=/usr/bin:/bin).
	Env []string
	// RuntimeProfile is the named profile last applied via ApplyProfile.
	RuntimeProfile string
	// ProfileDir is the resolved host directory for RuntimeProfile, if any.
	ProfileDir string
	// UseHostToolBinds mounts host /usr /bin /lib /lib64 when true.
	// Default (zero value) is true for backward-compatible dev/tests; ApplyProfile
	// sets it false when a Nix profile dir is available.
	// Use the pointer-like tri-state via hostToolBindsSet + useHostToolBinds.
	hostToolBinds *bool
}

// UseHostToolBinds reports whether host tool trees should be bound.
// Zero-value Config defaults to true (dev/tests). Call SetUseHostToolBinds or
// ApplyProfile to change.
func (c Config) UseHostToolBinds() bool {
	if c.hostToolBinds == nil {
		return true
	}
	return *c.hostToolBinds
}

// SetUseHostToolBinds controls host /usr+/bin+/lib binds.
func (c *Config) SetUseHostToolBinds(v bool) {
	c.hostToolBinds = &v
}

// Available reports whether bwrap is on PATH (or BwrapPath exists).
func Available() bool {
	_, err := exec.LookPath(DefaultBwrap)
	return err == nil
}

// LookPath returns the absolute bwrap path or ErrUnavailable.
func LookPath(explicit string) (string, error) {
	if explicit != "" {
		if st, err := os.Stat(explicit); err == nil && !st.IsDir() {
			return explicit, nil
		}
		return "", fmt.Errorf("%w: %s", ErrUnavailable, explicit)
	}
	p, err := exec.LookPath(DefaultBwrap)
	if err != nil {
		return "", ErrUnavailable
	}
	return p, nil
}

// BuildArgs returns the full argv for bwrap including the binary path.
// cmd is the program and args to run inside the sandbox.
func BuildArgs(cfg Config, cmd []string) ([]string, error) {
	if cfg.Workspace == "" {
		return nil, fmt.Errorf("sandbox workspace is required")
	}
	ws, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(ws)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", ws)
	}
	bwrap, err := LookPath(cfg.BwrapPath)
	if err != nil {
		return nil, err
	}
	if len(cmd) == 0 {
		return nil, fmt.Errorf("command is required")
	}
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "/workspace"
	}

	args := []string{
		bwrap,
		"--unshare-user",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--die-with-parent",
		"--new-session",
		"--dir", "/tmp",
		"--dir", "/var/tmp",
		"--proc", "/proc",
		"--dev", "/dev",
		"--bind", ws, "/workspace",
		"--chdir", workDir,
	}
	if !cfg.Network {
		args = append(args, "--unshare-net")
	}

	// Host binaries for coreutils (dev/tests). Disabled when a Nix runtime
	// profile directory is bound so the sandbox only sees the pinned closure.
	if cfg.UseHostToolBinds() {
		roDefaults := []struct{ host, dest string }{
			{"/usr", "/usr"},
			{"/bin", "/bin"},
			{"/lib", "/lib"},
			{"/lib64", "/lib64"},
		}
		for _, b := range roDefaults {
			if st, err := os.Stat(b.host); err == nil && st.IsDir() {
				args = append(args, "--ro-bind", b.host, b.dest)
			}
		}
	}
	// On NixOS, /nix/store is required to resolve profile binaries' dynamic
	// linker and shared libraries when a profile dir under /nix/store is used.
	if cfg.ProfileDir != "" || !cfg.UseHostToolBinds() {
		if st, err := os.Stat("/nix/store"); err == nil && st.IsDir() {
			args = append(args, "--ro-bind", "/nix/store", "/nix/store")
		}
	}

	// Deterministic order for map binds.
	if len(cfg.ReadOnlyBinds) > 0 {
		hosts := make([]string, 0, len(cfg.ReadOnlyBinds))
		for host := range cfg.ReadOnlyBinds {
			hosts = append(hosts, host)
		}
		slices.Sort(hosts)
		for _, host := range hosts {
			args = append(args, "--ro-bind", host, cfg.ReadOnlyBinds[host])
		}
	}
	for _, b := range cfg.ExtraBinds {
		flag := "--bind"
		if b.ReadOnly {
			flag = "--ro-bind"
		}
		args = append(args, flag, b.Host, b.Dest)
	}
	args = append(args, cfg.ExtraArgs...)
	// Clear environment then set a minimal one.
	args = append(args, "--clearenv")
	env := cfg.Env
	if len(env) == 0 {
		env = []string{"PATH=/usr/bin:/bin", "HOME=/workspace", "TERM=xterm-256color"}
	}
	for _, e := range env {
		// bwrap --setenv KEY VAL
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		args = append(args, "--setenv", parts[0], parts[1])
	}
	args = append(args, "--")
	args = append(args, cmd...)
	return args, nil
}

// Command returns an *exec.Cmd ready to Start/Run for the sandboxed command.
func Command(ctx context.Context, cfg Config, name string, arg ...string) (*exec.Cmd, error) {
	argv, err := BuildArgs(cfg, append([]string{name}, arg...))
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, argv[0], argv[1:]...), nil
}

// Run executes cmd inside the sandbox and returns combined output.
func Run(ctx context.Context, cfg Config, name string, arg ...string) ([]byte, error) {
	c, err := Command(ctx, cfg, name, arg...)
	if err != nil {
		return nil, err
	}
	return c.CombinedOutput()
}

// RequireBwrap skips the test when bubblewrap is not installed.
func RequireBwrap(t interface {
	Helper()
	Skip(...any)
}) {
	t.Helper()
	if !Available() {
		t.Skip("bwrap not installed; skipping sandbox test")
	}
}

// CanaryTestScript is a shell snippet used by isolation tests.
// It tries to read a host canary path passed as $1 and exits 0 only if unreadable.
func CanaryProbeArgs(canaryPath string) []string {
	// Use a tiny shell program: exit 0 if cannot read canary; exit 1 if can.
	return []string{
		"/bin/sh", "-c",
		`if cat "$1" >/dev/null 2>&1; then echo CANARY_LEAK; exit 1; else echo ISOLATED; exit 0; fi`,
		"canary-probe",
		canaryPath,
	}
}

// DefaultTimeout for short sandbox probes.
const DefaultTimeout = 10 * time.Second
