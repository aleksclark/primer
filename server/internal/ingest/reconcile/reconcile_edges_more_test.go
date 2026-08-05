package reconcile_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/radarr"
	"github.com/aleksclark/primer/server/internal/ingest/reconcile"
	"github.com/aleksclark/primer/server/internal/ingest/sonarr"
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

func TestResolveAndAcquireDeterministicEdges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()

	m := &manifest.Manifest{Items: []manifest.Item{
		{
			ID: "movie-resolved", Title: "Resolved Movie", Year: 2001,
			Kind: manifest.KindMovie, Class: manifest.ClassEntertainment,
			Provider: manifest.Provider{TMDB: 11}, Priority: 1,
		},
		{
			ID: "movie-multi", Title: "Multi Movie", Year: 2002,
			Kind: manifest.KindMovie, Class: manifest.ClassEntertainment, Priority: 2,
		},
		{
			ID: "movie-none", Title: "Ghost Movie", Year: 2003,
			Kind: manifest.KindMovie, Class: manifest.ClassEntertainment, Priority: 3,
		},
		{
			ID: "series-one", Title: "One Hit Series", Year: 2010,
			Kind: manifest.KindSeries, Class: manifest.ClassEducational, Priority: 1,
		},
		{
			ID: "yt-no-cfg", Title: "YT", Kind: manifest.KindYouTubeChannel,
			URL: "https://youtube.com/@x", Class: manifest.ClassMixed, Priority: 4,
		},
		{
			ID: "manual-m", Title: "Manual", Kind: manifest.KindManual,
			Class: manifest.ClassEducational, Priority: 5,
		},
	}}

	// Lookup for multi returns 2 hits; none returns empty; series one hit.
	rf := radarr.NewFake()
	// Default LookupResults used for any lookup — multi needs 2, none needs 0.
	// Fake returns LookupResults for all lookups; tests that need per-term
	// behavior rely on year/title filtering. Use multi hits; FilterYear keeps both.
	rf.LookupResults = []radarr.Movie{
		{Title: "Multi Movie", Year: 2002, TmdbID: 21},
		{Title: "Multi Movie Alt", Year: 2002, TmdbID: 22},
	}
	sf := sonarr.NewFake()
	sf.LookupResults = []sonarr.Series{
		{Title: "One Hit Series", Year: 2010, TvdbID: 501, TmdbID: 601},
	}

	tv := tvclient.NewFake()
	// Pre-seed failed + present statuses (acquire skip paths).
	tv.Manifest = []tvclient.ManifestEntry{
		{ID: "1", Slug: "movie-resolved", Title: "Resolved Movie", Status: tvclient.ManifestStatusPresent},
		{ID: "2", Slug: "manual-m", Title: "Manual", Status: tvclient.ManifestStatusFailed},
	}

	eng := reconcile.New(reconcile.Deps{
		Radarr: rf, Sonarr: sf, Jellyfin: jellyfin.NewFake(), TV: tv,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/movies",
		SonarrQualityProfileID: 1, SonarrRootFolder: "/tv",
		ManifestPath: filepath.Join(dir, "m.yaml"),
		ReviewPath:   filepath.Join(dir, "r.yaml"),
		ReportDir:    filepath.Join(dir, "reports"),
		SyncWait:     time.Millisecond, SyncPollInterval: time.Millisecond,
	})

	review := &manifest.Review{Entries: []manifest.ReviewEntry{
		{ID: "missing-item", ChosenTMDB: 9},
	}}

	res, err := eng.Run(ctx, m, review, reconcile.Options{
		DryRun: true, SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	rep := res.Report
	require.NotNil(t, rep)
	assert.Contains(t, strings.Join(rep.Errors, "\n"), "missing-item")
	assert.Contains(t, strings.Join(rep.AlreadyResolved, "\n"), "movie-resolved")
	assert.Contains(t, strings.Join(rep.Resolved, "\n"), "series-one")
	assert.Equal(t, 501, m.ByID("series-one").Provider.TVDB)
	// Multi-hit and no-hit movies land in the review queue (either or both depending on lookup filter).
	queued := strings.Join(rep.ReviewQueued, "\n")
	assert.True(t,
		strings.Contains(queued, "movie-multi") || strings.Contains(queued, "movie-none"),
		queued)
	// Failed catalog status surfaces in FailedQueue (before manual branch).
	assert.Contains(t, strings.Join(rep.FailedQueue, "\n"), "manual-m")
	assert.Contains(t, strings.Join(rep.Errors, "\n"), "yt-dlp not configured")

	// Apply: already-held movie without file, series add, yt-dlp failure, synthesize movie add.
	require.NoError(t, m.SetProvider("movie-multi", manifest.Provider{TMDB: 21}))
	require.NoError(t, m.SetProvider("movie-none", manifest.Provider{TMDB: 99}))

	rf2 := radarr.NewFake()
	rf2.Library = []radarr.Movie{{ID: 7, Title: "Resolved Movie", TmdbID: 11, HasFile: false}}
	rf2.LookupResults = []radarr.Movie{{Title: "Multi Movie", Year: 2002, TmdbID: 21}}
	sf2 := sonarr.NewFake()
	sf2.LookupResults = []sonarr.Series{{Title: "One Hit Series", Year: 2010, TvdbID: 501}}
	yt := &ytdlp.FakeRunner{Err: errors.New("download failed")}

	// Fresh TV without failed skip so youtube attempts record.
	tv2 := tvclient.NewFake()
	eng2 := reconcile.New(reconcile.Deps{
		Radarr: rf2, Sonarr: sf2, Jellyfin: jellyfin.NewFake(), TV: tv2, YtDlp: yt,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/movies",
		SonarrQualityProfileID: 1, SonarrRootFolder: "/tv",
		YtDlpOutputDir: filepath.Join(dir, "media"), YtDlpArchivePath: filepath.Join(dir, "a.txt"),
		ManifestPath: filepath.Join(dir, "m2.yaml"),
		ReviewPath:   filepath.Join(dir, "r2.yaml"),
		ReportDir:    filepath.Join(dir, "reports2"),
		SyncWait:     time.Millisecond, SyncPollInterval: time.Millisecond,
	})

	res, err = eng2.Run(ctx, m, &manifest.Review{}, reconcile.Options{SkipSync: true, SkipImport: true})
	require.NoError(t, err)
	rep = res.Report
	assert.Contains(t, strings.Join(rep.AlreadyHeld, "\n"), "movie-resolved")
	assert.Contains(t, strings.Join(rep.AwaitingDownload, "\n"), "movie-resolved")
	assert.NotEmpty(t, rep.AcquiredMovies)
	assert.NotEmpty(t, rep.AcquiredSeries)
	assert.Contains(t, strings.Join(rep.Errors, "\n"), "yt-dlp")
	assert.NotEmpty(t, rep.AttemptsRecorded)
}

func TestAcquireNilClientsAndDryRunAdds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()

	// Providers already set so resolve is a no-op and acquire sees configured IDs.
	m := &manifest.Manifest{Items: []manifest.Item{
		{
			ID: "m1", Title: "M1", Year: 1999, Kind: manifest.KindMovie,
			Class: manifest.ClassEntertainment, Provider: manifest.Provider{TMDB: 1},
		},
		{
			ID: "s1", Title: "S1", Year: 2000, Kind: manifest.KindSeries,
			Class: manifest.ClassEducational, Provider: manifest.Provider{TVDB: 2},
		},
		{
			ID: "y1", Title: "Y1", Kind: manifest.KindYouTubePlaylist,
			URL: "https://youtube.com/playlist?list=1", Class: manifest.ClassMixed,
		},
	}}

	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jellyfin.NewFake(), TV: tvclient.NewFake(),
		ReportDir: filepath.Join(dir, "r"), SyncWait: time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	res, err := eng.Run(ctx, m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	errs := strings.Join(res.Report.Errors, "\n")
	assert.Contains(t, errs, "radarr not configured")
	assert.Contains(t, errs, "sonarr not configured")
	assert.Contains(t, errs, "yt-dlp not configured")

	rf := radarr.NewFake()
	sf := sonarr.NewFake()
	yt := &ytdlp.FakeRunner{}
	eng = reconcile.New(reconcile.Deps{
		Radarr: rf, Sonarr: sf, YtDlp: yt, Jellyfin: jellyfin.NewFake(), TV: tvclient.NewFake(),
		RadarrQualityProfileID: 1, RadarrRootFolder: "/m",
		SonarrQualityProfileID: 1, SonarrRootFolder: "/t",
		YtDlpOutputDir: "/media",
		ReportDir:      filepath.Join(dir, "r2"), SyncWait: time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	res, err = eng.Run(ctx, m, &manifest.Review{}, reconcile.Options{
		DryRun: true, SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Report.AcquiredMovies, "\n"), "would add")
	assert.Contains(t, strings.Join(res.Report.AcquiredSeries, "\n"), "would add")
	assert.Contains(t, strings.Join(res.Report.AcquiredYouTube, "\n"), "would download")
	assert.Empty(t, rf.AddCalls)
	assert.Empty(t, yt.Calls)

	// YtDlp present but empty output dir.
	eng = reconcile.New(reconcile.Deps{
		YtDlp: yt, ReportDir: filepath.Join(dir, "r3"),
		SyncWait: time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m2 := &manifest.Manifest{Items: []manifest.Item{{
		ID: "y2", Title: "Y2", Kind: manifest.KindYouTubeChannel,
		URL: "https://youtube.com/@z", Class: manifest.ClassMixed,
	}}}
	res, err = eng.Run(ctx, m2, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Report.Errors, "\n"), "YTDLP_OUTPUT_DIR")
}

func TestSeriesAlreadyHeldUnmonitorAndStats(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()

	sf := sonarr.NewFake()
	sf.Library = []sonarr.Series{{
		ID: 42, Title: "Held", Year: 2011, TvdbID: 900,
		Statistics: &sonarr.Statistics{EpisodeCount: 10, EpisodeFileCount: 3},
	}}
	sf.EpisodeMap[42] = []sonarr.Episode{
		{ID: 1, SeriesID: 42, SeasonNumber: 1, EpisodeNumber: 1, Monitored: true},
		{ID: 2, SeriesID: 42, SeasonNumber: 1, EpisodeNumber: 2, Monitored: true},
	}

	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "held-series", Title: "Held", Year: 2011, Kind: manifest.KindSeries,
		Class: manifest.ClassEducational, Provider: manifest.Provider{TVDB: 900},
		ExcludeEpisodes: []string{"S01E02"},
	}}}

	eng := reconcile.New(reconcile.Deps{
		Sonarr: sf, Jellyfin: jellyfin.NewFake(), TV: tvclient.NewFake(),
		SonarrQualityProfileID: 1, SonarrRootFolder: "/tv",
		ReportDir: filepath.Join(dir, "rep"), SyncWait: time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	res, err := eng.Run(ctx, m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Report.AlreadyHeld, "\n"), "held-series")
	assert.Contains(t, strings.Join(res.Report.AwaitingDownload, "\n"), "held-series")
	assert.Contains(t, sf.Unmonitor, 2)
}

func TestTVListManifestErrorsSurface(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()
	tv := tvclient.NewFake()
	tv.Err = errors.New("tv down")

	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "only-manual", Title: "M", Kind: manifest.KindManual, Class: manifest.ClassMixed,
	}}}
	eng := reconcile.New(reconcile.Deps{
		TV: tv, Jellyfin: jellyfin.NewFake(),
		ReportDir: filepath.Join(dir, "r"), SyncWait: time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	res, err := eng.Run(ctx, m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	// Catalog sync and/or list statuses record the TV error.
	assert.NotEmpty(t, res.Report.Errors)
	joined := strings.Join(res.Report.Errors, "\n")
	assert.True(t,
		strings.Contains(joined, "tv down") ||
			strings.Contains(joined, "manifest") ||
			strings.Contains(joined, "list manifest"),
		joined)
}
