package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Environment variables that select a Nix-built (or local) runtime closure.
const (
	// EnvRuntimeProfileDir is a single profile tree bound read-only into the
	// sandbox (typically /nix/store/...-primer-runtime-coreutils-basic).
	EnvRuntimeProfileDir = "PRIMER_RUNTIME_PROFILE_DIR"
	// EnvRuntimeProfilesDir is a parent directory containing named profile
	// subdirectories (e.g. $dir/coreutils-basic).
	EnvRuntimeProfilesDir = "PRIMER_RUNTIME_PROFILES_DIR"
)

// Profile describes an allowlisted terminal runtime tool closure.
type Profile struct {
	// Name matches contracts.Runtime* / activity.yaml runtime_profile.
	Name string
	// Binaries documents the tools the profile is expected to provide.
	Binaries []string
}

// KnownProfiles is the allowlist of runtime_profile values.
var KnownProfiles = map[string]Profile{
	"coreutils-basic": {
		Name: "coreutils-basic",
		Binaries: []string{
			"sh", "bash", "ls", "pwd", "cat", "mv", "cp", "mkdir", "rm", "rmdir",
			"echo", "true", "false", "head", "tail", "grep", "sed", "awk", "find",
			"diff", "touch", "chmod", "ln", "wc", "sort", "uniq", "tee", "env",
			"printf", "test", "[", "basename", "dirname", "realpath", "sleep",
		},
	},
	"text-processing": {
		Name: "text-processing",
		Binaries: []string{
			"sh", "bash", "ls", "pwd", "cat", "mv", "cp", "mkdir", "rm",
			"echo", "true", "false", "head", "tail", "grep", "sed", "awk",
			"find", "diff", "sort", "uniq", "cut", "tr", "wc", "tee",
		},
	},
}

// KnownProfileNames returns sorted profile names for errors and docs.
func KnownProfileNames() []string {
	names := make([]string, 0, len(KnownProfiles))
	for n := range KnownProfiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LookupProfile returns a known profile or an error.
func LookupProfile(name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, fmt.Errorf("runtime profile name is required")
	}
	p, ok := KnownProfiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown runtime profile %q (known: %s)", name, strings.Join(KnownProfileNames(), ", "))
	}
	return p, nil
}

// ResolveProfileDir finds the host directory for a named profile.
//
// Resolution order:
//  1. PRIMER_RUNTIME_PROFILES_DIR/<name> when that directory exists
//  2. PRIMER_RUNTIME_PROFILE_DIR when set (single-profile deployments)
//  3. empty string — caller should use host fallback binds (dev/tests)
//
// A missing named subdirectory under PROFILES_DIR is an error when that env is
// set, so production misconfiguration fails closed. PROFILE_DIR alone is used
// as the sole installed profile regardless of name (workstation default).
func ResolveProfileDir(name string) (string, error) {
	if _, err := LookupProfile(name); err != nil {
		return "", err
	}
	if parent := strings.TrimSpace(os.Getenv(EnvRuntimeProfilesDir)); parent != "" {
		dir := filepath.Join(parent, name)
		st, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("runtime profile %q: %w (under %s)", name, err, parent)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("runtime profile %q path is not a directory: %s", name, dir)
		}
		return dir, nil
	}
	if dir := strings.TrimSpace(os.Getenv(EnvRuntimeProfileDir)); dir != "" {
		st, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("%s=%s: %w", EnvRuntimeProfileDir, dir, err)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("%s is not a directory: %s", EnvRuntimeProfileDir, dir)
		}
		return dir, nil
	}
	return "", nil
}

// ApplyProfile mutates cfg to use the named runtime profile's tool closure.
//
// When a profile directory is resolved, host /usr+/bin+/lib binds are skipped
// and the profile tree is bound read-only. When no profile directory is
// configured (dev/tests), host fallback binds stay enabled so existing sandbox
// tests keep working.
func ApplyProfile(cfg *Config, profileName string) error {
	if cfg == nil {
		return fmt.Errorf("sandbox config is nil")
	}
	name := strings.TrimSpace(profileName)
	if name == "" {
		// No profile requested: keep host fallback (dev default).
		return nil
	}
	if _, err := LookupProfile(name); err != nil {
		return err
	}
	dir, err := ResolveProfileDir(name)
	if err != nil {
		return err
	}
	cfg.RuntimeProfile = name
	if dir == "" {
		// Dev/tests: host binds remain.
		cfg.SetUseHostToolBinds(true)
		return nil
	}
	cfg.SetUseHostToolBinds(false)
	cfg.ProfileDir = dir
	if cfg.ReadOnlyBinds == nil {
		cfg.ReadOnlyBinds = map[string]string{}
	}
	cfg.ReadOnlyBinds[dir] = "/runtime"
	// Bind bin to both classic locations. ReadOnlyBinds is a map so we use
	// ordered ExtraBinds for the second destination.
	bin := filepath.Join(dir, "bin")
	if st, err := os.Stat(bin); err == nil && st.IsDir() {
		cfg.ReadOnlyBinds[bin] = "/bin"
		cfg.ExtraBinds = append(cfg.ExtraBinds, Bind{Host: bin, Dest: "/usr/bin", ReadOnly: true})
	}
	if len(cfg.Env) == 0 {
		cfg.Env = []string{
			"PATH=/runtime/bin:/bin:/usr/bin",
			"HOME=/workspace",
			"TERM=xterm-256color",
		}
	}
	return nil
}
