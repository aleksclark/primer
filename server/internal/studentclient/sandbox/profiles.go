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
//
// Manifest notes (Phase 5 audit):
//   - whoami and id are provided by coreutils and included in coreutils-basic.
//   - file(1) and process tools (ps, top) are intentionally omitted until a
//     course objective requires them; do not widen closures preemptively.
//   - Prefer PRIMER_RUNTIME_PROFILES_DIR with per-name subdirectories so
//     coreutils-basic and text-processing resolve to distinct Nix closures.
//   - PRIMER_RUNTIME_PROFILE_DIR remains a single-closure migration fallback:
//     when set alone, every known profile name maps to that one directory.
//     Production should migrate to PROFILES_DIR; health checks warn on the
//     singular fallback.
type Profile struct {
	// Name matches contracts.Runtime* / activity.yaml runtime_profile.
	Name string
	// Binaries documents the tools the profile is expected to provide.
	// Health checks verify these exist under <profileDir>/bin when configured.
	Binaries []string
}

// KnownProfiles is the allowlist of runtime_profile values.
// Binary lists are the authored contract; verify against the Nix closure at
// health-check time rather than treating this list as ground truth alone.
var KnownProfiles = map[string]Profile{
	"coreutils-basic": {
		Name: "coreutils-basic",
		Binaries: []string{
			"sh", "bash", "ls", "pwd", "cat", "mv", "cp", "mkdir", "rm", "rmdir",
			"echo", "true", "false", "head", "tail", "grep", "sed", "awk", "find",
			"diff", "touch", "chmod", "ln", "wc", "sort", "uniq", "tee", "env",
			"printf", "test", "[", "basename", "dirname", "realpath", "sleep",
			// Identity tools from coreutils (lessons may reference them).
			"whoami", "id",
		},
	},
	"text-processing": {
		Name: "text-processing",
		Binaries: []string{
			"sh", "bash", "ls", "pwd", "cat", "mv", "cp", "mkdir", "rm",
			"echo", "true", "false", "head", "tail", "grep", "sed", "awk",
			"find", "diff", "sort", "uniq", "cut", "tr", "wc", "tee",
			"printf", "env",
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
//  1. PRIMER_RUNTIME_PROFILES_DIR/<name> when PROFILES_DIR is set (preferred)
//  2. PRIMER_RUNTIME_PROFILE_DIR when set (single-profile migration fallback)
//  3. empty string — caller should use host fallback binds (dev/tests only)
//
// A missing named subdirectory under PROFILES_DIR is an error when that env is
// set, so production misconfiguration fails closed. PROFILE_DIR alone is used
// as the sole installed profile regardless of name (legacy workstation default).
// When both are set, PROFILES_DIR wins and PROFILE_DIR is ignored.
func ResolveProfileDir(name string) (string, error) {
	if _, err := LookupProfile(name); err != nil {
		return "", err
	}
	if parent := strings.TrimSpace(os.Getenv(EnvRuntimeProfilesDir)); parent != "" {
		dir := filepath.Join(parent, name)
		st, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("runtime profile %q: %w (under %s=%s)", name, err, EnvRuntimeProfilesDir, parent)
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

// UsingSingularProfileFallback reports whether only PROFILE_DIR is configured
// (migration path). Production should prefer PROFILES_DIR.
func UsingSingularProfileFallback() bool {
	return strings.TrimSpace(os.Getenv(EnvRuntimeProfilesDir)) == "" &&
		strings.TrimSpace(os.Getenv(EnvRuntimeProfileDir)) != ""
}

// VerifyProfileBinaries checks that declared binaries exist under profileDir/bin.
// Missing entries are returned as a list; empty means all present (or profileDir empty).
func VerifyProfileBinaries(profileName, profileDir string) (missing []string, err error) {
	p, err := LookupProfile(profileName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(profileDir) == "" {
		return nil, nil
	}
	binDir := filepath.Join(profileDir, "bin")
	st, err := os.Stat(binDir)
	if err != nil || !st.IsDir() {
		// Some closures put binaries at the root; try profileDir itself.
		binDir = profileDir
	}
	for _, b := range p.Binaries {
		path := filepath.Join(binDir, b)
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, b)
		}
	}
	return missing, nil
}

// InstalledProfileNames returns known profile names that currently resolve to a directory.
func InstalledProfileNames() []string {
	var out []string
	for _, name := range KnownProfileNames() {
		dir, err := ResolveProfileDir(name)
		if err == nil && dir != "" {
			out = append(out, name)
		}
	}
	return out
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
