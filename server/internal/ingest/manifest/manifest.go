// Package manifest loads and writes the content-ingest desired-state YAML and
// the human review queue for ambiguous title lookups.
package manifest

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind values for a manifest item.
const (
	KindMovie           = "movie"
	KindSeries          = "series"
	KindYouTubeChannel  = "youtube_channel"
	KindYouTubePlaylist = "youtube_playlist"
	KindManual          = "manual"
)

// Class values mirror the TV server media item classes.
const (
	ClassEducational   = "educational"
	ClassEntertainment = "entertainment"
	ClassMixed         = "mixed"
)

// Provider holds external catalog IDs. Empty means unresolved.
type Provider struct {
	TMDB int `yaml:"tmdb,omitempty" json:"tmdb,omitempty"`
	TVDB int `yaml:"tvdb,omitempty" json:"tvdb,omitempty"`
}

// Empty reports whether no provider ID is set.
func (p Provider) Empty() bool { return p.TMDB == 0 && p.TVDB == 0 }

// DefaultMinDurationSeconds is applied when Filters.MinDurationSeconds is unset (0).
const DefaultMinDurationSeconds = 60

// VideoOverride is a per-video metadata or exclusion override keyed by YouTube id.
type VideoOverride struct {
	ID            string   `yaml:"id" json:"id"`
	Title         string   `yaml:"title,omitempty" json:"title,omitempty"`
	Class         string   `yaml:"class,omitempty" json:"class,omitempty"`
	SubjectTags   []string `yaml:"subject_tags,omitempty" json:"subject_tags,omitempty"`
	StandardCodes []string `yaml:"standard_codes,omitempty" json:"standard_codes,omitempty"`
	Exclude       bool     `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// Filters narrows a YouTube source.
// Empty Playlists on a youtube_channel means import from /videos only
// (not playlist-scoped). Non-empty Playlists means do not download the
// whole channel — only the named playlists (resolved by Lane B).
type Filters struct {
	Playlists          []string `yaml:"playlists,omitempty" json:"playlists,omitempty"`
	MinDurationSeconds int      `yaml:"min_duration_seconds,omitempty" json:"min_duration_seconds,omitempty"`
	ExcludeShorts      *bool    `yaml:"exclude_shorts,omitempty" json:"exclude_shorts,omitempty"`
	ExcludeLive        *bool    `yaml:"exclude_live,omitempty" json:"exclude_live,omitempty"`
}

// Item is one desired-state entry in the content manifest.
type Item struct {
	ID              string   `yaml:"id" json:"id"`
	Title           string   `yaml:"title" json:"title"`
	Year            int      `yaml:"year,omitempty" json:"year,omitempty"`
	Kind            string   `yaml:"kind" json:"kind"`
	Provider        Provider `yaml:"provider" json:"provider"`
	URL             string   `yaml:"url,omitempty" json:"url,omitempty"`
	Filters         Filters  `yaml:"filters" json:"filters"`
	Class           string   `yaml:"class" json:"class"`
	SubjectTags     []string `yaml:"subject_tags,omitempty" json:"subject_tags,omitempty"`
	StandardCodes   []string `yaml:"standard_codes,omitempty" json:"standard_codes,omitempty"`
	Priority        int      `yaml:"priority,omitempty" json:"priority,omitempty"`
	ExcludeEpisodes []string `yaml:"exclude_episodes,omitempty" json:"exclude_episodes,omitempty"`
	// MaxEpisodes caps how many videos/episodes to import for Video+Episode
	// sources (YouTube channel/playlist and series). It does not apply to
	// Folder-style sources. See AppliesToYouTubeImport, CapsImports, ImportLimit.
	MaxEpisodes int             `yaml:"max_episodes,omitempty" json:"max_episodes,omitempty"`
	Videos      []VideoOverride `yaml:"videos,omitempty" json:"videos,omitempty"`
	Notes       string          `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Manifest is the desired-state document.
type Manifest struct {
	Items []Item `yaml:"items"`
}

// Load reads a manifest from path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes the manifest to path with stable formatting.
func Save(path string, m *Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	return nil
}

// Validate checks required fields and uniqueness.
func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	seen := make(map[string]bool, len(m.Items))
	for i, it := range m.Items {
		if it.ID == "" {
			return fmt.Errorf("items[%d]: id is required", i)
		}
		if seen[it.ID] {
			return fmt.Errorf("duplicate item id %q", it.ID)
		}
		seen[it.ID] = true
		if it.Title == "" {
			return fmt.Errorf("item %q: title is required", it.ID)
		}
		switch it.Kind {
		case KindMovie, KindSeries, KindYouTubeChannel, KindYouTubePlaylist, KindManual:
		default:
			return fmt.Errorf("item %q: unknown kind %q", it.ID, it.Kind)
		}
		switch it.Class {
		case ClassEducational, ClassEntertainment, ClassMixed:
		default:
			return fmt.Errorf("item %q: unknown class %q", it.ID, it.Class)
		}
		if (it.Kind == KindYouTubeChannel || it.Kind == KindYouTubePlaylist) && it.URL == "" {
			return fmt.Errorf("item %q: url is required for %s", it.ID, it.Kind)
		}
		if err := validateVideos(it); err != nil {
			return err
		}
		if err := ValidatePlaylists(it); err != nil {
			return err
		}
		if err := validateMinDuration(it); err != nil {
			return err
		}
	}
	return nil
}

func validateVideos(it Item) error {
	seen := make(map[string]bool, len(it.Videos))
	for i, v := range it.Videos {
		if strings.TrimSpace(v.ID) == "" {
			return fmt.Errorf("item %q: videos[%d]: id is required", it.ID, i)
		}
		if seen[v.ID] {
			return fmt.Errorf("item %q: duplicate video id %q", it.ID, v.ID)
		}
		seen[v.ID] = true
		if v.Class != "" {
			switch v.Class {
			case ClassEducational, ClassEntertainment, ClassMixed:
			default:
				return fmt.Errorf("item %q: videos[%d]: unknown class %q", it.ID, i, v.Class)
			}
		}
	}
	return nil
}

// ByID returns the item with the given id, or nil.
func (m *Manifest) ByID(id string) *Item {
	for i := range m.Items {
		if m.Items[i].ID == id {
			return &m.Items[i]
		}
	}
	return nil
}

// SetProvider writes a provider ID onto the named item.
func (m *Manifest) SetProvider(id string, p Provider) error {
	it := m.ByID(id)
	if it == nil {
		return fmt.Errorf("item %q not found", id)
	}
	it.Provider = p
	return nil
}

// SortedByPriority returns items ordered by priority ascending (zero last),
// then by id for stability.
func (m *Manifest) SortedByPriority() []Item {
	out := make([]Item, len(m.Items))
	copy(out, m.Items)
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].Priority, out[j].Priority
		if pi == 0 {
			pi = 1 << 30
		}
		if pj == 0 {
			pj = 1 << 30
		}
		if pi != pj {
			return pi < pj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// episodeKeyRE matches S##E## with optional zero-padding on the episode number.
var episodeKeyRE = regexp.MustCompile(`(?i)^S(\d+)E(\d+)$`)

// normalizeEpisodeKey returns a canonical "S{season}E{episode}" form with
// unpadded numeric parts, or "" if key is not an episode key.
func normalizeEpisodeKey(key string) string {
	m := episodeKeyRE.FindStringSubmatch(strings.TrimSpace(key))
	if m == nil {
		return ""
	}
	season, err1 := strconv.Atoi(m[1])
	episode, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return ""
	}
	return fmt.Sprintf("S%dE%d", season, episode)
}

// excludeEntryMatches reports whether a single exclude_episodes entry matches key.
// Episode keys match case-insensitively and ignore zero-padding (S01E07 == S01E007).
// YouTube ids (and other non-episode tokens) match case-sensitively as stored.
func excludeEntryMatches(entry, key string) bool {
	entry = strings.TrimSpace(entry)
	key = strings.TrimSpace(key)
	if entry == "" || key == "" {
		return false
	}
	if ek := normalizeEpisodeKey(entry); ek != "" {
		if kk := normalizeEpisodeKey(key); kk != "" {
			return ek == kk
		}
		// entry is episode form; key is not — no match
		return false
	}
	// Non-episode tokens (e.g. YouTube ids): case-sensitive exact match.
	return entry == key
}

// Excluded reports whether key is on the skip list.
// key may be an episode key (e.g. "S01E07" / "S01E007", case-insensitive,
// padding-insensitive) or a YouTube video id (case-sensitive as stored).
func (it Item) Excluded(key string) bool {
	for _, ex := range it.ExcludeEpisodes {
		if excludeEntryMatches(ex, key) {
			return true
		}
	}
	return false
}

// ExcludedVideo reports whether a video should be skipped for this item.
// True if exclude_episodes contains youtubeID or episodeKey, or videos[] has
// that youtube id with Exclude true.
func ExcludedVideo(it Item, youtubeID, episodeKey string) bool {
	if youtubeID != "" && it.Excluded(youtubeID) {
		return true
	}
	if episodeKey != "" && it.Excluded(episodeKey) {
		return true
	}
	if ov := OverrideFor(it, youtubeID); ov != nil && ov.Exclude {
		return true
	}
	return false
}

// OverrideFor returns the VideoOverride for youtubeID, or nil.
func OverrideFor(it Item, youtubeID string) *VideoOverride {
	if youtubeID == "" {
		return nil
	}
	for i := range it.Videos {
		if it.Videos[i].ID == youtubeID {
			return &it.Videos[i]
		}
	}
	return nil
}

// EffectiveMinDuration returns Filters.MinDurationSeconds, or DefaultMinDurationSeconds when unset (0).
// -1 disables the duration floor (explicit shorts). Other negatives are rejected by Validate.
func EffectiveMinDuration(f Filters) int {
	if f.MinDurationSeconds == 0 {
		return DefaultMinDurationSeconds
	}
	return f.MinDurationSeconds
}

func validateMinDuration(it Item) error {
	if it.Filters.MinDurationSeconds < -1 {
		return fmt.Errorf("item %q: filters.min_duration_seconds must be -1 or nonnegative", it.ID)
	}
	return nil
}

// EffectiveExcludeShorts returns the exclude_shorts setting; nil pointer defaults to true.
func EffectiveExcludeShorts(f Filters) bool {
	if f.ExcludeShorts == nil {
		return true
	}
	return *f.ExcludeShorts
}

// EffectiveExcludeLive returns the exclude_live setting; nil pointer defaults to true.
func EffectiveExcludeLive(f Filters) bool {
	if f.ExcludeLive == nil {
		return true
	}
	return *f.ExcludeLive
}

// UsesPlaylistFilter reports whether the item scopes import to named playlists.
// Empty playlists on a youtube_channel means /videos only (no playlist filter).
func UsesPlaylistFilter(it Item) bool {
	return len(PlaylistNames(it)) > 0
}

// PlaylistNames returns the configured playlist name list (may be empty).
func PlaylistNames(it Item) []string {
	if len(it.Filters.Playlists) == 0 {
		return nil
	}
	out := make([]string, len(it.Filters.Playlists))
	copy(out, it.Filters.Playlists)
	return out
}

// ValidatePlaylists fails closed if any required playlist name is empty/whitespace.
// Actual yt-dlp playlist name resolution is Lane B; this only rejects blank names.
func ValidatePlaylists(it Item) error {
	for i, name := range it.Filters.Playlists {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("item %q: filters.playlists[%d]: playlist name is required (empty names are not silently ignored)", it.ID, i)
		}
	}
	return nil
}

// AppliesToYouTubeImport reports whether MaxEpisodes is meaningful for YouTube
// video import on this item (youtube_channel or youtube_playlist).
func (it Item) AppliesToYouTubeImport() bool {
	return it.Kind == KindYouTubeChannel || it.Kind == KindYouTubePlaylist
}

// CapsImports reports whether MaxEpisodes is set (> 0) and should limit imports.
// MaxEpisodes applies to Video+Episode sources (YouTube and series), not Folder.
func (it Item) CapsImports() bool {
	return it.MaxEpisodes > 0
}

// ImportLimit returns MaxEpisodes (0 means uncapped).
func (it Item) ImportLimit() int {
	return it.MaxEpisodes
}

// NeedsResolve reports whether this item still needs a provider ID lookup.
func (it Item) NeedsResolve() bool {
	switch it.Kind {
	case KindMovie, KindSeries:
		return it.Provider.Empty()
	default:
		return false
	}
}

// Candidate is one lookup hit presented in review.yaml.
type Candidate struct {
	Title    string `yaml:"title"`
	Year     int    `yaml:"year,omitempty"`
	TMDB     int    `yaml:"tmdb,omitempty"`
	TVDB     int    `yaml:"tvdb,omitempty"`
	Overview string `yaml:"overview,omitempty"`
}

// ReviewEntry is one unresolved (or multi-hit) item awaiting a human pick.
// The human picks by setting ChosenTMDB or ChosenTVDB (and optionally
// uncommenting a candidate line in the YAML).
type ReviewEntry struct {
	ID         string      `yaml:"id"`
	Title      string      `yaml:"title"`
	Year       int         `yaml:"year,omitempty"`
	Kind       string      `yaml:"kind"`
	Reason     string      `yaml:"reason"`
	Candidates []Candidate `yaml:"candidates,omitempty"`
	// ChosenTMDB / ChosenTVDB are filled by a human. On the next resolve pass
	// they are applied to the manifest and the entry is dropped.
	ChosenTMDB int `yaml:"chosen_tmdb,omitempty"`
	ChosenTVDB int `yaml:"chosen_tvdb,omitempty"`
}

// Review is the human-resolution working file.
type Review struct {
	Entries []ReviewEntry `yaml:"entries"`
}

// LoadReview reads review.yaml. A missing file is an empty review.
func LoadReview(path string) (*Review, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Review{}, nil
		}
		return nil, fmt.Errorf("read review %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return &Review{}, nil
	}
	var r Review
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse review %s: %w", path, err)
	}
	return &r, nil
}

// SaveReview writes review.yaml.
func SaveReview(path string, r *Review) error {
	if r == nil {
		r = &Review{}
	}
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode review: %w", err)
	}
	header := "# Human review queue for content-ingest.\n" +
		"# For each entry, set chosen_tmdb or chosen_tvdb from the candidates\n" +
		"# (or look the title up yourself). The next `ingest apply` resolve pass\n" +
		"# writes the pick into the manifest and drops the entry.\n\n"
	if err := os.WriteFile(path, append([]byte(header), data...), 0o644); err != nil {
		return fmt.Errorf("write review %s: %w", path, err)
	}
	return nil
}

// Upsert replaces or appends a review entry by id.
func (r *Review) Upsert(e ReviewEntry) {
	for i := range r.Entries {
		if r.Entries[i].ID == e.ID {
			// Preserve a human choice if the new entry has none.
			if e.ChosenTMDB == 0 {
				e.ChosenTMDB = r.Entries[i].ChosenTMDB
			}
			if e.ChosenTVDB == 0 {
				e.ChosenTVDB = r.Entries[i].ChosenTVDB
			}
			r.Entries[i] = e
			return
		}
	}
	r.Entries = append(r.Entries, e)
}

// Remove drops the entry with the given id.
func (r *Review) Remove(id string) {
	out := r.Entries[:0]
	for _, e := range r.Entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	r.Entries = out
}

// ByID returns the review entry with the given id, or nil.
func (r *Review) ByID(id string) *ReviewEntry {
	for i := range r.Entries {
		if r.Entries[i].ID == id {
			return &r.Entries[i]
		}
	}
	return nil
}
