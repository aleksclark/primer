package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

func TestCoverageAppServiceAdapterGetSchedule(t *testing.T) {
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	ns := "coverage-" + uuid.NewString()
	schedule, err := svc.CreateSchedule(context.Background(), appservice.CreateScheduleCmd{
		OwnerNamespace: ns,
		Profile:        "job",
		JobType:        "coverage",
		CronExpr:       "1h",
		Timezone:       "UTC",
		MaxCatchUp:     1,
	})
	require.NoError(t, err)
	adapter := &appServiceAdapter{svc: svc}
	got, err := adapter.GetSchedule(context.Background(), schedule.ID, ns)
	require.NoError(t, err)
	require.Equal(t, schedule.ID, got.ID)
}
