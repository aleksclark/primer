package tvclient

import (
	"context"
	"fmt"
	"sync"
)

// Fake is an in-memory Client for tests.
type Fake struct {
	mu sync.Mutex

	Items     []MediaItem
	Manifest  []ManifestEntry
	Err       error
	// NextID is assigned to newly created items.
	NextID int

	CreateCalls []MediaItemCreate
	UpdateCalls []struct {
		ID string
		In MediaItemUpdate
	}
	SyncCalls         int
	ManifestSyncCalls int
	AttemptCalls      []struct {
		Slug  string
		Error string
	}
	PresentCalls []string
}

var _ Client = (*Fake)(nil)

// NewFake builds a fake client.
func NewFake(items ...MediaItem) *Fake {
	return &Fake{Items: append([]MediaItem{}, items...), NextID: 1}
}

// ListMediaItems returns the library.
func (f *Fake) ListMediaItems(context.Context) ([]MediaItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]MediaItem, len(f.Items))
	copy(out, f.Items)
	return out, nil
}

// CreateMediaItem appends an item.
func (f *Fake) CreateMediaItem(_ context.Context, in MediaItemCreate) (*MediaItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	for _, existing := range f.Items {
		if existing.JellyfinItemID == in.JellyfinItemID {
			return nil, fmt.Errorf("conflict: jellyfin item already imported")
		}
	}
	item := MediaItem{
		ID:             fmt.Sprintf("mi-%d", f.NextID),
		JellyfinItemID: in.JellyfinItemID,
		Title:          in.Title,
		SortTitle:      in.SortTitle,
		Overview:       in.Overview,
		Class:          in.Class,
		RuntimeSeconds: in.RuntimeSeconds,
		SubjectTags:    in.SubjectTags,
		StandardCodes:  in.StandardCodes,
		Container:      in.Container,
		VideoCodec:     in.VideoCodec,
		AudioCodec:     in.AudioCodec,
		ImageTag:       in.ImageTag,
		DirectPlayOK:   true,
	}
	f.NextID++
	f.Items = append(f.Items, item)
	f.CreateCalls = append(f.CreateCalls, in)
	return &item, nil
}

// UpdateMediaItem patches an item.
func (f *Fake) UpdateMediaItem(_ context.Context, id string, in MediaItemUpdate) (*MediaItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	for i := range f.Items {
		if f.Items[i].ID != id {
			continue
		}
		if in.Title != nil {
			f.Items[i].Title = *in.Title
		}
		if in.Class != nil {
			f.Items[i].Class = *in.Class
		}
		if in.SubjectTags != nil {
			f.Items[i].SubjectTags = *in.SubjectTags
		}
		if in.StandardCodes != nil {
			f.Items[i].StandardCodes = *in.StandardCodes
		}
		if in.Overview != nil {
			f.Items[i].Overview = *in.Overview
		}
		f.UpdateCalls = append(f.UpdateCalls, struct {
			ID string
			In MediaItemUpdate
		}{ID: id, In: in})
		item := f.Items[i]
		return &item, nil
	}
	return nil, fmt.Errorf("media item %s not found", id)
}

// SyncJellyfin records a sync call.
func (f *Fake) SyncJellyfin(context.Context) (*SyncResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	f.SyncCalls++
	return &SyncResult{Checked: len(f.Items), Updated: 0}, nil
}

// SyncManifest upserts desired-state rows.
func (f *Fake) SyncManifest(_ context.Context, items []ManifestDesired) (*ManifestSyncResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	f.ManifestSyncCalls++
	created, updated := 0, 0
	bySlug := make(map[string]int, len(f.Manifest))
	for i, e := range f.Manifest {
		bySlug[e.Slug] = i
	}
	for _, in := range items {
		if idx, ok := bySlug[in.Slug]; ok {
			e := f.Manifest[idx]
			e.Title = in.Title
			e.Year = in.Year
			e.Kind = in.Kind
			e.TMDBID = in.TMDBID
			e.TVDBID = in.TVDBID
			e.URL = in.URL
			e.Class = in.Class
			e.SubjectTags = in.SubjectTags
			e.StandardCodes = in.StandardCodes
			e.Priority = in.Priority
			e.ExcludeEpisodes = in.ExcludeEpisodes
			e.MaxEpisodes = in.MaxEpisodes
			e.Notes = in.Notes
			f.Manifest[idx] = e
			updated++
			continue
		}
		status := ManifestStatusMissing
		if in.Kind == "manual" {
			status = ManifestStatusManual
		}
		f.NextID++
		f.Manifest = append(f.Manifest, ManifestEntry{
			ID:              fmt.Sprintf("me-%d", f.NextID),
			Slug:            in.Slug,
			Title:           in.Title,
			Year:            in.Year,
			Kind:            in.Kind,
			TMDBID:          in.TMDBID,
			TVDBID:          in.TVDBID,
			URL:             in.URL,
			Class:           in.Class,
			SubjectTags:     in.SubjectTags,
			StandardCodes:   in.StandardCodes,
			Priority:        in.Priority,
			ExcludeEpisodes: in.ExcludeEpisodes,
			MaxEpisodes:     in.MaxEpisodes,
			Notes:           in.Notes,
			Status:          status,
		})
		bySlug[in.Slug] = len(f.Manifest) - 1
		created++
	}
	return &ManifestSyncResult{Created: created, Updated: updated, Total: len(items)}, nil
}

// ListManifestEntries returns the catalog.
func (f *Fake) ListManifestEntries(context.Context) ([]ManifestEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]ManifestEntry, len(f.Manifest))
	copy(out, f.Manifest)
	return out, nil
}

// RecordManifestAttempt increments attempt counters.
func (f *Fake) RecordManifestAttempt(_ context.Context, slug, lastError string) (*ManifestEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	f.AttemptCalls = append(f.AttemptCalls, struct {
		Slug  string
		Error string
	}{Slug: slug, Error: lastError})
	for i := range f.Manifest {
		if f.Manifest[i].Slug != slug {
			continue
		}
		if f.Manifest[i].Status == ManifestStatusMissing {
			f.Manifest[i].AttemptCount++
			f.Manifest[i].LastError = lastError
		}
		e := f.Manifest[i]
		return &e, nil
	}
	return nil, fmt.Errorf("manifest entry %s not found", slug)
}

// MarkManifestPresent marks a slug present.
func (f *Fake) MarkManifestPresent(_ context.Context, slug string) (*ManifestEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	f.PresentCalls = append(f.PresentCalls, slug)
	for i := range f.Manifest {
		if f.Manifest[i].Slug != slug {
			continue
		}
		f.Manifest[i].Status = ManifestStatusPresent
		f.Manifest[i].LastError = ""
		e := f.Manifest[i]
		return &e, nil
	}
	return nil, fmt.Errorf("manifest entry %s not found", slug)
}
