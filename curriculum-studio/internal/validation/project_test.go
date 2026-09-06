package validation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func TestP16S1ProjectRequiresOrderedPhasesAndOutcomeRoles(t *testing.T) {
	projectID := uuid.New()
	math := uuid.New()
	science := uuid.New()
	stretch := uuid.New()
	phases, err := domain.EncodeProjectPhases([]domain.ProjectPhase{
		{ID: "design", Name: "Design", Position: 1},
		{ID: "build", Name: "Build", Position: 2},
		{ID: "present", Name: "Present", Position: 3},
	})
	require.NoError(t, err)

	g := &domain.PlanGraph{
		Projects: []domain.Project{{ID: projectID, Title: "Chicken coop", Phases: phases}},
		Outcomes: []domain.Outcome{
			{ID: math, Title: "Scale drawings"},
			{ID: science, Title: "Load paths"},
			{ID: stretch, Title: "Cost estimate"},
		},
		ProjectOutcomes: []domain.ProjectOutcome{
			{ProjectID: projectID, OutcomeID: math, Role: domain.ProjectOutcomeRoleTarget},
			{ProjectID: projectID, OutcomeID: science, Role: domain.ProjectOutcomeRolePrior},
			{ProjectID: projectID, OutcomeID: stretch, Role: domain.ProjectOutcomeRoleStretch},
		},
		EvidenceRequirements: []domain.EvidenceRequirement{
			{OutcomeID: math, Kind: "portfolio", Description: "design journal"},
		},
		OutcomeStandardMappings: []domain.OutcomeStandardMapping{
			{OutcomeID: math, Alignment: "addresses"},
			{OutcomeID: science, Alignment: "addresses"},
			{OutcomeID: stretch, Alignment: "addresses"},
		},
	}
	result := Run(g)
	require.NotEqual(t, "failed", result.Status)
	require.False(t, hasCode(result, ProjectNoTarget))
	require.False(t, hasCode(result, ProjectPhaseInvalid))

	empty := *g
	empty.ProjectOutcomes = nil
	failed := Run(&empty)
	require.Equal(t, "failed", failed.Status)
	require.True(t, hasCode(failed, ProjectNoTarget))
}

func TestP16S4PortfolioEvidenceAndReinforcementWithoutMasteryWrite(t *testing.T) {
	projectID := uuid.New()
	target := uuid.New()
	g := &domain.PlanGraph{
		Projects: []domain.Project{{ID: projectID, Title: "Bridge span", Phases: json.RawMessage(`[{"id":"design","name":"Design","position":1}]`)}},
		Outcomes: []domain.Outcome{{ID: target, Title: "Document a span"}},
		ProjectOutcomes: []domain.ProjectOutcome{
			{ProjectID: projectID, OutcomeID: target, Role: domain.ProjectOutcomeRoleTarget},
		},
		OutcomeStandardMappings: []domain.OutcomeStandardMapping{
			{OutcomeID: target, Alignment: "reinforces"},
		},
	}
	missing := Run(g)
	require.True(t, hasCode(missing, PortfolioEvidence))
	require.True(t, hasCode(missing, ReinforcementNoted))
	for _, f := range missing.Findings {
		require.NotContains(t, strings.ToLower(f.Message), "mastery_records")
		require.NotContains(t, strings.ToLower(f.Code), "lms")
	}

	g.EvidenceRequirements = []domain.EvidenceRequirement{
		{OutcomeID: target, Kind: "portfolio", Description: "photo essay", Criteria: json.RawMessage(`{"kind":"portfolio"}`)},
		{OutcomeID: target, Kind: "performance", Description: "load test"},
	}
	present := Run(g)
	require.False(t, hasCode(present, PortfolioEvidence))
	require.True(t, hasCode(present, ReinforcementNoted))
	require.NotEqual(t, "failed", present.Status)
}

func hasCode(r Result, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}
