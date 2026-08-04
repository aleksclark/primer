// Command primer-student-stub is a tiny work-queue TUI for Phase 1 demos/tests.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aleksclark/primer/server/internal/studentclient/worktui"
)

func main() {
	base := flag.String("base-url", "http://127.0.0.1:8080/api/v1", "LMS API base URL")
	token := flag.String("token", os.Getenv("PRIMER_DEVICE_TOKEN"), "student device token")
	flag.Parse()
	if *token == "" {
		fmt.Fprintln(os.Stderr, "device token required (-token or PRIMER_DEVICE_TOKEN)")
		os.Exit(2)
	}
	if err := worktui.Run(worktui.Options{BaseURL: *base, DeviceToken: *token}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
