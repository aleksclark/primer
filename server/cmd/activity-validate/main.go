// Command activity-validate loads curriculum/activities, validates contracts,
// optionally materializes fixtures, and prints OK/ERR lines. Use --tui for a
// Bubble Tea summary list (Phase 0 offline publish slice).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aleksclark/primer/server/internal/studentclient/validatecmd"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "activity-validate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("activity-validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "path to curriculum/activities (default: auto-detect)")
	tui := fs.Bool("tui", false, "show Bubble Tea list of validation results")
	noMaterialize := fs.Bool("no-materialize", false, "skip fixture materialization and baseline checks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := validatecmd.Options{
		ActivitiesDir: *dir,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		Materialize:   !*noMaterialize,
	}
	if opts.ActivitiesDir == "" {
		opts.ActivitiesDir = validatecmd.DefaultActivitiesDir()
	}
	if *tui {
		return validatecmd.RunTUI(opts)
	}
	results, err := validatecmd.Run(opts)
	if err != nil {
		return err
	}
	if !validatecmd.AllOK(results) {
		return fmt.Errorf("validation failed")
	}
	fmt.Fprintf(os.Stdout, "validated %d activities\n", len(results))
	return nil
}
