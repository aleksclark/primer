package sonarr

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake is an in-memory Client for tests.
type Fake struct {
	mu sync.Mutex

	Library       []Series
	LookupResults []Series
	EpisodeMap    map[int][]Episode // seriesID → episodes
	Tags          []Tag
	Err           error
	NextID        int
	NextTagID     int
	NextEpisodeID int

	AddCalls    []Series
	LookupCalls []string
	Unmonitor   []int // episode IDs unmonitored
}

var _ Client = (*Fake)(nil)

// NewFake builds a fake client.
func NewFake() *Fake {
	return &Fake{
		NextID:        1,
		NextTagID:     1,
		NextEpisodeID: 1,
		EpisodeMap:    map[int][]Episode{},
	}
}

// Lookup returns seeded results.
func (f *Fake) Lookup(_ context.Context, term string) ([]Series, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LookupCalls = append(f.LookupCalls, term)
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]Series, len(f.LookupResults))
	copy(out, f.LookupResults)
	return out, nil
}

// List returns the library.
func (f *Fake) List(context.Context) ([]Series, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]Series, len(f.Library))
	copy(out, f.Library)
	return out, nil
}

// Add appends a series to the library.
func (f *Fake) Add(_ context.Context, series Series, qualityProfileID int, rootFolder string, _ []int) (*Series, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	if series.TvdbID == 0 {
		return nil, fmt.Errorf("tvdb id required")
	}
	for _, existing := range f.Library {
		if existing.TvdbID == series.TvdbID {
			return nil, fmt.Errorf("series already exists")
		}
	}
	series.ID = f.NextID
	f.NextID++
	series.QualityProfileID = qualityProfileID
	series.RootFolderPath = rootFolder
	series.Monitored = true
	series.SeasonFolder = true
	f.Library = append(f.Library, series)
	f.AddCalls = append(f.AddCalls, series)
	return &series, nil
}

// EnsureTag returns or creates a tag by label.
func (f *Fake) EnsureTag(_ context.Context, label string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	for _, t := range f.Tags {
		if strings.EqualFold(t.Label, label) {
			return t.ID, nil
		}
	}
	id := f.NextTagID
	f.NextTagID++
	f.Tags = append(f.Tags, Tag{ID: id, Label: label})
	return id, nil
}

// Episodes returns seeded episodes for a series.
func (f *Fake) Episodes(_ context.Context, seriesID int) ([]Episode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	eps := f.EpisodeMap[seriesID]
	out := make([]Episode, len(eps))
	copy(out, eps)
	return out, nil
}

// SetEpisodeMonitored toggles monitoring.
func (f *Fake) SetEpisodeMonitored(_ context.Context, episodeID int, monitored bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	for sid, eps := range f.EpisodeMap {
		for i := range eps {
			if eps[i].ID == episodeID {
				eps[i].Monitored = monitored
				f.EpisodeMap[sid] = eps
				if !monitored {
					f.Unmonitor = append(f.Unmonitor, episodeID)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("episode %d not found", episodeID)
}
