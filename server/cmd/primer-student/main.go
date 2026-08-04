// Command primer-student is the student workstation client.
//
//	primer-student broker -socket PATH -db PATH -base-url URL -token-file PATH
//	primer-student tui -socket PATH
//	primer-student health -socket PATH | -db PATH
//	primer-student -version
//
// Default (no subcommand): if the broker socket exists and is healthy, run tui;
// else require PRIMER_STUDENT_DIRECT=1 for legacy direct mode (tests only).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/app"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
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
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "broker":
			return runBroker(args[1:])
		case "tui":
			return runTUI(args[1:])
		case "health":
			return runHealthCmd(args[1:])
		case "version", "-version", "--version":
			fmt.Printf("primer-student %s\n", versionpkg.String())
			return nil
		case "-h", "-help", "--help", "help":
			printUsage()
			return nil
		}
	}

	// Legacy flags without subcommand (tests + gradual migration).
	fs := flag.NewFlagSet("primer-student", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	base := fs.String("base-url", envOr("PRIMER_BASE_URL", "http://127.0.0.1:8080/api/v1"), "LMS API base URL")
	dbPath := fs.String("db", envOr("PRIMER_STUDENT_DB", "primer-student-state.db"), "SQLite cache path (direct mode)")
	token := fs.String("token", os.Getenv("PRIMER_DEVICE_TOKEN"), "optional device token (direct mode)")
	deviceName := fs.String("device-name", envOr("PRIMER_DEVICE_NAME", "workstation"), "device name used when pairing")
	offline := fs.Bool("offline", false, "use cache only; queue completion")
	ws := fs.String("workspace", "", "workspace root (default: <db-dir>/workspaces)")
	socket := fs.String("socket", envOr("PRIMER_BROKER_SOCKET", defaultSocket()), "broker Unix socket")
	showVersion := fs.Bool("version", false, "print version and exit")
	health := fs.Bool("health", false, "run local health checks and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("primer-student %s\n", versionpkg.String())
		return nil
	}
	if *health {
		return runHealth(*socket, *base, *dbPath)
	}

	// Prefer broker when socket is healthy.
	if sockOK(*socket) {
		return runTUI([]string{"-socket", *socket, "-device-name", *deviceName, offlineFlag(*offline)})
	}

	// Direct mode only when explicitly enabled (unit/e2e tests).
	if os.Getenv("PRIMER_STUDENT_DIRECT") != "1" {
		printUsage()
		return fmt.Errorf("broker not available at %s; start `primer-student broker` or set PRIMER_STUDENT_DIRECT=1 for tests", *socket)
	}

	return runDirect(*base, *dbPath, *token, *deviceName, *ws, *offline)
}

func offlineFlag(v bool) string {
	if v {
		return "-offline"
	}
	return ""
}

func runBroker(args []string) error {
	fs := flag.NewFlagSet("broker", flag.ExitOnError)
	socket := fs.String("socket", envOr("PRIMER_BROKER_SOCKET", defaultSocket()), "Unix domain socket path")
	dbPath := fs.String("db", envOr("PRIMER_STUDENT_DB", "/var/lib/primer-student/state.db"), "SQLite cache path")
	tokenFile := fs.String("token-file", envOr("PRIMER_TOKEN_FILE", ""), "device token file (0600); default <db-dir>/device.token")
	base := fs.String("base-url", envOr("PRIMER_BASE_URL", "http://127.0.0.1:8080/api/v1"), "LMS API base URL")
	ws := fs.String("workspace", "", "workspace root (default: <db-dir>/workspaces)")
	deviceName := fs.String("device-name", envOr("PRIMER_DEVICE_NAME", "workstation"), "device name used when pairing")
	legacyDB := fs.String("legacy-db", envOr("PRIMER_LEGACY_DB", ""), "optional student-owned state.db to migrate")
	allowUnsandboxed := fs.Bool("allow-unsandboxed", false, "permit plain exec without bwrap (tests only)")
	offline := fs.Bool("offline", false, "skip network")
	skipPeer := fs.Bool("skip-peer-cred", false, "disable SO_PEERCRED checks (tests only)")
	socketGroup := fs.String("socket-group", envOr("PRIMER_SOCKET_GROUP", "students"), "group owner for the listening socket")
	_ = fs.Parse(args)

	tf := *tokenFile
	if tf == "" {
		tf = filepath.Join(filepath.Dir(absPath(*dbPath)), broker.DefaultTokenFileName)
	}
	wsRoot := *ws
	if wsRoot == "" {
		wsRoot = filepath.Join(filepath.Dir(absPath(*dbPath)), "workspaces")
	}

	srv, err := broker.New(broker.Options{
		SocketPath:       *socket,
		DBPath:           *dbPath,
		TokenFile:        tf,
		BaseURL:          *base,
		WorkspaceRoot:    wsRoot,
		DeviceName:       *deviceName,
		LegacyDBPath:     *legacyDB,
		UseSandbox:       !*allowUnsandboxed,
		AllowUnsandboxed: *allowUnsandboxed,
		Offline:          *offline,
		SocketGroup:      *socketGroup,
		SkipPeerCred:     *skipPeer || os.Getenv("PRIMER_BROKER_SKIP_PEERCRED") == "1",
		Version:          versionpkg.String(),
	})
	if err != nil {
		return err
	}
	defer srv.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		_ = srv.Close()
		return nil
	case err := <-errCh:
		return err
	}
}

func runTUI(args []string) error {
	// Filter empty args from offlineFlag helper.
	clean := make([]string, 0, len(args))
	for _, a := range args {
		if a != "" {
			clean = append(clean, a)
		}
	}
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	socket := fs.String("socket", envOr("PRIMER_BROKER_SOCKET", defaultSocket()), "broker Unix socket")
	deviceName := fs.String("device-name", envOr("PRIMER_DEVICE_NAME", "workstation"), "device name used when pairing")
	offline := fs.Bool("offline", false, "use cache only (broker still owns network)")
	// Direct-mode flags kept for PRIMER_STUDENT_DIRECT tests invoked as `tui`.
	base := fs.String("base-url", envOr("PRIMER_BASE_URL", "http://127.0.0.1:8080/api/v1"), "LMS API base URL (direct mode)")
	dbPath := fs.String("db", envOr("PRIMER_STUDENT_DB", "primer-student-state.db"), "SQLite path (direct mode)")
	token := fs.String("token", os.Getenv("PRIMER_DEVICE_TOKEN"), "token (direct mode)")
	ws := fs.String("workspace", "", "workspace root (direct mode)")
	_ = fs.Parse(clean)

	if sockOK(*socket) {
		cl, err := broker.Dial(*socket)
		if err != nil {
			return err
		}
		defer cl.Close()
		return app.Run(app.Options{
			Broker:     cl,
			DeviceName: *deviceName,
			Offline:    *offline,
		})
	}

	if os.Getenv("PRIMER_STUDENT_DIRECT") == "1" {
		return runDirect(*base, *dbPath, *token, *deviceName, *ws, *offline)
	}
	return fmt.Errorf("broker socket not available: %s", *socket)
}

func runDirect(base, dbPath, token, deviceName, ws string, offline bool) error {
	store, err := cache.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	cl := studentapi.New(base, token)
	wsRoot := ws
	if wsRoot == "" {
		wsRoot = filepath.Join(filepath.Dir(absPath(dbPath)), "workspaces")
	}
	// Direct mode is for tests; allow unsandboxed so CI without bwrap works.
	return app.Run(app.Options{
		BaseURL:          base,
		Store:            store,
		Client:           cl,
		WorkspaceRoot:    wsRoot,
		DeviceName:       deviceName,
		AllowUnsandboxed: true,
		Offline:          offline,
	})
}

func runHealthCmd(args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	socket := fs.String("socket", envOr("PRIMER_BROKER_SOCKET", defaultSocket()), "broker Unix socket")
	base := fs.String("base-url", envOr("PRIMER_BASE_URL", "http://127.0.0.1:8080/api/v1"), "LMS API base URL")
	dbPath := fs.String("db", envOr("PRIMER_STUDENT_DB", "primer-student-state.db"), "SQLite path for local checks")
	_ = fs.Parse(args)
	return runHealth(*socket, *base, *dbPath)
}

func runHealth(socket, baseURL, dbPath string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "primer-student"
	}
	fmt.Printf("ok: version %s\n", versionpkg.String())
	fmt.Printf("ok: package_path %s\n", exe)
	fmt.Printf("ok: base_url %s\n", baseURL)

	errors := 0

	if socket != "" {
		if sockOK(socket) {
			fmt.Printf("ok: broker socket %s\n", socket)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			cl, err := broker.Dial(socket)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: broker dial: %v\n", err)
				errors++
			} else {
				h, err := cl.Health(ctx)
				_ = cl.Close()
				if err != nil {
					fmt.Fprintf(os.Stderr, "FAIL: broker health: %v\n", err)
					errors++
				} else {
					fmt.Printf("ok: broker health paired=%v sandbox=%v\n", h.Paired, h.SandboxOK)
					if h.AllowUnsandboxed {
						fmt.Fprintf(os.Stderr, "WARN: broker AllowUnsandboxed=true (not for production)\n")
					}
				}
			}
			cancel()
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: broker socket missing/unhealthy %s\n", socket)
			errors++
		}
	}

	dbDir := filepath.Dir(absPath(dbPath))
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot create state dir %s: %v\n", dbDir, err)
		errors++
	} else {
		probe := filepath.Join(dbDir, ".health-write")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			// Broker-owned dir may not be writable by student — warn only.
			fmt.Printf("WARN: state dir not writable by this user %s: %v\n", dbDir, err)
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

func sockOK(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cl, err := broker.Dial(path)
	if err != nil {
		return false
	}
	defer cl.Close()
	_, err = cl.Health(ctx)
	return err == nil
}

func defaultSocket() string {
	return "/run/primer-student/broker.sock"
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `primer-student %s — Primer student workstation client

Usage:
  primer-student broker  [flags]   privileged broker (token, SQLite, sandbox, API)
  primer-student tui     [flags]   unprivileged TUI over Unix socket
  primer-student health  [flags]   health checks (broker socket and/or local)
  primer-student -version

Broker flags:
  -socket PATH       Unix socket (default %s)
  -db PATH           SQLite path
  -token-file PATH   device token file (mode 0600)
  -base-url URL      LMS API base URL
  -legacy-db PATH    migrate old student-owned state.db on first start

TUI flags:
  -socket PATH       broker socket

Environment:
  PRIMER_BROKER_SOCKET   default socket path
  PRIMER_STUDENT_DIRECT=1  allow direct Store/Client mode (tests only)
  PRIMER_BROKER_SKIP_PEERCRED=1  disable SO_PEERCRED (tests only)
`, versionpkg.String(), defaultSocket())
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
