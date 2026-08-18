package ytdlp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestBuildArgs_StagingAndFlags(t *testing.T) {
	t.Parallel()
	out := "/media/tv/Primer"
	slug := "paul-sellers"
	args := ytdlp.BuildArgs(ytdlp.DownloadOpts{
		URL:         "https://www.youtube.com/@PaulSellersWoodwork",
		Slug:        slug,
		OutputDir:   out,
		ArchivePath: ytdlp.PerShowArchivePath(out, slug),
		Format:      "best",
	})
	joined := strings.Join(args, "\x00")
	assert.Contains(t, args, "--ignore-errors")
	assert.Contains(t, args, "--no-abort-on-error")
	assert.Contains(t, args, "--no-overwrites")
	assert.Contains(t, args, "--continue")
	assert.Contains(t, args, "--no-write-playlist-metafiles")
	assert.Contains(t, args, "--embed-metadata")
	assert.Contains(t, args, "--write-info-json")
	assert.Contains(t, args, "--write-thumbnail")
	assert.Contains(t, args, "--convert-thumbnails")
	assert.Contains(t, args, "jpg")
	assert.Contains(t, args, "--merge-output-format")
	assert.Contains(t, args, "mkv")
	assert.Contains(t, args, "--extractor-args")
	assert.Contains(t, args, "youtube:player_client=android,web")
	assert.Contains(t, args, "-f")
	assert.Contains(t, args, "best")
	assert.Contains(t, args, "-o")
	// staging template with %(id)s
	var oIdx = -1
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			oIdx = i + 1
			break
		}
	}
	require.Greater(t, oIdx, 0)
	assert.Contains(t, args[oIdx], "_staging")
	assert.Contains(t, args[oIdx], "%(id)s")
	assert.NotContains(t, args[oIdx], "playlist_index")
	// archive
	assert.Contains(t, args, "--download-archive")
	assert.Contains(t, args, ytdlp.PerShowArchivePath(out, slug))
	// match-filter default
	assert.Contains(t, args, "--match-filter")
	var mf string
	for i, a := range args {
		if a == "--match-filter" && i+1 < len(args) {
			mf = args[i+1]
		}
	}
	assert.Contains(t, mf, "!is_live")
	assert.Contains(t, mf, "duration>59")
	// normalized URL last
	assert.Equal(t, "https://www.youtube.com/@PaulSellersWoodwork/videos", args[len(args)-1])
	// no cookies/js by default
	assert.NotContains(t, joined, "--cookies")
	assert.NotContains(t, joined, "--js-runtimes")
}

func TestBuildArgs_CookiesAndJSRuntime(t *testing.T) {
	t.Parallel()
	args := ytdlp.BuildArgs(ytdlp.DownloadOpts{
		URL:         "https://www.youtube.com/@x/videos",
		Slug:        "x",
		OutputDir:   "/m",
		Format:      "best",
		CookiesPath: "/secret/cookies.txt",
		JSRuntime:   "node",
	})
	assert.Contains(t, args, "--cookies")
	assert.Contains(t, args, "/secret/cookies.txt")
	assert.Contains(t, args, "--js-runtimes")
	assert.Contains(t, args, "node")
}

func TestBuildArgs_JSRuntimeEmptyOmits(t *testing.T) {
	t.Parallel()
	// explicit empty means omit (vs default applied at Download layer)
	args := ytdlp.BuildArgs(ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@x/videos", Slug: "x", OutputDir: "/m", Format: "b",
		JSRuntime: "",
	})
	assert.NotContains(t, args, "--js-runtimes")
}

func TestDownload_RejectsShortsWithoutOptOut(t *testing.T) {
	t.Parallel()
	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@x/shorts", Slug: "x", OutputDir: t.TempDir(),
		Binary: "/bin/false",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shorts")
}

func TestDownload_RejectsStreamsWithoutOptOut(t *testing.T) {
	t.Parallel()
	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@x/streams", Slug: "x", OutputDir: t.TempDir(),
		Binary: "/bin/false",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "streams")
}

func TestDownload_AllowsShortsWhenExcludeShortsFalse(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "yt-dlp-stub")
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	ex := false
	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@x/shorts", Slug: "x", OutputDir: dir,
		Binary: stub, ExcludeShorts: &ex,
	})
	require.NoError(t, err)
}

func TestExecRunner_ArgvDumpStub(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	stub := filepath.Join(dir, "yt-dlp-stub")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\nexit 0\n"
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o755))

	out := filepath.Join(dir, "lib")
	slug := "demo-show"
	archive := ytdlp.PerShowArchivePath(out, slug)
	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL:         "https://www.youtube.com/@Demo",
		Slug:        slug,
		OutputDir:   out,
		ArchivePath: archive,
		Binary:      stub,
		Format:      "best",
		CookiesPath: filepath.Join(dir, "cookies.txt"),
		JSRuntime:   "node",
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	assert.Contains(t, lines, "--write-info-json")
	assert.Contains(t, lines, "--continue")
	assert.Contains(t, lines, "--no-write-playlist-metafiles")
	assert.Contains(t, lines, "--cookies")
	assert.Contains(t, lines, "--js-runtimes")
	assert.Contains(t, lines, "node")
	assert.Contains(t, lines, "--download-archive")
	var usedArchive string
	for i, l := range lines {
		if l == "--download-archive" && i+1 < len(lines) {
			usedArchive = lines[i+1]
		}
	}
	require.NotEmpty(t, usedArchive)
	assert.NotEqual(t, archive, usedArchive, "yt-dlp must receive the run-temp archive, not the persistent file")
	assert.Contains(t, usedArchive, ".ytdlp-archive-run-")
	// staging -o
	foundO := false
	for i, l := range lines {
		if l == "-o" && i+1 < len(lines) {
			foundO = true
			assert.Contains(t, lines[i+1], "_staging")
			assert.Contains(t, lines[i+1], "%(id)s")
		}
	}
	assert.True(t, foundO)
}

func TestExecRunner_ErrorSanitizedNoFullArgv(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "yt-dlp-bad")
	require.NoError(t, os.WriteFile(bad, []byte("#!/bin/sh\nexit 2\n"), 0o755))
	cookies := filepath.Join(dir, "secret-cookies.txt")
	require.NoError(t, os.WriteFile(cookies, []byte("secret"), 0o600))

	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@Demo", Slug: "x", OutputDir: dir,
		Binary: bad, CookiesPath: cookies, JSRuntime: "node",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ytdlp:")
	assert.Contains(t, err.Error(), "download failed")
	assert.NotContains(t, err.Error(), cookies)
	assert.NotContains(t, err.Error(), "--cookies")
	assert.NotContains(t, err.Error(), "--js-runtimes")
}

func TestFinalizeStaging_AssignsRenamesNFOArchive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "essential-craftsman"
	title := "Essential Craftsman"
	staging := ytdlp.StagingDir(dir, slug)
	require.NoError(t, os.MkdirAll(staging, 0o755))

	id := "dQw4w9wgxcQ"
	stem := slug + " - Fire as a Tool [" + id + "]"
	mkv := filepath.Join(staging, stem+".mkv")
	info := filepath.Join(staging, stem+".info.json")
	jpg := filepath.Join(staging, stem+".jpg")
	require.NoError(t, os.WriteFile(mkv, []byte("video"), 0o644))
	require.NoError(t, os.WriteFile(jpg, []byte("img"), 0o644))

	infoObj := map[string]any{
		"id":          id,
		"title":       "Fire as a Tool",
		"description": "About fire.",
		"channel":     "Essential Craftsman",
		"channel_id":  "UCabc",
		"upload_date": "20180312",
		"duration":    1100.0,
		"was_live":    false,
		"live_status": "not_live",
	}
	raw, err := json.Marshal(infoObj)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(info, raw, 0o644))

	res, err := ytdlp.FinalizeStaging(ytdlp.FinalizeOpts{
		OutputDir:   dir,
		Slug:        slug,
		ShowTitle:   title,
		Now:         time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC),
		MinDuration: 59,
	})
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, id, res[0].ID)
	assert.Equal(t, 1, res[0].Episode)

	finalBase := ytdlp.FinalBasename(slug, 1, 1, "Fire as a Tool", id)
	season := ytdlp.SeasonDir(dir, slug)
	finalMKV := filepath.Join(season, finalBase+".mkv")
	_, err = os.Stat(finalMKV)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(season, finalBase+".info.json"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(season, finalBase+".jpg"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(season, finalBase+".nfo"))
	require.NoError(t, err)

	// staging gone
	_, err = os.Stat(mkv)
	assert.True(t, os.IsNotExist(err))

	// show nfo
	showNFO, err := os.ReadFile(filepath.Join(ytdlp.ShowDir(dir, slug), "tvshow.nfo"))
	require.NoError(t, err)
	assert.Contains(t, string(showNFO), title)

	// archive appended after rename
	arch, err := os.ReadFile(ytdlp.PerShowArchivePath(dir, slug))
	require.NoError(t, err)
	assert.Contains(t, string(arch), "youtube "+id)

	// episode nfo youtube uniqueid
	epNFO, err := os.ReadFile(filepath.Join(season, finalBase+".nfo"))
	require.NoError(t, err)
	assert.Contains(t, string(epNFO), `type="youtube" default="true">`+id)
}

func TestFinalizeStaging_RejectsLiveAndShort(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "x"
	staging := ytdlp.StagingDir(dir, slug)
	require.NoError(t, os.MkdirAll(staging, 0o755))

	writeInfo := func(id string, extra map[string]any) {
		stem := "x - t [" + id + "]"
		require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".mkv"), []byte("v"), 0o644))
		m := map[string]any{"id": id, "title": "t", "upload_date": "20200101", "duration": 120.0}
		for k, v := range extra {
			m[k] = v
		}
		b, _ := json.Marshal(m)
		require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".info.json"), b, 0o644))
	}
	writeInfo("livevideoid", map[string]any{"is_live": true, "duration": 600.0})
	writeInfo("shortvideoid", map[string]any{"duration": 30.0})

	res, err := ytdlp.FinalizeStaging(ytdlp.FinalizeOpts{
		OutputDir: dir, Slug: slug, ShowTitle: "X", Now: time.Now().UTC(), MinDuration: 59,
	})
	require.NoError(t, err)
	assert.Empty(t, res)
	// nothing archived
	_, err = os.Stat(ytdlp.PerShowArchivePath(dir, slug))
	assert.True(t, os.IsNotExist(err) || fileEmpty(t, ytdlp.PerShowArchivePath(dir, slug)))
}

func fileEmpty(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return len(strings.TrimSpace(string(b))) == 0
}

func TestDefaultMatchFilter(t *testing.T) {
	t.Parallel()
	mf := ytdlp.DefaultMatchFilter(nil)
	assert.Contains(t, mf, "!is_live")
	assert.Contains(t, mf, "!was_live")
	assert.Contains(t, mf, "live_status!=is_upcoming")
	assert.Contains(t, mf, "duration>59")
}

func TestDefaultMatchFilter_AllowPastLiveOmitsWasLive(t *testing.T) {
	t.Parallel()
	mf := ytdlp.DefaultMatchFilter(&ytdlp.DownloadOpts{AllowPastLive: true})
	assert.Contains(t, mf, "!is_live")
	assert.Contains(t, mf, "live_status!=is_upcoming")
	assert.NotContains(t, mf, "!was_live")
	assert.Contains(t, mf, "duration>59")
}

func TestDefaultMatchFilter_NegativeDurationDisablesFloor(t *testing.T) {
	t.Parallel()
	mf := ytdlp.DefaultMatchFilter(&ytdlp.DownloadOpts{MinDurationSeconds: -1})
	assert.Contains(t, mf, "!is_live")
	assert.NotContains(t, mf, "duration>")
}

func TestFinalizeStaging_AllowsPastLiveWhenConfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "mark-rober-science-class"
	staging := ytdlp.StagingDir(dir, slug)
	require.NoError(t, os.MkdirAll(staging, 0o755))

	id := "pastliveid1"
	stem := slug + " - Science Class [" + id + "]"
	require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".mkv"), []byte("video"), 0o644))
	infoObj := map[string]any{
		"id":          id,
		"title":       "Science Class",
		"upload_date": "20220101",
		"duration":    2400.0,
		"was_live":    true,
		"live_status": "was_live",
	}
	raw, err := json.Marshal(infoObj)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".info.json"), raw, 0o644))

	res, err := ytdlp.FinalizeStaging(ytdlp.FinalizeOpts{
		OutputDir:     dir,
		Slug:          slug,
		ShowTitle:     "Mark Rober Science Class",
		Now:           time.Now().UTC(),
		MinDuration:   59,
		AllowPastLive: true,
	})
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, id, res[0].ID)
}

func TestFinalizeStaging_StillRejectsLiveAndUpcomingWhenAllowPastLive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "x"
	staging := ytdlp.StagingDir(dir, slug)
	require.NoError(t, os.MkdirAll(staging, 0o755))

	writeInfo := func(id string, extra map[string]any) {
		stem := "x - t [" + id + "]"
		require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".mkv"), []byte("v"), 0o644))
		m := map[string]any{"id": id, "title": "t", "upload_date": "20200101", "duration": 600.0}
		for k, v := range extra {
			m[k] = v
		}
		b, err := json.Marshal(m)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".info.json"), b, 0o644))
	}
	writeInfo("nowlivevide", map[string]any{"is_live": true, "live_status": "is_live"})
	writeInfo("upcomingvid", map[string]any{"live_status": "is_upcoming"})

	res, err := ytdlp.FinalizeStaging(ytdlp.FinalizeOpts{
		OutputDir: dir, Slug: slug, ShowTitle: "X", Now: time.Now().UTC(),
		MinDuration: 59, AllowPastLive: true,
	})
	require.NoError(t, err)
	assert.Empty(t, res)
}

func TestFinalizeStaging_AllowsShortWhenDurationDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "mark-rober-shorts"
	staging := ytdlp.StagingDir(dir, slug)
	require.NoError(t, os.MkdirAll(staging, 0o755))

	id := "shortclip01"
	stem := slug + " - Tiny [" + id + "]"
	require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".mkv"), []byte("v"), 0o644))
	infoObj := map[string]any{
		"id":          id,
		"title":       "Tiny",
		"upload_date": "20230101",
		"duration":    30.0,
		"was_live":    false,
		"live_status": "not_live",
	}
	raw, err := json.Marshal(infoObj)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(staging, stem+".info.json"), raw, 0o644))

	res, err := ytdlp.FinalizeStaging(ytdlp.FinalizeOpts{
		OutputDir: dir, Slug: slug, ShowTitle: "Mark Rober Shorts",
		Now: time.Now().UTC(), MinDuration: -1,
	})
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, id, res[0].ID)
}
