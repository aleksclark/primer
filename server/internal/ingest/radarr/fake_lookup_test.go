package radarr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/radarr"
)

func TestFakeLookupEdges(t *testing.T) {
	t.Parallel()
	f := &radarr.Fake{
		LookupResults: []radarr.Movie{
			{Title: "The Matrix", Year: 1999, TmdbID: 1},
			{Title: "Inception", Year: 2010, TmdbID: 2},
		},
	}
	// empty term returns all
	all, err := f.Lookup(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	hits, err := f.Lookup(context.Background(), "matrix")
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, 1, hits[0].TmdbID)

	// year-suffixed / fallback
	hits, err = f.Lookup(context.Background(), "zzz")
	require.NoError(t, err)
	assert.NotEmpty(t, hits) // last-resort returns all

	f.Err = errors.New("down")
	_, err = f.Lookup(context.Background(), "x")
	assert.Error(t, err)
}
