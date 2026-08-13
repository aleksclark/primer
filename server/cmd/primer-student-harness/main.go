// Command primer-student-harness runs the Phase 2 headless student engine.
//
// TEST-ONLY: CI and scripts/student-acceptance.sh. Do not ship on workstations.
// Prefer the packaged primer-student TUI/broker for real instruction.
//
//	primer-student-harness \
//	  -base-url http://127.0.0.1:8080/api/v1 \
//	  -token "$PRIMER_DEVICE_TOKEN" \
//	  -db ./state.db \
//	  -slug basic-navigation
//
// With -tui, shows a small Bubble Tea status view after the run (or instead of
// running when -status-only is set).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	base := flag.String("base-url", envOr("PRIMER_BASE_URL", "http://127.0.0.1:8080/api/v1"), "LMS API base URL")
	token := flag.String("token", os.Getenv("PRIMER_DEVICE_TOKEN"), "student device token")
	dbPath := flag.String("db", envOr("PRIMER_STUDENT_DB", "primer-student-state.db"), "SQLite cache path")
	slug := flag.String("slug", "basic-navigation", "activity slug to run")
	assignment := flag.String("assignment", "", "assignment id (overrides slug lookup)")
	offline := flag.Bool("offline", false, "use cache only; queue completion")
	syncOnly := flag.Bool("sync-only", false, "only pull/flush; do not run activity")
	useSandbox := flag.Bool("sandbox", false, "run commands under bubblewrap")
	tui := flag.Bool("tui", false, "show status TUI after run")
	statusOnly := flag.Bool("status-only", false, "show TUI with last known empty status and exit")
	pairCode := flag.String("pair", "", "pairing code (stores token in db)")
	deviceName := flag.String("device-name", "harness", "device name when pairing")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(absPath(*dbPath)), 0o700); err != nil && !os.IsExist(err) {
		// Dir may be "."
		_ = err
	}
	store, err := cache.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	cl := studentapi.New(*base, *token)
	if tok, _ := store.DeviceToken(ctx); tok != "" && *token == "" {
		cl.SetToken(tok)
		*token = tok
	}

	if *pairCode != "" {
		pair, err := cl.Pair(ctx, *pairCode, *deviceName)
		if err != nil {
			return fmt.Errorf("pair: %w", err)
		}
		if err := store.SetDeviceToken(ctx, pair.Token); err != nil {
			return err
		}
		if err := store.SetDeviceIdentity(ctx, pair.DeviceID, pair.Student.ID, pair.Device.Name); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "paired device %s for student %s\n", pair.DeviceID, pair.Student.ID)
	}

	if *statusOnly {
		return engine.RunStatusTUI("Primer student harness", engine.Status{Phase: "idle", Message: "status-only"})
	}

	if *useSandbox && !sandbox.Available() {
		return fmt.Errorf("%w: install bubblewrap or omit -sandbox", sandbox.ErrUnavailable)
	}

	eng, err := engine.New(engine.Options{
		Client:                    cl,
		Store:                     store,
		Offline:                   *offline,
		UseSandbox:                *useSandbox,
		AllowUnsandboxed:          !*useSandbox,
		// Harness uses scripted RunShell with structured command observations.
		StructuredCommandEvidence: true,
		WorkspaceRoot:             filepath.Join(filepath.Dir(absPath(*dbPath)), "workspaces"),
	})
	if err != nil {
		return err
	}

	if *syncOnly {
		res := eng.SyncOnce(ctx)
		if res.Err != nil {
			return res.Err
		}
		fmt.Printf("sync ok: work=%d events=%d completions=%d pending_events=%d pending_completions=%d status=%s\n",
			res.WorkItems, res.EventsFlushed, res.CompletionsSent, res.PendingEvents, res.PendingCompletes, res.Status)
		if *tui {
			return engine.RunStatusTUI("Primer student harness", eng.Status())
		}
		return nil
	}

	if cl.Token == "" && !*offline {
		return fmt.Errorf("device token required (-token, PRIMER_DEVICE_TOKEN, or -pair)")
	}

	// Ensure work is cached when online.
	if !*offline {
		res := eng.SyncOnce(ctx)
		if res.Err != nil {
			fmt.Fprintf(os.Stderr, "warning: sync: %v\n", res.Err)
		}
	}

	asgID := *assignment
	if asgID == "" {
		asgID, err = eng.FindAssignmentBySlug(ctx, *slug)
		if err != nil {
			return fmt.Errorf("find assignment for slug %q: %w", *slug, err)
		}
	}

	start := time.Now()
	runErr := eng.RunAssignment(ctx, asgID, engine.BasicNavigationScript())
	st := eng.Status()
	fmt.Printf("phase=%s slug=%s checks=%d/%d required=%v completion_queued=%v acked=%v duration=%s\n",
		st.Phase, st.ActivitySlug, st.ChecksPassed, st.ChecksTotal, st.RequiredPassed,
		st.CompletionQueued, st.CompletionAcked, time.Since(start).Round(time.Millisecond))
	if runErr != nil {
		if *tui {
			_ = engine.RunStatusTUI("Primer student harness", st)
		}
		return runErr
	}
	if *tui {
		return engine.RunStatusTUI("Primer student harness", st)
	}
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
