package recovery_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/recovery"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

const (
	idFire   = "eKnuQfUSfyk"
	idSecond = "dQw4w9wgxcQ"
	idShort  = "shortclip01"
)

func TestNormalizeSlugs(t *testing.T) {
	t.Parallel()
	got := recovery.NormalizeSlugs("essential-craftsman, townsends", "townsends", "  primitive-technology ")
	assert.Equal(t, []string{"essential-craftsman", "townsends", "primitive-technology"}, got)
}

func TestRun_RequiresExplicitSlugsAndPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "Shows")
	require.NoError(t, os.MkdirAll(src, 0o755))
	man := writeManifest(t, dir)

	_, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: filepath.Join(dir, "out"), ManifestPath: man,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")
}

func TestRun_RefusesSourceOutputOverlap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "Shows")
	require.NoError(t, os.MkdirAll(src, 0o755))
	man := writeManifest(t, dir)
	opts := recovery.Options{
		SourceRoot: src, ManifestPath: man, Slugs: []string{"essential-craftsman"},
		Probe: fakeProbe(nil),
	}

	opts.OutputDir = src
	_, err := recovery.Run(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")

	opts.OutputDir = dir // would write dir/Shows, colliding with source
	_, err = recovery.Run(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")

	opts.OutputDir = filepath.Join(src, "nested")
	_, err = recovery.Run(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
}

func TestRun_RefusesJustinRhodesWrongSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	_, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"justin-rhodes"},
		Probe: fakeProbe(nil),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), recovery.ReasonQuarantined)
	assert.Contains(t, err.Error(), "wrong musician")
	assertEmptyDir(t, out)
}

func TestRun_DryRun_NoDirectoriesOrWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	srcFile := writeLegacy(t, src, "essential-craftsman", "Fire is a Tool.mp4", "video-bytes")
	srcNames := listNames(t, filepath.Dir(srcFile))
	srcIno := inodeOf(t, srcFile)

	res, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Probe: fakeProbe(map[string]recovery.ProbeFormat{
			srcFile: fireFormat(),
		}),
	})
	require.NoError(t, err)
	require.True(t, res.DryRun)
	require.Len(t, res.Items, 1)
	assert.Equal(t, recovery.StatusRecovered, res.Items[0].Status)
	assert.Equal(t, idFire, res.Items[0].YouTubeID)
	assert.Equal(t, "essential-craftsman/Season 01/Fire is a Tool.mp4", res.Items[0].Source)
	assert.Empty(t, res.Items[0].FinalPath)
	assert.Equal(t, 1, res.Counts.Recovered)
	assert.Equal(t, "Essential Craftsman", res.Shows[0].Title)
	assert.Equal(t, "mixed", res.Shows[0].Class)

	_, err = os.Stat(out)
	assert.True(t, os.IsNotExist(err), "dry-run must not create output")
	assert.Equal(t, srcNames, listNames(t, filepath.Dir(srcFile)))
	assert.Equal(t, srcIno, inodeOf(t, srcFile))
}

func TestRun_Apply_HardlinkSourceInodeUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	srcFile := writeLegacy(t, src, "essential-craftsman", "Fire is a Tool.mp4", "video-bytes")
	srcThumb := writeLegacy(t, src, "essential-craftsman", "Fire is a Tool.jpg", "img")
	srcIno := inodeOf(t, srcFile)
	srcThumbIno := inodeOf(t, srcThumb)
	srcNames := listNames(t, filepath.Dir(srcFile))

	now := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	res, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Apply: true,
		Now:   now,
		Probe: fakeProbe(map[string]recovery.ProbeFormat{
			srcFile: fireFormat(),
		}),
	})
	require.NoError(t, err)
	require.False(t, res.DryRun)
	require.Equal(t, 1, res.Counts.Recovered)
	require.Len(t, res.Items, 1)
	assert.Equal(t, idFire, res.Items[0].YouTubeID)
	require.NotEmpty(t, res.Items[0].FinalPath)

	assert.Equal(t, srcIno, inodeOf(t, srcFile), "source inode must not change")
	assert.Equal(t, srcThumbIno, inodeOf(t, srcThumb))
	assert.Equal(t, srcNames, listNames(t, filepath.Dir(srcFile)), "legacy tree must gain no names")
	got, err := os.ReadFile(srcFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("video-bytes"), got)

	final := res.Items[0].FinalPath
	assert.Equal(t, srcIno, inodeOf(t, final), "final media must be a hardlink of source")
	assert.Contains(t, filepath.Base(final), "["+idFire+"]")
	assert.Contains(t, filepath.Base(final), "S01E001")

	_, err = os.Stat(filepath.Join(ytdlp.SeasonDir(out, "essential-craftsman"), filepath.Base(final)))
	require.NoError(t, err)
	infoJSON := filepath.Join(ytdlp.SeasonDir(out, "essential-craftsman"), trimExt(filepath.Base(final))+".info.json")
	_, err = os.Stat(infoJSON)
	require.NoError(t, err)
	nfo := filepath.Join(ytdlp.SeasonDir(out, "essential-craftsman"), trimExt(filepath.Base(final))+".nfo")
	raw, err := os.ReadFile(nfo)
	require.NoError(t, err)
	assert.Contains(t, string(raw), idFire)
	assert.Contains(t, string(raw), "Regardless of the work")
	arch, err := os.ReadFile(ytdlp.PerShowArchivePath(out, "essential-craftsman"))
	require.NoError(t, err)
	assert.Contains(t, string(arch), "youtube "+idFire)

	staging := ytdlp.StagingDir(out, "essential-craftsman")
	entries, err := os.ReadDir(staging)
	if err == nil {
		assert.Empty(t, entries)
	}
}

func TestRun_Replay_IdempotentNoDuplicates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	srcFile := writeLegacy(t, src, "essential-craftsman", "Fire is a Tool.mp4", "video-bytes")
	probe := fakeProbe(map[string]recovery.ProbeFormat{srcFile: fireFormat()})
	now := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	opts := recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Apply: true, Now: now, Probe: probe,
	}
	first, err := recovery.Run(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 1, first.Counts.Recovered)
	final := first.Items[0].FinalPath
	season := ytdlp.SeasonDir(out, "essential-craftsman")
	before := listNames(t, season)

	second, err := recovery.Run(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 0, second.Counts.Recovered)
	require.Equal(t, 1, second.Counts.Existing)
	require.Len(t, second.Items, 1)
	assert.Equal(t, recovery.StatusExisting, second.Items[0].Status)
	assert.Equal(t, idFire, second.Items[0].YouTubeID)
	assert.Equal(t, final, second.Items[0].FinalPath)
	assert.Equal(t, before, listNames(t, season))
}

func TestRun_SkipsInvalidIDsAndShortDuration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	bad := writeLegacy(t, src, "essential-craftsman", "No ID.mp4", "x")
	short := writeLegacy(t, src, "essential-craftsman", "Tiny.mp4", "y")
	good := writeLegacy(t, src, "essential-craftsman", "Fire.mp4", "z")

	res, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Probe: fakeProbe(map[string]recovery.ProbeFormat{
			bad: {
				Title:    "No ID",
				Artist:   "EC",
				Date:     "20200101",
				Duration: 100,
				Comment:  "https://www.youtube.com/watch?v=nope",
			},
			short: {
				Title:    "Tiny",
				Artist:   "EC",
				Date:     "20200102",
				Duration: 12,
				Comment:  "https://www.youtube.com/watch?v=" + idShort,
			},
			good: fireFormat(),
		}),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Counts.Recovered)
	assert.Equal(t, 2, res.Counts.Skipped)
	bySrc := map[string]recovery.Item{}
	for _, it := range res.Items {
		bySrc[filepath.Base(it.Source)] = it
	}
	assert.Equal(t, recovery.ReasonInvalidID, bySrc["No ID.mp4"].Reason)
	assert.Empty(t, bySrc["No ID.mp4"].YouTubeID)
	assert.Equal(t, recovery.ReasonShort, bySrc["Tiny.mp4"].Reason)
	assert.Equal(t, idShort, bySrc["Tiny.mp4"].YouTubeID)
	assert.Equal(t, recovery.StatusRecovered, bySrc["Fire.mp4"].Status)
	assert.Equal(t, idFire, bySrc["Fire.mp4"].YouTubeID)
}

func TestRun_SkipsAmbiguousAndMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	amb := writeLegacy(t, src, "essential-craftsman", "Amb.mp4", "a")
	miss := writeLegacy(t, src, "essential-craftsman", "Miss.mp4", "b")
	nodate := writeLegacy(t, src, "essential-craftsman", "NoDate.mp4", "c")

	res, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Probe: fakeProbe(map[string]recovery.ProbeFormat{
			amb: {
				Title: "Amb", Date: "20200101", Duration: 90,
				Comment: "https://www.youtube.com/watch?v=" + idFire + "&v=" + idSecond,
			},
			miss: {Title: "Miss", Date: "20200101", Duration: 90},
			nodate: {
				Title: "NoDate", Duration: 90,
				Comment: "https://www.youtube.com/watch?v=" + idSecond,
			},
		}),
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Counts.Skipped)
	assert.Equal(t, 0, res.Counts.Recovered)
	reasons := map[string]string{}
	for _, it := range res.Items {
		reasons[filepath.Base(it.Source)] = it.Reason
		assert.Empty(t, it.FinalPath)
	}
	assert.Equal(t, recovery.ReasonAmbiguousID, reasons["Amb.mp4"])
	assert.Equal(t, recovery.ReasonMissingURL, reasons["Miss.mp4"])
	assert.Equal(t, recovery.ReasonInvalidDate, reasons["NoDate.mp4"])
}

func TestRun_Apply_BatchChronologicalLedger(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	later := writeLegacy(t, src, "essential-craftsman", "Later.mp4", "later")
	earlier := writeLegacy(t, src, "essential-craftsman", "Earlier.mp4", "earlier")
	res, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Apply: true,
		Now:   time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC),
		Probe: fakeProbe(map[string]recovery.ProbeFormat{
			later: {
				Duration: 200, Title: "Later", Artist: "EC", Date: "20200102",
				Comment: "https://www.youtube.com/watch?v=" + idSecond,
			},
			earlier: {
				Duration: 200, Title: "Earlier", Artist: "EC", Date: "20200101",
				Comment: "https://www.youtube.com/watch?v=" + idFire,
			},
		}),
	})
	require.NoError(t, err)
	require.Equal(t, 2, res.Counts.Recovered)
	byID := map[string]recovery.Item{}
	for _, it := range res.Items {
		byID[it.YouTubeID] = it
	}
	assert.Contains(t, byID[idFire].FinalPath, "S01E001")
	assert.Contains(t, byID[idSecond].FinalPath, "S01E002")
}

func TestRun_JSONSummaryHasNoGiantDescription(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	srcFile := writeLegacy(t, src, "essential-craftsman", "Fire.mp4", "v")
	fmt := fireFormat()
	fmt.Description = "Regardless of the work you do in your shop, sometimes fire is an indispensable tool."
	res, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"essential-craftsman"},
		Probe: fakeProbe(map[string]recovery.ProbeFormat{srcFile: fmt}),
	})
	require.NoError(t, err)
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "Regardless of the work")
	assert.Contains(t, string(raw), idFire)
	assert.Contains(t, string(raw), "Fire.mp4")
}

func TestRun_UnknownSlugRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, out, man := layout(t, dir)
	_, err := recovery.Run(context.Background(), recovery.Options{
		SourceRoot: src, OutputDir: out, ManifestPath: man,
		Slugs: []string{"not-a-show"},
		Probe: fakeProbe(nil),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the manifest")
}

func layout(t *testing.T, dir string) (src, out, man string) {
	t.Helper()
	src = filepath.Join(dir, "Shows")
	out = filepath.Join(dir, "primer")
	require.NoError(t, os.MkdirAll(src, 0o755))
	man = writeManifest(t, dir)
	return src, out, man
}

func writeManifest(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "content-manifest.yaml")
	yaml := `
items:
  - id: essential-craftsman
    title: Essential Craftsman
    kind: youtube_channel
    url: https://www.youtube.com/@EssentialCraftsman
    class: mixed
    filters: {}
  - id: justin-rhodes
    title: Justin Rhodes
    kind: youtube_channel
    url: https://www.youtube.com/@justinrhodes
    class: mixed
    filters: {}
  - id: townsends
    title: Townsends
    kind: youtube_channel
    url: https://www.youtube.com/@townsends
    class: mixed
    filters: {}
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))
	return path
}

func writeLegacy(t *testing.T, src, slug, name, body string) string {
	t.Helper()
	dir := filepath.Join(src, slug, "Season 01")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func fireFormat() recovery.ProbeFormat {
	return recovery.ProbeFormat{
		Duration:    1483.197823,
		Title:       "Fire is a Tool, Here's How to Use it.",
		Artist:      "Essential Craftsman",
		Date:        "20180312",
		Comment:     "https://www.youtube.com/watch?v=" + idFire,
		Description: "Regardless of the work you do in your shop.",
	}
}

func fakeProbe(byPath map[string]recovery.ProbeFormat) recovery.Prober {
	return func(_ context.Context, path string) (recovery.ProbeFormat, error) {
		if byPath == nil {
			return recovery.ProbeFormat{}, os.ErrNotExist
		}
		f, ok := byPath[path]
		if !ok {
			return recovery.ProbeFormat{}, os.ErrNotExist
		}
		return f, nil
	}
}

func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	st, err := os.Stat(path)
	require.NoError(t, err)
	sys, ok := st.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	return sys.Ino
}

func listNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
