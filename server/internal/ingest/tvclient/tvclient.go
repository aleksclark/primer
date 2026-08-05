// Package tvclient talks to the Primer TV server admin API: media-item CRUD
// and Jellyfin metadata sync. content-ingest uses it to classify library items
// once files are on disk and scanned.
package tvclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotConfigured reports that no TV base URL was set.
var ErrNotConfigured = errors.New("tv base url is not configured")

// AdminKeyHeader is the shared-secret header the TV admin API expects.
const AdminKeyHeader = "X-Admin-Key"

// MediaItem is the subset of a TV media item the reconciler reads and writes.
type MediaItem struct {
	ID             string   `json:"id"`
	JellyfinItemID string   `json:"jellyfinItemId"`
	Title          string   `json:"title"`
	SortTitle      string   `json:"sortTitle,omitempty"`
	Overview       string   `json:"overview,omitempty"`
	Class          string   `json:"class"`
	RuntimeSeconds int      `json:"runtimeSeconds,omitempty"`
	SubjectTags    []string `json:"subjectTags,omitempty"`
	StandardCodes  []string `json:"standardCodes,omitempty"`
	Container      string   `json:"container,omitempty"`
	VideoCodec     string   `json:"videoCodec,omitempty"`
	AudioCodec     string   `json:"audioCodec,omitempty"`
	DirectPlayOK   bool     `json:"directPlayOk,omitempty"`
	ImageTag       string   `json:"imageTag,omitempty"`
}

// Manifest entry acquisition statuses (mirror TV domain).
const (
	ManifestStatusMissing = "missing"
	ManifestStatusPresent = "present"
	ManifestStatusFailed  = "failed"
	ManifestStatusManual  = "manual"
)

// ManifestEntry is one TV-persisted content-manifest row.
type ManifestEntry struct {
	ID              string   `json:"id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Year            int      `json:"year,omitempty"`
	Kind            string   `json:"kind"`
	TMDBID          int      `json:"tmdbId,omitempty"`
	TVDBID          int      `json:"tvdbId,omitempty"`
	URL             string   `json:"url,omitempty"`
	Class           string   `json:"class"`
	SubjectTags     []string `json:"subjectTags,omitempty"`
	StandardCodes   []string `json:"standardCodes,omitempty"`
	Priority        int      `json:"priority,omitempty"`
	ExcludeEpisodes []string `json:"excludeEpisodes,omitempty"`
	MaxEpisodes     int      `json:"maxEpisodes,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	Status          string   `json:"status"`
	AttemptCount    int      `json:"attemptCount"`
	LastError       string   `json:"lastError,omitempty"`
}

// ManifestDesired is one desired-state item pushed by content-ingest.
type ManifestDesired struct {
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Year            int      `json:"year,omitempty"`
	Kind            string   `json:"kind"`
	TMDBID          int      `json:"tmdbId,omitempty"`
	TVDBID          int      `json:"tvdbId,omitempty"`
	URL             string   `json:"url,omitempty"`
	Class           string   `json:"class"`
	SubjectTags     []string `json:"subjectTags,omitempty"`
	StandardCodes   []string `json:"standardCodes,omitempty"`
	Priority        int      `json:"priority,omitempty"`
	ExcludeEpisodes []string `json:"excludeEpisodes,omitempty"`
	MaxEpisodes     int      `json:"maxEpisodes,omitempty"`
	Notes           string   `json:"notes,omitempty"`
}

// ManifestSyncResult summarizes POST /content-manifest/sync.
type ManifestSyncResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Total   int `json:"total"`
}

// MediaItemCreate is the body for POST /media-items.
type MediaItemCreate struct {
	JellyfinItemID string   `json:"jellyfinItemId"`
	Title          string   `json:"title"`
	SortTitle      string   `json:"sortTitle,omitempty"`
	Overview       string   `json:"overview,omitempty"`
	Class          string   `json:"class"`
	RuntimeSeconds int      `json:"runtimeSeconds,omitempty"`
	SubjectTags    []string `json:"subjectTags,omitempty"`
	StandardCodes  []string `json:"standardCodes,omitempty"`
	Container      string   `json:"container,omitempty"`
	VideoCodec     string   `json:"videoCodec,omitempty"`
	AudioCodec     string   `json:"audioCodec,omitempty"`
	ImageTag       string   `json:"imageTag,omitempty"`
}

// MediaItemUpdate is the body for PATCH /media-items/{id}.
type MediaItemUpdate struct {
	Title         *string   `json:"title,omitempty"`
	Class         *string   `json:"class,omitempty"`
	SubjectTags   *[]string `json:"subjectTags,omitempty"`
	StandardCodes *[]string `json:"standardCodes,omitempty"`
	Overview      *string   `json:"overview,omitempty"`
	SortTitle     *string   `json:"sortTitle,omitempty"`
	Container     *string   `json:"container,omitempty"`
	VideoCodec    *string   `json:"videoCodec,omitempty"`
	AudioCodec    *string   `json:"audioCodec,omitempty"`
	ImageTag      *string   `json:"imageTag,omitempty"`
}

// SyncResult summarizes POST /jellyfin/sync.
type SyncResult struct {
	Checked  int      `json:"checked"`
	Updated  int      `json:"updated"`
	Orphaned []string `json:"orphaned"`
}

// listEnvelope is the generic list response from the TV admin API.
type listEnvelope[T any] struct {
	Items      []T `json:"items"`
	TotalCount int `json:"totalCount"`
	// Total is accepted for older fakes/tests that used the short name.
	Total int `json:"total"`
}

// Client is the TV admin behaviour the reconciler depends on.
type Client interface {
	// ListMediaItems returns every media item (paged internally).
	ListMediaItems(ctx context.Context) ([]MediaItem, error)
	// CreateMediaItem imports a Jellyfin item.
	CreateMediaItem(ctx context.Context, in MediaItemCreate) (*MediaItem, error)
	// UpdateMediaItem patches classification fields.
	UpdateMediaItem(ctx context.Context, id string, in MediaItemUpdate) (*MediaItem, error)
	// SyncJellyfin refreshes cached metadata for already-imported items.
	SyncJellyfin(ctx context.Context) (*SyncResult, error)
	// SyncManifest upserts desired-state catalog rows from the YAML manifest.
	SyncManifest(ctx context.Context, items []ManifestDesired) (*ManifestSyncResult, error)
	// ListManifestEntries returns every content-manifest row (paged).
	ListManifestEntries(ctx context.Context) ([]ManifestEntry, error)
	// RecordManifestAttempt increments attempt counters for a missing slug.
	RecordManifestAttempt(ctx context.Context, slug, lastError string) (*ManifestEntry, error)
	// MarkManifestPresent records that the slug is available in Jellyfin.
	MarkManifestPresent(ctx context.Context, slug string) (*ManifestEntry, error)
}

// Options configures an HTTP client.
type Options struct {
	// BaseURL is the TV API root including the /api/v1 prefix, e.g.
	// http://localhost:8081/api/v1.
	BaseURL    string
	AdminKey   string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// HTTPClient is the live TV admin client.
type HTTPClient struct {
	baseURL  string
	adminKey string
	http     *http.Client
}

var _ Client = (*HTTPClient)(nil)

// New builds a TV admin client. Empty base URL returns ErrNotConfigured.
func New(opts Options) (*HTTPClient, error) {
	base := strings.TrimSuffix(opts.BaseURL, "/")
	if base == "" {
		return nil, ErrNotConfigured
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("tvclient: parse base url: %w", err)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &HTTPClient{baseURL: base, adminKey: opts.AdminKey, http: httpClient}, nil
}

// ListMediaItems pages through every media item.
func (c *HTTPClient) ListMediaItems(ctx context.Context) ([]MediaItem, error) {
	return listAll[MediaItem](ctx, c, "/media-items")
}

// CreateMediaItem imports a Jellyfin item.
func (c *HTTPClient) CreateMediaItem(ctx context.Context, in MediaItemCreate) (*MediaItem, error) {
	var out MediaItem
	if err := c.do(ctx, http.MethodPost, "/media-items", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMediaItem patches classification fields.
func (c *HTTPClient) UpdateMediaItem(ctx context.Context, id string, in MediaItemUpdate) (*MediaItem, error) {
	var out MediaItem
	if err := c.do(ctx, http.MethodPatch, "/media-items/"+url.PathEscape(id), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SyncJellyfin refreshes cached metadata.
func (c *HTTPClient) SyncJellyfin(ctx context.Context) (*SyncResult, error) {
	var out SyncResult
	if err := c.do(ctx, http.MethodPost, "/jellyfin/sync", nil, map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SyncManifest upserts desired-state catalog rows.
func (c *HTTPClient) SyncManifest(ctx context.Context, items []ManifestDesired) (*ManifestSyncResult, error) {
	if items == nil {
		items = []ManifestDesired{}
	}
	var out ManifestSyncResult
	if err := c.do(ctx, http.MethodPost, "/content-manifest/sync", nil, map[string]any{"items": items}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListManifestEntries pages through every content-manifest row.
func (c *HTTPClient) ListManifestEntries(ctx context.Context) ([]ManifestEntry, error) {
	return listAll[ManifestEntry](ctx, c, "/content-manifest-entries")
}

// RecordManifestAttempt increments attempt counters for a missing slug.
func (c *HTTPClient) RecordManifestAttempt(ctx context.Context, slug, lastError string) (*ManifestEntry, error) {
	var out ManifestEntry
	body := map[string]any{}
	if lastError != "" {
		body["error"] = lastError
	}
	path := "/content-manifest-entries/" + url.PathEscape(slug) + "/attempt"
	if err := c.do(ctx, http.MethodPost, path, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MarkManifestPresent records that the slug is available in Jellyfin.
func (c *HTTPClient) MarkManifestPresent(ctx context.Context, slug string) (*ManifestEntry, error) {
	var out ManifestEntry
	path := "/content-manifest-entries/" + url.PathEscape(slug) + "/present"
	if err := c.do(ctx, http.MethodPost, path, nil, map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func listAll[T any](ctx context.Context, c *HTTPClient, path string) ([]T, error) {
	const pageSize = 200
	var all []T
	for offset := 0; ; offset += pageSize {
		q := url.Values{
			"limit":  {fmt.Sprintf("%d", pageSize)},
			"offset": {fmt.Sprintf("%d", offset)},
		}
		var env listEnvelope[T]
		if err := c.do(ctx, http.MethodGet, path, q, nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Items...)
		total := env.TotalCount
		if total == 0 {
			total = env.Total
		}
		if len(env.Items) < pageSize || (total > 0 && len(all) >= total) {
			break
		}
	}
	return all, nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("tvclient: encode body: %w", err)
		}
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("tvclient: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.adminKey != "" {
		req.Header.Set(AdminKeyHeader, c.adminKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tvclient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tvclient: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("tvclient: decode %s: %w", path, err)
	}
	return nil
}

// ByJellyfinID indexes media items by jellyfin item id.
func ByJellyfinID(items []MediaItem) map[string]MediaItem {
	out := make(map[string]MediaItem, len(items))
	for _, it := range items {
		out[it.JellyfinItemID] = it
	}
	return out
}

// ClassificationChanged reports whether class/tags/codes differ from desired.
func ClassificationChanged(existing MediaItem, class string, tags, codes []string) bool {
	if existing.Class != class {
		return true
	}
	if !stringSlicesEqual(existing.SubjectTags, tags) {
		return true
	}
	if !stringSlicesEqual(existing.StandardCodes, codes) {
		return true
	}
	return false
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
