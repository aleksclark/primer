package primer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/tool"
)

// DefaultSSEBridgeCapacity is the default event buffer for the async SSE bridge.
const DefaultSSEBridgeCapacity = 64

// SSEHandler is a minimal Primer-shaped SSE adapter over public MAF streams.
// It exists because AG-UI hosts a single agent stream and does not natively
// attribute nested child agent deltas when children are invoked as tools.
// MAF contributes ResponseUpdate iteration; Primer owns the envelope + attribution.
//
// Wire path uses an asynchronous bounded bridge so a slow ResponseWriter cannot
// block the agent/child runner. Stream loss never fails the underlying run.
type SSEHandler struct {
	Agent     *agent.Agent
	AgentType string
	// Spec optional budgets for Runner-backed child starts.
	Spec AgentSpec
	// BuildChildTool optionally injects a streaming child tool into the run.
	// Called once per request with the request context, parent runner, and bridge sink.
	// Prefer returning a tool from parent.StartChild so depth/lineage stay orchestrator-controlled.
	BuildChildTool func(ctx context.Context, parent *Runner, sink EventSink) tool.FuncTool
	// ExtraOptions optional extra agent options per request.
	ExtraOptions func(r *http.Request) []agent.Option
	// BridgeCapacity bounds the async SSE queue (default DefaultSSEBridgeCapacity).
	BridgeCapacity int
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
	flusher.Flush()

	ctx := r.Context()
	capN := h.BridgeCapacity
	if capN < 1 {
		capN = DefaultSSEBridgeCapacity
	}

	// Async bridge: runner emits into BoundedAsyncSink; writer drains to ResponseWriter.
	bridge := NewSSEBridge(capN)
	defer bridge.Close()

	// Writer goroutine: drain bridge → wire. Slow/disconnected writer does not block runner.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		bridge.WriteTo(ctx, w, flusher)
	}()

	spec := h.Spec
	if spec.Type == "" {
		spec.Type = h.AgentType
	}
	if spec.Name == "" && h.Agent != nil {
		spec.Name = h.Agent.Name()
	}
	runner := NewRunner(spec, h.Agent, bridge)

	var opts []agent.Option
	if h.BuildChildTool != nil {
		if ct := h.BuildChildTool(ctx, runner, bridge); ct != nil {
			opts = append(opts, agent.WithTool(ct))
		}
	}
	if h.ExtraOptions != nil {
		opts = append(opts, h.ExtraOptions(r)...)
	}

	// Run agent to completion regardless of bridge/writer health.
	_ = runner.Run(ctx, body.Text, opts...)

	// Signal end-of-events and wait for writer drain (bounded by request ctx + grace).
	bridge.Close()
	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		// Bound shutdown; do not hang ServeHTTP forever.
	}
}

// SSEBridge is a bounded asynchronous event bridge from runner → HTTP writer.
// Emit never blocks on the ResponseWriter. Dropped events increment Dropped.
// Close ends the queue; WriteTo returns after drain or ctx cancel.
type SSEBridge struct {
	ch      chan RunEvent
	mu      sync.Mutex
	closed  bool
	Dropped int
	// done is closed when WriteTo exits (for tests / leak checks).
	writeDone chan struct{}
	onceDone  sync.Once
}

// NewSSEBridge creates a bridge with the given capacity (>=1).
func NewSSEBridge(capacity int) *SSEBridge {
	if capacity < 1 {
		capacity = 1
	}
	return &SSEBridge{
		ch:        make(chan RunEvent, capacity),
		writeDone: make(chan struct{}),
	}
}

// Emit enqueues e without blocking on the consumer. Drops when full or closed.
func (b *SSEBridge) Emit(ctx context.Context, e RunEvent) {
	if b == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	select {
	case <-ctx.Done():
		// Best-effort: drop on cancel; do not fail run.
		b.mu.Lock()
		b.Dropped++
		b.mu.Unlock()
		return
	default:
	}
	b.mu.Lock()
	if b.closed {
		b.Dropped++
		b.mu.Unlock()
		return
	}
	ch := b.ch
	b.mu.Unlock()

	select {
	case ch <- e:
	default:
		b.mu.Lock()
		b.Dropped++
		b.mu.Unlock()
	}
}

// Close stops accepting events and closes the channel once.
func (b *SSEBridge) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.ch)
}

// DroppedCount returns dropped event count.
func (b *SSEBridge) DroppedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Dropped
}

// WriteDone returns a channel closed when WriteTo exits.
func (b *SSEBridge) WriteDone() <-chan struct{} {
	return b.writeDone
}

// WriteTo drains events to w until the channel is closed or ctx is done.
func (b *SSEBridge) WriteTo(ctx context.Context, w http.ResponseWriter, f http.Flusher) {
	defer b.onceDone.Do(func() { close(b.writeDone) })
	for {
		select {
		case <-ctx.Done():
			// Drain remaining without blocking forever on slow write.
			for {
				select {
				case e, ok := <-b.ch:
					if !ok {
						return
					}
					_ = writeSSE(w, f, e) // best-effort; ignore write errors
				default:
					return
				}
			}
		case e, ok := <-b.ch:
			if !ok {
				return
			}
			if err := writeSSE(w, f, e); err != nil {
				// Writer broken: drain+drop rest, do not fail runner (already independent).
				for range b.ch {
					b.mu.Lock()
					b.Dropped++
					b.mu.Unlock()
				}
				return
			}
		}
	}
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
	if f != nil {
		f.Flush()
	}
	return nil
}
