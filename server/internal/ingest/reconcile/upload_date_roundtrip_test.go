package reconcile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/reconcile"
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYouTubeUploadDateRoundTripDoesNotCausePerpetualUpdates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Shows", "demo", "Season 01")
	base := filepath.Join(dir, "demo - S01E001 - Clip [dQw4w9wgxcQ]")
	_, err := ytdlp.WriteEpisodeNFO(base+".mp4", ytdlp.EpisodeNFO{Title: "Clip", ShowTitle: "Demo", Season: 1, Episode: 1, UploadDate: "20200101", YouTubeID: "dQw4w9wgxcQ", Slug: "demo"})
	require.NoError(t, err)
	jf := jellyfin.NewFake(jellyfin.Item{ID: "jf", Name: "Clip", Type: "Video", Path: base + ".mp4", Runtime: time.Minute * 2})
	tv := tvclient.NewFake(tvclient.MediaItem{ID: "tv", JellyfinItemID: "jf", Title: "Demo S01E001 — Clip", SortTitle: "demo S01E001", Class: "mixed", YouTubeVideoID: "dQw4w9wgxcQ", ManifestSlug: "demo", EpisodeKey: "S01E001", UploadDate: "2020-01-01T00:00:00Z"})
	e := reconcile.New(reconcile.Deps{Jellyfin: jf, TV: tv})
	m := &manifest.Manifest{Items: []manifest.Item{{ID: "demo", Title: "Demo", Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@Demo/videos", Class: manifest.ClassMixed}}}
	r, err := e.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{SkipAcquire: true, SkipSync: true})
	require.NoError(t, err)
	assert.Empty(t, r.Report.Errors)
	assert.Empty(t, tv.UpdateCalls, "PostgreSQL DATE is serialized as RFC3339; compare calendar dates, not transport spellings")
	assert.Empty(t, r.Report.Updated)
}
