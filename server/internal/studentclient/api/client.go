// Package api is a typed HTTP client for the Primer student work API.
//
// Callers own retries; methods are idempotent-friendly and return typed
// errors for status codes the client should handle specially (401 revoked).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Device token auth headers accepted by the server.
const (
	HeaderAuthorization = "Authorization"
	HeaderDeviceToken   = "X-Device-Token"
)

// Client talks to the student API. BaseURL should include any path prefix
// (e.g. http://host/api/v1). Trailing slashes are stripped.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.HTTPClient = c }
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(cl *Client) { cl.UserAgent = ua }
}

// New builds a Client. token may be empty until Pair succeeds or SetToken.
func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		UserAgent: "primer-student/0.1",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// SetToken updates the device bearer token.
func (c *Client) SetToken(token string) { c.Token = token }

// ErrUnauthorized is returned on 401 (missing/unknown/revoked device token).
type ErrUnauthorized struct {
	Message string
}

func (e *ErrUnauthorized) Error() string {
	if e.Message == "" {
		return "unauthorized"
	}
	return e.Message
}

// ErrHTTP is a non-2xx response that is not specifically typed.
type ErrHTTP struct {
	StatusCode int
	Body       string
	Op         string
}

func (e *ErrHTTP) Error() string {
	msg := e.Body
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Op, e.StatusCode, msg)
}

// PairRequest is the body for POST /student-devices/pair.
type PairRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"deviceName,omitempty"`
}

// PairResponse is returned once at pairing time.
type PairResponse struct {
	DeviceID string               `json:"deviceId"`
	Token    string               `json:"token"`
	Student  domain.Student       `json:"student"`
	Device   domain.StudentDevice `json:"device"`
}

// StudentProfile is GET /student/profile.
type StudentProfile struct {
	Student    domain.Student `json:"student"`
	DeviceID   string         `json:"deviceId"`
	DeviceName string         `json:"deviceName"`
}

// WorkItem is one assignment plus revision payload in the work queue.
type WorkItem struct {
	Assignment domain.StudentAssignment        `json:"assignment"`
	Revision   domain.LearningActivityRevision `json:"revision"`
	Activity   domain.LearningActivity         `json:"activity"`
}

// WorkResponse is GET /student/work.
type WorkResponse struct {
	Items  []WorkItem `json:"items"`
	Cursor string     `json:"cursor,omitempty"`
}

// StartSessionRequest is POST /student/sessions.
type StartSessionRequest struct {
	ClientSessionID string `json:"clientSessionId"`
	AssignmentID    string `json:"assignmentId"`
}

// EventsRequest is POST /student/sessions/{id}/events.
type EventsRequest struct {
	Events []contracts.SessionEvent `json:"events"`
}

// EventsAck is the events ingest response.
type EventsAck struct {
	AcknowledgedSequence int64 `json:"acknowledgedSequence"`
}

// Pair exchanges a pairing code for a device token and stores it on the client.
func (c *Client) Pair(ctx context.Context, code, deviceName string) (*PairResponse, error) {
	var out PairResponse
	if err := c.doJSON(ctx, http.MethodPost, "/student-devices/pair", false, PairRequest{
		Code:       code,
		DeviceName: deviceName,
	}, &out); err != nil {
		return nil, err
	}
	if out.Token != "" {
		c.Token = out.Token
	}
	return &out, nil
}

// Profile returns the paired student identity.
func (c *Client) Profile(ctx context.Context) (*StudentProfile, error) {
	var out StudentProfile
	if err := c.doJSON(ctx, http.MethodGet, "/student/profile", true, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Work returns the student work queue. after is an opaque cursor from a prior response.
func (c *Client) Work(ctx context.Context, after string, limit int) (*WorkResponse, error) {
	q := url.Values{}
	if after != "" {
		q.Set("after", after)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/student/work"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out WorkResponse
	if err := c.doJSON(ctx, http.MethodGet, path, true, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartSession starts or resumes a learning session.
func (c *Client) StartSession(ctx context.Context, clientSessionID, assignmentID string) (*domain.LearningSession, error) {
	var out domain.LearningSession
	if err := c.doJSON(ctx, http.MethodPost, "/student/sessions", true, StartSessionRequest{
		ClientSessionID: clientSessionID,
		AssignmentID:    assignmentID,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PostEvents sends an idempotent event batch.
func (c *Client) PostEvents(ctx context.Context, sessionID string, events []contracts.SessionEvent) (*EventsAck, error) {
	var out EventsAck
	path := "/student/sessions/" + url.PathEscape(sessionID) + "/events"
	if err := c.doJSON(ctx, http.MethodPost, path, true, EventsRequest{Events: events}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PostArtifact registers artifact metadata for a session.
func (c *Client) PostArtifact(ctx context.Context, sessionID string, meta contracts.ArtifactMeta) (*domain.LearningSessionArtifact, error) {
	var out domain.LearningSessionArtifact
	path := "/student/sessions/" + url.PathEscape(sessionID) + "/artifacts"
	if err := c.doJSON(ctx, http.MethodPost, path, true, meta, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Complete posts an idempotent completion request.
func (c *Client) Complete(ctx context.Context, sessionID string, req contracts.CompletionRequest) (*contracts.CompletionResult, error) {
	var out contracts.CompletionResult
	path := "/student/sessions/" + url.PathEscape(sessionID) + "/complete"
	if err := c.doJSON(ctx, http.MethodPost, path, true, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TutorMessageRequest is POST /student/sessions/{id}/tutor/messages.
type TutorMessageRequest struct {
	Message string `json:"message"`
}

// TutorMessageResponse is a short coaching reply.
type TutorMessageResponse struct {
	Reply string `json:"reply"`
}

// TutorMessage posts a student message and returns a coaching reply.
func (c *Client) TutorMessage(ctx context.Context, sessionID, message string) (*TutorMessageResponse, error) {
	var out TutorMessageResponse
	path := "/student/sessions/" + url.PathEscape(sessionID) + "/tutor/messages"
	if err := c.doJSON(ctx, http.MethodPost, path, true, TutorMessageRequest{Message: message}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, auth bool, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if auth {
		if c.Token == "" {
			return &ErrUnauthorized{Message: "missing device token"}
		}
		req.Header.Set(HeaderAuthorization, "Bearer "+c.Token)
		req.Header.Set(HeaderDeviceToken, c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return &ErrUnauthorized{Message: strings.TrimSpace(string(respBody))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ErrHTTP{StatusCode: resp.StatusCode, Body: string(respBody), Op: method + " " + path}
	}
	if out == nil || len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response: %w; body=%s", err, truncate(string(respBody), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
