package tutor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tutor"
)

func TestNewFromConfigProviderMatrixMargin(t *testing.T) {
	t.Parallel()
	svc, err := tutor.NewFromConfig(tutor.Config{Provider: tutor.ProviderFake})
	require.NoError(t, err)
	assert.NotNil(t, svc)

	svc, err = tutor.NewFromConfig(tutor.Config{Provider: tutor.ProviderEcho})
	require.NoError(t, err)
	assert.NotNil(t, svc)

	// Bedrock without URL falls back to fake
	svc, err = tutor.NewFromConfig(tutor.Config{Provider: tutor.ProviderBedrock})
	require.NoError(t, err)
	assert.NotNil(t, svc)

	_, err = tutor.NewFromConfig(tutor.Config{Provider: "nope"})
	assert.Error(t, err)

	// empty provider defaults to fake
	svc, err = tutor.NewFromConfig(tutor.Config{})
	require.NoError(t, err)
	assert.NotNil(t, svc)
}
