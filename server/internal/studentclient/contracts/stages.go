package contracts

// EffectiveStages returns the stages a check participates in.
// Omitted stages default to final-only so repair activities do not silently
// treat outcome checks as fixture baseline assertions.
func EffectiveStages(ch Check) []string {
	if len(ch.Stages) > 0 {
		return ch.Stages
	}
	return []string{StageFinal}
}

// HasStage reports whether ch is expected at stage.
func HasStage(ch Check, stage string) bool {
	for _, s := range EffectiveStages(ch) {
		if s == stage {
			return true
		}
	}
	return false
}

// IsEvidenceBearing reports whether a passing check may contribute student evidence.
// Default is true when unset.
func IsEvidenceBearing(ch Check) bool {
	if ch.EvidenceBearing == nil {
		return true
	}
	return *ch.EvidenceBearing
}

// IsInvariant reports whether the check is an invariant (has invariant boundaries).
func IsInvariant(ch Check) bool {
	return len(ch.InvariantAt) > 0
}

// HasInvariantAt reports whether an invariant is evaluated at boundary.
func HasInvariantAt(ch Check, boundary string) bool {
	for _, b := range ch.InvariantAt {
		if b == boundary {
			return true
		}
	}
	return false
}

// ChecksForStage returns checks expected at the given stage (non-invariant role).
func ChecksForStage(checks []Check, stage string) []Check {
	var out []Check
	for _, ch := range checks {
		if HasStage(ch, stage) {
			out = append(out, ch)
		}
	}
	return out
}

// InvariantsAt returns invariant checks declared for a boundary.
func InvariantsAt(checks []Check, boundary string) []Check {
	var out []Check
	for _, ch := range checks {
		if HasInvariantAt(ch, boundary) {
			out = append(out, ch)
		}
	}
	return out
}

// StageCounts tallies checks by effective stage and invariant boundaries.
func StageCounts(checks []Check) map[string]int {
	out := map[string]int{
		StageFixture:             0,
		StageTask:                0,
		StageFinal:               0,
		"invariant:" + InvariantAtFixture:   0,
		"invariant:" + InvariantAtAfterTask: 0,
		"invariant:" + InvariantAtFinal:     0,
	}
	for _, ch := range checks {
		for _, s := range EffectiveStages(ch) {
			out[s]++
		}
		for _, b := range ch.InvariantAt {
			out["invariant:"+b]++
		}
	}
	return out
}

// MaterializeChecks returns filesystem checks that must pass after fixture
// materialization: explicit fixture-stage checks plus fixture-boundary invariants.
// Command/cwd/pipeline checks are excluded; they need shell state.
func MaterializeChecks(checks []Check) []Check {
	seen := map[string]struct{}{}
	var out []Check
	add := func(ch Check) {
		if _, ok := seen[ch.ID]; ok {
			return
		}
		if !isFilesystemCheckKind(ch.Kind) {
			return
		}
		seen[ch.ID] = struct{}{}
		out = append(out, ch)
	}
	for _, ch := range ChecksForStage(checks, StageFixture) {
		add(ch)
	}
	for _, ch := range InvariantsAt(checks, InvariantAtFixture) {
		add(ch)
	}
	return out
}

func isFilesystemCheckKind(kind string) bool {
	switch kind {
	case CheckFileExists, CheckFileNotExists, CheckContentEquals, CheckContentContains,
		CheckContentMatch, CheckPathType, CheckPathMode:
		return true
	default:
		return false
	}
}

// BoolPtr is a helper for optional boolean fields in tests and fixtures.
func BoolPtr(v bool) *bool { return &v }
