package api

import (
	"testing"

	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/stretchr/testify/assert"
)

func TestMetadataSyncPreservesYouTubeEpisodeSortKey(t *testing.T) {
	item := domain.MediaItem{Title: "Demo S01E001 — Clip", SortTitle: "demo S01E001", ManifestSlug: "demo", EpisodeKey: "S01E001"}
	remote := &jellyfin.Item{Name: "Clip", SortName: "clip"}
	changes := metadataDiff(item, remote)
	assert.NotContains(t, changes, "sort_title", "ingest owns stable ledger ordering; remote sort names caused perpetual reconcile churn")
}
