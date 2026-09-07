package reconcile_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/reconcile"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scanFailure struct {
	*jellyfin.Fake
	err error
}

func (s scanFailure) ScanRunning(context.Context) (bool, error) { return true, s.err }

func TestScanTimeoutAndStatusFailureNeverReportRefreshComplete(t *testing.T) {
	for _, errStatus := range []error{nil, errors.New("task status unavailable")} {
		name := "timeout"
		if errStatus != nil {
			name = "status-error"
		}
		t.Run(name, func(t *testing.T) {
			client := scanFailure{Fake: jellyfin.NewFake(), err: errStatus}
			e := reconcile.New(reconcile.Deps{Jellyfin: client, SyncWait: 3 * time.Millisecond, SyncPollInterval: time.Millisecond})
			r, err := e.Run(context.Background(), &manifest.Manifest{}, &manifest.Review{}, reconcile.Options{SkipAcquire: true, SkipImport: true})
			require.NoError(t, err)
			assert.False(t, r.Report.JellyfinRefreshed)
			assert.Contains(t, strings.Join(r.Report.Errors, " "), "scan")
		})
	}
}
