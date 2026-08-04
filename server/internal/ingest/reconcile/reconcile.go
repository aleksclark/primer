// Package reconcile implements the content-ingest pipeline: resolve → acquire
// → sync → import → report. Each stage is idempotent; the manifest is desired
// state and a run converges toward it. Plan mode (DryRun) computes the diff
// without mutating upstream systems (except writing review.yaml candidates).
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/radarr"
	"github.com/aleksclark/primer/server/internal/ingest/sonarr"
	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

// Deps holds the collaborators the reconciler needs. Any may be nil when that
// stage's upstream is unconfigured; the stage then no-ops with a note.
type Deps struct {
	Radarr   radarr.Client
	Sonarr   sonarr.Client
	Jellyfin jellyfin.LibraryAdmin
	TV       tvclient.Client
	YtDlp    ytdlp.Runner

	// RadarrQualityProfileID / RadarrRootFolder configure movie adds.
	RadarrQualityProfileID int
	RadarrRootFolder       string
	RadarrTag              string

	// SonarrQualityProfileID / SonarrRootFolder configure series adds.
	SonarrQualityProfileID int
	SonarrRootFolder       string
	SonarrTag              string

	// YtDlpOutputDir / YtDlpArchivePath / YtDlpBinary configure downloads.
	YtDlpOutputDir   string
	YtDlpArchivePath string
	YtDlpBinary      string

	// SyncWait / SyncPollInterval control Jellyfin scan waiting.
	SyncWait         time.Duration
	SyncPollInterval time.Duration

	// ManifestPath / ReviewPath are rewritten when resolve fills provider IDs.
	ManifestPath string
	ReviewPath   string
	ReportDir    string

	Log *slog.Logger
}

// Engine runs the pipeline.
type Engine struct {
	deps Deps
	log  *slog.Logger
}

// New builds an engine.
func New(deps Deps) *Engine {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	if deps.SyncWait <= 0 {
		deps.SyncWait = 2 * time.Minute
	}
	if deps.SyncPollInterval <= 0 {
		deps.SyncPollInterval = 2 * time.Second
	}
	if deps.RadarrTag == "" {
		deps.RadarrTag = "primer"
	}
	if deps.SonarrTag == "" {
		deps.SonarrTag = "primer"
	}
	return &Engine{deps: deps, log: log}
}

// Result is the outcome of one plan/apply.
type Result struct {
	Report        *Report
	ReportPath    string
	Manifest      *manifest.Manifest
	Review        *manifest.Review
	ManifestDirty bool
	ReviewDirty   bool
}

// Options controls one run.
type Options struct {
	// DryRun (plan) computes the diff without mutating Radarr/Sonarr/yt-dlp/
	// Jellyfin/TV. Resolve still writes review.yaml candidates and applies
	// human choices from review.yaml into the in-memory manifest (persisted
	// only on apply, or always for review.yaml so the human can answer).
	DryRun bool
	// SkipAcquire skips Radarr/Sonarr/yt-dlp.
	SkipAcquire bool
	// SkipSync skips Jellyfin refresh + TV jellyfin/sync.
	SkipSync bool
	// SkipImport skips TV media-item create/update.
	SkipImport bool
}

// Run executes the full pipeline against the loaded manifest.
func (e *Engine) Run(ctx context.Context, m *manifest.Manifest, review *manifest.Review, opts Options) (*Result, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest is required")
	}
	if review == nil {
		review = &manifest.Review{}
	}
	rep := NewReport(opts.DryRun)
	res := &Result{Report: rep, Manifest: m, Review: review}

	// Mirror desired state into the TV server first so attempt/present tracking
	// has a row for every catalog slug, even when later stages no-op.
	if err := e.syncManifestCatalog(ctx, m, rep, opts); err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("manifest catalog: %v", err))
	}

	if err := e.resolve(ctx, m, review, rep, res, opts); err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("resolve: %v", err))
	}
	// Re-sync after resolve so newly filled provider IDs land in TV immediately.
	if res.ManifestDirty {
		if err := e.syncManifestCatalog(ctx, m, rep, opts); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("manifest catalog: %v", err))
		}
	}
	if !opts.SkipAcquire {
		if err := e.acquire(ctx, m, rep, opts); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("acquire: %v", err))
		}
	}
	if !opts.SkipSync && !opts.DryRun {
		if err := e.sync(ctx, rep); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("sync: %v", err))
		}
	}
	if !opts.SkipImport {
		if err := e.importItems(ctx, m, rep, opts); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("import: %v", err))
		}
	}

	// Persist working state.
	if res.ManifestDirty && !opts.DryRun && e.deps.ManifestPath != "" {
		if err := manifest.Save(e.deps.ManifestPath, m); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("save manifest: %v", err))
		}
	}
	// review.yaml is always written when dirty so plan mode still surfaces candidates.
	if res.ReviewDirty && e.deps.ReviewPath != "" {
		if err := manifest.SaveReview(e.deps.ReviewPath, review); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("save review: %v", err))
		}
	}
	if e.deps.ReportDir != "" {
		path, err := rep.WriteMarkdown(e.deps.ReportDir)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("write report: %v", err))
		} else {
			res.ReportPath = path
		}
	}
	return res, nil
}

// resolve fills provider IDs and manages the review queue.
func (e *Engine) resolve(ctx context.Context, m *manifest.Manifest, review *manifest.Review, rep *Report, res *Result, opts Options) error {
	// First: apply any human choices already in review.yaml.
	for _, entry := range append([]manifest.ReviewEntry{}, review.Entries...) {
		if entry.ChosenTMDB == 0 && entry.ChosenTVDB == 0 {
			continue
		}
		p := manifest.Provider{TMDB: entry.ChosenTMDB, TVDB: entry.ChosenTVDB}
		if err := m.SetProvider(entry.ID, p); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("apply review %s: %v", entry.ID, err))
			continue
		}
		review.Remove(entry.ID)
		res.ManifestDirty = true
		res.ReviewDirty = true
		rep.ReviewApplied = append(rep.ReviewApplied, fmt.Sprintf("%s → tmdb=%d tvdb=%d", entry.ID, p.TMDB, p.TVDB))
		e.log.Info("applied review choice", "id", entry.ID, "tmdb", p.TMDB, "tvdb", p.TVDB)
	}

	for _, it := range m.SortedByPriority() {
		if !it.NeedsResolve() {
			if (it.Kind == manifest.KindMovie || it.Kind == manifest.KindSeries) && !it.Provider.Empty() {
				rep.AlreadyResolved = append(rep.AlreadyResolved, it.ID)
			}
			continue
		}
		switch it.Kind {
		case manifest.KindMovie:
			if e.deps.Radarr == nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: radarr not configured", it.ID))
				continue
			}
			term := it.Title
			if it.Year > 0 {
				term = fmt.Sprintf("%s %d", it.Title, it.Year)
			}
			hits, err := e.deps.Radarr.Lookup(ctx, term)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s lookup: %v", it.ID, err))
				continue
			}
			hits = radarr.FilterYear(hits, it.Year)
			if len(hits) == 1 {
				p := manifest.Provider{TMDB: hits[0].TmdbID}
				if err := m.SetProvider(it.ID, p); err != nil {
					rep.Errors = append(rep.Errors, err.Error())
					continue
				}
				res.ManifestDirty = true
				review.Remove(it.ID)
				res.ReviewDirty = true
				rep.Resolved = append(rep.Resolved, fmt.Sprintf("%s → tmdb=%d (%s)", it.ID, p.TMDB, hits[0].Title))
				continue
			}
			cands := make([]manifest.Candidate, 0, len(hits))
			for _, h := range hits {
				cands = append(cands, manifest.Candidate{
					Title: h.Title, Year: h.Year, TMDB: h.TmdbID, Overview: truncate(h.Overview, 160),
				})
			}
			reason := "no hits"
			if len(hits) > 1 {
				reason = fmt.Sprintf("%d hits", len(hits))
			}
			review.Upsert(manifest.ReviewEntry{
				ID: it.ID, Title: it.Title, Year: it.Year, Kind: it.Kind,
				Reason: reason, Candidates: cands,
			})
			res.ReviewDirty = true
			rep.ReviewQueued = append(rep.ReviewQueued, fmt.Sprintf("%s (%s)", it.ID, reason))

		case manifest.KindSeries:
			if e.deps.Sonarr == nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: sonarr not configured", it.ID))
				continue
			}
			term := it.Title
			if it.Year > 0 {
				term = fmt.Sprintf("%s %d", it.Title, it.Year)
			}
			hits, err := e.deps.Sonarr.Lookup(ctx, term)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s lookup: %v", it.ID, err))
				continue
			}
			hits = sonarr.FilterYear(hits, it.Year)
			if len(hits) == 1 {
				p := manifest.Provider{TVDB: hits[0].TvdbID, TMDB: hits[0].TmdbID}
				if err := m.SetProvider(it.ID, p); err != nil {
					rep.Errors = append(rep.Errors, err.Error())
					continue
				}
				res.ManifestDirty = true
				review.Remove(it.ID)
				res.ReviewDirty = true
				rep.Resolved = append(rep.Resolved, fmt.Sprintf("%s → tvdb=%d (%s)", it.ID, p.TVDB, hits[0].Title))
				continue
			}
			cands := make([]manifest.Candidate, 0, len(hits))
			for _, h := range hits {
				cands = append(cands, manifest.Candidate{
					Title: h.Title, Year: h.Year, TVDB: h.TvdbID, TMDB: h.TmdbID,
					Overview: truncate(h.Overview, 160),
				})
			}
			reason := "no hits"
			if len(hits) > 1 {
				reason = fmt.Sprintf("%d hits", len(hits))
			}
			review.Upsert(manifest.ReviewEntry{
				ID: it.ID, Title: it.Title, Year: it.Year, Kind: it.Kind,
				Reason: reason, Candidates: cands,
			})
			res.ReviewDirty = true
			rep.ReviewQueued = append(rep.ReviewQueued, fmt.Sprintf("%s (%s)", it.ID, reason))
		}
		_ = opts
	}
	return nil
}

// syncManifestCatalog pushes the YAML desired state into the TV server.
func (e *Engine) syncManifestCatalog(ctx context.Context, m *manifest.Manifest, rep *Report, opts Options) error {
	if e.deps.TV == nil {
		return nil
	}
	items := make([]tvclient.ManifestDesired, 0, len(m.Items))
	for _, it := range m.Items {
		items = append(items, toManifestDesired(it))
	}
	if opts.DryRun {
		rep.ManifestSynced = fmt.Sprintf("would upsert %d entries", len(items))
		return nil
	}
	res, err := e.deps.TV.SyncManifest(ctx, items)
	if err != nil {
		return err
	}
	rep.ManifestSynced = fmt.Sprintf("created=%d updated=%d total=%d", res.Created, res.Updated, res.Total)
	return nil
}

func toManifestDesired(it manifest.Item) tvclient.ManifestDesired {
	return tvclient.ManifestDesired{
		Slug:            it.ID,
		Title:           it.Title,
		Year:            it.Year,
		Kind:            it.Kind,
		TMDBID:          it.Provider.TMDB,
		TVDBID:          it.Provider.TVDB,
		URL:             it.URL,
		Class:           it.Class,
		SubjectTags:     it.SubjectTags,
		StandardCodes:   it.StandardCodes,
		Priority:        it.Priority,
		ExcludeEpisodes: it.ExcludeEpisodes,
		MaxEpisodes:     it.MaxEpisodes,
		Notes:           it.Notes,
	}
}

// acquire adds missing movies/series and runs yt-dlp for YouTube sources.
func (e *Engine) acquire(ctx context.Context, m *manifest.Manifest, rep *Report, opts Options) error {
	var (
		radarrLib []radarr.Movie
		sonarrLib []sonarr.Series
		err       error
	)
	if e.deps.Radarr != nil {
		radarrLib, err = e.deps.Radarr.List(ctx)
		if err != nil {
			return fmt.Errorf("list radarr: %w", err)
		}
	}
	if e.deps.Sonarr != nil {
		sonarrLib, err = e.deps.Sonarr.List(ctx)
		if err != nil {
			return fmt.Errorf("list sonarr: %w", err)
		}
	}

	skipAcquire := e.manifestStatuses(ctx, rep)

	var radarrTagID, sonarrTagID int
	if !opts.DryRun && e.deps.Radarr != nil {
		radarrTagID, err = e.deps.Radarr.EnsureTag(ctx, e.deps.RadarrTag)
		if err != nil {
			return fmt.Errorf("radarr tag: %w", err)
		}
	}
	if !opts.DryRun && e.deps.Sonarr != nil {
		sonarrTagID, err = e.deps.Sonarr.EnsureTag(ctx, e.deps.SonarrTag)
		if err != nil {
			return fmt.Errorf("sonarr tag: %w", err)
		}
	}

	// Movies/series first so a slow yt-dlp pass cannot starve Radarr/Sonarr
	// adds. Priority still orders within each wave.
	acquireOrder := make([]manifest.Item, 0, len(m.Items))
	for _, it := range m.SortedByPriority() {
		switch it.Kind {
		case manifest.KindMovie, manifest.KindSeries, manifest.KindManual:
			acquireOrder = append(acquireOrder, it)
		}
	}
	for _, it := range m.SortedByPriority() {
		switch it.Kind {
		case manifest.KindYouTubeChannel, manifest.KindYouTubePlaylist:
			acquireOrder = append(acquireOrder, it)
		}
	}

	for _, it := range acquireOrder {
		status := skipAcquire[it.ID]
		if status == tvclient.ManifestStatusFailed {
			rep.FailedQueue = append(rep.FailedQueue, fmt.Sprintf("%s — %s (human intervention)", it.ID, it.Title))
			continue
		}
		if status == tvclient.ManifestStatusPresent {
			// Already obtained; nothing to acquire. Import still runs later.
			continue
		}

		switch it.Kind {
		case manifest.KindManual:
			rep.ManualQueue = append(rep.ManualQueue, fmt.Sprintf("%s — %s", it.ID, it.Title))
			continue

		case manifest.KindMovie:
			if it.Provider.TMDB == 0 {
				continue
			}
			if e.deps.Radarr == nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: radarr not configured", it.ID))
				continue
			}
			if existing := radarr.ByTMDB(radarrLib, it.Provider.TMDB); existing != nil {
				rep.AlreadyHeld = append(rep.AlreadyHeld, fmt.Sprintf("%s (radarr id=%d hasFile=%v)", it.ID, existing.ID, existing.HasFile))
				if !existing.HasFile {
					rep.AwaitingDownload = append(rep.AwaitingDownload, it.ID)
					e.recordAttempt(ctx, it.ID, "", rep, opts)
				}
				continue
			}
			if opts.DryRun {
				rep.AcquiredMovies = append(rep.AcquiredMovies, fmt.Sprintf("%s (would add tmdb=%d)", it.ID, it.Provider.TMDB))
				continue
			}
			// Lookup by tmdb via title to get the full payload Radarr needs.
			term := it.Title
			if it.Year > 0 {
				term = fmt.Sprintf("%s %d", it.Title, it.Year)
			}
			hits, err := e.deps.Radarr.Lookup(ctx, term)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s lookup: %v", it.ID, err))
				e.recordAttempt(ctx, it.ID, err.Error(), rep, opts)
				continue
			}
			var movie *radarr.Movie
			for i := range hits {
				if hits[i].TmdbID == it.Provider.TMDB {
					movie = &hits[i]
					break
				}
			}
			if movie == nil {
				// Synthesize a minimal add payload.
				movie = &radarr.Movie{Title: it.Title, Year: it.Year, TmdbID: it.Provider.TMDB}
			}
			added, err := e.deps.Radarr.Add(ctx, *movie, e.deps.RadarrQualityProfileID, e.deps.RadarrRootFolder, []int{radarrTagID})
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s add: %v", it.ID, err))
				e.recordAttempt(ctx, it.ID, err.Error(), rep, opts)
				continue
			}
			rep.AcquiredMovies = append(rep.AcquiredMovies, fmt.Sprintf("%s → radarr id=%d", it.ID, added.ID))
			rep.AwaitingDownload = append(rep.AwaitingDownload, it.ID)
			e.recordAttempt(ctx, it.ID, "", rep, opts)

		case manifest.KindSeries:
			if it.Provider.TVDB == 0 {
				continue
			}
			if e.deps.Sonarr == nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: sonarr not configured", it.ID))
				continue
			}
			if existing := sonarr.ByTVDB(sonarrLib, it.Provider.TVDB); existing != nil {
				rep.AlreadyHeld = append(rep.AlreadyHeld, fmt.Sprintf("%s (sonarr id=%d)", it.ID, existing.ID))
				// Still apply exclude_episodes monitor toggles.
				if !opts.DryRun && len(it.ExcludeEpisodes) > 0 {
					if err := e.unmonitorExcluded(ctx, existing.ID, it, rep); err != nil {
						rep.Errors = append(rep.Errors, fmt.Sprintf("%s unmonitor: %v", it.ID, err))
					}
				}
				if existing.Statistics != nil && existing.Statistics.EpisodeFileCount < existing.Statistics.EpisodeCount {
					rep.AwaitingDownload = append(rep.AwaitingDownload, it.ID)
					e.recordAttempt(ctx, it.ID, "", rep, opts)
				}
				continue
			}
			if opts.DryRun {
				rep.AcquiredSeries = append(rep.AcquiredSeries, fmt.Sprintf("%s (would add tvdb=%d)", it.ID, it.Provider.TVDB))
				continue
			}
			term := it.Title
			if it.Year > 0 {
				term = fmt.Sprintf("%s %d", it.Title, it.Year)
			}
			hits, err := e.deps.Sonarr.Lookup(ctx, term)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s lookup: %v", it.ID, err))
				e.recordAttempt(ctx, it.ID, err.Error(), rep, opts)
				continue
			}
			var series *sonarr.Series
			for i := range hits {
				if hits[i].TvdbID == it.Provider.TVDB {
					series = &hits[i]
					break
				}
			}
			if series == nil {
				series = &sonarr.Series{Title: it.Title, Year: it.Year, TvdbID: it.Provider.TVDB}
			}
			added, err := e.deps.Sonarr.Add(ctx, *series, e.deps.SonarrQualityProfileID, e.deps.SonarrRootFolder, []int{sonarrTagID})
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s add: %v", it.ID, err))
				e.recordAttempt(ctx, it.ID, err.Error(), rep, opts)
				continue
			}
			rep.AcquiredSeries = append(rep.AcquiredSeries, fmt.Sprintf("%s → sonarr id=%d", it.ID, added.ID))
			rep.AwaitingDownload = append(rep.AwaitingDownload, it.ID)
			e.recordAttempt(ctx, it.ID, "", rep, opts)
			if len(it.ExcludeEpisodes) > 0 {
				if err := e.unmonitorExcluded(ctx, added.ID, it, rep); err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s unmonitor: %v", it.ID, err))
				}
			}

		case manifest.KindYouTubeChannel, manifest.KindYouTubePlaylist:
			if e.deps.YtDlp == nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: yt-dlp not configured", it.ID))
				continue
			}
			if e.deps.YtDlpOutputDir == "" {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: YTDLP_OUTPUT_DIR not set", it.ID))
				continue
			}
			if opts.DryRun {
				rep.AcquiredYouTube = append(rep.AcquiredYouTube, fmt.Sprintf("%s (would download %s)", it.ID, it.URL))
				continue
			}
			dlOpts := ytdlp.DownloadOpts{
				URL:         it.URL,
				Slug:        it.ID,
				OutputDir:   e.deps.YtDlpOutputDir,
				ArchivePath: e.deps.YtDlpArchivePath,
				Binary:      e.deps.YtDlpBinary,
			}
			// When filters name specific playlists, pass them as extra match hints.
			// Callers who want exact playlist URLs should put them in item.URL.
			if err := e.deps.YtDlp.Download(ctx, dlOpts); err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s yt-dlp: %v", it.ID, err))
				e.recordAttempt(ctx, it.ID, err.Error(), rep, opts)
				continue
			}
			rep.AcquiredYouTube = append(rep.AcquiredYouTube, it.ID)
			e.recordAttempt(ctx, it.ID, "", rep, opts)
		}
	}
	return nil
}

// manifestStatuses loads TV catalog status by slug. Missing TV client → empty map.
func (e *Engine) manifestStatuses(ctx context.Context, rep *Report) map[string]string {
	out := map[string]string{}
	if e.deps.TV == nil {
		return out
	}
	entries, err := e.deps.TV.ListManifestEntries(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("list manifest entries: %v", err))
		return out
	}
	for _, ent := range entries {
		out[ent.Slug] = ent.Status
		if ent.Status == tvclient.ManifestStatusFailed {
			// Surface failed rows even when we did not attempt this run.
			// acquire also appends; de-dupe in the report section is fine.
		}
	}
	return out
}

// recordAttempt tells the TV server one acquisition pass touched this slug.
func (e *Engine) recordAttempt(ctx context.Context, slug, lastError string, rep *Report, opts Options) {
	if opts.DryRun || e.deps.TV == nil {
		return
	}
	entry, err := e.deps.TV.RecordManifestAttempt(ctx, slug, lastError)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s attempt: %v", slug, err))
		return
	}
	rep.AttemptsRecorded = append(rep.AttemptsRecorded, fmt.Sprintf("%s attempt=%d status=%s", slug, entry.AttemptCount, entry.Status))
	if entry.Status == tvclient.ManifestStatusFailed {
		rep.FailedQueue = append(rep.FailedQueue, fmt.Sprintf("%s (failed after %d attempts)", slug, entry.AttemptCount))
	}
}

// markPresent tells the TV server the slug is available in Jellyfin.
func (e *Engine) markPresent(ctx context.Context, slug string, rep *Report, opts Options) {
	if opts.DryRun || e.deps.TV == nil {
		return
	}
	if _, err := e.deps.TV.MarkManifestPresent(ctx, slug); err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s present: %v", slug, err))
		return
	}
	rep.MarkedPresent = append(rep.MarkedPresent, slug)
}

func (e *Engine) unmonitorExcluded(ctx context.Context, seriesID int, it manifest.Item, rep *Report) error {
	eps, err := e.deps.Sonarr.Episodes(ctx, seriesID)
	if err != nil {
		return err
	}
	for _, ep := range eps {
		if !it.Excluded(ep.Key()) {
			continue
		}
		if !ep.Monitored {
			continue
		}
		if err := e.deps.Sonarr.SetEpisodeMonitored(ctx, ep.ID, false); err != nil {
			return err
		}
		rep.SkippedExcluded = append(rep.SkippedExcluded, fmt.Sprintf("%s %s unmonitored", it.ID, ep.Key()))
	}
	return nil
}

// sync triggers Jellyfin library refresh, waits, then TV jellyfin/sync.
func (e *Engine) sync(ctx context.Context, rep *Report) error {
	if e.deps.Jellyfin != nil {
		if err := e.deps.Jellyfin.RefreshLibrary(ctx); err != nil {
			return fmt.Errorf("jellyfin refresh: %w", err)
		}
		deadline := time.Now().Add(e.deps.SyncWait)
		for time.Now().Before(deadline) {
			running, err := e.deps.Jellyfin.ScanRunning(ctx)
			if err != nil {
				e.log.Warn("scan status check failed", "error", err)
				break
			}
			if !running {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(e.deps.SyncPollInterval):
			}
		}
		rep.JellyfinRefreshed = true
	}
	if e.deps.TV != nil {
		if _, err := e.deps.TV.SyncJellyfin(ctx); err != nil {
			return fmt.Errorf("tv jellyfin sync: %w", err)
		}
		rep.TVSynced = true
	}
	return nil
}

// importItems matches Jellyfin items to the manifest and upserts TV media_items.
func (e *Engine) importItems(ctx context.Context, m *manifest.Manifest, rep *Report, opts Options) error {
	if e.deps.Jellyfin == nil || e.deps.TV == nil {
		if e.deps.Jellyfin == nil {
			e.log.Info("import skipped: jellyfin not configured")
		}
		if e.deps.TV == nil {
			e.log.Info("import skipped: tv client not configured")
		}
		return nil
	}

	existing, err := e.deps.TV.ListMediaItems(ctx)
	if err != nil {
		return fmt.Errorf("list media items: %w", err)
	}
	byJF := tvclient.ByJellyfinID(existing)

	for _, it := range m.SortedByPriority() {
		jfItems, err := e.findJellyfinItems(ctx, it)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s jellyfin: %v", it.ID, err))
			continue
		}
		if len(jfItems) == 0 {
			switch it.Kind {
			case manifest.KindManual:
				// Manual items are expected to be absent until ripped.
			case manifest.KindMovie, manifest.KindSeries, manifest.KindYouTubeChannel, manifest.KindYouTubePlaylist:
				if !it.NeedsResolve() || it.Kind == manifest.KindYouTubeChannel || it.Kind == manifest.KindYouTubePlaylist {
					rep.NotInJellyfin = append(rep.NotInJellyfin, it.ID)
				}
			}
			continue
		}

		// At least one Jellyfin hit means the title is present on disk.
		e.markPresent(ctx, it.ID, rep, opts)

		imported := 0
		for _, jf := range jfItems {
			if jf.Type == "Episode" {
				key := jf.EpisodeKey()
				if key != "" && it.Excluded(key) {
					rep.SkippedExcluded = append(rep.SkippedExcluded, fmt.Sprintf("%s %s", it.ID, key))
					continue
				}
			}
			if it.MaxEpisodes > 0 && jf.Type == "Episode" && imported >= it.MaxEpisodes {
				continue
			}

			title := jf.Name
			if jf.Type == "Episode" && jf.SeriesName != "" {
				title = fmt.Sprintf("%s — %s", jf.SeriesName, jf.Name)
				if key := jf.EpisodeKey(); key != "" {
					title = fmt.Sprintf("%s %s — %s", jf.SeriesName, key, jf.Name)
				}
			}

			if existingItem, ok := byJF[jf.ID]; ok {
				if tvclient.ClassificationChanged(existingItem, it.Class, it.SubjectTags, it.StandardCodes) {
					if opts.DryRun {
						rep.Updated = append(rep.Updated, fmt.Sprintf("%s (%s) would update class", it.ID, jf.ID))
						continue
					}
					class := it.Class
					tags := append([]string{}, it.SubjectTags...)
					codes := append([]string{}, it.StandardCodes...)
					_, err := e.deps.TV.UpdateMediaItem(ctx, existingItem.ID, tvclient.MediaItemUpdate{
						Class: &class, SubjectTags: &tags, StandardCodes: &codes,
					})
					if err != nil {
						rep.Errors = append(rep.Errors, fmt.Sprintf("%s update %s: %v", it.ID, jf.ID, err))
						continue
					}
					rep.Updated = append(rep.Updated, fmt.Sprintf("%s (%s)", it.ID, title))
				}
				if jf.Type == "Episode" {
					imported++
				}
				continue
			}

			if opts.DryRun {
				rep.Imported = append(rep.Imported, fmt.Sprintf("%s would import %s (%s)", it.ID, title, jf.ID))
				if jf.Type == "Episode" {
					imported++
				}
				continue
			}
			created, err := e.deps.TV.CreateMediaItem(ctx, tvclient.MediaItemCreate{
				JellyfinItemID: jf.ID,
				Title:          title,
				SortTitle:      jf.SortName,
				Overview:       jf.Overview,
				Class:          it.Class,
				RuntimeSeconds: jf.RuntimeSeconds(),
				SubjectTags:    it.SubjectTags,
				StandardCodes:  it.StandardCodes,
				Container:      jf.Container,
				VideoCodec:     jf.VideoCodec,
				AudioCodec:     jf.AudioCodec,
				ImageTag:       jf.ImageTag,
			})
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s import %s: %v", it.ID, jf.ID, err))
				continue
			}
			byJF[jf.ID] = *created
			rep.Imported = append(rep.Imported, fmt.Sprintf("%s → %s (%s)", it.ID, created.ID, title))
			if jf.Type == "Episode" {
				imported++
			}
		}
	}
	return nil
}

// findJellyfinItems locates Jellyfin library entries for a manifest item.
// Matching is exact and client-verified:
//   - movies: only items whose ProviderIds.Tmdb equals the manifest TMDB id
//   - series: find the Series row by TVDB/TMDB, then list its episodes by SeriesId
//   - youtube: path prefix Shows/<manifest-id>/
//
// Jellyfin's AnyProviderIdEquals query is treated as a hint only; every hit is
// re-checked against ProviderIds before it is returned. A server that ignores
// the filter therefore cannot leak unrelated library items into the import.
func (e *Engine) findJellyfinItems(ctx context.Context, it manifest.Item) ([]jellyfin.Item, error) {
	switch it.Kind {
	case manifest.KindMovie:
		if it.Provider.TMDB == 0 {
			return nil, nil
		}
		expr := "Tmdb=" + strconv.Itoa(it.Provider.TMDB)
		hits, err := e.browseAll(ctx, jellyfin.BrowseParams{
			AnyProviderIDEquals: expr,
			IncludeProviderIDs:  true,
			IncludeItemTypes:    "Movie",
			// Prefer a title search as a server-side narrow; still verified below.
			SearchTerm: it.Title,
		})
		if err != nil {
			return nil, err
		}
		return filterByProvider(hits, expr), nil

	case manifest.KindSeries:
		if it.Provider.TVDB == 0 && it.Provider.TMDB == 0 {
			return nil, nil
		}
		series, err := e.findSeries(ctx, it)
		if err != nil {
			return nil, err
		}
		if series == nil {
			return nil, nil
		}
		// Episodes of this series only — never a library-wide provider scan.
		eps, err := e.browseAll(ctx, jellyfin.BrowseParams{
			SeriesID:         series.ID,
			IncludeItemTypes: "Episode",
			IncludePath:      true,
		})
		if err != nil {
			return nil, err
		}
		return eps, nil

	case manifest.KindYouTubeChannel, manifest.KindYouTubePlaylist:
		// Path-only match. Do not pass SearchTerm: yt-dlp episode titles rarely
		// contain the channel name, and a title filter would drop real hits.
		prefix := ytdlp.PathPrefix(it.ID)
		hits, err := e.browseAll(ctx, jellyfin.BrowseParams{
			PathContains:     prefix,
			IncludePath:      true,
			IncludeItemTypes: "Movie,Episode,Video",
		})
		if err != nil {
			return nil, err
		}
		out := make([]jellyfin.Item, 0, len(hits))
		for _, h := range hits {
			if strings.Contains(h.Path, prefix) {
				out = append(out, h)
			}
		}
		return out, nil

	default:
		return nil, nil
	}
}

// findSeries returns the single Jellyfin Series whose provider ids match the
// manifest item, or nil if none. Multiple matches are an error so a human can
// disambiguate rather than importing the wrong show's episodes.
func (e *Engine) findSeries(ctx context.Context, it manifest.Item) (*jellyfin.Item, error) {
	var expr parts
	if it.Provider.TVDB != 0 {
		expr = append(expr, "Tvdb="+strconv.Itoa(it.Provider.TVDB))
	}
	if it.Provider.TMDB != 0 {
		expr = append(expr, "Tmdb="+strconv.Itoa(it.Provider.TMDB))
	}
	providerExpr := expr.Join("|")

	// Prefer Series-type rows with the provider id. SearchTerm narrows the page
	// Jellyfin returns; client-side ProviderMatch is authoritative.
	hits, err := e.browseAll(ctx, jellyfin.BrowseParams{
		AnyProviderIDEquals: providerExpr,
		IncludeProviderIDs:  true,
		IncludeItemTypes:    "Series",
		SearchTerm:          it.Title,
	})
	if err != nil {
		return nil, err
	}
	hits = filterByProvider(hits, providerExpr)
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &hits[0], nil
	default:
		names := make([]string, 0, len(hits))
		for _, h := range hits {
			names = append(names, h.Name+"("+h.ID+")")
		}
		return nil, fmt.Errorf("ambiguous series match for %s (%s): %s",
			it.ID, providerExpr, strings.Join(names, ", "))
	}
}

// browseAll pages through Jellyfin Browse until a short page is returned.
// Limit is applied per page; provider/path filters are applied by Browse itself.
func (e *Engine) browseAll(ctx context.Context, p jellyfin.BrowseParams) ([]jellyfin.Item, error) {
	const pageSize = 200
	if p.Limit <= 0 || p.Limit > pageSize {
		p.Limit = pageSize
	}
	var all []jellyfin.Item
	for start := 0; ; start += p.Limit {
		page := p
		page.StartIndex = start
		items, err := e.deps.Jellyfin.Browse(ctx, page)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < p.Limit {
			break
		}
		// Hard ceiling so a broken filter cannot pull the whole library forever.
		if len(all) >= 5000 {
			break
		}
	}
	return all, nil
}

// filterByProvider keeps only items whose ProviderIds satisfy expr.
func filterByProvider(items []jellyfin.Item, expr string) []jellyfin.Item {
	if expr == "" {
		return items
	}
	out := make([]jellyfin.Item, 0, len(items))
	for _, it := range items {
		if jellyfin.ProviderMatch(it, expr) {
			out = append(out, it)
		}
	}
	return out
}

type parts []string

func (p parts) Join(sep string) string { return strings.Join(p, sep) }

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
