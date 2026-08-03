// Package sonarr is a thin client for the Sonarr v3 API: series lookup, add,
// tag management, episode listing, and monitor toggles for excluded episodes.
package sonarr

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

// ErrNotConfigured reports that no Sonarr base URL was set.
var ErrNotConfigured = errors.New("sonarr base url is not configured")

// Series is the subset of a Sonarr series the reconciler cares about.
type Series struct {
	ID               int      `json:"id"`
	Title            string   `json:"title"`
	Year             int      `json:"year"`
	TvdbID           int      `json:"tvdbId"`
	TmdbID           int      `json:"tmdbId"`
	Overview         string   `json:"overview"`
	TitleSlug        string   `json:"titleSlug,omitempty"`
	Monitored        bool     `json:"monitored"`
	QualityProfileID int      `json:"qualityProfileId"`
	RootFolderPath   string   `json:"rootFolderPath"`
	SeasonFolder     bool     `json:"seasonFolder"`
	Images           []Image  `json:"images,omitempty"`
	Seasons          []Season `json:"seasons,omitempty"`
	// Statistics is populated on list responses.
	Statistics *Statistics `json:"statistics,omitempty"`
}

// Statistics holds episode counts for a series.
type Statistics struct {
	EpisodeFileCount int `json:"episodeFileCount"`
	EpisodeCount     int `json:"episodeCount"`
}

// Season is one season of a series.
type Season struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
}

// Image is artwork carried from lookup to add.
type Image struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url"`
	RemoteURL string `json:"remoteUrl,omitempty"`
}

// Episode is one episode of a series.
type Episode struct {
	ID            int    `json:"id"`
	SeriesID      int    `json:"seriesId"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Title         string `json:"title"`
	Monitored     bool   `json:"monitored"`
	HasFile       bool   `json:"hasFile"`
}

// Key returns the SxxEyy identifier used by exclude_episodes.
func (e Episode) Key() string {
	return fmt.Sprintf("S%02dE%02d", e.SeasonNumber, e.EpisodeNumber)
}

// Tag is a Sonarr tag.
type Tag struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// Client is the Sonarr behaviour the reconciler depends on.
type Client interface {
	Lookup(ctx context.Context, term string) ([]Series, error)
	List(ctx context.Context) ([]Series, error)
	Add(ctx context.Context, series Series, qualityProfileID int, rootFolder string, tagIDs []int) (*Series, error)
	EnsureTag(ctx context.Context, label string) (int, error)
	Episodes(ctx context.Context, seriesID int) ([]Episode, error)
	// SetEpisodeMonitored toggles monitoring on one episode.
	SetEpisodeMonitored(ctx context.Context, episodeID int, monitored bool) error
}

// Options configures an HTTP client.
type Options struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// HTTPClient is the live Sonarr client.
type HTTPClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

var _ Client = (*HTTPClient)(nil)

// New builds a Sonarr client. Empty base URL returns ErrNotConfigured.
func New(opts Options) (*HTTPClient, error) {
	base := strings.TrimSuffix(opts.BaseURL, "/")
	if base == "" {
		return nil, ErrNotConfigured
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("sonarr: parse base url: %w", err)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &HTTPClient{baseURL: base, apiKey: opts.APIKey, http: httpClient}, nil
}

// Lookup searches for series by title.
func (c *HTTPClient) Lookup(ctx context.Context, term string) ([]Series, error) {
	q := url.Values{"term": {term}}
	var out []Series
	if err := c.do(ctx, http.MethodGet, "/api/v3/series/lookup", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// List returns every series in the library.
func (c *HTTPClient) List(ctx context.Context) ([]Series, error) {
	var out []Series
	if err := c.do(ctx, http.MethodGet, "/api/v3/series", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type addRequest struct {
	Title            string   `json:"title"`
	Year             int      `json:"year"`
	TvdbID           int      `json:"tvdbId"`
	TitleSlug        string   `json:"titleSlug"`
	QualityProfileID int      `json:"qualityProfileId"`
	RootFolderPath   string   `json:"rootFolderPath"`
	Monitored        bool     `json:"monitored"`
	SeasonFolder     bool     `json:"seasonFolder"`
	Tags             []int    `json:"tags"`
	Images           []Image  `json:"images,omitempty"`
	Seasons          []Season `json:"seasons,omitempty"`
	AddOptions       struct {
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// Add queues a series for download from a lookup result.
func (c *HTTPClient) Add(ctx context.Context, series Series, qualityProfileID int, rootFolder string, tagIDs []int) (*Series, error) {
	if series.TvdbID == 0 {
		return nil, errors.New("sonarr: tvdb id is required to add a series")
	}
	if qualityProfileID == 0 {
		return nil, errors.New("sonarr: quality profile id is required")
	}
	if rootFolder == "" {
		return nil, errors.New("sonarr: root folder is required")
	}
	body := addRequest{
		Title:            series.Title,
		Year:             series.Year,
		TvdbID:           series.TvdbID,
		TitleSlug:        series.TitleSlug,
		QualityProfileID: qualityProfileID,
		RootFolderPath:   rootFolder,
		Monitored:        true,
		SeasonFolder:     true,
		Tags:             tagIDs,
		Images:           series.Images,
		Seasons:          series.Seasons,
	}
	body.AddOptions.SearchForMissingEpisodes = true
	var out Series
	if err := c.do(ctx, http.MethodPost, "/api/v3/series", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EnsureTag returns the id of the named tag, creating it if needed.
func (c *HTTPClient) EnsureTag(ctx context.Context, label string) (int, error) {
	var tags []Tag
	if err := c.do(ctx, http.MethodGet, "/api/v3/tag", nil, nil, &tags); err != nil {
		return 0, err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Label, label) {
			return t.ID, nil
		}
	}
	var created Tag
	if err := c.do(ctx, http.MethodPost, "/api/v3/tag", nil, Tag{Label: label}, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

// Episodes lists every episode for a series.
func (c *HTTPClient) Episodes(ctx context.Context, seriesID int) ([]Episode, error) {
	q := url.Values{"seriesId": {fmt.Sprintf("%d", seriesID)}}
	var out []Episode
	if err := c.do(ctx, http.MethodGet, "/api/v3/episode", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetEpisodeMonitored toggles monitoring on one episode.
func (c *HTTPClient) SetEpisodeMonitored(ctx context.Context, episodeID int, monitored bool) error {
	// GET the episode, flip monitored, PUT it back.
	var ep Episode
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v3/episode/%d", episodeID), nil, nil, &ep); err != nil {
		return err
	}
	ep.Monitored = monitored
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v3/episode/%d", episodeID), nil, ep, &ep)
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
			return fmt.Errorf("sonarr: encode body: %w", err)
		}
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("sonarr: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sonarr: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sonarr: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("sonarr: decode %s: %w", path, err)
	}
	return nil
}

// ByTVDB finds the series with the given TVDB id in a list, or nil.
func ByTVDB(series []Series, tvdbID int) *Series {
	for i := range series {
		if series[i].TvdbID == tvdbID {
			return &series[i]
		}
	}
	return nil
}

// FilterYear keeps lookup hits whose year matches, or all hits when year is 0.
func FilterYear(series []Series, year int) []Series {
	if year == 0 {
		return series
	}
	out := make([]Series, 0, len(series))
	for _, s := range series {
		if s.Year == year {
			out = append(out, s)
		}
	}
	return out
}
