package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	versionpkg "github.com/aleksclark/primer/server/internal/studentclient/version"
)

// CollectDeviceCapabilities inspects the local runtime profile environment and
// runner flags. Returns nil when nothing useful is known.
func CollectDeviceCapabilities() *studentapi.DeviceCapabilities {
	caps := studentapi.DeviceCapabilities{
		RunnerVersion:  versionpkg.String(),
		ProfileDigests: map[string]string{},
	}

	// Structured evidence when bash is available (same heuristic as engine).
	if _, err := exec.LookPath("bash"); err == nil {
		caps.Capabilities = append(caps.Capabilities, contracts.CapStructuredCommandEvidence)
	}

	profilesDir := strings.TrimSpace(os.Getenv(sandbox.EnvRuntimeProfilesDir))
	profileDir := strings.TrimSpace(os.Getenv(sandbox.EnvRuntimeProfileDir))

	switch {
	case profilesDir != "":
		for _, name := range sandbox.KnownProfileNames() {
			dir := filepath.Join(profilesDir, name)
			st, err := os.Stat(dir)
			if err != nil || !st.IsDir() {
				continue
			}
			caps.RuntimeProfiles = append(caps.RuntimeProfiles, name)
			// Digest: prefer a PROFILE_DIGEST file, else the resolved path (Nix store hash).
			if d := readProfileDigest(dir); d != "" {
				caps.ProfileDigests[name] = d
			} else {
				caps.ProfileDigests[name] = filepath.Base(dir)
			}
		}
	case profileDir != "":
		// Singular PROFILE_DIR migration fallback: report every known name that
		// resolves, but record that they share one closure path.
		st, err := os.Stat(profileDir)
		if err == nil && st.IsDir() {
			digest := readProfileDigest(profileDir)
			if digest == "" {
				digest = filepath.Base(profileDir)
			}
			for _, name := range sandbox.KnownProfileNames() {
				// ResolveProfileDir accepts PROFILE_DIR for any known name.
				if dir, err := sandbox.ResolveProfileDir(name); err == nil && dir != "" {
					caps.RuntimeProfiles = append(caps.RuntimeProfiles, name)
					caps.ProfileDigests[name] = digest
				}
			}
		}
	}

	sort.Strings(caps.RuntimeProfiles)
	sort.Strings(caps.Capabilities)

	if len(caps.RuntimeProfiles) == 0 && len(caps.Capabilities) == 0 && caps.RunnerVersion == "" {
		return nil
	}
	return &caps
}

func readProfileDigest(dir string) string {
	for _, name := range []string{"PROFILE_DIGEST", "DIGEST", ".primer-profile-digest"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(b))
		if s != "" {
			return s
		}
	}
	return ""
}
