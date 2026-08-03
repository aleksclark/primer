// Package manifest loads and writes the content-ingest desired-state YAML and
// the human review queue for ambiguous title lookups.
package manifest

import (
	"fmt"
	"os"
	"sort"
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

// Filters narrows a YouTube source.
type Filters struct {
	Playlists []string `yaml:"playlists,omitempty" json:"playlists,omitempty"`
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
	MaxEpisodes     int      `yaml:"max_episodes,omitempty" json:"max_episodes,omitempty"`
	Notes           string   `yaml:"notes,omitempty" json:"notes,omitempty"`
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

// Excluded reports whether episodeKey (e.g. "S01E07") is on the skip list.
// Matching is case-insensitive.
func (it Item) Excluded(episodeKey string) bool {
	key := strings.ToUpper(strings.TrimSpace(episodeKey))
	for _, ex := range it.ExcludeEpisodes {
		if strings.ToUpper(strings.TrimSpace(ex)) == key {
			return true
		}
	}
	return false
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
