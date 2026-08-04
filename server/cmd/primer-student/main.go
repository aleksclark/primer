// Command primer-student is the Phase 3 interactive student workstation TUI.
//
//	primer-student -base-url http://127.0.0.1:8080/api/v1 -db ./state.db
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/app"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
)

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
	flag.Parse()

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
