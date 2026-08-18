package api

import (
	"context"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/agent"
	"github.com/aleksclark/primer/server/internal/repo"
)

// registerAgentRuntime exposes the preview runtime only when explicitly
// injected. Parent authentication is the server-side authority boundary; the
// student API is never wired to this controller and therefore retains the
// max_children=0 policy.
func registerAgentRuntime(h huma.API, q repo.Querier, opts Options) {
	controller := opts.AgentController

	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "start-agent-run",
		Method:        http.MethodPost,
		Path:          "/agent/runs",
		Summary:       "Start a process-local agent run",
		Description:   "Starts a bounded, non-durable MAF run. The returned ID may be streamed, inspected, or cancelled by the authenticated parent.",
		Tags:          []string{"Agent Runtime"},
		DefaultStatus: http.StatusAccepted,
	}), func(ctx context.Context, in *startAgentRunInput) (*agentRunOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		if controller == nil {
			return nil, huma.Error503ServiceUnavailable("agent runtime is disabled")
		}
		snapshot, err := controller.Start(in.Body.Text)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("agent runtime is unavailable")
		}
		return &agentRunOutput{Body: snapshot}, nil
	})

	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "get-agent-run",
		Method:      http.MethodGet,
		Path:        "/agent/runs/{id}",
		Summary:     "Get agent run status",
		Tags:        []string{"Agent Runtime"},
	}), func(ctx context.Context, in *agentRunPathInput) (*agentRunOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		if controller == nil {
			return nil, huma.Error503ServiceUnavailable("agent runtime is disabled")
		}
		snapshot, ok := controller.Get(in.ID)
		if !ok {
			return nil, huma.Error404NotFound("agent run not found")
		}
		return &agentRunOutput{Body: snapshot}, nil
	})

	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "cancel-agent-run",
		Method:        http.MethodPost,
		Path:          "/agent/runs/{id}/cancel",
		Summary:       "Cancel an agent run",
		Tags:          []string{"Agent Runtime"},
		DefaultStatus: http.StatusAccepted,
	}), func(ctx context.Context, in *agentRunPathInput) (*agentRunOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		if controller == nil {
			return nil, huma.Error503ServiceUnavailable("agent runtime is disabled")
		}
		snapshot, ok := controller.Cancel(in.ID)
		if !ok {
			return nil, huma.Error404NotFound("agent run not found")
		}
		return &agentRunOutput{Body: snapshot}, nil
	})

	streamOp := parentOp(h, q, huma.Operation{
		OperationID: "stream-agent-run",
		Method:      http.MethodGet,
		Path:        "/agent/runs/{id}/events",
		Summary:     "Stream attributed agent run events",
		Description: "Streams Primer SSE events. Disconnecting this subscriber does not cancel the process-local run.",
		Tags:        []string{"Agent Runtime"},
	})
	if streamOp.Responses == nil {
		streamOp.Responses = map[string]*huma.Response{}
	}
	if streamOp.Responses["200"] == nil {
		streamOp.Responses["200"] = &huma.Response{}
	}
	if streamOp.Responses["200"].Content == nil {
		streamOp.Responses["200"].Content = map[string]*huma.MediaType{}
	}
	streamOp.Responses["200"].Content["text/event-stream"] = &huma.MediaType{}
	huma.Register(h, streamOp, func(ctx context.Context, in *agentRunPathInput) (*huma.StreamResponse, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		if controller == nil {
			return nil, huma.Error503ServiceUnavailable("agent runtime is disabled")
		}
		if _, ok := controller.Get(in.ID); !ok {
			return nil, huma.Error404NotFound("agent run not found")
		}
		return &huma.StreamResponse{Body: func(streamCtx huma.Context) {
			streamCtx.SetHeader("Content-Type", "text/event-stream")
			streamCtx.SetHeader("Cache-Control", "no-cache")
			streamCtx.SetHeader("Connection", "keep-alive")
			streamCtx.SetStatus(http.StatusOK)
			writer := streamCtx.BodyWriter()
			flusher, _ := writer.(http.Flusher)
			if flusher == nil {
				return
			}
			_ = controller.StreamWriter(streamCtx.Context(), in.ID, io.Writer(writer), flusher)
		}}, nil
	})
}

type startAgentRunInput struct {
	Body struct {
		Text string `json:"text" minLength:"1"`
	}
}

type agentRunPathInput struct {
	ID string `path:"id" minLength:"1"`
}

type agentRunOutput struct {
	Body agent.RunSnapshot
}
