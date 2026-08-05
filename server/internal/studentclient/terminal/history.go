package terminal

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// MaxHistoryEvents bounds persisted per-session command history.
const MaxHistoryEvents = 200

// History is a bounded, task-aware list of command observations.
type History struct {
	Events []contracts.CommandObservation `json:"events,omitempty"`
	// TaskStartSeq[i] is the next sequence number when task i became current.
	// Index 0 is session start (usually 1).
	TaskStartSeq map[int]int64 `json:"taskStartSeq,omitempty"`
}

// Append adds an observation, dropping oldest when over capacity.
func (h *History) Append(obs contracts.CommandObservation) {
	if h.TaskStartSeq == nil {
		h.TaskStartSeq = map[int]int64{}
	}
	h.Events = append(h.Events, obs)
	if len(h.Events) > MaxHistoryEvents {
		h.Events = append([]contracts.CommandObservation(nil), h.Events[len(h.Events)-MaxHistoryEvents:]...)
	}
}

// MarkTaskStart records that taskIndex becomes active at the next sequence.
func (h *History) MarkTaskStart(taskIndex int, nextSeq int64) {
	if h.TaskStartSeq == nil {
		h.TaskStartSeq = map[int]int64{}
	}
	if _, ok := h.TaskStartSeq[taskIndex]; !ok {
		h.TaskStartSeq[taskIndex] = nextSeq
	}
}

// SinceTask returns observations recorded at or after the task's start sequence.
func (h *History) SinceTask(taskIndex int) []contracts.CommandObservation {
	if h == nil || len(h.Events) == 0 {
		return nil
	}
	minSeq := int64(0)
	if h.TaskStartSeq != nil {
		if s, ok := h.TaskStartSeq[taskIndex]; ok {
			minSeq = s
		}
	}
	out := make([]contracts.CommandObservation, 0, len(h.Events))
	for _, e := range h.Events {
		if e.Sequence >= minSeq {
			out = append(out, e)
		}
	}
	return out
}

// Last returns the most recent observation, if any.
func (h *History) Last() *contracts.CommandObservation {
	if h == nil || len(h.Events) == 0 {
		return nil
	}
	e := h.Events[len(h.Events)-1]
	return &e
}

// CommandMatch is the predicate for history-scoped command_properties checks.
type CommandMatch struct {
	Executable string
	Args       []string
	// ArgsSet when true requires Args equality (with path equivalence).
	ArgsSet bool
	// ExitCode when non-nil must match.
	ExitCode *int
	// RequireSuccess forces exit 0 when ExitCode is nil.
	RequireSuccess bool
	// StdoutEquals / StdoutContains / StdoutPattern apply when set.
	StdoutEquals   string
	StdoutContains string
	StdoutPattern  string
	// StderrEquals / StderrContains / StderrPattern apply when set.
	StderrEquals   string
	StderrContains string
	StderrPattern  string
	// RequireStructured rejects untrusted observations.
	RequireStructured bool
	// RequireStdoutTrusted when output predicates are used.
	RequireStdoutTrusted bool
	RequireStderrTrusted bool
}

// FindMatch returns the first observation since taskIndex that satisfies m.
func (h *History) FindMatch(taskIndex int, m CommandMatch) (contracts.CommandObservation, bool) {
	for _, e := range h.SinceTask(taskIndex) {
		if matchObservation(e, m) {
			return e, true
		}
	}
	return contracts.CommandObservation{}, false
}

func matchObservation(e contracts.CommandObservation, m CommandMatch) bool {
	if m.RequireStructured {
		if !e.Structured || !e.Quality.MeetsStructuredBar() {
			return false
		}
		if e.Source == contracts.SourcePTYShell || e.Source == contracts.SourceSyntheticPTY || e.Source == contracts.SourceScreen {
			return false
		}
	}
	if m.Executable != "" {
		if !e.ArgvAvailable && e.Executable == "" {
			return false
		}
		if !executableEqual(e.Executable, m.Executable) {
			return false
		}
	}
	if m.ArgsSet {
		if !e.ArgvAvailable {
			return false
		}
		if !argsEquivalent(e.Argv, m.Args) {
			return false
		}
	}
	if m.ExitCode != nil {
		if !e.ExitAvailable || e.ExitCode != *m.ExitCode {
			return false
		}
	} else if m.RequireSuccess {
		if !e.ExitAvailable || e.ExitCode != 0 {
			return false
		}
	}
	if m.StdoutEquals != "" || m.StdoutContains != "" || m.StdoutPattern != "" {
		if m.RequireStdoutTrusted && !e.Stdout.Trusted {
			return false
		}
		out := normalizeOutput(e.Stdout.Text)
		if m.StdoutEquals != "" && out != normalizeOutput(m.StdoutEquals) {
			return false
		}
		if m.StdoutContains != "" && !strings.Contains(out, normalizeOutput(m.StdoutContains)) {
			return false
		}
		if m.StdoutPattern != "" {
			re, err := regexp.Compile(m.StdoutPattern)
			if err != nil || !re.MatchString(out) {
				return false
			}
		}
	}
	if m.StderrEquals != "" || m.StderrContains != "" || m.StderrPattern != "" {
		if m.RequireStderrTrusted && !e.Stderr.Trusted {
			return false
		}
		errOut := normalizeOutput(e.Stderr.Text)
		if m.StderrEquals != "" && errOut != normalizeOutput(m.StderrEquals) {
			return false
		}
		if m.StderrContains != "" && !strings.Contains(errOut, normalizeOutput(m.StderrContains)) {
			return false
		}
		if m.StderrPattern != "" {
			re, err := regexp.Compile(m.StderrPattern)
			if err != nil || !re.MatchString(errOut) {
				return false
			}
		}
	}
	return true
}

func executableEqual(got, want string) bool {
	if got == want {
		return true
	}
	if filepath.Base(got) == want || filepath.Base(want) == got {
		return true
	}
	if filepath.Base(got) == filepath.Base(want) {
		return true
	}
	// Common busybox/coreutils path variants.
	g := strings.TrimPrefix(got, "/usr")
	w := strings.TrimPrefix(want, "/usr")
	return g == w || filepath.Base(g) == filepath.Base(w)
}

func argsEquivalent(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] == want[i] {
			continue
		}
		// Path-ish equivalence: clean and base compare.
		cg := filepath.Clean(got[i])
		cw := filepath.Clean(want[i])
		if cg == cw {
			continue
		}
		if filepath.Base(cg) == filepath.Base(cw) && (strings.Contains(got[i], "/") || strings.Contains(want[i], "/")) {
			// Only treat as equal when both look like paths with same basename
			// and cleaned forms share a suffix — keep conservative: require Clean equal
			// after optional ./ strip.
			if strings.TrimPrefix(cg, "./") == strings.TrimPrefix(cw, "./") {
				continue
			}
		}
		return false
	}
	return true
}

// ShellStateFromObservation builds a legacy ShellState from a history hit
// for callers that still thread a single shell pointer.
func ShellStateFromObservation(o contracts.CommandObservation) *ShellState {
	return &ShellState{
		Cwd:                       o.CwdAfter,
		Executable:                o.Executable,
		Args:                      append([]string(nil), o.Argv...),
		ExitCode:                  o.ExitCode,
		Stdout:                    o.Stdout.Text,
		Stderr:                    o.Stderr.Text,
		StructuredCommandEvidence: o.Structured && o.Quality.MeetsStructuredBar(),
		Source:                    o.Source,
	}
}
