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

	// Collections maps collection id → member item ids (unrelated members preserved).
	Collections map[string][]string
	// CollectionNames maps collection id → exact name.
	CollectionNames map[string]string
	// CreateCollectionCalls records CreateCollection invocations.
	CreateCollectionCalls []struct {
		Name string
		IDs  []string
	}
	// AddToCollectionCalls records AddToCollection invocations.
	AddToCollectionCalls []struct {
		CollectionID string
		IDs          []string
	}
}

var _ Client = (*Fake)(nil)
var _ LibraryAdmin = (*Fake)(nil)
var _ CollectionAdmin = (*Fake)(nil)

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
func (f *Fake) Browse(ctx context.Context, p BrowseParams) ([]Item, error) {
	page, err := f.BrowsePage(ctx, p)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// BrowsePage pages the type-filtered library first, then applies PathContains /
// provider filters. TotalRecordCount / RawPageLen reflect the unfiltered
// (type-filtered) library so an empty first filtered page still paginates.
func (f *Fake) BrowsePage(_ context.Context, p BrowseParams) (Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BrowseCalls++
	if f.Err != nil {
		return Page{}, f.Err
	}
	wantTypes := map[string]bool{}
	if p.IncludeItemTypes != "" {
		for _, t := range strings.Split(p.IncludeItemTypes, ",") {
			wantTypes[strings.TrimSpace(t)] = true
		}
	}
	collectionMembers := map[string]struct{}{}
	if p.ParentID != "" {
		for _, id := range f.Collections[p.ParentID] {
			collectionMembers[id] = struct{}{}
		}
	}
	unfiltered := make([]Item, 0, len(f.Items))
	for _, it := range f.Items {
		if len(wantTypes) > 0 && !wantTypes[it.Type] {
			continue
		}
		if p.SearchTerm != "" && !strings.Contains(strings.ToLower(it.Name), strings.ToLower(p.SearchTerm)) &&
			!strings.Contains(strings.ToLower(it.SeriesName), strings.ToLower(p.SearchTerm)) {
			continue
		}
		if p.SeriesID != "" {
			// Exact SeriesID only. Blank SeriesID never matches (fail closed).
			// The series row itself is not an episode listing hit.
			if it.SeriesID != p.SeriesID {
				continue
			}
		}
		if p.ParentID != "" {
			_, inCollection := collectionMembers[it.ID]
			if !inCollection && it.ParentID != p.ParentID && it.SeriesID != p.ParentID && it.ID != p.ParentID {
				continue
			}
		}
		unfiltered = append(unfiltered, it)
	}
	total := len(unfiltered)
	if p.StartIndex > 0 {
		if p.StartIndex >= len(unfiltered) {
			return Page{TotalRecordCount: total}, nil
		}
		unfiltered = unfiltered[p.StartIndex:]
	}
	if p.Limit > 0 && p.Limit < len(unfiltered) {
		unfiltered = unfiltered[:p.Limit]
	}
	out := make([]Item, 0, len(unfiltered))
	for _, it := range unfiltered {
		if p.PathContains != "" && !strings.Contains(it.Path, p.PathContains) {
			continue
		}
		if p.AnyProviderIDEquals != "" && !ProviderMatch(it, p.AnyProviderIDEquals) {
			continue
		}
		out = append(out, it)
	}
	return Page{Items: out, TotalRecordCount: total, RawPageLen: len(unfiltered)}, nil
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

// CreateCollection records a named BoxSet and optionally seeds members.
func (f *Fake) CreateCollection(_ context.Context, name string, ids []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("jellyfin: collection name is required")
	}
	copied := append([]string{}, ids...)
	f.CreateCollectionCalls = append(f.CreateCollectionCalls, struct {
		Name string
		IDs  []string
	}{Name: name, IDs: copied})
	id := fmt.Sprintf("col-%d", len(f.CreateCollectionCalls))
	if f.Collections == nil {
		f.Collections = map[string][]string{}
	}
	if f.CollectionNames == nil {
		f.CollectionNames = map[string]string{}
	}
	f.CollectionNames[id] = name
	f.Collections[id] = uniqueIDs(ids)
	f.Items = append(f.Items, Item{ID: id, Name: name, Type: "BoxSet"})
	return id, nil
}

// AddToCollection appends missing members; existing unrelated entries stay.
func (f *Fake) AddToCollection(_ context.Context, collectionID string, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return fmt.Errorf("jellyfin: collection id is required")
	}
	copied := append([]string{}, ids...)
	f.AddToCollectionCalls = append(f.AddToCollectionCalls, struct {
		CollectionID string
		IDs          []string
	}{CollectionID: collectionID, IDs: copied})
	if f.Collections == nil {
		f.Collections = map[string][]string{}
	}
	have := map[string]struct{}{}
	for _, id := range f.Collections[collectionID] {
		have[id] = struct{}{}
	}
	for _, id := range uniqueIDs(ids) {
		if _, ok := have[id]; ok {
			continue
		}
		f.Collections[collectionID] = append(f.Collections[collectionID], id)
		have[id] = struct{}{}
	}
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
