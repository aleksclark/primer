package config_test

import (
	"github.com/aleksclark/primer/server/internal/ingest/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDownloadBudgetConfiguration(t *testing.T) {
	for _, value := range []string{"25", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("INGEST_YTDLP_MAX_DOWNLOADS", value)
			cfg, err := config.Load()
			if value == "-1" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if value == "0" {
				require.Zero(t, cfg.YtDlpMaxDownloads)
			} else {
				require.Equal(t, 25, cfg.YtDlpMaxDownloads)
			}
		})
	}
}
