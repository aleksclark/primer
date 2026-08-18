package ytdlp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Ledger is the durable per-show episode assignment (.primer-index.json).
type Ledger struct {
	Version     int                      `json:"version"`
	Slug        string                   `json:"slug"`
	Title       string                   `json:"title"`
	ChannelID   string                   `json:"channel_id"`
	Numbering   string                   `json:"numbering"`
	Season      int                      `json:"season"`
	NextEpisode int                      `json:"next_episode"`
	Episodes    map[string]LedgerEpisode `json:"episodes"`
}

// LedgerEpisode is one assigned video.
type LedgerEpisode struct {
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	UploadDate string `json:"upload_date"`
	Title      string `json:"title"`
	AssignedAt string `json:"assigned_at"`
}

// EpisodeCandidate is a newly accepted video awaiting (or already having) a number.
type EpisodeCandidate struct {
	ID         string
	Title      string
	UploadDate string // YYYYMMDD
}

// LoadLedger reads a ledger from path. Missing file yields an empty v1 ledger.
func LoadLedger(path string) (*Ledger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyLedger("", ""), nil
		}
		return nil, fmt.Errorf("ytdlp: read ledger: %w", err)
	}
	var led Ledger
	if err := json.Unmarshal(data, &led); err != nil {
		return nil, fmt.Errorf("ytdlp: parse ledger: %w", err)
	}
	if led.Episodes == nil {
		led.Episodes = map[string]LedgerEpisode{}
	}
	if led.Version == 0 {
		led.Version = 1
	}
	if led.NextEpisode < 1 {
		led.NextEpisode = 1
	}
	if led.Season < 1 {
		led.Season = 1
	}
	if led.Numbering == "" {
		led.Numbering = "first_seen_sequential"
	}
	return &led, nil
}

// SaveLedger writes the ledger atomically (temp + rename).
func SaveLedger(path string, led *Ledger) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ytdlp: ledger dir: %w", err)
	}
	data, err := json.MarshalIndent(led, "", "  ")
	if err != nil {
		return fmt.Errorf("ytdlp: marshal ledger: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("ytdlp: write ledger tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ytdlp: rename ledger: %w", err)
	}
	return nil
}

// AssignEpisodes loads the show ledger, assigns numbers to new ids only, and saves.
// First ingest of a show sorts new ids by upload_date then id before assigning E001….
// Later runs assign only unseen ids with next_episode++. Existing numbers never move.
func AssignEpisodes(outputDir, slug, title, channelID string, candidates []EpisodeCandidate, now time.Time) (*Ledger, error) {
	path := LedgerPath(outputDir, slug)
	led, err := LoadLedger(path)
	if err != nil {
		return nil, err
	}
	if led.Slug == "" {
		led.Slug = slug
	}
	if title != "" {
		led.Title = title
	}
	if channelID != "" {
		led.ChannelID = channelID
	}
	led.Version = 1
	led.Numbering = "first_seen_sequential"
	if led.Season < 1 {
		led.Season = 1
	}
	if led.Episodes == nil {
		led.Episodes = map[string]LedgerEpisode{}
	}
	if led.NextEpisode < 1 {
		led.NextEpisode = 1
	}

	// Dedupe candidates by id (last wins for title/date of new only).
	byID := map[string]EpisodeCandidate{}
	for _, c := range candidates {
		if c.ID == "" {
			continue
		}
		byID[c.ID] = c
	}

	var newcomers []EpisodeCandidate
	for id, c := range byID {
		if _, exists := led.Episodes[id]; exists {
			continue
		}
		newcomers = append(newcomers, c)
	}

	// First ingest (no prior episodes): sort all newcomers by date then id.
	// Later runs: still sort newcomers so multi-id batches are deterministic,
	// but assignment starts at next_episode (never reshuffles existing).
	sort.Slice(newcomers, func(i, j int) bool {
		if newcomers[i].UploadDate != newcomers[j].UploadDate {
			return newcomers[i].UploadDate < newcomers[j].UploadDate
		}
		return newcomers[i].ID < newcomers[j].ID
	})

	assignedAt := now.UTC().Format(time.RFC3339)
	for _, c := range newcomers {
		ep := led.NextEpisode
		led.Episodes[c.ID] = LedgerEpisode{
			Season:     led.Season,
			Episode:    ep,
			UploadDate: c.UploadDate,
			Title:      c.Title,
			AssignedAt: assignedAt,
		}
		led.NextEpisode = ep + 1
	}

	if err := SaveLedger(path, led); err != nil {
		return nil, err
	}
	return led, nil
}

// AssignOne assigns a single id if new and returns its season/episode.
func AssignOne(outputDir, slug, title, channelID string, c EpisodeCandidate, now time.Time) (season, episode int, err error) {
	led, err := AssignEpisodes(outputDir, slug, title, channelID, []EpisodeCandidate{c}, now)
	if err != nil {
		return 0, 0, err
	}
	e, ok := led.Episodes[c.ID]
	if !ok {
		return 0, 0, fmt.Errorf("ytdlp: ledger missing id %q after assign", c.ID)
	}
	return e.Season, e.Episode, nil
}

func emptyLedger(slug, title string) *Ledger {
	return &Ledger{
		Version:     1,
		Slug:        slug,
		Title:       title,
		ChannelID:   "",
		Numbering:   "first_seen_sequential",
		Season:      1,
		NextEpisode: 1,
		Episodes:    map[string]LedgerEpisode{},
	}
}
