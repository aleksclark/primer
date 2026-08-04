// Package radarr is a thin client for the Radarr v3 API: movie lookup, add,
// tag management, and presence checks. Only the endpoints content-ingest needs.
package radarr

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

// ErrNotConfigured reports that no Radarr base URL was set.
var ErrNotConfigured = errors.New("radarr base url is not configured")

// Movie is the subset of a Radarr movie the reconciler cares about.
type Movie struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	TmdbID           int    `json:"tmdbId"`
	ImdbID           string `json:"imdbId"`
	Overview         string `json:"overview"`
	HasFile          bool   `json:"hasFile"`
	Monitored        bool   `json:"monitored"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	// TitleSlug and Images are required when adding from a lookup result.
	TitleSlug string  `json:"titleSlug,omitempty"`
	Images    []Image `json:"images,omitempty"`
}

// Image is a Radarr poster/fanart reference carried through from lookup to add.
type Image struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url"`
	RemoteURL string `json:"remoteUrl,omitempty"`
}

// Tag is a Radarr tag.
type Tag struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// Client is the Radarr behaviour the reconciler depends on.
type Client interface {
	// Lookup searches Radarr's indexer for movies matching term.
	Lookup(ctx context.Context, term string) ([]Movie, error)
	// List returns every movie already in the library.
	List(ctx context.Context) ([]Movie, error)
	// Add queues a movie for download. The movie should come from Lookup.
	Add(ctx context.Context, movie Movie, qualityProfileID int, rootFolder string, tagIDs []int) (*Movie, error)
	// EnsureTag returns the id of the named tag, creating it if needed.
	EnsureTag(ctx context.Context, label string) (int, error)
}

// Options configures an HTTP client.
type Options struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// HTTPClient is the live Radarr client.
type HTTPClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

var _ Client = (*HTTPClient)(nil)

// New builds a Radarr client. Empty base URL returns ErrNotConfigured.
func New(opts Options) (*HTTPClient, error) {
	base := strings.TrimSuffix(opts.BaseURL, "/")
	if base == "" {
		return nil, ErrNotConfigured
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("radarr: parse base url: %w", err)
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

// Lookup searches for movies by title (and optional year in the term).
func (c *HTTPClient) Lookup(ctx context.Context, term string) ([]Movie, error) {
	q := url.Values{"term": {term}}
	var out []Movie
	if err := c.do(ctx, http.MethodGet, "/api/v3/movie/lookup", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// List returns every movie in the library.
func (c *HTTPClient) List(ctx context.Context) ([]Movie, error) {
	var out []Movie
	if err := c.do(ctx, http.MethodGet, "/api/v3/movie", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// addRequest is the body Radarr expects when adding a movie.
type addRequest struct {
	Title            string  `json:"title"`
	Year             int     `json:"year"`
	TmdbID           int     `json:"tmdbId"`
	TitleSlug        string  `json:"titleSlug"`
	QualityProfileID int     `json:"qualityProfileId"`
	RootFolderPath   string  `json:"rootFolderPath"`
	Monitored        bool    `json:"monitored"`
	Tags             []int   `json:"tags"`
	Images           []Image `json:"images,omitempty"`
	AddOptions       struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// Add queues a movie for download from a lookup result.
func (c *HTTPClient) Add(ctx context.Context, movie Movie, qualityProfileID int, rootFolder string, tagIDs []int) (*Movie, error) {
	if movie.TmdbID == 0 {
		return nil, errors.New("radarr: tmdb id is required to add a movie")
	}
	if qualityProfileID == 0 {
		return nil, errors.New("radarr: quality profile id is required")
	}
	if rootFolder == "" {
		return nil, errors.New("radarr: root folder is required")
	}
	body := addRequest{
		Title:            movie.Title,
		Year:             movie.Year,
		TmdbID:           movie.TmdbID,
		TitleSlug:        movie.TitleSlug,
		QualityProfileID: qualityProfileID,
		RootFolderPath:   rootFolder,
		Monitored:        true,
		Tags:             tagIDs,
		Images:           movie.Images,
	}
	body.AddOptions.SearchForMovie = true
	var out Movie
	if err := c.do(ctx, http.MethodPost, "/api/v3/movie", nil, body, &out); err != nil {
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

func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("radarr: encode body: %w", err)
		}
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("radarr: build request: %w", err)
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
		return fmt.Errorf("radarr: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("radarr: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("radarr: decode %s: %w", path, err)
	}
	return nil
}

// ByTMDB finds the movie with the given TMDB id in a list, or nil.
func ByTMDB(movies []Movie, tmdbID int) *Movie {
	for i := range movies {
		if movies[i].TmdbID == tmdbID {
			return &movies[i]
		}
	}
	return nil
}

// FilterYear keeps lookup hits whose year matches, or all hits when year is 0.
func FilterYear(movies []Movie, year int) []Movie {
	if year == 0 {
		return movies
	}
	out := make([]Movie, 0, len(movies))
	for _, m := range movies {
		if m.Year == year {
			out = append(out, m)
		}
	}
	return out
}
