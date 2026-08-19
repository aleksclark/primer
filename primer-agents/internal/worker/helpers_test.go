package worker_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/agents/internal/worker"
)

// TestWorkerHelpers covers small utility functions in the worker package
// that are not exercised by the full integration path.

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := worker.DefaultConfig()
	assert.True(t, cfg.LeaseDuration > 0)
	assert.True(t, cfg.HeartbeatInterval > 0)
	assert.True(t, cfg.CancelPollInterval > 0)
	assert.True(t, cfg.PollInterval > 0)
	assert.Equal(t, 1, cfg.MaxConcurrent)
	assert.Nil(t, cfg.ProviderCfg, "default config must not have a provider (no billable defaults)")
}
