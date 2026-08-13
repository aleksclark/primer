package api

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tutor"
)

func TestNormalizeOptions(t *testing.T) {
	t.Parallel()
	// nil tutor -> default wiring
	o := normalizeOptions(Options{})
	require.NotNil(t, o.Tutor)
	assert.NotEmpty(t, o.TutorProviderName)

	// explicit fake keeps provider defaults
	fake := tutor.NewFake()
	o2 := normalizeOptions(Options{Tutor: fake})
	assert.Equal(t, fake, o2.Tutor)
	assert.Equal(t, "fake", o2.TutorProviderName)
	assert.True(t, o2.TutorEnabled)

	// provider name already set
	o3 := normalizeOptions(Options{Tutor: fake, TutorProviderName: "echo", TutorEnabled: false})
	assert.Equal(t, "echo", o3.TutorProviderName)
}

func TestMapArtifactErrAndPlanToMap(t *testing.T) {
	t.Parallel()
	assert.Nil(t, mapArtifactErr(nil))
	err := mapArtifactErr(repo.ErrConflict)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict") // huma wraps message

	err2 := mapArtifactErr(errors.New("boom"))
	require.Error(t, err2)

	assert.Equal(t, map[string]any{}, planToMap(nil))
	m := planToMap(&curriculum.ImportPlan{})
	assert.NotNil(t, m)
}
