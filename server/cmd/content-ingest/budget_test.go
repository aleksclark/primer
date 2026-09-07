package main

import (
	"github.com/aleksclark/primer/server/internal/ingest/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuildDepsWiresDownloadBudget(t *testing.T) {
	deps, err := buildDeps(&config.Config{YtDlpMaxDownloads: 7})
	require.NoError(t, err)
	require.Equal(t, 7, deps.YtDlpMaxDownloads)
}
