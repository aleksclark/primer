package reconcile_test

import (
	"context"
	"testing"
	"time"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/reconcile"
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectionNeverAddsUnimportedMedia(t *testing.T) {
	for _, brokenTV := range []bool{false, true} {
		t.Run(map[bool]string{false: "import-skipped", true: "TV-unavailable"}[brokenTV], func(t *testing.T) {
			jf := jellyfin.NewFake(jellyfin.Item{ID: "movie", Name: "The Matrix", Type: "Movie", Runtime: time.Hour, ProviderIds: map[string]string{"Tmdb": "603"}})
			jf.Collections = map[string][]string{"primer": {}}
			jf.CollectionNames = map[string]string{"primer": "Primer"}
			jf.Items = append(jf.Items, jellyfin.Item{ID: "primer", Name: "Primer", Type: "BoxSet"})
			tv := tvclient.NewFake()
			if brokenTV {
				tv.Err = assert.AnError
			}
			e := reconcile.New(reconcile.Deps{Jellyfin: jf, TV: tv, JellyfinCollectionName: "Primer"})
			m := &manifest.Manifest{Items: []manifest.Item{{ID: "matrix", Title: "The Matrix", Kind: manifest.KindMovie, Provider: manifest.Provider{TMDB: 603}, Class: manifest.ClassEntertainment}}}
			result, err := e.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{SkipAcquire: true, SkipSync: true, SkipImport: !brokenTV})
			require.NoError(t, err)
			assert.Empty(t, jf.AddToCollectionCalls, "a Jellyfin hit alone is not successful TV import")
			assert.Empty(t, jf.Collections["primer"])
			if brokenTV {
				assert.NotEmpty(t, result.Report.Errors)
			}
		})
	}
}
