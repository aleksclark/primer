package validatecmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

// Result is the outcome for one activity document.
type Result struct {
	Slug         string
	Path         string
	Title        string
	OK           bool
	Error        string
	Checks       int
	Tasks        int
	Fixtures     int
	Warnings     []string
	StageSummary string
	Capabilities []string
}

// Options configures a validation run.
type Options struct {
	ActivitiesDir string
	// SingleFile validates one activity path instead of a directory of activities.
	SingleFile string
	Stdout     io.Writer
	Stderr     io.Writer
	// Materialize runs fixture materialization + staged fixture/invariant checks.
	Materialize bool
	// ReplayReference runs optional authoring-only reference solutions in a sandbox.
	ReplayReference bool
	// Quiet suppresses per-activity OK lines (errors/warnings still print).
	Quiet bool
}

// DefaultActivitiesDir resolves curriculum/activities relative to common CWDs.
func DefaultActivitiesDir() string {
	candidates := []string{
		filepath.Join("curriculum", "activities"),
		filepath.Join("..", "curriculum", "activities"),
		filepath.Join("..", "..", "curriculum", "activities"),
		filepath.Join("..", "..", "..", "curriculum", "activities"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return filepath.Join("curriculum", "activities")
}

// Run loads and validates all activities under opts.ActivitiesDir.
func Run(opts Options) ([]Result, error) {
	out := opts.Stdout
	if out == nil {
		out = io.Discard
	}
	errOut := opts.Stderr
	if errOut == nil {
		errOut = io.Discard
	}

	if opts.SingleFile != "" {
		res := validateFile(opts.SingleFile, opts)
		printResult(out, errOut, res, opts.Quiet)
		return []Result{res}, nil
	}

	dir := opts.ActivitiesDir
	if dir == "" {
		dir = DefaultActivitiesDir()
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read activities dir %s: %w", dir, err)
	}

	var results []Result
	var subdirs []string
	for _, e := range entries {
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
		}
	}
	sort.Strings(subdirs)
	if len(subdirs) == 0 {
		return nil, fmt.Errorf("no activity directories in %s", dir)
	}

	for _, name := range subdirs {
		res := validateOne(filepath.Join(dir, name), name, opts)
		results = append(results, res)
		printResult(out, errOut, res, opts.Quiet)
	}
	return results, nil
}

func printResult(out, errOut io.Writer, res Result, quiet bool) {
	if res.OK {
		if quiet {
			return
		}
		extra := res.StageSummary
		if len(res.Capabilities) > 0 {
			extra += " caps=" + strings.Join(res.Capabilities, ",")
		}
		fmt.Fprintf(out, "OK  %s (%d tasks, %d checks, %d fixtures; %s)\n",
			res.Slug, res.Tasks, res.Checks, res.Fixtures, extra)
		for _, w := range res.Warnings {
			fmt.Fprintf(out, "  warn: %s\n", w)
		}
		return
	}
	fmt.Fprintf(errOut, "ERR %s: %s\n", res.Slug, res.Error)
	for _, w := range res.Warnings {
		fmt.Fprintf(errOut, "  warn: %s\n", w)
	}
}

func validateOne(dir, name string, opts Options) Result {
	res := Result{Slug: name, Path: dir}
	path, err := findActivityFile(dir)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Path = path
	return validateFile(path, opts)
}

func validateFile(path string, opts Options) Result {
	res := Result{Path: path, Slug: filepath.Base(filepath.Dir(path))}
	doc, err := contracts.LoadDocument(path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if opts.SingleFile == "" {
		want := filepath.Base(filepath.Dir(path))
		if doc.Slug != want {
			res.Error = fmt.Sprintf("slug %q must match directory %q", doc.Slug, want)
			return res
		}
	}

	res.Slug = doc.Slug
	res.Title = doc.Title
	res.Tasks = len(doc.Content.Tasks)
	res.Checks = len(doc.Content.Checks)
	if doc.Content.Terminal != nil {
		res.Fixtures = len(doc.Content.Terminal.Fixtures)
	}

	diag := contracts.AnalyzeDocument(doc)
	res.Warnings = append(res.Warnings, diag.Warnings...)
	res.StageSummary = contracts.FormatStageSummary(diag.StageCounts)
	res.Capabilities = diag.Capabilities
	if len(diag.Errors) > 0 {
		res.Error = strings.Join(diag.Errors, "; ")
		return res
	}

	if opts.Materialize && doc.Kind == contracts.KindTerminal && doc.Content.Terminal != nil {
		if err := runMaterializedValidation(doc, opts.ReplayReference); err != nil {
			res.Error = err.Error()
			return res
		}
	} else if opts.ReplayReference && doc.ReferenceSolution != nil {
		res.Error = "reference replay requires materialize for terminal activities"
		return res
	}

	res.OK = true
	return res
}

func runMaterializedValidation(doc *contracts.ActivityDocument, replay bool) error {
	tmp, err := os.MkdirTemp("", "primer-activity-"+doc.Slug+"-*")
	if err != nil {
		return fmt.Errorf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	if err := terminal.Materialize(tmp, doc.Content.Terminal.Fixtures); err != nil {
		return fmt.Errorf("materialize: %v", err)
	}

	// Fixture-stage checks and fixture-boundary invariants must hold now.
	for _, ch := range contracts.MaterializeChecks(doc.Content.Checks) {
		obs := terminal.VerifyCheck(tmp, ch, nil)
		if !obs.Passed && !ch.Optional {
			return fmt.Errorf("fixture-stage check %s failed: %s", ch.ID, obs.Message)
		}
	}

	if !replay || doc.ReferenceSolution == nil {
		return nil
	}
	return replayReference(tmp, doc)
}

func replayReference(workspace string, doc *contracts.ActivityDocument) error {
	ref := doc.ReferenceSolution
	if ref == nil {
		return nil
	}
	if !sandbox.Available() {
		return fmt.Errorf("reference replay requires bubblewrap (bwrap) on PATH")
	}

	profile := ""
	initialCwd := ""
	if doc.Content.Terminal != nil {
		profile = doc.Content.Terminal.RuntimeProfile
		initialCwd = doc.Content.Terminal.InitialCwd
	}

	runOnce := func() error {
		entries, err := os.ReadDir(workspace)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := os.RemoveAll(filepath.Join(workspace, e.Name())); err != nil {
				return err
			}
		}
		if err := terminal.Materialize(workspace, doc.Content.Terminal.Fixtures); err != nil {
			return fmt.Errorf("rematerialize: %w", err)
		}
		for _, ch := range contracts.MaterializeChecks(doc.Content.Checks) {
			obs := terminal.VerifyCheck(workspace, ch, nil)
			if !obs.Passed && !ch.Optional {
				return fmt.Errorf("fixture-stage check %s failed before replay: %s", ch.ID, obs.Message)
			}
		}

		for i, step := range ref.Steps {
			workDir := "/workspace"
			if step.WorkDir != "" {
				workDir = filepath.Join("/workspace", filepath.ToSlash(step.WorkDir))
			} else if initialCwd != "" {
				workDir = filepath.Join("/workspace", filepath.ToSlash(initialCwd))
			}
			cfg := sandbox.Config{
				Workspace: workspace,
				WorkDir:   workDir,
				Network:   false,
			}
			if profile != "" {
				// Best-effort profile bind; host tools remain available for dev CI.
				_ = sandbox.ApplyProfile(&cfg, profile)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			cmd, err := sandbox.Command(ctx, cfg, step.Argv[0], step.Argv[1:]...)
			if err != nil {
				cancel()
				return fmt.Errorf("reference step %d: %w", i, err)
			}
			if step.Stdin != "" {
				cmd.Stdin = strings.NewReader(step.Stdin)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := cmd.Run()
			cancel()
			if runErr != nil {
				return fmt.Errorf("reference step %d (%v) failed: %v\nstderr: %s", i, step.Argv, runErr, stderr.String())
			}

			for _, ch := range contracts.InvariantsAt(doc.Content.Checks, contracts.InvariantAtAfterTask) {
				if !isFilesystemKind(ch.Kind) {
					continue
				}
				obs := terminal.VerifyCheck(workspace, ch, nil)
				if !obs.Passed && !ch.Optional {
					return fmt.Errorf("after_task invariant %s failed after step %d: %s", ch.ID, i, obs.Message)
				}
			}
		}

		for _, ch := range doc.Content.Checks {
			if !contracts.HasStage(ch, contracts.StageFinal) && !contracts.HasInvariantAt(ch, contracts.InvariantAtFinal) {
				continue
			}
			if !isFilesystemKind(ch.Kind) {
				continue
			}
			obs := terminal.VerifyCheck(workspace, ch, nil)
			if !obs.Passed && !ch.Optional {
				return fmt.Errorf("final check %s failed after reference replay: %s", ch.ID, obs.Message)
			}
		}
		return nil
	}

	if err := runOnce(); err != nil {
		return err
	}
	if ref.Deterministic {
		if err := runOnce(); err != nil {
			return fmt.Errorf("deterministic replay second pass: %w", err)
		}
	}
	return nil
}

func isFilesystemKind(kind string) bool {
	switch kind {
	case contracts.CheckFileExists, contracts.CheckFileNotExists, contracts.CheckContentEquals,
		contracts.CheckContentContains, contracts.CheckContentMatch, contracts.CheckPathType, contracts.CheckPathMode:
		return true
	default:
		return false
	}
}

func findActivityFile(dir string) (string, error) {
	for _, name := range []string{"activity.yaml", "activity.yml", "activity.json"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no activity.yaml in %s", dir)
}

// AllOK reports whether every result succeeded.
func AllOK(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return len(results) > 0
}
