package activities_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestKindOf(t *testing.T) {
	t.Parallel()
	assert.Equal(t, contracts.KindTerminal, activities.KindOf(contracts.KindTerminal))
	assert.Equal(t, contracts.KindTyping, activities.KindOf(contracts.KindTyping))
	assert.Equal(t, "future", activities.KindOf("future"))
}

type stubRunner struct{}

func (stubRunner) Kind() string { return contracts.KindTerminal }
func (stubRunner) Open(context.Context, string, contracts.ActivityContent) error {
	return nil
}
func (stubRunner) Close() error { return nil }

func TestRunnerInterfaceImplemented(t *testing.T) {
	t.Parallel()
	var r activities.Runner = stubRunner{}
	require.Equal(t, contracts.KindTerminal, r.Kind())
	require.NoError(t, r.Open(context.Background(), "/tmp", contracts.ActivityContent{}))
	require.NoError(t, r.Close())
}
