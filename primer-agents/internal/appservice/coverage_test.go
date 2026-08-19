package appservice_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

func TestCoverageCreateJobAndScheduleOperations(t *testing.T) {
	ctx := context.Background()
	svc := appservice.New(testutil.DB(t))
	ns := "coverage-job-" + uuid.NewString()
	job, err := svc.CreateJob(ctx, appservice.CreateJobCmd{OwnerNamespace: ns, IdempotencyKey: "job-1", JobType: "sync"})
	require.NoError(t, err)
	require.Equal(t, "job", job.Profile)
	jobAgain, err := svc.CreateJob(ctx, appservice.CreateJobCmd{OwnerNamespace: ns, IdempotencyKey: "job-1", JobType: "sync"})
	require.NoError(t, err)
	require.Equal(t, job.ID, jobAgain.ID)
	preview := "bounded job input"
	withPreview, err := svc.CreateJob(ctx, appservice.CreateJobCmd{OwnerNamespace: ns, IdempotencyKey: "job-2", InputPreview: &preview})
	require.NoError(t, err)
	require.Equal(t, "job", withPreview.Profile)

	schedule, err := svc.CreateSchedule(ctx, appservice.CreateScheduleCmd{OwnerNamespace: ns, Profile: "job", JobType: "sync", CronExpr: "1h", Timezone: "UTC", MaxCatchUp: 1})
	require.NoError(t, err)
	got, err := svc.GetSchedule(ctx, schedule.ID, ns)
	require.NoError(t, err)
	require.Equal(t, schedule.ID, got.ID)
	changed, err := svc.SetScheduleEnabled(ctx, schedule.ID, ns, false)
	require.NoError(t, err)
	require.False(t, changed.Enabled)
	_, err = svc.GetSchedule(ctx, schedule.ID, "other")
	require.ErrorIs(t, err, repo.ErrNotFound)
}
