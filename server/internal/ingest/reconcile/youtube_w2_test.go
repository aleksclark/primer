package reconcile_test

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

func TestYouTubePresentStillDownloadsMoviePresentSkips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	rf := radarr.NewFake()
	rf.LookupResults = []radarr.Movie{{Title: "The Matrix", Year: 1999, TmdbID: 603}}
	yt := &ytdlp.FakeRunner{}
	tv := tvclient.NewFake()
	tv.Manifest = []tvclient.ManifestEntry{
		{ID: "1", Slug: "paul-sellers", Title: "Paul Sellers", Kind: manifest.KindYouTubeChannel, Status: tvclient.ManifestStatusPresent},
		{ID: "2", Slug: "matrix", Title: "The Matrix", Kind: manifest.KindMovie, Status: tvclient.ManifestStatusPresent},
	}

	eng := reconcile.New(reconcile.Deps{
		Radarr: rf, TV: tv, YtDlp: yt,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/movies",
		YtDlpOutputDir: filepath.Join(dir, "media"),
		ReportDir:      filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{
		{
			ID: "paul-sellers", Title: "Paul Sellers",
			Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
			Class: manifest.ClassMixed,
		},
		{
			ID: "matrix", Title: "The Matrix", Year: 1999,
			Kind: manifest.KindMovie, Provider: manifest.Provider{TMDB: 603},
			Class: manifest.ClassEntertainment,
		},
	}}

	_, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	require.Len(t, yt.Calls, 1, "YouTube present must still call yt-dlp")
	assert.Equal(t, "paul-sellers", yt.Calls[0].Slug)
	assert.Empty(t, rf.AddCalls, "present movie must not Radarr-add")
}

func TestYouTubeBrowseAllFindsHitAfterEmptyFilteredPage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	items := make([]jellyfin.Item, 0, 201)
	for i := 0; i < 200; i++ {
		items = append(items, jellyfin.Item{
			ID:   fmt.Sprintf("noise-%03d", i),
			Name: fmt.Sprintf("Noise %d", i),
			Type: "Movie",
			Path: fmt.Sprintf("/media/Movies/noise-%03d.mkv", i),
		})
	}
	items = append(items, jellyfin.Item{
		ID:      "jf-ps-late",
		Name:    "Dovetails",
		Type:    "Video",
		Path:    "/media/tv/Primer/Shows/paul-sellers/Season 01/paul-sellers - S01E001 - Dovetails [dQw4w9wgxcQ].mkv",
		Runtime: 20 * time.Minute,
	})
	jf := jellyfin.NewFake(items...)
	tv := tvclient.NewFake()

	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jf, TV: tv,
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
		Class: manifest.ClassMixed,
	}}}

	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipAcquire: true, SkipSync: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, tv.CreateCalls, "browseAll must page past an empty first PathContains page")
	assert.Equal(t, "jf-ps-late", tv.CreateCalls[0].JellyfinItemID)
	assert.NotContains(t, res.Report.NotInJellyfin, "paul-sellers")
}

func TestYouTubeFolderSiblingDoesNotImportOrMarkPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	jf := jellyfin.NewFake(
		jellyfin.Item{
			ID:   "jf-ec-folder",
			Name: "essential-craftsman",
			Type: "Folder",
			Path: "/media/tv/Primer/Shows/essential-craftsman",
		},
		jellyfin.Item{
			ID:      "jf-ec-short",
			Name:    "Short clip",
			Type:    "Video",
			Path:    "/media/tv/Primer/Shows/essential-craftsman/Season 01/essential-craftsman - clip.mkv",
			Runtime: 30 * time.Second,
		},
	)
	tv := tvclient.NewFake()
	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jf, TV: tv,
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "essential-craftsman", Title: "Essential Craftsman",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@EssentialCraftsman",
		Class: manifest.ClassMixed,
	}}}

	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipAcquire: true, SkipSync: true,
	})
	require.NoError(t, err)
	assert.Empty(t, tv.CreateCalls, "Folder/Series must not be imported")
	assert.NotContains(t, res.Report.MarkedPresent, "essential-craftsman")
	assert.Empty(t, tv.PresentCalls)
}

func TestYouTubeFailedStillSkipsAcquire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yt := &ytdlp.FakeRunner{}
	tv := tvclient.NewFake()
	tv.Manifest = []tvclient.ManifestEntry{{
		ID: "1", Slug: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, Status: tvclient.ManifestStatusFailed, AttemptCount: 10,
	}}
	eng := reconcile.New(reconcile.Deps{
		TV: tv, YtDlp: yt,
		YtDlpOutputDir: filepath.Join(dir, "media"),
		ReportDir:      filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
		Class: manifest.ClassMixed,
	}}}
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Empty(t, yt.Calls, "failed YouTube stays operator-gated")
	assert.Contains(t, strings.Join(res.Report.FailedQueue, "\n"), "paul-sellers")
}

func TestYouTubeAcquireWiresPerShowArchiveCookiesAndCleanup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "media")
	showDir := filepath.Join(out, "Shows", "paul-sellers")
	require.NoError(t, os.MkdirAll(showDir, 0o755))
	poison := filepath.Join(out, "tvshow.nfo")
	require.NoError(t, os.WriteFile(poison, []byte("poison"), 0o644))
	debris := filepath.Join(showDir, "clip.part")
	require.NoError(t, os.WriteFile(debris, []byte("x"), 0o644))

	yt := &ytdlp.FakeRunner{}
	minOff := false
	eng := reconcile.New(reconcile.Deps{
		YtDlp: yt, TV: tvclient.NewFake(),
		YtDlpOutputDir:   out,
		YtDlpArchivePath: filepath.Join(dir, "deprecated-global.txt"),
		YtDlpCookiesPath: "/run/secrets/youtube.cookies",
		YtDlpJSRuntime:   "node",
		ReportDir:        filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
		Class: manifest.ClassMixed,
		Filters: manifest.Filters{
			MinDurationSeconds: 90,
			ExcludeShorts:      &minOff,
			ExcludeLive:        &minOff,
		},
	}}}
	_, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	require.Len(t, yt.Calls, 1)
	got := yt.Calls[0]
	assert.Equal(t, "https://www.youtube.com/@PaulSellersWoodwork", got.URL)
	assert.Equal(t, "paul-sellers", got.Slug)
	assert.Equal(t, out, got.OutputDir)
	assert.Equal(t, ytdlp.PerShowArchivePath(out, "paul-sellers"), got.ArchivePath)
	assert.NotEqual(t, filepath.Join(dir, "deprecated-global.txt"), got.ArchivePath)
	assert.Equal(t, "/run/secrets/youtube.cookies", got.CookiesPath)
	assert.Equal(t, "node", got.JSRuntime)
	assert.Equal(t, 90, got.MinDurationSeconds)
	require.NotNil(t, got.ExcludeShorts)
	assert.False(t, *got.ExcludeShorts)
	require.NotNil(t, got.ExcludeLive)
	assert.False(t, *got.ExcludeLive)
	assert.Equal(t, "Paul Sellers", got.ShowTitle)
	_, err = os.Stat(poison)
	assert.True(t, os.IsNotExist(err), "root tvshow.nfo must be removed before download")
	_, err = os.Stat(debris)
	assert.True(t, os.IsNotExist(err), "CleanupShowDir must run before download")
}

func TestYouTubeUnresolvedPlaylistFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yt := &ytdlp.FakeRunner{}
	eng := reconcile.New(reconcile.Deps{
		YtDlp: yt, TV: tvclient.NewFake(),
		YtDlpOutputDir: filepath.Join(dir, "media"),
		ReportDir:      filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
		Class:   manifest.ClassMixed,
		Filters: manifest.Filters{Playlists: []string{"Woodworking Projects"}},
	}}}
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Empty(t, yt.Calls, "named playlists must not download the channel URL")
	assert.Contains(t, strings.Join(res.Report.Errors, "\n"), "unresolved playlist")
}

func TestYouTubeLeftoverExit1AfterCompleteArchiveIsNotNewFail(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "media")
	show := filepath.Join(out, "Shows", "paul-sellers", "Season 01")
	require.NoError(t, os.MkdirAll(show, 0o755))
	require.NoError(t, os.WriteFile(ytdlp.PerShowArchivePath(out, "paul-sellers"), []byte("youtube dQw4w9wgxcQ\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(show, "paul-sellers - S01E001 - Dovetails [dQw4w9wgxcQ].mkv"), []byte("mkv"), 0o644))

	yt := &ytdlp.FakeRunner{Err: errors.New("yt-dlp: download failed: exit status 1")}
	tv := tvclient.NewFake()
	eng := reconcile.New(reconcile.Deps{
		YtDlp: yt, TV: tv,
		YtDlpOutputDir: out,
		ReportDir:      filepath.Join(dir, "reports"),
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
		Class: manifest.ClassMixed,
	}}}
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{
		SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	assert.Empty(t, tv.AttemptCalls, "leftover exit 1 after complete archive is not a new fail")
	joined := strings.Join(append(append([]string{}, res.Report.Errors...), res.Report.AcquiredYouTube...), "\n")
	assert.NotContains(t, strings.ToLower(joined), "--cookies")
	assert.NotContains(t, strings.ToLower(joined), "yt-dlp --")
}

func TestYouTubeImportRunsBeforeTVJellyfinSync(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	order := make([]string, 0, 4)
	jf := &orderJellyfin{
		Fake: jellyfin.NewFake(jellyfin.Item{
			ID: "jf-ps-1", Name: "Dovetails", Type: "Video",
			Path:    "/media/tv/Primer/Shows/paul-sellers/Season 01/paul-sellers - S01E001 - Dovetails [dQw4w9wgxcQ].mkv",
			Runtime: 20 * time.Minute,
		}),
		order: &order,
	}
	tv := &orderTV{Fake: tvclient.NewFake(), order: &order}
	eng := reconcile.New(reconcile.Deps{
		Jellyfin: jf, TV: tv,
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID: "paul-sellers", Title: "Paul Sellers",
		Kind: manifest.KindYouTubeChannel, URL: "https://www.youtube.com/@PaulSellersWoodwork",
		Class: manifest.ClassMixed,
	}}}
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{SkipAcquire: true})
	require.NoError(t, err)
	assert.True(t, res.Report.JellyfinRefreshed)
	assert.True(t, res.Report.TVSynced)
	assert.Equal(t, []string{"refresh", "import", "tv-sync"}, compactOrder(order))
}

type orderJellyfin struct {
	*jellyfin.Fake
	order *[]string
}

func (f *orderJellyfin) RefreshLibrary(ctx context.Context) error {
	*f.order = append(*f.order, "refresh")
	return f.Fake.RefreshLibrary(ctx)
}

type orderTV struct {
	*tvclient.Fake
	order *[]string
}

func (f *orderTV) CreateMediaItem(ctx context.Context, in tvclient.MediaItemCreate) (*tvclient.MediaItem, error) {
	*f.order = append(*f.order, "import")
	return f.Fake.CreateMediaItem(ctx, in)
}

func (f *orderTV) SyncJellyfin(ctx context.Context) (*tvclient.SyncResult, error) {
	*f.order = append(*f.order, "tv-sync")
	return f.Fake.SyncJellyfin(ctx)
}

func compactOrder(in []string) []string {
	var out []string
	for _, s := range in {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
