package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestP6E1ValidationReportWithFindings(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	reports := repo.NewValidationReportRepo(tx)
	created, err := reports.CreateReportWithFindings(ctx, ws.ID, &domain.ValidationReport{PlanRevisionID: rev.ID, Status: "failed"}, []domain.ValidationFinding{
		{Severity: "warning", Code: "coverage", Message: "coverage is thin"},
		{Severity: "error", Code: "cycle", Message: "cycle found"},
		{Severity: "info", Code: "note", Message: "informational"},
	})
	require.NoError(t, err)
	got, err := reports.GetReport(ctx, ws.ID, created.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", got.Status)
	findings, err := reports.ListFindings(ctx, ws.ID, created.ID)
	require.NoError(t, err)
	require.Len(t, findings, 3)
	require.Equal(t, "error", findings[0].Severity)
	require.Equal(t, "warning", findings[1].Severity)
	require.Equal(t, "info", findings[2].Severity)
}

func TestP6E2ValidationHistoryAndWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	reports := repo.NewValidationReportRepo(tx)
	first, err := reports.CreateReportWithFindings(ctx, ws.ID, &domain.ValidationReport{PlanRevisionID: rev.ID, Status: "warning"}, nil)
	require.NoError(t, err)
	second, err := reports.CreateReportWithFindings(ctx, ws.ID, &domain.ValidationReport{PlanRevisionID: rev.ID, Status: "passed"}, nil)
	require.NoError(t, err)
	list, err := reports.ListReports(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	latest, err := reports.Latest(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	require.Contains(t, []uuid.UUID{first.ID, second.ID}, latest.ID)
	foreign := uuid.New()
	_, err = reports.GetReport(ctx, foreign, first.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	_, err = reports.ListFindings(ctx, foreign, first.ID)
	require.NoError(t, err)
}
