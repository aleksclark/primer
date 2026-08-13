package reconcile_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

const (
	e2eSlug       = "paul-sellers"
	e2eShowTitle  = "Paul Sellers"
	e2eOlderID    = "aaaaaaaaaaa"
	e2eNewerID    = "bbbbbbbbbbb"
	e2eOlderTitle = "Older Video"
	e2eNewerTitle = "Newer Video"
)

var e2eEpisodeKeyRE = regexp.MustCompile(`(?i)S(\d+)E(\d+)`)

// countingExecRunner wraps ExecRunner so the E2E can assert present does not skip.
type countingExecRunner struct {
	inner ytdlp.ExecRunner
	n     int
}

func (r *countingExecRunner) Download(ctx context.Context, opts ytdlp.DownloadOpts) error {
	r.n++
	return r.inner.Download(ctx, opts)
}

// fsJellyfin wraps Fake and rebuilds Episode items from finalized *.mkv files
// (never _staging) on every Browse. RefreshLibrary is a no-op beyond the Fake.
type fsJellyfin struct {
	*jellyfin.Fake
	outputDir string
	slug      string
	showTitle string
}

func (f *fsJellyfin) seedFromDisk() {
	season := ytdlp.SeasonDir(f.outputDir, f.slug)
	entries, err := os.ReadDir(season)
	if err != nil {
		f.Items = []jellyfin.Item{{
			ID:   "jf-" + f.slug + "-folder",
			Name: f.slug,
			Type: "Folder",
			Path: ytdlp.ShowDir(f.outputDir, f.slug),
		}}
		return
	}
	items := []jellyfin.Item{{
		ID:   "jf-" + f.slug + "-folder",
		Name: f.slug,
		Type: "Folder",
		Path: ytdlp.ShowDir(f.outputDir, f.slug),
	}}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".mkv") {
			continue
		}
		id, ok := ytdlp.ParseYouTubeID(name)
		if !ok {
			continue
		}
		key := e2eEpisodeKey(name)
		idx := 0
		if key == "S01E001" {
			idx = 1
		}
		if key == "S01E002" {
			idx = 2
		}
		title := e2eVideoTitle(name)
		runtime := 120 * time.Second
		if id == e2eNewerID {
			runtime = 180 * time.Second
		}
		items = append(items, jellyfin.Item{
			ID:                "jf-" + id,
			Name:              title,
			Type:              "Episode",
			Path:              filepath.Join(season, name),
			SeriesName:        f.showTitle,
			IndexNumber:       idx,
			ParentIndexNumber: 1,
			Runtime:           runtime,
			ProviderIds:       map[string]string{"Youtube": id},
		})
	}
	f.Items = items
}

func (f *fsJellyfin) Browse(ctx context.Context, p jellyfin.BrowseParams) ([]jellyfin.Item, error) {
	f.seedFromDisk()
	return f.Fake.Browse(ctx, p)
}

func (f *fsJellyfin) BrowsePage(ctx context.Context, p jellyfin.BrowseParams) (jellyfin.Page, error) {
	f.seedFromDisk()
	return f.Fake.BrowsePage(ctx, p)
}

func writeYouTubeStub(t *testing.T, dir, argsFile string) string {
	t.Helper()
	stub := filepath.Join(dir, "yt-dlp-stub")
	// Parse -o; write TWO staging bundles only. Never write S01E names.
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > ` + strconv.Quote(argsFile) + `
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then
    out="$a"
  fi
  prev="$a"
done
if [ -z "$out" ]; then
  echo "stub: missing -o" >&2
  exit 2
fi
staging=$(dirname "$out")
case "$staging" in
  *_staging) ;;
  *)
    echo "stub: -o is not under _staging: $out" >&2
    exit 2
    ;;
esac
mkdir -p "$staging"
slug=$(basename "$(dirname "$(dirname "$staging")")")
write_bundle() {
  id="$1"
  title="$2"
  date="$3"
  dur="$4"
  stem="$staging/$slug - $title [$id]"
  printf 'mkv' > "$stem.mkv"
  printf 'jpg' > "$stem.jpg"
  cat > "$stem.info.json" <<EOF
{"id":"$id","title":"$title","upload_date":"$date","duration":$dur,"channel":"Paul Sellers"}
EOF
}
write_bundle aaaaaaaaaaa "Older Video" 20160501 120
write_bundle bbbbbbbbbbb "Newer Video" 20180615 180
exit 0
`
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o755))
	return stub
}

func e2eEpisodeKey(name string) string {
	m := e2eEpisodeKeyRE.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	season, err1 := strconv.Atoi(m[1])
	ep, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return ""
	}
	return "S" + pad2(season) + "E" + pad3(ep)
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return strings.Repeat("0", 2-len(s)) + s
	}
	return s
}

func pad3(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 3 {
		return strings.Repeat("0", 3-len(s)) + s
	}
	return s
}

func e2eVideoTitle(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.SplitN(base, " - ", 3)
	if len(parts) < 3 {
		return base
	}
	rest := parts[2]
	if i := strings.LastIndex(rest, " ["); i >= 0 {
		return rest[:i]
	}
	return rest
}

func TestYouTubeE2E_DownloadNFOImportStableRerun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "yt-dlp-argv.txt")
	stub := writeYouTubeStub(t, dir, argsFile)
	out := filepath.Join(dir, "media")
	reports := filepath.Join(dir, "reports")

	yt := &countingExecRunner{}
	jf := &fsJellyfin{
		Fake:      jellyfin.NewFake(),
		outputDir: out,
		slug:      e2eSlug,
		showTitle: e2eShowTitle,
	}
	tv := tvclient.NewFake()

	eng := reconcile.New(reconcile.Deps{
		Jellyfin:         jf,
		TV:               tv,
		YtDlp:            yt,
		YtDlpOutputDir:   out,
		YtDlpBinary:      stub,
		ReportDir:        reports,
		SyncWait:         time.Millisecond,
		SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{{
		ID:    e2eSlug,
		Title: e2eShowTitle,
		Kind:  manifest.KindYouTubeChannel,
		URL:   "https://www.youtube.com/@PaulSellersWoodwork",
		Class: manifest.ClassMixed,
	}}}

	// First apply: stub writes staging only; production FinalizeStaging must
	// assign S01E001/002. fsJellyfin Browse then lists finalized files.
	res, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 1, yt.n, "first apply must invoke ExecRunner")

	// Stub must never have written final names.
	argv, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.NotContains(t, string(argv), "S01E001")
	assert.NotContains(t, string(argv), "S01E002")

	season := ytdlp.SeasonDir(out, e2eSlug)
	entries, err := os.ReadDir(season)
	require.NoError(t, err)
	var finals []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".mkv") {
			finals = append(finals, e.Name())
		}
	}
	require.Len(t, finals, 2, "production finalize must produce two final mkvs: %v", finals)

	var e001, e002 string
	for _, name := range finals {
		assert.NotContains(t, name, "_staging")
		switch {
		case strings.Contains(name, "S01E001") && strings.Contains(name, e2eOlderID):
			e001 = name
		case strings.Contains(name, "S01E002") && strings.Contains(name, e2eNewerID):
			e002 = name
		}
	}
	require.NotEmpty(t, e001, "2016 upload must be S01E001: %v", finals)
	require.NotEmpty(t, e002, "2018 upload must be S01E002: %v", finals)
	assert.Contains(t, e001, e2eOlderTitle)
	assert.Contains(t, e002, e2eNewerTitle)

	nfo1, err := os.ReadFile(filepath.Join(season, strings.TrimSuffix(e001, ".mkv")+".nfo"))
	require.NoError(t, err)
	nfo2, err := os.ReadFile(filepath.Join(season, strings.TrimSuffix(e002, ".mkv")+".nfo"))
	require.NoError(t, err)
	assert.Contains(t, string(nfo1), "<episode>1</episode>")
	assert.Contains(t, string(nfo2), "<episode>2</episode>")
	assert.Contains(t, string(nfo1), `type="youtube" default="true">`+e2eOlderID)
	assert.Contains(t, string(nfo2), `type="youtube" default="true">`+e2eNewerID)

	ledRaw, err := os.ReadFile(ytdlp.LedgerPath(out, e2eSlug))
	require.NoError(t, err)
	var led struct {
		NextEpisode int `json:"next_episode"`
	}
	require.NoError(t, json.Unmarshal(ledRaw, &led))
	assert.Equal(t, 3, led.NextEpisode)

	arch, err := os.ReadFile(ytdlp.PerShowArchivePath(out, e2eSlug))
	require.NoError(t, err)
	assert.Contains(t, string(arch), "youtube "+e2eOlderID)
	assert.Contains(t, string(arch), "youtube "+e2eNewerID)

	items, err := tv.ListMediaItems(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2, "folder sibling must not import; only two episodes")
	byYT := map[string]tvclient.MediaItem{}
	for _, it := range items {
		byYT[it.YouTubeVideoID] = it
		assert.Equal(t, string(manifest.ClassMixed), it.Class)
		assert.Equal(t, e2eSlug, it.ManifestSlug)
		assert.Contains(t, it.Title, e2eShowTitle)
		assert.Contains(t, it.Title, "S01E00")
	}
	require.Contains(t, byYT, e2eOlderID)
	require.Contains(t, byYT, e2eNewerID)
	assert.Equal(t, "S01E001", byYT[e2eOlderID].EpisodeKey)
	assert.Equal(t, "S01E002", byYT[e2eNewerID].EpisodeKey)
	assert.Contains(t, byYT[e2eOlderID].Title, "S01E001")
	assert.Contains(t, byYT[e2eNewerID].Title, "S01E002")

	for _, c := range tv.CreateCalls {
		assert.NotContains(t, c.JellyfinItemID, "folder")
	}

	// Second apply: present must not skip ExecRunner; numbers stay put; no third import.
	createsBefore := len(tv.CreateCalls)
	res2, err := eng.Run(context.Background(), m, &manifest.Review{}, reconcile.Options{})
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.Equal(t, 2, yt.n, "second apply must still invoke ExecRunner (present must not skip)")

	entries2, err := os.ReadDir(season)
	require.NoError(t, err)
	var finals2 []string
	for _, e := range entries2 {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mkv") {
			finals2 = append(finals2, e.Name())
		}
	}
	require.Len(t, finals2, 2)
	assert.ElementsMatch(t, finals, finals2)

	ledRaw2, err := os.ReadFile(ytdlp.LedgerPath(out, e2eSlug))
	require.NoError(t, err)
	var led2 struct {
		NextEpisode int `json:"next_episode"`
	}
	require.NoError(t, json.Unmarshal(ledRaw2, &led2))
	assert.Equal(t, 3, led2.NextEpisode)

	items2, err := tv.ListMediaItems(context.Background())
	require.NoError(t, err)
	assert.Len(t, items2, 2)
	assert.Equal(t, createsBefore, len(tv.CreateCalls), "second apply must not create a third item")
	assert.Empty(t, res2.Report.Imported)
}
