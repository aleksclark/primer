package reconcile_test

import (
	"context"
	"os"
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

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{Items: []manifest.Item{
		{
			ID: "living-planet", Title: "The Living Planet", Year: 1984,
			Kind: manifest.KindSeries, Provider: manifest.Provider{TVDB: 79165},
			Class: manifest.ClassEducational, SubjectTags: []string{"science"},
			StandardCodes: []string{"TN.SCI.6.LS2.1"}, Priority: 1,
			ExcludeEpisodes: []string{"S01E07"},
		},
		{
			ID: "matrix", Title: "The Matrix", Year: 1999,
			Kind: manifest.KindMovie, Class: manifest.ClassEntertainment, Priority: 2,
		},
		{
			ID: "paul-sellers", Title: "Paul Sellers",
			Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
			Class: manifest.ClassMixed, SubjectTags: []string{"practical"},
		},
		{
			ID: "bernstein-ypc", Title: "Leonard Bernstein Young People's Concerts",
			Kind: manifest.KindManual, Class: manifest.ClassEducational,
			SubjectTags: []string{"music"},
		},
		{
			ID: "ambiguous", Title: "Legacy", Year: 1991,
			Kind: manifest.KindSeries, Class: manifest.ClassEducational,
		},
	}}
}

func TestPlanResolveAndAcquire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	rf := radarr.NewFake()
	rf.LookupResults = []radarr.Movie{
		{Title: "The Matrix", Year: 1999, TmdbID: 603, TitleSlug: "the-matrix"},
	}

	sf := sonarr.NewFake()
	sf.LookupResults = []sonarr.Series{
		{Title: "Legacy", Year: 1991, TvdbID: 111},
		{Title: "Legacy", Year: 1991, TvdbID: 222},
	}
	sf.Library = []sonarr.Series{
		{ID: 10, Title: "The Living Planet", Year: 1984, TvdbID: 79165},
	}
	sf.EpisodeMap[10] = []sonarr.Episode{
		{ID: 100, SeriesID: 10, SeasonNumber: 1, EpisodeNumber: 1, Monitored: true},
		{ID: 107, SeriesID: 10, SeasonNumber: 1, EpisodeNumber: 7, Monitored: true},
	}

	yt := &ytdlp.FakeRunner{}
	jf := jellyfin.NewFake(
		jellyfin.Item{
			ID: "series-lp", Name: "The Living Planet", Type: "Series",
			ProviderIds: map[string]string{"Tvdb": "79165"},
		},
		jellyfin.Item{
			ID: "jf-lp-e1", Name: "The Building of the Earth", Type: "Episode",
			SeriesName: "The Living Planet", SeriesID: "series-lp",
			ParentIndexNumber: 1, IndexNumber: 1,
			ProviderIds: map[string]string{"Tvdb": "79165"},
			Runtime:     55 * time.Minute,
		},
		jellyfin.Item{
			ID: "jf-lp-e7", Name: "Our Blue Planet", Type: "Episode",
			SeriesName: "The Living Planet", SeriesID: "series-lp",
			ParentIndexNumber: 1, IndexNumber: 7,
			ProviderIds: map[string]string{"Tvdb": "79165"},
			Runtime:     55 * time.Minute,
		},
		jellyfin.Item{
			ID: "jf-matrix", Name: "The Matrix", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "603"},
			Runtime:     136 * time.Minute,
		},
		jellyfin.Item{
			ID: "jf-ps-1", Name: "Dovetails", Type: "Video",
			Path:    "/media/Shows/paul-sellers/Season 01/paul-sellers - S01E001 - Dovetails.mkv",
			Runtime: 20 * time.Minute,
		},
	)
	tv := tvclient.NewFake()

	eng := reconcile.New(reconcile.Deps{
		Radarr: rf, Sonarr: sf, Jellyfin: jf, TV: tv, YtDlp: yt,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/movies",
		SonarrQualityProfileID: 1, SonarrRootFolder: "/tv",
		YtDlpOutputDir: "/media", YtDlpArchivePath: filepath.Join(dir, "archive.txt"),
		ManifestPath: filepath.Join(dir, "manifest.yaml"),
		ReviewPath:   filepath.Join(dir, "review.yaml"),
		ReportDir:    filepath.Join(dir, "reports"),
		SyncWait:     10 * time.Millisecond, SyncPollInterval: time.Millisecond,
	})

	m := testManifest()
	review := &manifest.Review{}

	// Plan first — resolve writes review, no acquires.
	res, err := eng.Run(context.Background(), m, review, reconcile.Options{DryRun: true})
	require.NoError(t, err)
	require.NotNil(t, res.Report)

	assert.Contains(t, strings.Join(res.Report.Resolved, "\n"), "matrix")
	assert.Equal(t, 603, m.ByID("matrix").Provider.TMDB)
	assert.Contains(t, strings.Join(res.Report.ReviewQueued, "\n"), "ambiguous")
	assert.Contains(t, strings.Join(res.Report.ManualQueue, "\n"), "bernstein-ypc")
	assert.Contains(t, strings.Join(res.Report.AlreadyHeld, "\n"), "living-planet")
	assert.Contains(t, strings.Join(res.Report.AcquiredMovies, "\n"), "would add")
	assert.Contains(t, strings.Join(res.Report.AcquiredYouTube, "\n"), "would download")
	assert.Empty(t, rf.AddCalls, "plan must not add to radarr")
	assert.Empty(t, yt.Calls, "plan must not run yt-dlp")
	assert.NotEmpty(t, res.ReportPath)
	body, err := os.ReadFile(res.ReportPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "content-ingest plan")

	// review.yaml written even in plan mode.
	_, err = os.Stat(filepath.Join(dir, "review.yaml"))
	require.NoError(t, err)

	// Apply human choice for ambiguous.
	review.ByID("ambiguous").ChosenTVDB = 111

	// Apply for real.
	res, err = eng.Run(context.Background(), m, review, reconcile.Options{})
	require.NoError(t, err)

	assert.Contains(t, strings.Join(res.Report.ReviewApplied, "\n"), "ambiguous")
	assert.Equal(t, 111, m.ByID("ambiguous").Provider.TVDB)
	require.Len(t, rf.AddCalls, 1)
	assert.Equal(t, 603, rf.AddCalls[0].TmdbID)
	require.Len(t, yt.Calls, 1)
	assert.Equal(t, "paul-sellers", yt.Calls[0].Slug)
	assert.Contains(t, sf.Unmonitor, 107, "excluded episode unmonitored")

	// Import created media items (matrix + living planet e1 + paul sellers; e7 excluded).
	assert.NotEmpty(t, res.Report.Imported)
	items, err := tv.ListMediaItems(context.Background())
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, it := range items {
		ids[it.JellyfinItemID] = true
	}
	assert.True(t, ids["jf-matrix"])
	assert.True(t, ids["jf-lp-e1"])
	assert.False(t, ids["jf-lp-e7"], "excluded episode must not be imported")
	assert.True(t, ids["jf-ps-1"])

	// TV catalog mirrored + present marked for Jellyfin hits.
	assert.GreaterOrEqual(t, tv.ManifestSyncCalls, 1)
	assert.NotEmpty(t, res.Report.MarkedPresent)
	present := map[string]bool{}
	for _, slug := range res.Report.MarkedPresent {
		present[slug] = true
	}
	assert.True(t, present["matrix"])
	assert.True(t, present["living-planet"])
	assert.True(t, present["paul-sellers"])
	entries, err := tv.ListManifestEntries(context.Background())
	require.NoError(t, err)
	bySlug := map[string]tvclient.ManifestEntry{}
	for _, e := range entries {
		bySlug[e.Slug] = e
	}
	assert.Equal(t, tvclient.ManifestStatusPresent, bySlug["matrix"].Status)
	assert.Equal(t, tvclient.ManifestStatusManual, bySlug["bernstein-ypc"].Status)
	assert.Greater(t, bySlug["matrix"].AttemptCount+bySlug["paul-sellers"].AttemptCount, 0)

	// Second apply is a no-op for already-held / already-imported.
	res2, err := eng.Run(context.Background(), m, review, reconcile.Options{SkipSync: true})
	require.NoError(t, err)
	assert.Empty(t, res2.Report.AcquiredMovies)
	assert.Empty(t, res2.Report.Imported)
}

func TestImportUpdatesClassification(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	jf := jellyfin.NewFake(jellyfin.Item{
		ID: "jf-1", Name: "The Matrix", Type: "Movie",
		ProviderIds: map[string]string{"Tmdb": "603"},
	})
	tv := tvclient.NewFake(tvclient.MediaItem{
		ID: "mi-1", JellyfinItemID: "jf-1", Title: "The Matrix",
		Class: "entertainment", SubjectTags: []string{"old"},
	})

	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jf, TV: tv,
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "matrix", Title: "The Matrix", Kind: manifest.KindMovie,
		Provider: manifest.Provider{TMDB: 603},
		Class:    manifest.ClassMixed, SubjectTags: []string{"new"},
	}}}

	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipAcquire: true, SkipSync: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Report.Updated)
	items, _ := tv.ListMediaItems(context.Background())
	assert.Equal(t, "mixed", items[0].Class)
	assert.Equal(t, []string{"new"}, items[0].SubjectTags)
}

func TestReportGoldenShape(t *testing.T) {
	t.Parallel()
	r := reconcile.NewReport(true)
	r.Resolved = []string{"matrix → tmdb=603"}
	r.ManualQueue = []string{"bernstein-ypc — Leonard Bernstein"}
	r.ReviewQueued = []string{"ambiguous (2 hits)"}
	r.FailedQueue = []string{"scarce — needs DVD"}
	r.ManifestSynced = "would upsert 5 entries"
	r.Errors = []string{"boom"}
	md := r.Markdown()
	assert.Contains(t, md, "# content-ingest plan")
	assert.Contains(t, md, "## Manual rip queue")
	assert.Contains(t, md, "bernstein-ypc")
	assert.Contains(t, md, "## Failed (human intervention)")
	assert.Contains(t, md, "TV content-manifest catalog: would upsert 5 entries")
	assert.Contains(t, md, "| Resolved | 1 |")
	assert.Contains(t, md, "## Errors")
}

func TestAcquireSkipsFailedManifestEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rf := radarr.NewFake()
	rf.LookupResults = []radarr.Movie{{Title: "Scarce", Year: 1990, TmdbID: 999}}
	tv := tvclient.NewFake()
	tv.Manifest = []tvclient.ManifestEntry{{
		ID: "me-1", Slug: "scarce", Title: "Scarce", Kind: "movie",
		Status: tvclient.ManifestStatusFailed, AttemptCount: 10,
	}}

	eng := reconcile.New(reconcile.Deps{
		Radarr: rf, TV: tv,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/movies",
		ReportDir: filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "scarce", Title: "Scarce", Year: 1990,
		Kind: manifest.KindMovie, Provider: manifest.Provider{TMDB: 999},
		Class: manifest.ClassEntertainment,
	}}}

	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Empty(t, rf.AddCalls, "failed entries must not be re-acquired")
	assert.Contains(t, strings.Join(res.Report.FailedQueue, "\n"), "scarce")
}

func TestAcquireNewSeriesAndSync(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	sf := sonarr.NewFake()
	sf.LookupResults = []sonarr.Series{{
		Title: "The Living Planet", Year: 1984, TvdbID: 79165, TitleSlug: "lp",
	}}
	jf := jellyfin.NewFake()
	tv := tvclient.NewFake()

	eng := reconcile.New(reconcile.Deps{
		Sonarr: sf, Jellyfin: jf, TV: tv,
		SonarrQualityProfileID: 1, SonarrRootFolder: "/tv",
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  5 * time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "living-planet", Title: "The Living Planet", Year: 1984,
		Kind: manifest.KindSeries, Provider: manifest.Provider{TVDB: 79165},
		Class: manifest.ClassEducational, ExcludeEpisodes: []string{"S01E07"},
	}}}

	// Seed episodes so unmonitor runs after add.
	// Add happens inside Run; hook episodes via post-add by pre-seeding empty map
	// and relying on unmonitor no-op when empty — then set and re-run.
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{})
	require.NoError(t, err)
	require.Len(t, sf.AddCalls, 1)
	assert.True(t, res.Report.JellyfinRefreshed)
	assert.True(t, res.Report.TVSynced)
	assert.Equal(t, 1, jf.RefreshCalls)
	assert.Equal(t, 1, tv.SyncCalls)

	// Second pass: series held, exclude unmonitor.
	require.Len(t, sf.Library, 1)
	sf.EpisodeMap[sf.Library[0].ID] = []sonarr.Episode{
		{ID: 7, SeriesID: sf.Library[0].ID, SeasonNumber: 1, EpisodeNumber: 7, Monitored: true},
		{ID: 1, SeriesID: sf.Library[0].ID, SeasonNumber: 1, EpisodeNumber: 1, Monitored: true},
	}
	sf.Library[0].Statistics = &sonarr.Statistics{EpisodeCount: 12, EpisodeFileCount: 2}
	res, err = eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{SkipSync: true})
	require.NoError(t, err)
	assert.Contains(t, sf.Unmonitor, 7)
	assert.Contains(t, strings.Join(res.Report.AwaitingDownload, "\n"), "living-planet")
}

func TestResolveNoHitsAndMovieAlreadyHeld(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rf := radarr.NewFake()
	rf.LookupResults = nil // no hits
	rf.Library = []radarr.Movie{{ID: 1, Title: "The Matrix", TmdbID: 603, HasFile: false}}

	eng := reconcile.New(reconcile.Deps{
		Radarr:                 rf,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/m",
		ManifestPath: filepath.Join(dir, "m.yaml"),
		ReviewPath:   filepath.Join(dir, "r.yaml"),
		ReportDir:    filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{
		{ID: "ghost", Title: "Ghost Show", Year: 1901, Kind: manifest.KindMovie, Class: manifest.ClassEntertainment},
		{ID: "matrix", Title: "The Matrix", Year: 1999, Kind: manifest.KindMovie,
			Provider: manifest.Provider{TMDB: 603}, Class: manifest.ClassEntertainment},
	}}
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{SkipSync: true, SkipImport: true})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Report.ReviewQueued, "\n"), "ghost")
	assert.Contains(t, strings.Join(res.Report.AlreadyHeld, "\n"), "matrix")
	assert.Contains(t, strings.Join(res.Report.AwaitingDownload, "\n"), "matrix")
	_, err = os.Stat(filepath.Join(dir, "r.yaml"))
	require.NoError(t, err)
}

func TestSkipImportWhenUnconfigured(t *testing.T) {
	t.Parallel()
	eng := reconcile.New(reconcile.Deps{ReportDir: t.TempDir()})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "x", Title: "X", Kind: manifest.KindManual, Class: manifest.ClassMixed,
	}}}
	res, err := eng.Run(context.Background(), m, nil, reconcile.Options{DryRun: true})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Report.ManualQueue, "\n"), "x")
}

func TestImportDoesNotCrossWireForeignLibrary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Library contains the target series plus unrelated movies/episodes that a
	// broken provider filter would previously have imported under every title.
	jf := jellyfin.NewFake(
		jellyfin.Item{
			ID: "series-lp", Name: "The Living Planet", Type: "Series",
			ProviderIds: map[string]string{"Tvdb": "79165"},
		},
		jellyfin.Item{
			ID: "jf-lp-e1", Name: "The Building of the Earth", Type: "Episode",
			SeriesName: "The Living Planet", SeriesID: "series-lp",
			ParentIndexNumber: 1, IndexNumber: 1,
			ProviderIds: map[string]string{"Tvdb": "79165"},
		},
		jellyfin.Item{
			ID: "jf-3ninjas", Name: "3 Ninjas", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "11234"},
		},
		jellyfin.Item{
			ID: "jf-50first", Name: "50 First Dates", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "1824"},
		},
		jellyfin.Item{
			ID: "jf-b5", Name: "Midnight on the Firing Line", Type: "Episode",
			SeriesName: "Babylon 5", SeriesID: "series-b5",
			ParentIndexNumber: 1, IndexNumber: 1,
			ProviderIds: map[string]string{"Tvdb": "70726"},
		},
		jellyfin.Item{
			ID: "series-b5", Name: "Babylon 5", Type: "Series",
			ProviderIds: map[string]string{"Tvdb": "70726"},
		},
		jellyfin.Item{
			ID: "jf-matrix", Name: "The Matrix", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "603"},
		},
	)
	tv := tvclient.NewFake()
	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jf, TV: tv,
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{
		{
			ID: "living-planet", Title: "The Living Planet", Year: 1984,
			Kind: manifest.KindSeries, Provider: manifest.Provider{TVDB: 79165},
			Class: manifest.ClassEducational,
		},
		{
			ID: "matrix", Title: "The Matrix", Year: 1999,
			Kind: manifest.KindMovie, Provider: manifest.Provider{TMDB: 603},
			Class: manifest.ClassEntertainment,
		},
		{
			// Resolved series that is not in Jellyfin at all.
			ID: "missing-show", Title: "Not In Library", Year: 2000,
			Kind: manifest.KindSeries, Provider: manifest.Provider{TVDB: 999999},
			Class: manifest.ClassEducational,
		},
	}}

	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipAcquire: true, SkipSync: true, DryRun: true,
	})
	require.NoError(t, err)

	imported := strings.Join(res.Report.Imported, "\n")
	assert.Contains(t, imported, "living-planet")
	assert.Contains(t, imported, "The Building of the Earth")
	assert.Contains(t, imported, "matrix")
	assert.NotContains(t, imported, "3 Ninjas")
	assert.NotContains(t, imported, "50 First Dates")
	assert.NotContains(t, imported, "Babylon")
	assert.NotContains(t, imported, "jf-3ninjas")
	assert.NotContains(t, imported, "jf-50first")
	assert.NotContains(t, imported, "jf-b5")
	assert.Contains(t, strings.Join(res.Report.NotInJellyfin, "\n"), "missing-show")

	// Only the two genuine hits.
	assert.Equal(t, 2, len(res.Report.Imported), "got: %v", res.Report.Imported)
}

func TestImportSeriesAmbiguousIsError(t *testing.T) {
	t.Parallel()
	jf := jellyfin.NewFake(
		jellyfin.Item{ID: "s1", Name: "Legacy A", Type: "Series", ProviderIds: map[string]string{"Tvdb": "111"}},
		jellyfin.Item{ID: "s2", Name: "Legacy B", Type: "Series", ProviderIds: map[string]string{"Tvdb": "111"}},
	)
	tv := tvclient.NewFake()
	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jf, TV: tv, ReportDir: t.TempDir(),
		SyncWait: time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "legacy", Title: "Legacy", Kind: manifest.KindSeries,
		Provider: manifest.Provider{TVDB: 111}, Class: manifest.ClassEducational,
	}}}
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipAcquire: true, SkipSync: true, DryRun: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Report.Errors)
	assert.Contains(t, strings.Join(res.Report.Errors, "\n"), "ambiguous series")
	assert.Empty(t, res.Report.Imported)
}
