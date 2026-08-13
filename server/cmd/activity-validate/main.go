// Command activity-validate loads curriculum/activities, validates contracts,
// optionally materializes fixtures, and prints OK/ERR lines. Use --tui for a
// Bubble Tea summary list (Phase 0 offline publish slice).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
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
	file := fs.String("file", "", "validate a single activity.yaml/json file")
	course := fs.String("course", "", "validate a course.json manifest")
	tui := fs.Bool("tui", false, "show Bubble Tea list of validation results")
	noMaterialize := fs.Bool("no-materialize", false, "skip fixture materialization and staged fixture checks")
	replay := fs.Bool("replay-reference", false, "replay authoring-only reference solutions in a sandbox")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *course != "" {
		doc, err := contracts.LoadCourseDocument(*course)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "OK  course %s (%d activities)\n", doc.Slug, len(doc.Activities))
		return nil
	}

	opts := validatecmd.Options{
		ActivitiesDir:   *dir,
		SingleFile:      *file,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Materialize:     !*noMaterialize,
		ReplayReference: *replay,
	}
	if opts.ActivitiesDir == "" && opts.SingleFile == "" {
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
	label := "activities"
	if opts.SingleFile != "" {
		label = filepath.Base(opts.SingleFile)
	}
	fmt.Fprintf(os.Stdout, "validated %d %s\n", len(results), label)
	return nil
}
