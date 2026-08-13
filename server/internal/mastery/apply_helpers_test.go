package mastery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestObservationHasStructuredCommandEvidence(t *testing.T) {
	t.Parallel()

	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{}))
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{Details: map[string]any{}}))

	// Untrusted sources fail closed even with forged flags.
	for _, src := range []string{"pty-shell", "synthetic-pty", "screen", "PTY-SHELL"} {
		assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
			Details: map[string]any{
				"source":                    src,
				"structuredCommandEvidence": true,
				"capability":                contracts.CapStructuredCommandEvidence,
			},
		}), "source=%s", src)
	}

	// Boolean true requires trusted source.
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"structuredCommandEvidence": true},
	}))
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"structuredCommandEvidence": true, "source": "unknown-client"},
	}))
	for _, src := range []string{"structured", "structured_command", "command_instrumentation", "observe-bash", "process-wait", "STRUCTURED"} {
		assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
			Details: map[string]any{"structuredCommandEvidence": true, "source": src},
		}), "source=%s", src)
	}

	// Capability path: trusted source, untrusted source, and legacy capability-only.
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"capability": contracts.CapStructuredCommandEvidence,
			"source":     "observe-bash",
		},
	}))
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"capability": contracts.CapStructuredCommandEvidence,
			"source":     "screen",
		},
	}))
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"capability": contracts.CapStructuredCommandEvidence},
	}), "legacy capability without source")
	// Unknown non-untrusted source with capability: switch does not match trusted or
	// untrusted lists, so legacy path accepts capability alone.
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"capability": contracts.CapStructuredCommandEvidence,
			"source":     "custom-runner",
		},
	}))

	// Source-only trusted labels.
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"source": "process-wait"},
	}))
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"source": "random"},
	}))
}

func TestPolicyFromLink(t *testing.T) {
	t.Parallel()

	// Explicit policy on the link wins.
	custom := repo.EvidencePolicyMap(contracts.EvidencePolicy{
		Version: 9,
		StatusRequirements: map[string][]string{
			"in_progress": {contracts.EvidenceProceduralContinuous},
		},
	})
	p := policyFromLink(domain.LearningActivityRevisionStandard{EvidencePolicy: custom}, contracts.KindTerminal)
	assert.Equal(t, 9, p.Version)
	require.Contains(t, p.StatusRequirements, "in_progress")

	// Defaults by activity kind when policy absent/unparseable.
	term := policyFromLink(domain.LearningActivityRevisionStandard{}, contracts.KindTerminal)
	assert.Equal(t, contracts.DefaultTerminalEvidencePolicy().Version, term.Version)
	assert.Contains(t, term.StatusRequirements["approaching"], contracts.EvidenceConceptualResponse)

	typ := policyFromLink(domain.LearningActivityRevisionStandard{}, contracts.KindTyping)
	assert.Equal(t, contracts.DefaultTypingEvidencePolicy().Version, typ.Version)
	assert.Equal(t, []string{contracts.EvidenceProceduralContinuous}, typ.StatusRequirements["mastered"])

	// Unknown kind falls back to terminal defaults.
	other := policyFromLink(domain.LearningActivityRevisionStandard{}, "quiz")
	assert.Equal(t, term.StatusRequirements["mastered"], other.StatusRequirements["mastered"])
}

func TestConfForStatusAndMaxFloat(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 2.0, maxFloat(2, 1))
	assert.Equal(t, 2.0, maxFloat(1, 2))
	assert.Equal(t, 0.0, maxFloat(0, 0))

	// weight <= 0 falls back to ConfidenceBump
	got := confForStatus("in_progress", 0, 0)
	assert.InDelta(t, 0.2, got, 1e-9)

	got = confForStatus("in_progress", 0.1, 1)
	assert.InDelta(t, clamp01(0.1+ConfidenceBump*0.25), got, 1e-9)

	got = confForStatus("approaching", 0.2, 1)
	assert.InDelta(t, 0.45, got, 1e-9)
	got = confForStatus("approaching", 0.7, 1)
	assert.InDelta(t, 0.7, got, 1e-9)

	got = confForStatus("mastered", 0.5, 1)
	assert.InDelta(t, MasteredThreshold, got, 1e-9)
	got = confForStatus("mastered", 0.95, 1)
	assert.InDelta(t, 0.95, got, 1e-9)

	assert.Equal(t, 0.33, confForStatus("not_introduced", 0.33, 1))
	assert.Equal(t, 0.1, confForStatus("", 0.1, -1))
}

func TestStandardEligibleForActivity(t *testing.T) {
	t.Parallel()
	assert.True(t, standardEligibleForActivity(contracts.KindTyping, "PRIMER.DL.6.TYPE.1"))
	assert.False(t, standardEligibleForActivity(contracts.KindTyping, "PRIMER.DL.6.NAV.1"))
	assert.True(t, standardEligibleForActivity(contracts.KindTerminal, "PRIMER.DL.6.NAV.1"))
	assert.False(t, standardEligibleForActivity(contracts.KindTerminal, "PRIMER.DL.6.TYPE.1"))
	assert.True(t, standardEligibleForActivity("other", "PRIMER.DL.6.TYPE.1"))
	assert.True(t, standardEligibleForActivity("other", "PRIMER.DL.6.NAV.1"))
}

func TestValidateObservationCapabilitiesBranches(t *testing.T) {
	t.Parallel()
	content := contracts.ActivityContent{
		Checks: []contracts.Check{
			{ID: "cmd", Kind: contracts.CheckCommandProperties},
			{ID: "file", Kind: "file_exists"},
			{ID: "opt", Kind: contracts.CheckCommandProperties, Optional: true},
		},
	}

	// Non-command check does not require structured evidence.
	require.NoError(t, validateObservationCapabilities(content, []contracts.Observation{
		{CheckID: "file", Passed: true},
	}))

	// Failed / optional command checks are ignored.
	require.NoError(t, validateObservationCapabilities(content, []contracts.Observation{
		{CheckID: "cmd", Passed: false},
		{CheckID: "opt", Passed: true, Optional: true},
	}))

	// Unknown check id is ignored.
	require.NoError(t, validateObservationCapabilities(content, []contracts.Observation{
		{CheckID: "missing", Passed: true},
	}))

	// Required command check without evidence fails.
	err := validateObservationCapabilities(content, []contracts.Observation{
		{CheckID: "cmd", Passed: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structured command evidence")

	// Trusted evidence passes.
	require.NoError(t, validateObservationCapabilities(content, []contracts.Observation{
		{CheckID: "cmd", Passed: true, Details: map[string]any{
			"structuredCommandEvidence": true,
			"source":                    "structured",
		}},
	}))
}

func TestProposedStatusWeightAndStatusRank(t *testing.T) {
	t.Parallel()
	st, conf := proposedStatus("not_introduced", 0, contracts.StandardRolePrimary, 1)
	assert.Equal(t, "in_progress", st)
	assert.InDelta(t, ConfidenceBump, conf, 1e-9)

	// weight <= 0 uses default bump
	_, conf = proposedStatus("not_introduced", 0, contracts.StandardRolePrimary, 0)
	assert.InDelta(t, ConfidenceBump, conf, 1e-9)

	assert.Equal(t, 0, statusRank(""))
	assert.Equal(t, 0, statusRank("not_introduced"))
	assert.Equal(t, 0, statusRank("weird"))
	assert.Equal(t, 1, statusRank("in_progress"))
	assert.Equal(t, 2, statusRank("approaching"))
	assert.Equal(t, 3, statusRank("mastered"))

	// addClass idempotent
	got := addClass([]string{"a"}, "a")
	assert.Equal(t, []string{"a"}, got)
	got = addClass([]string{"a"}, "b")
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestHighestAllowedAndMissingStatusEdges(t *testing.T) {
	t.Parallel()
	policy := contracts.DefaultTerminalEvidencePolicy()

	assert.Nil(t, missingForStatus("", nil, policy))
	assert.Nil(t, missingForStatus("not_introduced", nil, policy))
	assert.Nil(t, missingForStatus("unknown-status", []string{}, policy))

	// Partial advance: procedural only may reach in_progress, not approaching.
	allowed, missing, full := highestAllowedStatus("not_introduced", "approaching",
		[]string{contracts.EvidenceProceduralContinuous}, policy)
	assert.Equal(t, "in_progress", allowed)
	assert.False(t, full)
	assert.Contains(t, missing, contracts.EvidenceConceptualResponse)

	// Empty from status treated as not_introduced baseline.
	allowed, _, full = highestAllowedStatus("", "in_progress",
		[]string{contracts.EvidenceProceduralContinuous}, policy)
	assert.Equal(t, "in_progress", allowed)
	assert.True(t, full)

	// confForStatus path via partial advance in Apply* is covered by unit;
	// also lock nextUnsatisfiedStatus when already mastered with all evidence.
	assert.Equal(t, "", nextUnsatisfiedStatus("mastered", []string{
		contracts.EvidenceProceduralContinuous,
		contracts.EvidenceConceptualResponse,
		contracts.EvidenceFormalAssessment,
	}, policy))
}
