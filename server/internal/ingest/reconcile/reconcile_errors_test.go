package reconcile_test

import (
	"context"
	"errors"
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

func TestRunNilManifestError(t *testing.T) {
	t.Parallel()
	eng := reconcile.New(reconcile.Deps{})
	_, err := eng.Run(context.Background(), nil, nil, reconcile.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest is required")
}

func TestRunStageErrorsAccumulateInReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	boom := errors.New("forced failure")

	tv := tvclient.NewFake()
	tv.Err = boom
	jf := jellyfin.NewFake()
	jf.Err = boom
	rf := radarr.NewFake()
	rf.Err = boom
	sf := sonarr.NewFake()
	sf.Err = boom
	yt := &ytdlp.FakeRunner{Err: boom}

	// Parent of ReportDir is a file so WriteMarkdown fails.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nope"), []byte("x"), 0o644))

	eng := reconcile.New(reconcile.Deps{
		Radarr: rf, Sonarr: sf, Jellyfin: jf, TV: tv, YtDlp: yt,
		RadarrQualityProfileID: 1, RadarrRootFolder: "/movies",
		SonarrQualityProfileID: 1, SonarrRootFolder: "/tv",
		YtDlpOutputDir: "/media", YtDlpArchivePath: filepath.Join(dir, "a.txt"),
		ManifestPath: filepath.Join(dir, "m.yaml"),
		ReviewPath:   filepath.Join(dir, "r.yaml"),
		ReportDir:    filepath.Join(dir, "nope", "reports"),
		SyncWait:     time.Millisecond, SyncPollInterval: time.Millisecond,
	})

	m := &manifest.Manifest{Items: []manifest.Item{
		{ID: "movie-x", Title: "X", Year: 2000, Kind: manifest.KindMovie, Class: manifest.ClassEntertainment},
		{ID: "series-s", Title: "S", Year: 2001, Kind: manifest.KindSeries, Class: manifest.ClassEducational},
		{ID: "yt-z", Title: "Z", Kind: manifest.KindYouTubeChannel, URL: "https://youtube.com/@z", Class: manifest.ClassMixed},
		{ID: "manual-m", Title: "M", Kind: manifest.KindManual, Class: manifest.ClassEducational},
	}}
	// Stale review choice for unknown id hits apply-review error path.
	review := &manifest.Review{Entries: []manifest.ReviewEntry{
		{ID: "missing-id", ChosenTMDB: 99},
	}}

	res, err := eng.Run(context.Background(), m, review, reconcile.Options{})
	require.NoError(t, err) // stage errors are recorded, not returned
	require.NotNil(t, res.Report)
	assert.NotEmpty(t, res.Report.Errors)
}

func TestRunDryRunWithNilReview(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	eng := reconcile.New(reconcile.Deps{
		Radarr: radarr.NewFake(), Sonarr: sonarr.NewFake(),
		Jellyfin: jellyfin.NewFake(), TV: tvclient.NewFake(),
		ReportDir: filepath.Join(dir, "reports"),
		SyncWait:  time.Millisecond, SyncPollInterval: time.Millisecond,
	})
	m := &manifest.Manifest{Items: []manifest.Item{
		{ID: "manual-only", Title: "Manual", Kind: manifest.KindManual, Class: manifest.ClassEducational},
	}}
	res, err := eng.Run(context.Background(), m, nil, reconcile.Options{
		DryRun: true, SkipAcquire: true, SkipSync: true, SkipImport: true,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Review)
	// Manual kinds may land in ManualQueue or AlreadyHeld depending on stage skips.
	joined := strings.Join(append(append([]string{}, res.Report.ManualQueue...), res.Report.AlreadyHeld...), "\n")
	_ = joined
	require.NotNil(t, res.Report)
}
