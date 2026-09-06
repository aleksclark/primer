package projects

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func TestP16E2PhaseMaterializationScopesItemsAndSnapshot(t *testing.T) {
	projectID := uuid.New()
	phases, err := domain.EncodeProjectPhases([]domain.ProjectPhase{
		{ID: "design", Name: "Design", Position: 1, Activities: []domain.ProjectActivity{{Kind: "lesson", Title: "Sketch the coop"}}},
		{ID: "build", Name: "Build", Position: 2, OffScreen: true, Activities: []domain.ProjectActivity{{Kind: domain.ActivityKindOffScreen, Title: "Cut the lumber"}}},
	})
	require.NoError(t, err)
	g := &domain.PlanGraph{
		Projects: []domain.Project{{ID: projectID, Title: "Chicken coop", Phases: phases}},
	}
	design, err := MaterializePhase(g, Window{ProjectID: projectID, PhaseID: "design"})
	require.NoError(t, err)
	require.Equal(t, "design", design.PhaseID)
	require.Equal(t, projectID.String(), design.Snapshot[SnapshotProjectIDKey])
	require.Equal(t, "design", design.Snapshot[SnapshotPhaseIDKey])
	require.NotEmpty(t, design.Items)
	for _, item := range design.Items {
		require.Equal(t, "design", item.PhaseID)
		require.Equal(t, projectID, item.ProjectID)
		require.NotEqual(t, "build", item.PhaseID)
	}

	build, err := MaterializePhase(g, Window{ProjectID: projectID, PhaseID: "build"})
	require.NoError(t, err)
	require.Equal(t, "build", build.PhaseID)
	require.True(t, hasKind(build.Items, domain.ItemKindProjectTask))
	_, err = MaterializePhase(g, Window{ProjectID: projectID, PhaseID: "missing"})
	require.Error(t, err)
}

func TestP16E3OffScreenAndToolRequirements(t *testing.T) {
	projectID := uuid.New()
	resourceID := uuid.New()
	phases, err := domain.EncodeProjectPhases([]domain.ProjectPhase{
		{ID: "build", Name: "Build", Position: 1, OffScreen: true},
	})
	require.NoError(t, err)
	g := &domain.PlanGraph{
		Projects: []domain.Project{{ID: projectID, Title: "Chicken coop", Phases: phases}},
		PlanResources: []domain.PlanResource{{
			ProjectID:     &projectID,
			ResourceID:    resourceID,
			Role:          "required",
			ResourceKind:  domain.ResourceKindTool,
			ResourceTitle: "Circular saw",
		}},
	}
	result, err := MaterializePhase(g, Window{ProjectID: projectID, PhaseID: "build"})
	require.NoError(t, err)
	require.True(t, hasKind(result.Items, domain.ItemKindProjectTask))
	foundTool := false
	for _, item := range result.Items {
		tools, _ := item.Body[BodyToolsKey].([]string)
		for _, tool := range tools {
			if tool == "tool: Circular saw" {
				foundTool = true
			}
		}
		require.Equal(t, false, item.Body[BodyMasteryWriteKey])
	}
	require.True(t, foundTool)
	require.True(t, hasKind(result.Items, domain.ItemKindTeacherGuide))
}

func TestP16S4PortfolioEvidenceNotesDoNotWriteMastery(t *testing.T) {
	projectID := uuid.New()
	outcomeID := uuid.New()
	phases, err := domain.EncodeProjectPhases([]domain.ProjectPhase{
		{ID: "present", Name: "Present", Position: 1},
	})
	require.NoError(t, err)
	g := &domain.PlanGraph{
		Projects: []domain.Project{{ID: projectID, Title: "Bridge span", Phases: phases}},
		Outcomes: []domain.Outcome{{ID: outcomeID, Title: "Document a span"}},
		ProjectOutcomes: []domain.ProjectOutcome{
			{ProjectID: projectID, OutcomeID: outcomeID, Role: domain.ProjectOutcomeRoleTarget},
		},
		EvidenceRequirements: []domain.EvidenceRequirement{
			{OutcomeID: outcomeID, Kind: "portfolio", Description: "photo essay"},
		},
		OutcomeStandardMappings: []domain.OutcomeStandardMapping{
			{OutcomeID: outcomeID, Alignment: "reinforces"},
		},
	}
	result, err := MaterializePhase(g, Window{ProjectID: projectID, PhaseID: "present"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Items)
	raw, err := json.Marshal(result.Items[0].Body)
	require.NoError(t, err)
	require.Contains(t, string(raw), "portfolio: photo essay")
	require.Contains(t, string(raw), "do not write LMS mastery")
	require.Equal(t, false, result.Items[0].Body[BodyMasteryWriteKey])
}

func hasKind(items []Item, kind string) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}
