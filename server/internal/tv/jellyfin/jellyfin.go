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
}

// RuntimeSeconds returns the item's duration in whole seconds.
func (i Item) RuntimeSeconds() int { return int(i.Runtime.Seconds()) }

// BrowseParams filters a library listing.
type BrowseParams struct {
	// ParentID restricts results to one library folder or collection.
	ParentID string
	// SearchTerm filters by title.
	SearchTerm string
	// Limit caps the number of returned items.
	Limit int
	// StartIndex is the offset for paging.
	StartIndex int
}

// Client is the Jellyfin behaviour the TV server depends on. It is an
// interface so tests can substitute a fake without an HTTP server.
type Client interface {
	// Ping verifies connectivity and credentials.
	Ping(ctx context.Context) error
	// Browse lists library items available for import.
	Browse(ctx context.Context, p BrowseParams) ([]Item, error)
	// Item fetches metadata for a single item.
	Item(ctx context.Context, id string) (*Item, error)
	// StreamURL builds a direct-play URL for an item.
	StreamURL(itemID string) string
	// ImageURL builds an artwork URL for an item.
	ImageURL(itemID, imageType, tag string) string
	// FetchImage retrieves artwork bytes so the TV server can proxy artwork
	// without handing Jellyfin credentials to the client.
	FetchImage(ctx context.Context, itemID, imageType, tag string) (data []byte, contentType string, err error)
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
	Items []itemDTO `json:"Items"`
}

// itemDTO mirrors the Jellyfin BaseItemDto fields the TV server reads.
type itemDTO struct {
	ID           string            `json:"Id"`
	Name         string            `json:"Name"`
	SortName     string            `json:"SortName"`
	Overview     string            `json:"Overview"`
	Type         string            `json:"Type"`
	RunTimeTicks int64             `json:"RunTimeTicks"`
	Container    string            `json:"Container"`
	ImageTags    map[string]string `json:"ImageTags"`
	MediaStreams []struct {
		Type  string `json:"Type"`
		Codec string `json:"Codec"`
	} `json:"MediaStreams"`
}

func (d itemDTO) toItem() Item {
	item := Item{
		ID:        d.ID,
		Name:      d.Name,
		SortName:  d.SortName,
		Overview:  d.Overview,
		Type:      d.Type,
		Runtime:   time.Duration(d.RunTimeTicks) * tickDuration,
		Container: d.Container,
		ImageTag:  d.ImageTags["Primary"],
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

// Browse lists library items available for import.
func (c *HTTPClient) Browse(ctx context.Context, p BrowseParams) ([]Item, error) {
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Movie,Episode,Video")
	q.Set("Fields", "Overview,MediaStreams,SortName,Container")
	if p.ParentID != "" {
		q.Set("ParentId", p.ParentID)
	}
	if p.SearchTerm != "" {
		q.Set("SearchTerm", p.SearchTerm)
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
		return nil, err
	}
	items := make([]Item, 0, len(resp.Items))
	for _, d := range resp.Items {
		items = append(items, d.toItem())
	}
	return items, nil
}

// Item fetches metadata for a single item.
func (c *HTTPClient) Item(ctx context.Context, id string) (*Item, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	q := url.Values{}
	q.Set("Ids", id)
	q.Set("Recursive", "true")
	q.Set("Fields", "Overview,MediaStreams,SortName,Container")
	if c.userID != "" {
		q.Set("UserId", c.userID)
	}

	var resp itemsResponse
	if err := c.get(ctx, "/Items", q, &resp); err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, ErrNotFound
	}
	item := resp.Items[0].toItem()
	return &item, nil
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
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jellyfin: decode %s: %w", path, err)
	}
	return nil
}
