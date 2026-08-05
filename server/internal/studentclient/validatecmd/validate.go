package validatecmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

// Result is the outcome for one activity document.
type Result struct {
	Slug     string
	Path     string
	Title    string
	OK       bool
	Error    string
	Checks   int
	Tasks    int
	Fixtures int
}

// Options configures a validation run.
type Options struct {
	ActivitiesDir string
	Stdout        io.Writer
	Stderr        io.Writer
	// Materialize runs fixture materialization + baseline check evaluation.
	Materialize bool
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
		res := validateOne(filepath.Join(dir, name), name, opts.Materialize)
		results = append(results, res)
		if res.OK {
			fmt.Fprintf(out, "OK  %s (%d tasks, %d checks, %d fixtures)\n", res.Slug, res.Tasks, res.Checks, res.Fixtures)
		} else {
			fmt.Fprintf(errOut, "ERR %s: %s\n", res.Slug, res.Error)
		}
	}
	return results, nil
}

func validateOne(dir, name string, materialize bool) Result {
	res := Result{Slug: name, Path: dir}
	path, err := findActivityFile(dir)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Path = path
	doc, err := contracts.LoadDocument(path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if doc.Slug != name {
		res.Error = fmt.Sprintf("slug %q must match directory %q", doc.Slug, name)
		return res
	}
	res.Title = doc.Title
	res.Tasks = len(doc.Content.Tasks)
	res.Checks = len(doc.Content.Checks)
	if doc.Content.Terminal != nil {
		res.Fixtures = len(doc.Content.Terminal.Fixtures)
	}

	if materialize && doc.Kind == contracts.KindTerminal && doc.Content.Terminal != nil {
		tmp, err := os.MkdirTemp("", "primer-activity-"+doc.Slug+"-*")
		if err != nil {
			res.Error = fmt.Sprintf("temp dir: %v", err)
			return res
		}
		defer os.RemoveAll(tmp)
		if err := terminal.Materialize(tmp, doc.Content.Terminal.Fixtures); err != nil {
			res.Error = fmt.Sprintf("materialize: %v", err)
			return res
		}
		// Evaluate filesystem checks that do not require shell state; command/cwd/pipeline
		// checks are skipped in offline validate unless they can pass without shell.
		for _, ch := range doc.Content.Checks {
			switch ch.Kind {
			case contracts.CheckCwd, contracts.CheckCommandProperties, contracts.CheckPipelineOutput:
				continue
			}
			obs := terminal.VerifyCheck(tmp, ch, nil)
			// Fixture baseline: existence/type/content checks that describe the
			// starting tree should pass; "not exists" targets that students create
			// later may fail and are ignored here unless the path is part of fixtures.
			if isBaselineCheck(ch, doc.Content.Terminal.Fixtures) && !obs.Passed {
				res.Error = fmt.Sprintf("baseline check %s failed: %s", ch.ID, obs.Message)
				return res
			}
		}
	}

	res.OK = true
	return res
}

func isBaselineCheck(ch contracts.Check, fixtures []contracts.FixtureEntry) bool {
	switch ch.Kind {
	case contracts.CheckFileExists, contracts.CheckContentEquals, contracts.CheckContentContains,
		contracts.CheckContentMatch, contracts.CheckPathType, contracts.CheckPathMode:
	default:
		return false
	}
	p, _ := ch.Params["path"].(string)
	if p == "" {
		return false
	}
	var fix *contracts.FixtureEntry
	for i := range fixtures {
		if fixtures[i].Path == p {
			fix = &fixtures[i]
			break
		}
	}
	if fix == nil {
		return false
	}
	switch ch.Kind {
	case contracts.CheckFileExists, contracts.CheckPathType, contracts.CheckPathMode:
		return true
	case contracts.CheckContentEquals, contracts.CheckContentContains, contracts.CheckContentMatch:
		return fix.Type == contracts.FixtureFile
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
