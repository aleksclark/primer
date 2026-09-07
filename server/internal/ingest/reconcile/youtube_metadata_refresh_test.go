package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/reconcile"
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

func TestYouTubeImportRefreshesLocalMetadataBeforeCreatingTVRow(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Shows", "demo", "Season 01", "demo - S01E001 - Friendly Title [dQw4w9wgxcQ]")
	require.NoError(t, os.MkdirAll(filepath.Dir(base), 0o755))
	require.NoError(t, os.WriteFile(base+".mp4", []byte("video"), 0o644))
	_, err := ytdlp.WriteEpisodeNFO(base+".mp4", ytdlp.EpisodeNFO{
		Title:      "Friendly Title",
		ShowTitle:  "Demo",
		Season:     1,
		Episode:    1,
		Plot:       "Authoritative local plot.",
		UploadDate: "20200101",
		RuntimeSec: 90,
		YouTubeID:  "dQw4w9wgxcQ",
		Slug:       "demo",
	})
	require.NoError(t, err)

	jf := jellyfin.NewFake(jellyfin.Item{
		ID:       "jf-1",
		Name:     "demo - S01E001 - Friendly Title",
		SortName: "demo - S01E001 - Friendly Title",
		Type:     "Video",
		Path:     base + ".mp4",
		Runtime:  90 * time.Second,
	})
	jf.RefreshedItems = map[string]jellyfin.Item{
		"jf-1": {
			ID:          "jf-1",
			Name:        "Friendly Title",
			SortName:    "001 - 0001 - Friendly Title",
			Overview:    "Authoritative local plot.",
			Type:        "Video",
			Path:        base + ".mp4",
			Runtime:     90 * time.Second,
			ProviderIds: map[string]string{"youtube": "dQw4w9wgxcQ", "primer-slug": "demo"},
		},
	}
	v := tvclient.NewFake()
	e := reconcile.New(reconcile.Deps{Jellyfin: jf, TV: v, SyncWait: 5 * time.Second, SyncPollInterval: 10 * time.Millisecond})
	m := &manifest.Manifest{Items: []manifest.Item{{ID: "demo", Title: "Demo", Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@demo/videos", Class: manifest.ClassMixed}}}

	res, err := e.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{SkipAcquire: true})
	require.NoError(t, err)
	assert.Empty(t, res.Report.Errors)
	require.Len(t, jf.RefreshLocalMetadataCalls, 1)
	assert.Equal(t, "jf-1", jf.RefreshLocalMetadataCalls[0])
	require.Len(t, v.CreateCalls, 1)
	assert.Equal(t, "Demo S01E001 — Friendly Title", v.CreateCalls[0].Title)
	assert.Equal(t, "Authoritative local plot.", v.CreateCalls[0].Overview)
	assert.Equal(t, "dQw4w9wgxcQ", v.CreateCalls[0].YouTubeVideoID)
	assert.Equal(t, "demo", v.CreateCalls[0].ManifestSlug)
	assert.Equal(t, "S01E001", v.CreateCalls[0].EpisodeKey)
	assert.Equal(t, "2020-01-01", v.CreateCalls[0].UploadDate)
}
