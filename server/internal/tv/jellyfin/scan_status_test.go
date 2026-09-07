package jellyfin_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryScanWaitIgnoresUnrelatedBackgroundScans(t *testing.T) {
	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Name":"Scan Media Library","Key":"RefreshLibrary","State":"Idle"},{"Name":"Media Segment Scan","Key":"MediaSegmentScan","State":"Running"}]`))
	}))
	running, err := c.ScanRunning(context.Background())
	require.NoError(t, err)
	assert.False(t, running, "media segment generation is not the library refresh we requested")
}
