package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

const (
	StatusRecovered = "recovered"
	StatusExisting  = "existing"
	StatusSkipped   = "skipped"

	ReasonMissingURL      = "missing youtube url"
	ReasonInvalidURL      = "invalid youtube url"
	ReasonInvalidID       = "invalid youtube id"
	ReasonAmbiguousID     = "ambiguous youtube id"
	ReasonMissingTitle    = "missing title"
	ReasonInvalidDate     = "invalid date"
	ReasonMissingDuration = "missing duration"
	ReasonShort           = "short duration"
	ReasonExcluded        = "excluded by manifest"
	ReasonDuplicateID     = "duplicate youtube id"
	ReasonConflict        = "conflicting destination"
	ReasonAlready         = "already recovered"
	ReasonProbeFailed     = "probe failed"
	ReasonEmptyMedia      = "empty media"
	ReasonNotFinalized    = "not finalized"
	ReasonHardlink        = "hardlink failed"
	ReasonQuarantined     = "quarantined wrong source"

	maxPlotRunes = 4000

	quarantinedJustinRhodes = "justin-rhodes"
)

// quarantinedSlugs are never recovered. justin-rhodes on disk is the wrong
// musician (cover songs), not the allowlisted homesteading channel. Refusal
// stays hard until an operator deliberately changes this source list.
var quarantinedSlugs = map[string]string{
	quarantinedJustinRhodes: "justin-rhodes is quarantined: on-disk files are the wrong musician source, not the allowlisted homesteading channel; refuse recovery until the operator deliberately changes source",
}

// Options configures a recovery run. Dry-run is the default (Apply=false).
//
// SourceRoot is the parent of slug directories (e.g. /mnt/moosefs/media/Shows).
// OutputDir is the Primer library root; FinalizeStaging writes Shows/<slug>/...
type Options struct {
	SourceRoot   string
	OutputDir    string
	ManifestPath string
	Slugs        []string
	Apply        bool
	Now          time.Time
	Probe        Prober
	FFProbe      string
}

// Counts is the recovered/existing/skip tally.
type Counts struct {
	Recovered int `json:"recovered"`
	Existing  int `json:"existing"`
	Skipped   int `json:"skipped"`
}

// Show is allowlisted title + class from the manifest.
type Show struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Class string `json:"class"`
}

// Item is one source file's outcome. Descriptions are never included.
type Item struct {
	Slug      string `json:"slug"`
	Source    string `json:"source"`
	YouTubeID string `json:"youtube_id,omitempty"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	FinalPath string `json:"final_path,omitempty"`
}

// Result is JSON evidence for a recovery (or dry-run) pass.
type Result struct {
	DryRun     bool     `json:"dry_run"`
	SourceRoot string   `json:"source_root"`
	OutputDir  string   `json:"output_dir"`
	Slugs      []string `json:"slugs"`
	Shows      []Show   `json:"shows"`
	Counts     Counts   `json:"counts"`
	Items      []Item   `json:"items"`
}

type candidate struct {
	item      Item
	absSource string
	thumb     string
	ext       string
	format    ProbeFormat
}

type stagingInfo struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Channel     string  `json:"channel,omitempty"`
	Uploader    string  `json:"uploader,omitempty"`
	UploadDate  string  `json:"upload_date"`
	Duration    float64 `json:"duration"`
	WasLive     bool    `json:"was_live"`
	IsLive      bool    `json:"is_live"`
	LiveStatus  string  `json:"live_status"`
}

// Run classifies legacy Season 01 media and optionally hardlinks into
// yt-dlp staging then calls ytdlp.FinalizeStaging once per slug.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	probe := opts.Probe
	if probe == nil {
		probe = DefaultProbe(opts.FFProbe)
	}

	m, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return nil, err
	}

	shows := make([]Show, 0, len(opts.Slugs))
	itemsBySlug := make([]*manifest.Item, 0, len(opts.Slugs))
	for _, slug := range opts.Slugs {
		if msg, ok := quarantineMessage(slug); ok {
			return nil, fmt.Errorf("recovery: %s (%s)", ReasonQuarantined, msg)
		}
		it := m.ByID(slug)
		if it == nil {
			return nil, fmt.Errorf("recovery: slug %q is not in the manifest", slug)
		}
		if it.Kind != manifest.KindYouTubeChannel && it.Kind != manifest.KindYouTubePlaylist {
			return nil, fmt.Errorf("recovery: slug %q kind %q is not a youtube source", slug, it.Kind)
		}
		shows = append(shows, Show{Slug: it.ID, Title: it.Title, Class: it.Class})
		itemsBySlug = append(itemsBySlug, it)
	}

	res := &Result{
		DryRun:     !opts.Apply,
		SourceRoot: opts.SourceRoot,
		OutputDir:  opts.OutputDir,
		Slugs:      append([]string(nil), opts.Slugs...),
		Shows:      shows,
	}

	for _, it := range itemsBySlug {
		items, err := processSlug(ctx, opts, probe, *it)
		if err != nil {
			return nil, err
		}
		res.Items = append(res.Items, items...)
	}

	sort.SliceStable(res.Items, func(i, j int) bool {
		if res.Items[i].Slug != res.Items[j].Slug {
			return res.Items[i].Slug < res.Items[j].Slug
		}
		return res.Items[i].Source < res.Items[j].Source
	})
	for _, it := range res.Items {
		switch it.Status {
		case StatusRecovered:
			res.Counts.Recovered++
		case StatusExisting:
			res.Counts.Existing++
		default:
			res.Counts.Skipped++
		}
	}
	return res, nil
}

func (o Options) validate() error {
	if strings.TrimSpace(o.SourceRoot) == "" {
		return fmt.Errorf("recovery: --source-root is required")
	}
	if strings.TrimSpace(o.OutputDir) == "" {
		return fmt.Errorf("recovery: --output-dir is required")
	}
	if strings.TrimSpace(o.ManifestPath) == "" {
		return fmt.Errorf("recovery: --manifest is required")
	}
	if len(o.Slugs) == 0 {
		return fmt.Errorf("recovery: at least one --slug or --slugs is required")
	}
	st, err := os.Stat(o.SourceRoot)
	if err != nil {
		return fmt.Errorf("recovery: source-root: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("recovery: source-root is not a directory")
	}
	if err := refuseOverlap(o.SourceRoot, o.OutputDir); err != nil {
		return err
	}
	for _, slug := range o.Slugs {
		if err := validateSlug(slug); err != nil {
			return err
		}
	}
	return nil
}

func validateSlug(slug string) error {
	if slug == "" || slug != filepath.Base(slug) || slug == "." || slug == ".." {
		return fmt.Errorf("recovery: invalid slug %q", slug)
	}
	if strings.ContainsAny(slug, `/\`) {
		return fmt.Errorf("recovery: invalid slug %q", slug)
	}
	return nil
}

func quarantineMessage(slug string) (string, bool) {
	if msg, ok := quarantinedSlugs[strings.ToLower(slug)]; ok {
		return msg, true
	}
	return "", false
}

func refuseOverlap(sourceRoot, outputDir string) error {
	src, err := absClean(sourceRoot)
	if err != nil {
		return fmt.Errorf("recovery: source-root: %w", err)
	}
	out, err := absClean(outputDir)
	if err != nil {
		return fmt.Errorf("recovery: output-dir: %w", err)
	}
	outShows := filepath.Join(out, "Shows")
	if pathsOverlap(src, out) || pathsOverlap(src, outShows) {
		return fmt.Errorf("recovery: source-root and output-dir overlap; refuse to write into the legacy tree")
	}
	return nil
}

func absClean(p string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(a), nil
}

func pathsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}

func processSlug(ctx context.Context, opts Options, probe Prober, it manifest.Item) ([]Item, error) {
	slug := it.ID
	srcSeason := filepath.Join(opts.SourceRoot, slug, "Season 01")
	dstSeason := ytdlp.SeasonDir(opts.OutputDir, slug)

	entries, err := os.ReadDir(srcSeason)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("recovery: read %s: %w", srcSeason, err)
	}

	destIDs, err := scanDestIDs(dstSeason)
	if err != nil {
		return nil, err
	}
	minDur := manifest.EffectiveMinDuration(it.Filters)

	var cands []candidate
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !isMedia(e.Name()) {
			continue
		}
		abs := filepath.Join(srcSeason, e.Name())
		rel := filepath.ToSlash(filepath.Join(slug, "Season 01", e.Name()))
		info, statErr := e.Info()
		if statErr != nil {
			cands = append(cands, skipCand(slug, rel, abs, "", ReasonProbeFailed))
			continue
		}
		if info.Size() == 0 {
			cands = append(cands, skipCand(slug, rel, abs, "", ReasonEmptyMedia))
			continue
		}
		if id, ok := ytdlp.ParseYouTubeID(e.Name()); ok {
			item := Item{Slug: slug, Source: rel, YouTubeID: id, Status: StatusExisting, Reason: ReasonAlready}
			if p := destIDs[id]; p != "" {
				item.FinalPath = p
			}
			cands = append(cands, candidate{item: item, absSource: abs})
			continue
		}

		format, perr := probe(ctx, abs)
		if perr != nil {
			cands = append(cands, skipCand(slug, rel, abs, "", ReasonProbeFailed))
			continue
		}
		id, reason := YouTubeIDFromComment(format.Comment)
		if reason != "" {
			cands = append(cands, skipCand(slug, rel, abs, id, reason))
			continue
		}
		if strings.TrimSpace(format.Title) == "" {
			cands = append(cands, skipCand(slug, rel, abs, id, ReasonMissingTitle))
			continue
		}
		if !validUploadDate(strings.TrimSpace(format.Date)) {
			cands = append(cands, skipCand(slug, rel, abs, id, ReasonInvalidDate))
			continue
		}
		if format.Duration <= 0 {
			cands = append(cands, skipCand(slug, rel, abs, id, ReasonMissingDuration))
			continue
		}
		if minDur >= 0 && format.Duration <= float64(minDur) {
			cands = append(cands, skipCand(slug, rel, abs, id, ReasonShort))
			continue
		}
		if manifest.ExcludedVideo(it, id, "") {
			cands = append(cands, skipCand(slug, rel, abs, id, ReasonExcluded))
			continue
		}
		item := Item{Slug: slug, Source: rel, YouTubeID: id, Status: StatusRecovered}
		if destPath, ok := destIDs[id]; ok {
			item.Status = StatusExisting
			item.Reason = ReasonAlready
			item.FinalPath = destPath
		}
		thumb := siblingThumb(abs)
		cands = append(cands, candidate{
			item:      item,
			absSource: abs,
			thumb:     thumb,
			ext:       filepath.Ext(e.Name()),
			format:    format,
		})
	}

	markDuplicateIDs(cands)

	if !opts.Apply {
		return itemsOf(cands), nil
	}

	var toStage []candidate
	ids := map[string]bool{}
	for i := range cands {
		if cands[i].item.Status == StatusRecovered {
			toStage = append(toStage, cands[i])
			ids[cands[i].item.YouTubeID] = true
		}
	}
	if len(toStage) == 0 {
		return itemsOf(cands), nil
	}

	staging := ytdlp.StagingDir(opts.OutputDir, slug)
	if err := rejectForeignStaging(staging, ids); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, fmt.Errorf("recovery: staging dir: %w", err)
	}

	var staged int
	for _, c := range toStage {
		if err := stageCandidate(staging, c); err != nil {
			for j := range cands {
				if cands[j].absSource == c.absSource {
					cands[j].item.Status = StatusSkipped
					cands[j].item.Reason = ReasonHardlink
					var linkReason hardlinkError
					if errors.As(err, &linkReason) {
						cands[j].item.Reason = linkReason.reason
					}
					break
				}
			}
			continue
		}
		staged++
	}
	if staged == 0 {
		return itemsOf(cands), nil
	}

	finalized, err := ytdlp.FinalizeStaging(ytdlp.FinalizeOpts{
		OutputDir:   opts.OutputDir,
		Slug:        slug,
		ShowTitle:   it.Title,
		Now:         opts.Now,
		MinDuration: minDur,
	})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]string, len(finalized))
	for _, ep := range finalized {
		byID[ep.ID] = ep.Path
	}
	for i := range cands {
		if cands[i].item.Status != StatusRecovered {
			continue
		}
		p, ok := byID[cands[i].item.YouTubeID]
		if !ok {
			cands[i].item.Status = StatusSkipped
			cands[i].item.Reason = ReasonNotFinalized
			continue
		}
		cands[i].item.FinalPath = p
		cands[i].item.Reason = ""
	}
	return itemsOf(cands), nil
}

type hardlinkError struct {
	reason string
	err    error
}

func (e hardlinkError) Error() string {
	if e.err != nil {
		return e.reason + ": " + e.err.Error()
	}
	return e.reason
}

func (e hardlinkError) Unwrap() error { return e.err }

func stageCandidate(staging string, c candidate) error {
	id := c.item.YouTubeID
	mediaDst := filepath.Join(staging, id+c.ext)
	if err := hardlinkExclusive(c.absSource, mediaDst); err != nil {
		return err
	}
	info := stagingInfo{
		ID:          id,
		Title:       strings.TrimSpace(c.format.Title),
		Description: truncateRunes(c.format.Description, maxPlotRunes),
		Channel:     strings.TrimSpace(c.format.Artist),
		Uploader:    strings.TrimSpace(c.format.Artist),
		UploadDate:  strings.TrimSpace(c.format.Date),
		Duration:    c.format.Duration,
		WasLive:     false,
		IsLive:      false,
		LiveStatus:  "not_live",
	}
	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("recovery: marshal info.json: %w", err)
	}
	raw = append(raw, '\n')
	infoPath := filepath.Join(staging, id+".info.json")
	if err := os.WriteFile(infoPath, raw, 0o644); err != nil {
		return fmt.Errorf("recovery: write info.json: %w", err)
	}
	if c.thumb != "" {
		thumbDst := filepath.Join(staging, id+".jpg")
		_ = hardlinkExclusive(c.thumb, thumbDst) // optional
	}
	return nil
}

func hardlinkExclusive(src, dst string) error {
	if st, err := os.Lstat(dst); err == nil {
		if st.IsDir() {
			return hardlinkError{reason: ReasonConflict}
		}
		same, err := sameFile(src, dst)
		if err != nil {
			return hardlinkError{reason: ReasonHardlink, err: err}
		}
		if !same {
			return hardlinkError{reason: ReasonConflict}
		}
		return nil
	} else if !os.IsNotExist(err) {
		return hardlinkError{reason: ReasonHardlink, err: err}
	}
	if err := os.Link(src, dst); err != nil {
		return hardlinkError{reason: ReasonHardlink, err: err}
	}
	return nil
}

func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(sa, sb), nil
}

func scanDestIDs(seasonDir string) (map[string]string, error) {
	out := map[string]string{}
	entries, err := os.ReadDir(seasonDir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("recovery: read dest %s: %w", seasonDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !isMedia(e.Name()) {
			continue
		}
		id, ok := ytdlp.ParseYouTubeID(e.Name())
		if !ok {
			continue
		}
		out[id] = filepath.Join(seasonDir, e.Name())
	}
	return out, nil
}

func markDuplicateIDs(cands []candidate) {
	byID := map[string][]int{}
	for i, c := range cands {
		if c.item.YouTubeID == "" || c.item.Status == StatusSkipped {
			continue
		}
		byID[c.item.YouTubeID] = append(byID[c.item.YouTubeID], i)
	}
	for _, idxs := range byID {
		if len(idxs) < 2 {
			continue
		}
		for _, i := range idxs {
			if cands[i].item.Status == StatusExisting {
				continue
			}
			cands[i].item.Status = StatusSkipped
			cands[i].item.Reason = ReasonDuplicateID
			cands[i].item.FinalPath = ""
		}
	}
}

func skipCand(slug, rel, abs, id, reason string) candidate {
	return candidate{
		item: Item{
			Slug:      slug,
			Source:    rel,
			YouTubeID: id,
			Status:    StatusSkipped,
			Reason:    reason,
		},
		absSource: abs,
	}
}

func itemsOf(cands []candidate) []Item {
	out := make([]Item, len(cands))
	for i, c := range cands {
		out[i] = c.item
	}
	return out
}

func isMedia(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".webm":
		return true
	default:
		return false
	}
}

func siblingThumb(mediaPath string) string {
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	for _, ext := range []string{".jpg", ".jpeg"} {
		p := base + ext
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
			return p
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// NormalizeSlugs splits comma lists and repeated flags, dropping blanks and duplicates.
func NormalizeSlugs(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func rejectForeignStaging(staging string, ids map[string]bool) error {
	entries, err := os.ReadDir(staging)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("recovery: read staging: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id := stagingStem(e.Name())
		if id == "" || !ids[id] {
			return fmt.Errorf("recovery: staging contains unexpected %s; refuse to mix with FinalizeStaging", e.Name())
		}
	}
	return nil
}

func stagingStem(name string) string {
	if strings.HasSuffix(name, ".info.json") {
		return strings.TrimSuffix(name, ".info.json")
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}
