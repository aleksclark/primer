package jellyfin

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Fake is an in-memory Client for tests and local development without a
// Jellyfin server. It lives in the non-test build so both the tv api tests and
// any future tooling can use it.
type Fake struct {
	mu sync.Mutex

	// Items is the library the fake serves, in browse order.
	Items []Item
	// BaseURL is the prefix used when building stream and image URLs.
	BaseURL string
	// ImageData is the payload returned by FetchImage.
	ImageData []byte
	// ImageContentType is the content type returned by FetchImage.
	ImageContentType string
	// Err, when set, is returned by every method.
	Err error
	// Scanning, when true, makes ScanRunning return true.
	Scanning bool
	// RefreshCalls counts RefreshLibrary invocations.
	RefreshCalls int

	// BrowseCalls counts Browse invocations.
	BrowseCalls int
}

var _ Client = (*Fake)(nil)
var _ LibraryAdmin = (*Fake)(nil)

// NewFake builds a fake client seeded with the given items.
func NewFake(items ...Item) *Fake {
	return &Fake{
		Items:            items,
		BaseURL:          "http://jellyfin.test",
		ImageData:        []byte("fake-image"),
		ImageContentType: "image/jpeg",
	}
}

// Ping reports the configured error, if any.
func (f *Fake) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Err
}

// Browse returns the seeded items, honouring SearchTerm, StartIndex and Limit.
func (f *Fake) Browse(_ context.Context, p BrowseParams) ([]Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BrowseCalls++
	if f.Err != nil {
		return nil, f.Err
	}
	wantTypes := map[string]bool{}
	if p.IncludeItemTypes != "" {
		for _, t := range strings.Split(p.IncludeItemTypes, ",") {
			wantTypes[strings.TrimSpace(t)] = true
		}
	}
	out := make([]Item, 0, len(f.Items))
	for _, it := range f.Items {
		if len(wantTypes) > 0 && !wantTypes[it.Type] {
			continue
		}
		if p.SearchTerm != "" && !strings.Contains(strings.ToLower(it.Name), strings.ToLower(p.SearchTerm)) &&
			!strings.Contains(strings.ToLower(it.SeriesName), strings.ToLower(p.SearchTerm)) {
			continue
		}
		if p.PathContains != "" && !strings.Contains(it.Path, p.PathContains) {
			continue
		}
		if p.AnyProviderIDEquals != "" && !ProviderMatch(it, p.AnyProviderIDEquals) {
			continue
		}
		if p.SeriesID != "" && it.SeriesID != p.SeriesID && it.ID != p.SeriesID {
			// Episodes carry SeriesID; allow the series row itself through only
			// when its own ID matches (not used for episode listings).
			if it.Type == "Episode" || it.Type == "Video" {
				if it.SeriesID != p.SeriesID {
					continue
				}
			} else if it.ID != p.SeriesID {
				continue
			}
		}
		if p.ParentID != "" && it.ParentID != p.ParentID && it.SeriesID != p.ParentID {
			continue
		}
		out = append(out, it)
	}
	if p.StartIndex > 0 {
		if p.StartIndex >= len(out) {
			return nil, nil
		}
		out = out[p.StartIndex:]
	}
	if p.Limit > 0 && p.Limit < len(out) {
		out = out[:p.Limit]
	}
	return out, nil
}

// RefreshLibrary records a refresh call.
func (f *Fake) RefreshLibrary(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.RefreshCalls++
	return nil
}

// ScanRunning reports the configured scanning flag.
func (f *Fake) ScanRunning(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	return f.Scanning, nil
}

// Item returns the seeded item with the given ID.
func (f *Fake) Item(_ context.Context, id string) (*Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	for _, it := range f.Items {
		if it.ID == id {
			found := it
			return &found, nil
		}
	}
	return nil, ErrNotFound
}

// StreamURL builds a deterministic fake direct-play URL.
func (f *Fake) StreamURL(itemID string) string {
	return fmt.Sprintf("%s/Videos/%s/stream?static=true", f.BaseURL, url.PathEscape(itemID))
}

// ImageURL builds a deterministic fake artwork URL.
func (f *Fake) ImageURL(itemID, imageType, tag string) string {
	if imageType == "" {
		imageType = "Primary"
	}
	u := fmt.Sprintf("%s/Items/%s/Images/%s", f.BaseURL, url.PathEscape(itemID), url.PathEscape(imageType))
	if tag != "" {
		u += "?tag=" + url.QueryEscape(tag)
	}
	return u
}

// FetchImage returns the configured image payload.
func (f *Fake) FetchImage(_ context.Context, itemID, _, _ string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, "", f.Err
	}
	for _, it := range f.Items {
		if it.ID == itemID {
			return f.ImageData, f.ImageContentType, nil
		}
	}
	return nil, "", ErrNotFound
}
