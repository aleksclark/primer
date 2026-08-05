package engine_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
)

func TestEngineNewValidationAndOfflineSync(t *testing.T) {
	t.Parallel()
	_, err := engine.New(engine.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache store")

	store, err := cache.Open(filepath.Join(t.TempDir(), "e.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, err = engine.New(engine.Options{Store: store, Offline: false})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api client")

	eng, err := engine.New(engine.Options{Store: store, Offline: true})
	require.NoError(t, err)
	assert.Equal(t, "init", eng.Status().Phase)
	assert.Equal(t, store, eng.Store())

	res := eng.SyncOnce(context.Background())
	assert.Equal(t, "offline", string(res.Status))
	require.NoError(t, eng.ResumeAndSync(context.Background()))
}
