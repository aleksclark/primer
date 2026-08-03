package radarr

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake is an in-memory Client for tests.
type Fake struct {
	mu sync.Mutex

	// Library is the movies already in Radarr.
	Library []Movie
	// LookupResults is returned by Lookup (optionally filtered by term).
	LookupResults []Movie
	// Tags are known tags.
	Tags []Tag
	// Err, when set, is returned by every method.
	Err error
	// NextID is the next movie id assigned by Add.
	NextID int
	// NextTagID is the next tag id assigned by EnsureTag.
	NextTagID int

	// AddCalls records movies passed to Add.
	AddCalls []Movie
	// LookupCalls records terms passed to Lookup.
	LookupCalls []string
}

var _ Client = (*Fake)(nil)

// NewFake builds a fake client.
func NewFake() *Fake {
	return &Fake{NextID: 1, NextTagID: 1}
}

// Lookup returns seeded results whose title contains term (case-insensitive),
// or all results when term is empty.
func (f *Fake) Lookup(_ context.Context, term string) ([]Movie, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LookupCalls = append(f.LookupCalls, term)
	if f.Err != nil {
		return nil, f.Err
	}
	if term == "" {
		out := make([]Movie, len(f.LookupResults))
		copy(out, f.LookupResults)
		return out, nil
	}
	needle := strings.ToLower(term)
	var out []Movie
	for _, m := range f.LookupResults {
		if strings.Contains(strings.ToLower(m.Title), needle) ||
			strings.Contains(needle, strings.ToLower(m.Title)) {
			out = append(out, m)
		}
	}
	// Fall back to year-suffixed term matches: "Title 1999" still matches Title.
	if len(out) == 0 {
		for _, m := range f.LookupResults {
			if strings.Contains(needle, strings.ToLower(m.Title)) {
				out = append(out, m)
			}
		}
	}
	if len(out) == 0 {
		// Last resort: return all so tests can drive multi-hit via LookupResults alone.
		out = append(out, f.LookupResults...)
	}
	return out, nil
}

// List returns the library.
func (f *Fake) List(context.Context) ([]Movie, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]Movie, len(f.Library))
	copy(out, f.Library)
	return out, nil
}

// Add appends a movie to the library.
func (f *Fake) Add(_ context.Context, movie Movie, qualityProfileID int, rootFolder string, tagIDs []int) (*Movie, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	if movie.TmdbID == 0 {
		return nil, fmt.Errorf("tmdb id required")
	}
	for _, existing := range f.Library {
		if existing.TmdbID == movie.TmdbID {
			return nil, fmt.Errorf("movie already exists")
		}
	}
	movie.ID = f.NextID
	f.NextID++
	movie.QualityProfileID = qualityProfileID
	movie.RootFolderPath = rootFolder
	movie.Monitored = true
	_ = tagIDs
	f.Library = append(f.Library, movie)
	f.AddCalls = append(f.AddCalls, movie)
	return &movie, nil
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
