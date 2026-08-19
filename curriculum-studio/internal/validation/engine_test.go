package validation

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func TestRunIsDeterministicAndFindsCoverageAndEvidenceGaps(t *testing.T) {
	outcome := uuid.New()
	g := &domain.PlanGraph{Outcomes: []domain.Outcome{{ID: outcome, Title: "Explain fractions"}}}
	first := Run(g)
	second := Run(g)
	require.Equal(t, first, second)
	require.Equal(t, "failed", first.Status)
	require.Equal(t, []string{OutcomeUnmapped, OutcomeNoEvidence}, []string{first.Findings[0].Code, first.Findings[1].Code})
}

func TestRunFindsWorkloadAndCycles(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	minutes := 90
	payload, _ := json.Marshal(map[string]any{"minutes": 30})
	g := &domain.PlanGraph{Outcomes: []domain.Outcome{{ID: a, Title: "A"}, {ID: b, Title: "B"}}, OutcomePrerequisites: []domain.OutcomePrerequisite{{OutcomeID: a, PrerequisiteID: b}, {OutcomeID: b, PrerequisiteID: a}}, Units: []domain.Unit{{ID: uuid.New(), EstimatedMinutes: &minutes}}, SchedulingConstraints: []domain.SchedulingConstraint{{Kind: "workload_cap", Payload: payload}}}
	result := Run(g)
	codes := map[string]bool{}
	for _, finding := range result.Findings {
		codes[finding.Code] = true
	}
	require.True(t, codes[PrerequisiteCycle])
	require.True(t, codes[WorkloadOverloaded])
}
