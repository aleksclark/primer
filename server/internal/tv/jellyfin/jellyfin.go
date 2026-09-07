// Package jellyfin talks to a Jellyfin media server: it browses the library,
// fetches item metadata, and builds the direct-play stream and image URLs the
// device app uses. The TV server never proxies media bytes, so the client's
// job is metadata plus URL construction.
package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// tickDuration is Jellyfin's RunTimeTicks unit: one tick is 100 nanoseconds.
// Runtimes are converted by scaling ticks directly rather than multiplying by
// time.Second, which would overflow int64 for feature-length items.
const tickDuration = 100 * time.Nanosecond

// ErrNotFound is returned when Jellyfin has no such item.
var ErrNotFound = errors.New("jellyfin: item not found")

// Item is the subset of Jellyfin item metadata the TV server caches.
type Item struct {
	ID         string
	Name       string
	SortName   string
	Overview   string
	Type       string
	Runtime    time.Duration
	Container  string
	VideoCodec string
	AudioCodec string
	ImageTag   string
	// Path is the on-disk path when Jellyfin returns it (used by content-ingest
	// to match yt-dlp downloads by path prefix).
	Path string
	// ProviderIds holds external catalog ids (Tmdb, Tvdb, Imdb, …).
	ProviderIds map[string]string
	// IndexNumber is the episode number within a season (episodes only).
	IndexNumber int
	// ParentIndexNumber is the season number (episodes only).
	ParentIndexNumber int
	// SeriesName is the parent series title (episodes only).
	SeriesName string
	// SeriesID is the Jellyfin id of the parent series (episodes only).
	SeriesID string
	// ParentID is the immediate parent folder/season/series id when present.
	ParentID string
}

// RuntimeSeconds returns the item's duration in whole seconds.
func (i Item) RuntimeSeconds() int { return int(i.Runtime.Seconds()) }

// BrowseParams filters a library listing.
type BrowseParams struct {
	// ParentID restricts results to one library folder or collection.
	ParentID string
	// SeriesID restricts results to descendants of one series. Jellyfin 10.11
	// /Items has no seriesId query; this is sent as ParentId + Recursive=true
	// and then re-checked client-side against Item.SeriesID (blank does not match).
	SeriesID string
	// SearchTerm filters by title.
	SearchTerm string
	// IncludeItemTypes overrides the default Movie,Episode,Video set
	// (comma-separated Jellyfin types, e.g. "Series" or "Movie").
	IncludeItemTypes string
	// Limit caps the number of returned items.
	Limit int
	// StartIndex is the offset for paging.
	StartIndex int
	// AnyProviderIDEquals filters by a provider id value (e.g. TMDB/TVDB).
	// Format is "Key=Value" pairs joined by "|". Always enforced client-side
	// after the response; the Jellyfin query param is a hint only and is not
	// trusted (some builds ignore it and return unfiltered library pages).
	AnyProviderIDEquals string
	// PathContains restricts results to items whose Path contains this string.
	// Applied client-side after fetch when the server does not filter natively.
	PathContains string
	// IncludeProviderIDs requests ProviderIds on each item.
	IncludeProviderIDs bool
	// IncludePath requests Path on each item.
	IncludePath bool
}

// Page is one /Items response after optional client-side filters.
type Page struct {
	Items []Item
	// TotalRecordCount is Jellyfin's unfiltered library total for the query.
	TotalRecordCount int
	// RawPageLen is the number of items Jellyfin returned before client filters.
	RawPageLen int
}

// Client is the Jellyfin behaviour the TV server depends on. It is an
// interface so tests can substitute a fake without an HTTP server.
type Client interface {
	// Ping verifies connectivity and credentials.
	Ping(ctx context.Context) error
	// Browse lists library items available for import.
	Browse(ctx context.Context, p BrowseParams) ([]Item, error)
	// BrowsePage lists one page and reports unfiltered TotalRecordCount / RawPageLen.
	BrowsePage(ctx context.Context, p BrowseParams) (Page, error)
	// Item fetches metadata for a single item.
	Item(ctx context.Context, id string) (*Item, error)
	// ItemsByID fetches metadata for many items in one /Items?Ids= request.
	// The returned map contains only requested IDs that Jellyfin had; missing
	// IDs are omitted (callers treat them as not found). Extra/foreign IDs in
	// the response are an error so sync cannot silently orphan valid rows.
	ItemsByID(ctx context.Context, ids []string) (map[string]Item, error)
	// StreamURL builds a direct-play URL for an item.
	StreamURL(itemID string) string
	// ImageURL builds an artwork URL for an item.
	ImageURL(itemID, imageType, tag string) string
	// FetchImage retrieves artwork bytes so the TV server can proxy artwork
	// without handing Jellyfin credentials to the client.
	FetchImage(ctx context.Context, itemID, imageType, tag string) (data []byte, contentType string, err error)
}

// CollectionAdmin extends LibraryAdmin with named Collection create/add used by content-ingest.
// Collections are BoxSet items, not a separate Library / virtual folder.
type CollectionAdmin interface {
	LibraryAdmin
	// CreateCollection POSTs /Collections?name=… and returns the new BoxSet id.
	CreateCollection(ctx context.Context, name string, ids []string) (string, error)
	// AddToCollection POSTs /Collections/{id}/Items?ids=… (additive; existing members stay).
	AddToCollection(ctx context.Context, collectionID string, ids []string) error
}

// LibraryAdmin extends Client with library-scan operations used by content-ingest.
type LibraryAdmin interface {
	Client
	// RefreshLibrary triggers a full library scan.
	RefreshLibrary(ctx context.Context) error
	// ScanRunning reports whether a library scan is in progress.
	ScanRunning(ctx context.Context) (bool, error)
}

// Options configures an HTTP client.
type Options struct {
	// BaseURL is the root URL of the Jellyfin server.
	BaseURL string
	// APIKey is a Jellyfin API key with library read access.
	APIKey string
	// UserID scopes library browsing to a Jellyfin user. Optional.
	UserID string
	// HTTPClient overrides the default HTTP client (used by tests).
	HTTPClient *http.Client
}

// HTTPClient is the live Jellyfin client.
type HTTPClient struct {
	baseURL string
	apiKey  string
	userID  string
	http    *http.Client
}

var _ Client = (*HTTPClient)(nil)
var _ LibraryAdmin = (*HTTPClient)(nil)
var _ CollectionAdmin = (*HTTPClient)(nil)

// New builds a Jellyfin client. The base URL's trailing slash is optional.
func New(opts Options) (*HTTPClient, error) {
	base := strings.TrimSuffix(opts.BaseURL, "/")
	if base == "" {
		return nil, errors.New("jellyfin: base url is required")
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("jellyfin: parse base url: %w", err)
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPClient{baseURL: base, apiKey: opts.APIKey, userID: opts.UserID, http: httpClient}, nil
}

// Ping verifies connectivity and credentials against the system info endpoint.
func (c *HTTPClient) Ping(ctx context.Context) error {
	var out struct {
		Version string `json:"Version"`
	}
	return c.get(ctx, "/System/Info", nil, &out)
}

// itemsResponse is the envelope Jellyfin returns from /Items.
type itemsResponse struct {
	Items            []itemDTO `json:"Items"`
	TotalRecordCount int       `json:"TotalRecordCount"`
}

// itemDTO mirrors the Jellyfin BaseItemDto fields the TV server reads.
type itemDTO struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	SortName          string            `json:"SortName"`
	Overview          string            `json:"Overview"`
	Type              string            `json:"Type"`
	RunTimeTicks      int64             `json:"RunTimeTicks"`
	Container         string            `json:"Container"`
	Path              string            `json:"Path"`
	ProviderIds       map[string]string `json:"ProviderIds"`
	IndexNumber       int               `json:"IndexNumber"`
	ParentIndexNumber int               `json:"ParentIndexNumber"`
	SeriesName        string            `json:"SeriesName"`
	SeriesID          string            `json:"SeriesId"`
	ParentID          string            `json:"ParentId"`
	ImageTags         map[string]string `json:"ImageTags"`
	MediaStreams      []struct {
		Type  string `json:"Type"`
		Codec string `json:"Codec"`
	} `json:"MediaStreams"`
}

func (d itemDTO) toItem() Item {
	item := Item{
		ID:                d.ID,
		Name:              d.Name,
		SortName:          d.SortName,
		Overview:          d.Overview,
		Type:              d.Type,
		Runtime:           time.Duration(d.RunTimeTicks) * tickDuration,
		Container:         d.Container,
		Path:              d.Path,
		ProviderIds:       d.ProviderIds,
		IndexNumber:       d.IndexNumber,
		ParentIndexNumber: d.ParentIndexNumber,
		SeriesName:        d.SeriesName,
		SeriesID:          d.SeriesID,
		ParentID:          d.ParentID,
		ImageTag:          d.ImageTags["Primary"],
	}
	for _, s := range d.MediaStreams {
		switch s.Type {
		case "Video":
			if item.VideoCodec == "" {
				item.VideoCodec = s.Codec
			}
		case "Audio":
			if item.AudioCodec == "" {
				item.AudioCodec = s.Codec
			}
		}
	}
	return item
}

// EpisodeKey returns the SxxEyy key for an episode item, or "" if not an episode.
func (i Item) EpisodeKey() string {
	if i.Type != "Episode" || i.ParentIndexNumber == 0 && i.IndexNumber == 0 {
		return ""
	}
	return fmt.Sprintf("S%02dE%02d", i.ParentIndexNumber, i.IndexNumber)
}

// ProviderID returns a provider value by key (case-insensitive), or "".
func (i Item) ProviderID(key string) string {
	if i.ProviderIds == nil {
		return ""
	}
	if v, ok := i.ProviderIds[key]; ok {
		return v
	}
	for k, v := range i.ProviderIds {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// Browse lists library items available for import.
func (c *HTTPClient) Browse(ctx context.Context, p BrowseParams) ([]Item, error) {
	page, err := c.BrowsePage(ctx, p)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// BrowsePage lists one /Items page and reports Jellyfin's unfiltered
// TotalRecordCount plus the raw (pre-filter) page length so callers can
// paginate even when PathContains / provider filters empty the first page.
func (c *HTTPClient) BrowsePage(ctx context.Context, p BrowseParams) (Page, error) {
	q := url.Values{}
	q.Set("Recursive", "true")
	types := p.IncludeItemTypes
	if types == "" {
		types = "Movie,Episode,Video"
	}
	q.Set("IncludeItemTypes", types)
	// Always pull ProviderIds when filtering by them; Path when path-matching.
	fields := []string{"Overview", "MediaStreams", "SortName", "Container"}
	if p.IncludeProviderIDs || p.AnyProviderIDEquals != "" {
		fields = append(fields, "ProviderIds")
	}
	if p.IncludePath || p.PathContains != "" {
		fields = append(fields, "Path")
	}
	q.Set("Fields", strings.Join(fields, ","))
	parentID := p.ParentID
	if parentID == "" && p.SeriesID != "" {
		// 10.11 /Items has no seriesId parameter. ParentId + Recursive is the
		// supported series-scoped listing; SeriesId is never sent as a query.
		parentID = p.SeriesID
	}
	if parentID != "" {
		q.Set("ParentId", parentID)
	}
	if p.SearchTerm != "" {
		q.Set("SearchTerm", p.SearchTerm)
	}
	if p.AnyProviderIDEquals != "" {
		// Hint only — response is always re-filtered client-side below.
		q.Set("AnyProviderIdEquals", p.AnyProviderIDEquals)
	}
	if p.Limit > 0 {
		q.Set("Limit", strconv.Itoa(p.Limit))
	}
	if p.StartIndex > 0 {
		q.Set("StartIndex", strconv.Itoa(p.StartIndex))
	}
	if c.userID != "" {
		q.Set("UserId", c.userID)
	}

	var resp itemsResponse
	if err := c.get(ctx, "/Items", q, &resp); err != nil {
		return Page{}, err
	}
	raw := make([]Item, 0, len(resp.Items))
	for _, d := range resp.Items {
		raw = append(raw, d.toItem())
	}
	items := make([]Item, 0, len(raw))
	for _, it := range raw {
		if p.PathContains != "" && !strings.Contains(it.Path, p.PathContains) {
			continue
		}
		if p.AnyProviderIDEquals != "" && !ProviderMatch(it, p.AnyProviderIDEquals) {
			// Jellyfin's AnyProviderIdEquals is not reliable across versions;
			// never accept an item that does not actually carry the id.
			continue
		}
		if p.SeriesID != "" && !seriesMatch(it, p.SeriesID) {
			// Fail closed if the server ignored ParentId or omitted SeriesId.
			continue
		}
		items = append(items, it)
	}
	total := resp.TotalRecordCount
	if total == 0 && len(raw) > 0 {
		total = len(raw)
	}
	return Page{Items: items, TotalRecordCount: total, RawPageLen: len(raw)}, nil
}

// seriesMatch reports whether an item belongs to seriesID. Blank Item.SeriesID
// never matches: a server that omits SeriesId cannot bypass the filter.
func seriesMatch(it Item, seriesID string) bool {
	if seriesID == "" || it.SeriesID == "" {
		return false
	}
	return it.SeriesID == seriesID
}

// ProviderMatch checks the "Key=Value|…" form against an item's ProviderIds.
// Bare values (no '=') match any provider id equal to the value. Exported so
// the reconciler can re-check after multi-page scans.
func ProviderMatch(it Item, expr string) bool {
	matched := false
	for _, part := range strings.Split(expr, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		matched = true
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			for _, v := range it.ProviderIds {
				if v == part {
					return true
				}
			}
			continue
		}
		if it.ProviderID(strings.TrimSpace(key)) == strings.TrimSpace(val) {
			return true
		}
	}
	return !matched
}

// RefreshLibrary triggers a full Jellyfin library scan.
func (c *HTTPClient) RefreshLibrary(ctx context.Context) error {
	return c.post(ctx, "/Library/Refresh", nil, nil)
}

// CreateCollection POSTs /Collections with the collection name as a query param.
// ids, when non-empty, are seeded on create (Jellyfin accepts a comma-delimited ids query).
func (c *HTTPClient) CreateCollection(ctx context.Context, name string, ids []string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("jellyfin: collection name is required")
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("isLocked", "true") // prevent external metadata from renaming our grouping
	if cleaned := uniqueIDs(ids); len(cleaned) > 0 {
		q.Set("ids", strings.Join(cleaned, ","))
	}
	var out struct {
		ID string `json:"Id"`
	}
	if err := c.post(ctx, "/Collections", q, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("jellyfin: create collection: empty id")
	}
	return out.ID, nil
}

// AddToCollection POSTs /Collections/{id}/Items. Empty ids is a no-op.
func (c *HTTPClient) AddToCollection(ctx context.Context, collectionID string, ids []string) error {
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return errors.New("jellyfin: collection id is required")
	}
	cleaned := uniqueIDs(ids)
	if len(cleaned) == 0 {
		return nil
	}
	// IDs are query parameters in Jellyfin's public API. Bound each URI to
	// comfortably fit proxy request-line limits, even for thousands of videos.
	// On partial failure, a replay re-lists durable members before adding more.
	const batchSize = 100
	for start := 0; start < len(cleaned); start += batchSize {
		end := min(start+batchSize, len(cleaned))
		q := url.Values{}
		q.Set("ids", strings.Join(cleaned[start:end], ","))
		if err := c.post(ctx, "/Collections/"+url.PathEscape(collectionID)+"/Items", q, nil); err != nil {
			return fmt.Errorf("collection batch %d: %w", start/batchSize+1, err)
		}
	}
	return nil
}

func uniqueIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ScanRunning reports whether any scheduled task named like a library scan is running.
func (c *HTTPClient) ScanRunning(ctx context.Context) (bool, error) {
	var tasks []struct {
		Name  string `json:"Name"`
		State string `json:"State"`
		Key   string `json:"Key"`
	}
	if err := c.get(ctx, "/ScheduledTasks", nil, &tasks); err != nil {
		return false, err
	}
	for _, t := range tasks {
		if !strings.EqualFold(t.State, "Running") {
			continue
		}
		// Media-segment/trickplay scans can run for hours; only the actual
		// library refresh task establishes import visibility.
		if strings.EqualFold(t.Key, "RefreshLibrary") || (t.Key == "" && strings.EqualFold(t.Name, "Scan Media Library")) {
			return true, nil
		}
	}
	return false, nil
}

// Item fetches metadata for a single item.
func (c *HTTPClient) Item(ctx context.Context, id string) (*Item, error) {
	got, err := c.ItemsByID(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	item, ok := got[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &item, nil
}

// ItemsByID fetches metadata for many items in one GET /Items?Ids=id1,id2 request.
// Limit is the requested id count so Jellyfin cannot silently truncate the page.
// Extra IDs in the response (foreign/unrequested) are an error.
func (c *HTTPClient) ItemsByID(ctx context.Context, ids []string) (map[string]Item, error) {
	wanted := uniqueIDs(ids)
	if len(wanted) == 0 {
		return map[string]Item{}, nil
	}
	want := make(map[string]struct{}, len(wanted))
	for _, id := range wanted {
		want[id] = struct{}{}
	}
	q := url.Values{}
	q.Set("Ids", strings.Join(wanted, ","))
	q.Set("Limit", strconv.Itoa(len(wanted)))
	q.Set("Fields", "Overview,MediaStreams,SortName,Container")
	if c.userID != "" {
		q.Set("UserId", c.userID)
	}

	var resp itemsResponse
	if err := c.get(ctx, "/Items", q, &resp); err != nil {
		return nil, err
	}
	if resp.TotalRecordCount > 0 && resp.TotalRecordCount != len(resp.Items) {
		return nil, fmt.Errorf("jellyfin: incomplete batch response: got %d of %d items", len(resp.Items), resp.TotalRecordCount)
	}
	out := make(map[string]Item, len(resp.Items))
	for _, d := range resp.Items {
		it := d.toItem()
		if it.ID == "" {
			return nil, fmt.Errorf("jellyfin: batch response item has no id")
		}
		if _, duplicate := out[it.ID]; duplicate {
			return nil, fmt.Errorf("jellyfin: batch response repeats id %s", it.ID)
		}
		if _, ok := want[it.ID]; !ok {
			return nil, fmt.Errorf("jellyfin: items response included unrequested id %s", it.ID)
		}
		out[it.ID] = it
	}
	return out, nil
}

// StreamURL builds a direct-play URL. Transcoding is explicitly disabled: the
// target hardware cannot keep up with a Jellyfin transcode, so an
// incompatible file must fail loudly rather than silently transcode.
func (c *HTTPClient) StreamURL(itemID string) string {
	q := url.Values{}
	q.Set("static", "true")
	q.Set("api_key", c.apiKey)
	return fmt.Sprintf("%s/Videos/%s/stream?%s", c.baseURL, url.PathEscape(itemID), q.Encode())
}

// ImageURL builds an artwork URL. An empty imageType defaults to Primary.
func (c *HTTPClient) ImageURL(itemID, imageType, tag string) string {
	if imageType == "" {
		imageType = "Primary"
	}
	u := fmt.Sprintf("%s/Items/%s/Images/%s", c.baseURL, url.PathEscape(itemID), url.PathEscape(imageType))
	if tag != "" {
		u += "?" + url.Values{"tag": []string{tag}}.Encode()
	}
	return u
}

// FetchImage retrieves artwork bytes from Jellyfin.
func (c *HTTPClient) FetchImage(ctx context.Context, itemID, imageType, tag string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ImageURL(itemID, imageType, tag), nil)
	if err != nil {
		return nil, "", fmt.Errorf("jellyfin: build image request: %w", err)
	}
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("jellyfin: fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrNotFound
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", fmt.Errorf("jellyfin: fetch image: unexpected status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("jellyfin: read image: %w", err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}

// authorize applies the API key both as a header and as a query parameter,
// covering the two schemes Jellyfin accepts across versions.
func (c *HTTPClient) authorize(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	req.Header.Set("X-Emby-Token", c.apiKey)
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Client="primer-tv", Device="tv-server", DeviceId="tv-server", Version="1", Token="%s"`, c.apiKey))
}

func (c *HTTPClient) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, out)
}

func (c *HTTPClient) post(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodPost, path, query, out)
}

func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return fmt.Errorf("jellyfin: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("jellyfin: request %s: unexpected status %d", path, resp.StatusCode)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	// Some POST endpoints return an empty body on 204/200.
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("jellyfin: decode %s: %w", path, err)
	}
	return nil
}
