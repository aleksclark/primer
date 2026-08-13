package primer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/microsoft/agent-framework-go/tool"
)

// SSEHandler is a minimal Primer-shaped SSE adapter over public MAF streams.
// It exists because AG-UI hosts a single agent stream and does not natively
// attribute nested child agent deltas when children are invoked as tools.
// MAF contributes ResponseUpdate iteration; Primer owns the envelope + attribution.
type SSEHandler struct {
	Agent     *agent.Agent
	AgentType string
	// BuildChildTool optionally injects a streaming child tool into the run.
	// Called once per request with the request context and parent run id.
	BuildChildTool func(ctx context.Context, parentRunID string, sink EventSink) tool.FuncTool
	// After building tools, optional extra agent options.
	ExtraOptions func(r *http.Request) []agent.Option
}

// ServeHTTP handles POST {"text":"..."} and streams text/event-stream RunEvent JSON lines.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.Agent == nil {
		http.Error(w, "agent required", http.StatusInternalServerError)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Text == "" {
		body.Text = "hello"
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	runID := h.Agent.ID()
	sink := &sseWriterSink{w: w, f: flusher}

	_ = writeSSE(w, flusher, RunEvent{
		RunID:     runID,
		AgentType: h.AgentType,
		AgentName: h.Agent.Name(),
		AgentID:   h.Agent.ID(),
		Kind:      KindStart,
		Text:      body.Text,
		At:        time.Now().UTC(),
	})

	var opts []agent.Option
	if h.BuildChildTool != nil {
		if ct := h.BuildChildTool(ctx, runID, sink); ct != nil {
			opts = append(opts, agent.WithTool(ct))
		}
	}
	if h.ExtraOptions != nil {
		opts = append(opts, h.ExtraOptions(r)...)
	}

	var runErr error
	for update, err := range h.Agent.RunText(ctx, body.Text, opts...) {
		if err != nil {
			runErr = err
			_ = writeSSE(w, flusher, RunEvent{
				RunID:     runID,
				AgentType: h.AgentType,
				AgentName: h.Agent.Name(),
				AgentID:   h.Agent.ID(),
				Kind:      KindError,
				Err:       err.Error(),
				At:        time.Now().UTC(),
			})
			break
		}
		if update == nil {
			continue
		}
		for _, c := range update.Contents {
			if tc, ok := c.(*message.TextContent); ok && tc.Text != "" {
				_ = writeSSE(w, flusher, RunEvent{
					RunID:     runID,
					AgentType: h.AgentType,
					AgentName: h.Agent.Name(),
					AgentID:   h.Agent.ID(),
					Kind:      KindText,
					Text:      tc.Text,
					At:        time.Now().UTC(),
				})
			}
		}
	}

	_ = writeSSE(w, flusher, RunEvent{
		RunID:     runID,
		AgentType: h.AgentType,
		AgentName: h.Agent.Name(),
		AgentID:   h.Agent.ID(),
		Kind:      KindEnd,
		Err: func() string {
			if runErr != nil {
				return runErr.Error()
			}
			return ""
		}(),
		At: time.Now().UTC(),
	})
}

type sseWriterSink struct {
	w http.ResponseWriter
	f http.Flusher
	mu sync.Mutex
}

func (s *sseWriterSink) Emit(ctx context.Context, e RunEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-ctx.Done():
		// Best-effort: drop on cancel; do not fail run.
		return
	default:
	}
	_ = writeSSE(s.w, s.f, e)
}

func writeSSE(w http.ResponseWriter, f http.Flusher, e RunEvent) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: run\ndata: %s\n\n", b); err != nil {
		return err
	}
	f.Flush()
	return nil
}
