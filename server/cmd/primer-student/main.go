// Command primer-student is the Phase 3 interactive student workstation TUI.
//
//	primer-student -base-url http://127.0.0.1:8080/api/v1 -db ./state.db
//	primer-student -version
//	primer-student -health
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/app"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	versionpkg "github.com/aleksclark/primer/server/internal/studentclient/version"
)

// Populated via -ldflags at build time (Nix package / make student-build):
//
//	-X main.version=… -X main.commit=…
var (
	version = "dev"
	commit  = "unknown"
)

func init() {
	if version != "" {
		versionpkg.Version = version
	}
	if commit != "" {
		versionpkg.Commit = commit
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	base := flag.String("base-url", envOr("PRIMER_BASE_URL", "http://127.0.0.1:8080/api/v1"), "LMS API base URL")
	dbPath := flag.String("db", envOr("PRIMER_STUDENT_DB", "primer-student-state.db"), "SQLite cache path")
	token := flag.String("token", os.Getenv("PRIMER_DEVICE_TOKEN"), "optional device token (also read from db)")
	deviceName := flag.String("device-name", envOr("PRIMER_DEVICE_NAME", "workstation"), "device name used when pairing")
	offline := flag.Bool("offline", false, "use cache only; queue completion")
	ws := flag.String("workspace", "", "workspace root (default: <db-dir>/workspaces)")
	showVersion := flag.Bool("version", false, "print version and exit")
	health := flag.Bool("health", false, "run local health checks and exit")
	flag.Parse()

	// Also accept subcommand-style: primer-student version | health
	if !*showVersion && !*health && flag.NArg() > 0 {
		switch flag.Arg(0) {
		case "version", "-version", "--version":
			*showVersion = true
		case "health", "-health", "--health":
			*health = true
		}
	}

	if *showVersion {
		fmt.Printf("primer-student %s\n", versionpkg.String())
		return nil
	}
	if *health {
		return runHealth(*base, *dbPath)
	}

	store, err := cache.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	cl := studentapi.New(*base, *token)
	wsRoot := *ws
	if wsRoot == "" {
		wsRoot = filepath.Join(filepath.Dir(absPath(*dbPath)), "workspaces")
	}

	return app.Run(app.Options{
		BaseURL:          *base,
		Store:            store,
		Client:           cl,
		WorkspaceRoot:    wsRoot,
		DeviceName:       *deviceName,
		AllowUnsandboxed: true,
		Offline:          *offline,
	})
}

func runHealth(baseURL, dbPath string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "primer-student"
	}
	fmt.Printf("ok: version %s\n", versionpkg.String())
	fmt.Printf("ok: package_path %s\n", exe)
	fmt.Printf("ok: base_url %s\n", baseURL)

	errors := 0
	dbDir := filepath.Dir(absPath(dbPath))
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot create state dir %s: %v\n", dbDir, err)
		errors++
	} else {
		probe := filepath.Join(dbDir, ".health-write")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: state dir not writable %s: %v\n", dbDir, err)
			errors++
		} else {
			_ = os.Remove(probe)
			fmt.Printf("ok: state dir writable %s\n", dbDir)
		}
	}

	if sandbox.Available() {
		path, _ := exec.LookPath(sandbox.DefaultBwrap)
		fmt.Printf("ok: bwrap present %s\n", path)
	} else {
		fmt.Fprintf(os.Stderr, "FAIL: bubblewrap (bwrap) not on PATH\n")
		errors++
	}

	profileDir := strings.TrimSpace(os.Getenv(sandbox.EnvRuntimeProfileDir))
	profilesDir := strings.TrimSpace(os.Getenv(sandbox.EnvRuntimeProfilesDir))
	switch {
	case profilesDir != "":
		fmt.Printf("ok: runtime_profiles_dir %s\n", profilesDir)
		if _, err := sandbox.ResolveProfileDir("coreutils-basic"); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: coreutils-basic profile: %v\n", err)
			errors++
		} else {
			fmt.Printf("ok: runtime_profile coreutils-basic\n")
		}
	case profileDir != "":
		fmt.Printf("ok: runtime_profile_dir %s\n", profileDir)
		if st, err := os.Stat(profileDir); err != nil || !st.IsDir() {
			fmt.Fprintf(os.Stderr, "FAIL: %s not a directory\n", sandbox.EnvRuntimeProfileDir)
			errors++
		}
	default:
		fmt.Printf("WARN: no %s / %s (dev host binds will be used)\n",
			sandbox.EnvRuntimeProfileDir, sandbox.EnvRuntimeProfilesDir)
	}

	if errors != 0 {
		return fmt.Errorf("primer-student health: %d check(s) failed", errors)
	}
	fmt.Println("primer-student-health: ok")
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func absPath(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
