package ytdlp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FinalizeOpts configures FinalizeStaging.
type FinalizeOpts struct {
	OutputDir string
	Slug      string
	ShowTitle string
	ChannelID string
	// ArchivePath optionally overrides the default per-show archive location.
	ArchivePath   string
	Now           time.Time
	MinDuration   int // seconds; 0 => DefaultMinDurationSeconds; negative disables
	AllowPastLive bool
}

// FinalizedEpisode is one successfully renamed staging item.
type FinalizedEpisode struct {
	ID      string
	Season  int
	Episode int
	Title   string
	Path    string // final .mkv path
}

// infoJSON is the subset of yt-dlp info.json we consume.
type infoJSON struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Channel      string  `json:"channel"`
	ChannelID    string  `json:"channel_id"`
	Uploader     string  `json:"uploader"`
	UploadDate   string  `json:"upload_date"`
	Duration     float64 `json:"duration"`
	WasLive      bool    `json:"was_live"`
	IsLive       bool    `json:"is_live"`
	LiveStatus   string  `json:"live_status"`
	Availability string  `json:"availability"`
}

// FinalizeStaging walks Shows/<slug>/Season 01/_staging for *.info.json,
// validates, ledger-assigns, writes NFO, renames sidecars to final S01E{nnn}
// names, writes show NFO, and appends youtube {id} to the per-show archive
// only after rename succeeds.
func FinalizeStaging(opts FinalizeOpts) ([]FinalizedEpisode, error) {
	if opts.OutputDir == "" || opts.Slug == "" {
		return nil, fmt.Errorf("ytdlp: finalize: output dir and slug required")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	minDur := opts.MinDuration
	if minDur == 0 {
		minDur = DefaultMinDurationSeconds
	}
	staging := StagingDir(opts.OutputDir, opts.Slug)
	entries, err := os.ReadDir(staging)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ytdlp: finalize read staging: %w", err)
	}

	showTitle := opts.ShowTitle
	if showTitle == "" {
		showTitle = opts.Slug
	}
	channelID := opts.ChannelID

	// Collect valid info.json first so first-ingest can batch-sort.
	type item struct {
		infoPath string
		base     string // path without .info.json
		info     infoJSON
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".info.json") {
			continue
		}
		infoPath := filepath.Join(staging, name)
		raw, err := os.ReadFile(infoPath)
		if err != nil {
			continue
		}
		var info infoJSON
		if err := json.Unmarshal(raw, &info); err != nil {
			continue
		}
		if rejectInfo(info, minDur, opts.AllowPastLive) {
			continue
		}
		base := strings.TrimSuffix(infoPath, ".info.json")
		// Require a playable mkv (or any media) beside info.json.
		if !stagingHasMedia(base) {
			continue
		}
		items = append(items, item{infoPath: infoPath, base: base, info: info})
		if channelID == "" && info.ChannelID != "" {
			channelID = info.ChannelID
		}
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Batch assign for stable first-ingest ordering.
	cands := make([]EpisodeCandidate, 0, len(items))
	for _, it := range items {
		cands = append(cands, EpisodeCandidate{
			ID:         it.info.ID,
			Title:      it.info.Title,
			UploadDate: it.info.UploadDate,
		})
	}
	led, err := AssignEpisodes(opts.OutputDir, opts.Slug, showTitle, channelID, cands, opts.Now)
	if err != nil {
		return nil, err
	}
	if channelID == "" {
		channelID = led.ChannelID
	}

	seasonDir := SeasonDir(opts.OutputDir, opts.Slug)
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		return nil, fmt.Errorf("ytdlp: finalize season dir: %w", err)
	}

	var out []FinalizedEpisode
	for _, it := range items {
		ep, ok := led.Episodes[it.info.ID]
		if !ok {
			continue
		}
		finalBase := FinalBasename(opts.Slug, ep.Season, ep.Episode, sanitizeTitle(it.info.Title), it.info.ID)
		finalMKV, err := renameStagingBundle(it.base, filepath.Join(seasonDir, finalBase))
		if err != nil {
			return out, err
		}
		// Episode NFO next to final media.
		_, err = WriteEpisodeNFO(finalMKV, EpisodeNFO{
			Title:      it.info.Title,
			ShowTitle:  showTitle,
			Season:     ep.Season,
			Episode:    ep.Episode,
			Plot:       it.info.Description,
			UploadDate: it.info.UploadDate,
			RuntimeSec: int(it.info.Duration),
			YouTubeID:  it.info.ID,
			Slug:       opts.Slug,
		})
		if err != nil {
			return out, err
		}
		archivePath := opts.ArchivePath
		if archivePath == "" {
			archivePath = PerShowArchivePath(opts.OutputDir, opts.Slug)
		}
		if err := appendArchive(archivePath, it.info.ID); err != nil {
			return out, err
		}
		out = append(out, FinalizedEpisode{
			ID:      it.info.ID,
			Season:  ep.Season,
			Episode: ep.Episode,
			Title:   it.info.Title,
			Path:    finalMKV,
		})
	}

	if _, err := WriteShowNFO(opts.OutputDir, opts.Slug, showTitle, channelID); err != nil {
		return out, err
	}
	return out, nil
}

func rejectInfo(info infoJSON, minDur int, allowPastLive bool) bool {
	if info.ID == "" || !youtubeIDRe.MatchString(info.ID) {
		return true
	}
	if info.IsLive {
		return true
	}
	if info.WasLive && !allowPastLive {
		return true
	}
	switch info.LiveStatus {
	case "is_live", "is_upcoming":
		return true
	case "post_live":
		if !allowPastLive {
			return true
		}
	}
	// duration missing (0) with minDur set: still allow if yt-dlp wrote a file;
	// match-filter usually prevents this. Reject only when duration known and short.
	if minDur >= 0 && info.Duration > 0 && info.Duration <= float64(minDur) {
		return true
	}
	return false
}

func stagingHasMedia(base string) bool {
	for _, ext := range []string{".mkv", ".mp4", ".webm"} {
		if st, err := os.Stat(base + ext); err == nil && st.Size() > 0 {
			return true
		}
	}
	return false
}

func renameStagingBundle(stagingBase, finalBase string) (string, error) {
	// Prefer mkv as the media extension.
	mediaExt := ""
	for _, ext := range []string{".mkv", ".mp4", ".webm"} {
		if _, err := os.Stat(stagingBase + ext); err == nil {
			mediaExt = ext
			break
		}
	}
	if mediaExt == "" {
		return "", fmt.Errorf("ytdlp: finalize: no media for %s", stagingBase)
	}
	// Sidecars to move when present.
	type pair struct{ from, to string }
	var moves []pair
	moves = append(moves, pair{stagingBase + mediaExt, finalBase + mediaExt})
	for _, ext := range []string{".info.json", ".jpg", ".jpeg", ".webp", ".png", ".nfo"} {
		from := stagingBase + ext
		if _, err := os.Stat(from); err == nil {
			toExt := ext
			if ext == ".jpeg" {
				toExt = ".jpg"
			}
			moves = append(moves, pair{from, finalBase + toExt})
		}
	}
	for _, m := range moves {
		if err := os.MkdirAll(filepath.Dir(m.to), 0o755); err != nil {
			return "", fmt.Errorf("ytdlp: finalize mkdir: %w", err)
		}
		// Atomic-ish: remove destination if present then rename.
		_ = os.Remove(m.to)
		if err := os.Rename(m.from, m.to); err != nil {
			// Cross-device fallback
			data, rerr := os.ReadFile(m.from)
			if rerr != nil {
				return "", fmt.Errorf("ytdlp: finalize rename %s: %w", m.from, err)
			}
			if werr := os.WriteFile(m.to, data, 0o644); werr != nil {
				return "", fmt.Errorf("ytdlp: finalize copy %s: %w", m.to, werr)
			}
			_ = os.Remove(m.from)
		}
	}
	return finalBase + mediaExt, nil
}

func appendArchive(path, id string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ytdlp: archive dir: %w", err)
	}
	// Avoid duplicate lines.
	if existing, err := os.ReadFile(path); err == nil {
		line := "youtube " + id
		for _, l := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(l) == line {
				return nil
			}
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ytdlp: open archive: %w", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "youtube %s\n", id); err != nil {
		return fmt.Errorf("ytdlp: append archive: %w", err)
	}
	return nil
}

func sanitizeTitle(title string) string {
	// Keep filesystem-safe; strip path separators and NULs.
	title = strings.ReplaceAll(title, "/", "-")
	title = strings.ReplaceAll(title, "\\", "-")
	title = strings.ReplaceAll(title, "\x00", "")
	title = strings.TrimSpace(title)
	if title == "" {
		return "untitled"
	}
	// Bound length similar to yt-dlp .80B
	runes := []rune(title)
	if len(runes) > 80 {
		title = string(runes[:80])
	}
	return title
}
