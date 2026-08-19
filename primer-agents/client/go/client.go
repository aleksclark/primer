// Package agentsclient is the primer-agents Go client.
//
// # Source
//
// All models and HTTP operation bindings come from client.gen.go, which is
// produced from openapi30.yaml by oapi-codegen v2.4.1. Do not write duplicate
// DTOs or raw HTTP calls here.
//
// # Regeneration
//
//	make clients-go   (from the primer-agents module root)
//
// # Usage
//
//	c, err := agentsclient.NewAgentsClient("https://agents.example", "<token>")
//	run, err := c.CreateRun(ctx, "my-idempotency-key", agentsclient.CreateRunInputBody{
//	    Profile: "tutor",
//	})
package agentsclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// AgentsClient is a typed façade over ClientWithResponses. All DTOs are
// generated from openapi.yaml — this layer adds bearer auth and ergonomic
// helpers without redefining any wire types.
type AgentsClient struct {
	inner *ClientWithResponses
}

// NewAgentsClient constructs an AgentsClient for the given base URL.
// bearer is the raw token value (without "Bearer " prefix).
func NewAgentsClient(baseURL, bearer string, opts ...ClientOption) (*AgentsClient, error) {
	inject := WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+bearer)
		return nil
	})
	inner, err := NewClientWithResponses(baseURL, append([]ClientOption{inject}, opts...)...)
	if err != nil {
		return nil, err
	}
	return &AgentsClient{inner: inner}, nil
}

// CreateRun creates a run idempotently. idempotencyKey is the Idempotency-Key
// header. Returns the durable queued record or an error carrying the HTTP status.
func (c *AgentsClient) CreateRun(ctx context.Context, idempotencyKey string, body CreateRunJSONRequestBody) (*RunResponse, error) {
	resp, err := c.inner.CreateRunWithResponse(ctx, &CreateRunParams{IdempotencyKey: idempotencyKey}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("create run: status %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GetRun fetches a run by server-generated ID.
func (c *AgentsClient) GetRun(ctx context.Context, id string) (*RunResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("get run: invalid id: %w", err)
	}
	resp, err := c.inner.GetRunWithResponse(ctx, uid)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("get run: status %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListRuns returns recent runs owned by the authenticated caller.
func (c *AgentsClient) ListRuns(ctx context.Context, limit *int64) ([]RunResponse, error) {
	params := &ListRunsParams{Limit: limit}
	resp, err := c.inner.ListRunsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil || resp.JSON200.Runs == nil {
		return nil, fmt.Errorf("list runs: status %d", resp.StatusCode())
	}
	return *resp.JSON200.Runs, nil
}

// CancelRun requests cancellation; idempotent per the durable state machine.
func (c *AgentsClient) CancelRun(ctx context.Context, id string, body CancelRunJSONRequestBody) (*RunResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("cancel run: invalid id: %w", err)
	}
	resp, err := c.inner.CancelRunWithResponse(ctx, uid, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("cancel run: status %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListEvents returns a page of sequenced events for a run, after afterSeq.
func (c *AgentsClient) ListEvents(ctx context.Context, runID string, afterSeq *int64, limit *int64) ([]EventResponse, error) {
	uid, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("list events: invalid run id: %w", err)
	}
	params := &ListRunEventsParams{AfterSeq: afterSeq, Limit: limit}
	resp, err := c.inner.ListRunEventsWithResponse(ctx, uid, params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil || resp.JSON200.Events == nil {
		return nil, fmt.Errorf("list events: status %d", resp.StatusCode())
	}
	return *resp.JSON200.Events, nil
}

// CreateSession creates a new durable open session.
func (c *AgentsClient) CreateSession(ctx context.Context, body CreateSessionJSONRequestBody) (*SessionResponse, error) {
	resp, err := c.inner.CreateSessionWithResponse(ctx, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("create session: status %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// GetSession fetches a session by server-generated ID.
func (c *AgentsClient) GetSession(ctx context.Context, id string) (*SessionResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("get session: invalid id: %w", err)
	}
	resp, err := c.inner.GetSessionWithResponse(ctx, uid)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("get session: status %d", resp.StatusCode())
	}
	return resp.JSON200, nil
}
