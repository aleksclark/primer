// Package primer reports watched sessions to the Primer LMS as instructional
// time. It owns the HTTP client for the LMS ingest endpoint and the job that
// walks the TV server's own playback sessions, pushing the ones the household
// has not been credited for yet.
//
// Reporting is deliberately at-least-once. Both ends deduplicate — the TV
// server by the unique playback_session_id in primer_reports, the LMS by the
// (source, sourceRef) pair on the log — so a crash between the post and the
// bookkeeping write costs a repeated request, never a repeated hour.
package primer

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

// SourceTV is the producer name the LMS files these logs under.
const SourceTV = "tv"

// IngestPath is the LMS endpoint that accepts instruction logs.
const IngestPath = "/api/v1/instruction-logs/ingest"

// ServiceTokenHeader carries the LMS service token.
const ServiceTokenHeader = "X-Service-Token"

// DefaultTimeout bounds a single ingest call. An LMS that has stopped
// answering must not hold the reporting loop open indefinitely.
const DefaultTimeout = 15 * time.Second

// maxErrorBody caps how much of a failed response is quoted back in an error,
// so a stray HTML error page does not end up in the logs whole.
const maxErrorBody = 512

// ErrNotConfigured reports that no Primer base URL was configured, which
// leaves reporting switched off rather than failing.
var ErrNotConfigured = errors.New("primer base url is not configured")

// InstructionLog is one finished viewing as the LMS records it.
type InstructionLog struct {
	Source         string   `json:"source"`
	SourceRef      string   `json:"sourceRef"`
	MediaTitle     string   `json:"mediaTitle"`
	Class          string   `json:"class"`
	SubjectTags    []string `json:"subjectTags"`
	StandardCodes  []string `json:"standardCodes"`
	WatchedSeconds int      `json:"watchedSeconds"`
	OccurredOn     string   `json:"occurredOn"`
}

// IngestResult is the LMS's answer to an ingest.
type IngestResult struct {
	// LogID identifies the stored log, and becomes the primer_ref recorded in
	// the export ledger.
	LogID string
	// Created is false when the LMS already held this source reference.
	Created bool
}

// Ingester posts instruction logs to the LMS. The reporter depends on this
// rather than on Client so tests can drive it without a server.
type Ingester interface {
	Ingest(ctx context.Context, log InstructionLog) (*IngestResult, error)
}

// Options configures the LMS client.
type Options struct {
	// BaseURL is the root of the LMS deployment, e.g. https://primer.example.
	BaseURL string
	// ServiceToken authenticates against the LMS ingest.
	ServiceToken string
	// Timeout bounds a single request; zero selects DefaultTimeout.
	Timeout time.Duration
	// HTTPClient overrides the transport, for tests.
	HTTPClient *http.Client
}

// Client posts instruction logs to a Primer LMS.
type Client struct {
	ingestURL string
	token     string
	http      *http.Client
}

// New builds an LMS client. It returns ErrNotConfigured when no base URL is
// set, which the caller treats as "reporting is switched off" rather than as
// a failure: a household running the channel without an LMS is a supported
// configuration.
func New(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, ErrNotConfigured
	}
	base, err := url.Parse(strings.TrimSuffix(opts.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse primer base url: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("primer base url %q needs a scheme and host", opts.BaseURL)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		ingestURL: base.String() + IngestPath,
		token:     opts.ServiceToken,
		http:      httpClient,
	}, nil
}

// ingestResponse is the LMS ingest envelope.
type ingestResponse struct {
	Log struct {
		ID string `json:"id"`
	} `json:"log"`
	Created bool `json:"created"`
}

// Ingest posts one instruction log. A duplicate is not an error: the LMS
// answers 200 with created=false, which is exactly what a safe retry should
// look like.
func (c *Client) Ingest(ctx context.Context, log InstructionLog) (*IngestResult, error) {
	payload, err := json.Marshal(log)
	if err != nil {
		return nil, fmt.Errorf("encode instruction log: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ingestURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build ingest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set(ServiceTokenHeader, c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post instruction log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("primer ingest refused %s: %s: %s",
			log.SourceRef, resp.Status, strings.TrimSpace(string(detail)))
	}

	var body ingestResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode ingest response: %w", err)
	}
	return &IngestResult{LogID: body.Log.ID, Created: body.Created}, nil
}
