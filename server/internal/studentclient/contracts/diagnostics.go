package contracts

import (
	"fmt"
	"sort"
	"strings"
)

// Diagnostics holds authoring guidance for one activity document.
// Errors fail validation; warnings inform craftsmanship.
type Diagnostics struct {
	Errors   []string
	Warnings []string
	// StageCounts maps stage name -> check count (effective stages).
	StageCounts map[string]int
	// Capability requirements discovered from checks/tasks.
	Capabilities []string
	// UnusedChecks are defined but never referenced from any task tree.
	UnusedChecks []string
	// UnusedHints are defined but never referenced from any task.
	UnusedHints []string
}

// AnalyzeDocument produces authoring diagnostics. It does not re-run
// ValidateDocument; callers should validate first.
func AnalyzeDocument(doc *ActivityDocument) Diagnostics {
	var d Diagnostics
	if doc == nil {
		d.Errors = append(d.Errors, "activity document is nil")
		return d
	}
	d.StageCounts = StageCounts(doc.Content.Checks)

	checkIDs := map[string]bool{}
	usedChecks := map[string]bool{}
	for _, ch := range doc.Content.Checks {
		checkIDs[ch.ID] = true
	}
	hintIDs := map[string]bool{}
	usedHints := map[string]bool{}
	for _, h := range doc.Content.Hints {
		hintIDs[h.ID] = true
	}

	for _, t := range doc.Content.Tasks {
		collectCheckTreeIDs(t.Completion, usedChecks)
		for _, hid := range t.HintIDs {
			usedHints[hid] = true
		}
	}
	for id := range checkIDs {
		if !usedChecks[id] {
			// Invariants and fixture-only checks may intentionally be unreferenced.
			ch := findCheck(doc.Content.Checks, id)
			if ch != nil && (IsInvariant(*ch) || HasStage(*ch, StageFixture)) {
				continue
			}
			d.UnusedChecks = append(d.UnusedChecks, id)
		}
	}
	sort.Strings(d.UnusedChecks)
	for id := range hintIDs {
		if !usedHints[id] {
			d.UnusedHints = append(d.UnusedHints, id)
		}
	}
	sort.Strings(d.UnusedHints)

	if len(d.UnusedChecks) > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("unused checks: %s", strings.Join(d.UnusedChecks, ", ")))
	}
	if len(d.UnusedHints) > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("unused hints: %s", strings.Join(d.UnusedHints, ", ")))
	}

	if RevisionRequiresStructuredCommand(doc.Content) {
		d.Capabilities = append(d.Capabilities, CapStructuredCommandEvidence)
	}
	// Collect any structured-command checks even if optional.
	for _, ch := range doc.Content.Checks {
		if RequiresStructuredCommandEvidence(ch.Kind) {
			if !containsStr(d.Capabilities, CapStructuredCommandEvidence) {
				d.Capabilities = append(d.Capabilities, CapStructuredCommandEvidence)
			}
			break
		}
	}
	sort.Strings(d.Capabilities)

	if isCapstone(doc) && doc.ReferenceSolution == nil {
		d.Warnings = append(d.Warnings, "capstone activity has no referenceSolution (authoring policy recommends one)")
	}

	const warnInstructionRunes = 8000
	if len([]rune(doc.Content.Instructions)) > warnInstructionRunes {
		d.Warnings = append(d.Warnings, fmt.Sprintf("content.instructions is oversized (>%d characters)", warnInstructionRunes))
	}
	for i, t := range doc.Content.Tasks {
		if len([]rune(t.Instructions)) > 4000 {
			d.Warnings = append(d.Warnings, fmt.Sprintf("tasks[%d] (%s): instructions are oversized", i, t.ID))
		}
	}
	for i, ch := range doc.Content.Checks {
		if size := checkParamsSize(ch.Params); size > 20000 {
			d.Warnings = append(d.Warnings, fmt.Sprintf("checks[%d] (%s): params payload is oversized (%d bytes)", i, ch.ID, size))
		}
	}

	return d
}

func findCheck(checks []Check, id string) *Check {
	for i := range checks {
		if checks[i].ID == id {
			return &checks[i]
		}
	}
	return nil
}

func collectCheckTreeIDs(tree CheckTree, used map[string]bool) {
	if tree.CheckID != "" {
		used[tree.CheckID] = true
	}
	for _, c := range tree.All {
		collectCheckTreeIDs(c, used)
	}
	for _, c := range tree.Any {
		collectCheckTreeIDs(c, used)
	}
}

func isCapstone(doc *ActivityDocument) bool {
	if doc.Metadata != nil {
		if _, ok := doc.Metadata["capstone"]; ok {
			return true
		}
		if v := doc.Metadata["lesson"]; v == "19" || v == "20" {
			return true
		}
	}
	return strings.Contains(doc.Slug, "capstone")
}

func checkParamsSize(params map[string]any) int {
	if params == nil {
		return 0
	}
	n := 0
	for k, v := range params {
		n += len(k)
		switch t := v.(type) {
		case string:
			n += len(t)
		default:
			n += 8
		}
	}
	return n
}

func containsStr(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// FormatStageSummary returns a compact stage count string for CLI output.
func FormatStageSummary(counts map[string]int) string {
	return fmt.Sprintf("fixture=%d task=%d final=%d inv_fixture=%d inv_after_task=%d inv_final=%d",
		counts[StageFixture],
		counts[StageTask],
		counts[StageFinal],
		counts["invariant:"+InvariantAtFixture],
		counts["invariant:"+InvariantAtAfterTask],
		counts["invariant:"+InvariantAtFinal],
	)
}
