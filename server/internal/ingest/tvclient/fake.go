package tvclient

import (
	"context"
	"fmt"
	"sync"
)

// Fake is an in-memory Client for tests.
type Fake struct {
	mu sync.Mutex

	Items []MediaItem
	Err   error
	// NextID is assigned to newly created items.
	NextID int

	CreateCalls []MediaItemCreate
	UpdateCalls []struct {
		ID string
		In MediaItemUpdate
	}
	SyncCalls int
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
