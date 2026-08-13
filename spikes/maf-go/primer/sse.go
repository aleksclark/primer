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

// DefaultRunBudget bounds a runtime-owned run when SSEHandler.RunBudget is unset.
// Subscriber disconnect must not cancel the run; only this budget or RunHandle.Cancel does.
const DefaultRunBudget = 2 * time.Minute

// RunHandle is a runtime-owned cancellation handle for one agent run.
// Stream/client context loss does not cancel this handle. Call Cancel for
// explicit runtime cancellation (parent→child→MCP), or wait for the budget.
type RunHandle struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Context returns the runtime-owned run context (not the HTTP/stream context).
func (h *RunHandle) Context() context.Context {
	if h == nil {
		return context.Background()
	}
	return h.ctx
}

// Cancel explicitly cancels the underlying run (and nested child/MCP work that
// honors the run context). Safe to call multiple times / from any goroutine.
func (h *RunHandle) Cancel() {
	if h == nil || h.cancel == nil {
		return
	}
	h.cancel()
}

// Done is closed when the run context is canceled or the budget expires.
func (h *RunHandle) Done() <-chan struct{} {
	if h == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return h.ctx.Done()
}

// Err returns the run context error, if any.
func (h *RunHandle) Err() error {
	if h == nil {
		return nil
	}
	return h.ctx.Err()
}

// SSEHandler is a minimal Primer-shaped SSE adapter over public MAF streams.
// It exists because AG-UI hosts a single agent stream and does not natively
// attribute nested child agent deltas when children are invoked as tools.
// MAF contributes ResponseUpdate iteration; Primer owns the envelope + attribution.
//
// Wire path uses an asynchronous bounded bridge so a slow ResponseWriter cannot
// block the agent/child runner. Stream/subscriber loss never fails or cancels
// the underlying run; only RunHandle.Cancel or RunBudget does.
type SSEHandler struct {
	Agent     *agent.Agent
	AgentType string
	// Spec optional budgets for Runner-backed child starts.
	Spec AgentSpec
	// BuildChildTool optionally injects a streaming child tool into the run.
	// Called once per request with the runtime-owned run context, parent runner, and bridge sink.
	// Prefer returning a tool from parent.StartChild so depth/lineage stay orchestrator-controlled.
	BuildChildTool func(ctx context.Context, parent *Runner, sink EventSink) tool.FuncTool
	// ExtraOptions optional extra agent options per request.
	ExtraOptions func(r *http.Request) []agent.Option
	// BridgeCapacity bounds the async SSE queue (default DefaultSSEBridgeCapacity).
	BridgeCapacity int
	// RunBudget bounds the runtime-owned run context (default DefaultRunBudget).
	// Independent of the HTTP request/stream context.
	RunBudget time.Duration
	// OnRunStart is invoked after the run handle is created (tests / host hooks).
	// The handle's Context is what Runner.Run and child tools observe.
	OnRunStart func(h *RunHandle)
	// OnRunEnd is invoked after Runner.Run returns (tests / host hooks).
	OnRunEnd func(err error)
}

// ServeHTTP handles POST {"text":"..."} and streams text/event-stream RunEvent JSON lines.
//
// Two contexts:
//   - stream context = r.Context() — controls only the SSE writer/bridge drain;
//   - run context    = runtime-owned (budget + RunHandle.Cancel) — drives Runner/child/MCP.
//
// Client disconnect cancels the stream context only; the run continues to completion
// (or budget/explicit cancel). Remaining events after stream loss are dropped.
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

	// (a) stream/client context: writer only.
	streamCtx := r.Context()

	// (b) runtime-owned run context: not canceled by subscriber disconnect.
	budget := h.RunBudget
	if budget <= 0 {
		budget = DefaultRunBudget
	}
	runCtx, runCancel := context.WithTimeout(context.Background(), budget)
	defer runCancel()
	handle := &RunHandle{ctx: runCtx, cancel: runCancel}
	if h.OnRunStart != nil {
		h.OnRunStart(handle)
	}

	capN := h.BridgeCapacity
	if capN < 1 {
		capN = DefaultSSEBridgeCapacity
	}

	// Async bridge: runner emits into BoundedAsyncSink; writer drains to ResponseWriter.
	bridge := NewSSEBridge(capN)
	defer bridge.Close()

	// Writer goroutine: drain bridge → wire. Bound by streamCtx only.
	// Slow/disconnected writer does not block runner; does not cancel runCtx.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		bridge.WriteTo(streamCtx, w, flusher)
	}()

	spec := h.Spec
	if spec.Type == "" {
		spec.Type = h.AgentType
	}
	if spec.Name == "" && h.Agent != nil {
		spec.Name = h.Agent.Name()
	}
	runner := NewRunner(spec, h.Agent, bridge)
	// Assign run identity before BuildChildTool so StartChild parent/root ids
	// match the subsequent Run events (no synthetic-id drift).
	_ = runner.PrepareRun()

	var opts []agent.Option
	if h.BuildChildTool != nil {
		// Child wiring uses runCtx so StartChild/cancel path is runtime-owned.
		if ct := h.BuildChildTool(runCtx, runner, bridge); ct != nil {
			opts = append(opts, agent.WithTool(ct))
		}
	}
	if h.ExtraOptions != nil {
		opts = append(opts, h.ExtraOptions(r)...)
	}

	// Run agent to completion on runCtx — independent of stream/writer health.
	runErr := runner.Run(runCtx, body.Text, opts...)
	if h.OnRunEnd != nil {
		h.OnRunEnd(runErr)
	}

	// Signal end-of-events and wait for writer drain (or stream cancel).
	bridge.Close()
	select {
	case <-writerDone:
	case <-streamCtx.Done():
		// Stream already gone; writer should exit promptly without hanging ServeHTTP.
		select {
		case <-writerDone:
		case <-time.After(2 * time.Second):
		}
	case <-time.After(2 * time.Second):
		// Bound shutdown; do not hang ServeHTTP forever.
	}
}

// SSEBridge is a bounded asynchronous event bridge from runner → HTTP writer.
// Emit never blocks on the ResponseWriter. Dropped events increment Dropped.
// Close ends the queue; WriteTo returns after drain or stream-ctx cancel.
//
// Emit and Close are safe for concurrent use: send-on-closed-channel is impossible
// because Emit holds mu for the entire non-blocking send attempt.
type SSEBridge struct {
	ch     chan RunEvent
	mu     sync.Mutex
	closed bool
	// Dropped counts events dropped due to full/closed/stream-cancel/write-error.
	Dropped int
	// writeDone is closed when WriteTo exits (for tests / leak checks).
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
// Concurrent with Close: never panics (send holds mu with the closed check).
func (b *SSEBridge) Emit(ctx context.Context, e RunEvent) {
	if b == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	// Best-effort: if the *emit caller's* ctx is done, drop. Note: SSEHandler
	// runners pass runCtx here, so stream disconnect alone does not drop.
	select {
	case <-ctx.Done():
		b.mu.Lock()
		b.Dropped++
		b.mu.Unlock()
		return
	default:
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		b.Dropped++
		return
	}
	// Non-blocking send under the same lock as Close so Close cannot close(ch)
	// between the closed check and the send (send-on-closed panic).
	select {
	case b.ch <- e:
	default:
		b.Dropped++
	}
}

// Close stops accepting events and closes the channel once.
// Safe to call multiple times and concurrently with Emit/WriteTo.
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

// WriteTo drains events to w until the channel is closed or stream ctx is done.
// On stream cancel or write error: exit promptly; drop remaining queued events
// without attempting further synchronous writes to a slow/broken writer.
func (b *SSEBridge) WriteTo(ctx context.Context, w http.ResponseWriter, f http.Flusher) {
	defer b.onceDone.Do(func() { close(b.writeDone) })
	for {
		select {
		case <-ctx.Done():
			// Stream canceled/disconnected: drop residual; do not write further.
			b.dropQueued()
			return
		case e, ok := <-b.ch:
			if !ok {
				return
			}
			if err := writeSSE(w, f, e); err != nil {
				// Writer broken: drop rest, do not fail runner (already independent).
				b.dropQueued()
				return
			}
		}
	}
}

// dropQueued discards any currently buffered events (and races with late Emit
// until Close). Does not block on producers. Counts drops.
func (b *SSEBridge) dropQueued() {
	for {
		select {
		case _, ok := <-b.ch:
			if !ok {
				return
			}
			b.mu.Lock()
			b.Dropped++
			b.mu.Unlock()
		default:
			return
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
