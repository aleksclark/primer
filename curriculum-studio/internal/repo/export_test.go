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

func TestP10E1ExportObjectRefAndChecksum(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowFixture(t, tx)
	r := repo.NewExportRepo(tx)
	exp, err := r.Create(ctx, &domain.Export{WorkspaceID: ws.ID, RunID: &run.ID, PlanRevisionID: &run.PlanRevisionID, Format: domain.ExportFormatPDF, RequestedBySubjectRef: "identity:" + uuid.NewString()})
	require.NoError(t, err)
	require.Equal(t, domain.ExportStatusRequested, exp.Status)
	ready, err := r.Complete(ctx, ws.ID, exp.ID, "obj:exports/math.pdf", "0123456789abcdef")
	require.NoError(t, err)
	require.Equal(t, domain.ExportStatusReady, ready.Status)
	require.Equal(t, "obj:exports/math.pdf", ready.ArtifactRef)
	require.Equal(t, "0123456789abcdef", ready.Checksum)
}

func TestP10E2FailedExportAndNoBytes(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _ := workflowFixture(t, tx)
	r := repo.NewExportRepo(tx)
	exp, err := r.Create(ctx, &domain.Export{WorkspaceID: ws.ID, Format: domain.ExportFormatMarkdown})
	require.NoError(t, err)
	failed, err := r.Fail(ctx, ws.ID, exp.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ExportStatusFailed, failed.Status)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewExportRepo(sp).Create(ctx, &domain.Export{WorkspaceID: ws.ID, Format: "exe"})
	require.ErrorIs(t, err, repo.ErrCheckViolation)
	var n int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='curriculum_studio' AND table_name='exports' AND data_type='bytea'`).Scan(&n))
	require.Zero(t, n)
}
