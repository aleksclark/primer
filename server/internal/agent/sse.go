package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	mafagent "github.com/microsoft/agent-framework-go/agent"
	maftool "github.com/microsoft/agent-framework-go/tool"
)

// DefaultSSEBridgeCapacity is the default event buffer depth for the async bridge.
const DefaultSSEBridgeCapacity = 64

// DefaultRunBudget is the maximum runtime for a run when SSEHandler.RunBudget
// is unset. Subscriber disconnect must not cancel the run; only this timeout
// or RunHandle.Cancel does.
const DefaultRunBudget = 2 * time.Minute

// RunHandle is a runtime-owned cancellation handle for one agent run.
// Stream/client context loss does not cancel this handle. Call Cancel for
// explicit runtime cancellation (reaches parent→child→MCP), or let the budget
// expire.
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

// Cancel explicitly cancels the underlying run and all nested child/MCP work
// that honors the run context. Idempotent and goroutine-safe.
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

// SSEHandler is the Primer-shaped SSE adapter over public MAF streams.
//
// It exists because AG-UI hosts a single-agent stream and does not attribute
// nested child agent deltas when children are invoked as tools.
// MAF contributes ResponseUpdate iteration; Primer owns the event envelope,
// attribution, async bridge, and stream/run context split.
//
// Wire path: runner emits events into an SSEBridge; a writer goroutine drains
// the bridge to the ResponseWriter. A slow or disconnected ResponseWriter cannot
// block or cancel the runner. Only RunHandle.Cancel or the RunBudget timeout
// can cancel the underlying run.
type SSEHandler struct {
	Agent     *mafagent.Agent
	AgentType string
	// Spec optional budgets for Runner-backed child starts.
	Spec AgentSpec
	// BuildChildTool optionally injects a streaming child tool per request.
	// Called with the runtime-owned run context, parent Runner, and bridge EventSink.
	BuildChildTool func(ctx context.Context, parent *Runner, sink EventSink) maftool.FuncTool
	// ExtraOptions optional extra agent options per request.
	ExtraOptions func(r *http.Request) []mafagent.Option
	// BridgeCapacity bounds the async SSE queue (default DefaultSSEBridgeCapacity).
	BridgeCapacity int
	// RunBudget bounds the runtime-owned run context (default DefaultRunBudget).
	RunBudget time.Duration
	// OnRunStart is invoked after the RunHandle is created (tests / host hooks).
	OnRunStart func(h *RunHandle)
	// OnRunEnd is invoked after Runner.Run returns (tests / host hooks).
	OnRunEnd func(err error)
}

// ServeHTTP handles POST {"text":"..."} and streams text/event-stream RunEvent JSON.
//
// Two contexts:
//   - stream context  = r.Context()     — controls only the SSE writer/bridge drain
//   - run context     = runtime-owned   — drives Runner/child/MCP work
//
// Client disconnect cancels the stream context only. The run continues until
// completion, budget expiry, or explicit RunHandle.Cancel.
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

	// (a) stream context: writer only — canceled by client disconnect.
	streamCtx := r.Context()

	// (b) run context: runtime-owned — NOT canceled by subscriber disconnect.
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

	bridge := NewSSEBridge(capN)
	defer bridge.Close()

	// Writer goroutine: drain bridge → ResponseWriter. Bound by streamCtx only.
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

	var opts []mafagent.Option
	if h.BuildChildTool != nil {
		if ct := h.BuildChildTool(runCtx, runner, bridge); ct != nil {
			opts = append(opts, mafagent.WithTool(ct))
		}
	}
	if h.ExtraOptions != nil {
		opts = append(opts, h.ExtraOptions(r)...)
	}

	// Run on runtime context — independent of stream/writer health.
	runErr := runner.Run(runCtx, body.Text, opts...)
	if h.OnRunEnd != nil {
		h.OnRunEnd(runErr)
	}

	// Signal end-of-events and wait for writer drain (or stream cancel).
	bridge.Close()
	select {
	case <-writerDone:
	case <-streamCtx.Done():
		select {
		case <-writerDone:
		case <-time.After(2 * time.Second):
		}
	case <-time.After(2 * time.Second):
	}
}

// SSEBridge is a bounded asynchronous event bridge from runner → HTTP writer.
//
// Emit never blocks on the ResponseWriter. Dropped events increment Dropped.
// Close ends the queue; WriteTo returns after drain or stream-ctx cancel.
//
// Emit and Close are goroutine-safe: send-on-closed-channel is impossible
// because Emit holds mu across the non-blocking send attempt.
type SSEBridge struct {
	ch        chan RunEvent
	mu        sync.Mutex
	closed    bool
	Dropped   int
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
// Concurrent with Close: never panics (send holds mu alongside the closed check).
func (b *SSEBridge) Emit(ctx context.Context, e RunEvent) {
	if b == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	// Best-effort: if emit caller's ctx is done, drop rather than enqueue.
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
	// between the closed check and the send (preventing send-on-closed panic).
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

// DroppedCount returns the dropped event count.
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
// On stream cancel or write error: exits promptly; drops remaining queued events
// without attempting further synchronous writes.
func (b *SSEBridge) WriteTo(ctx context.Context, w http.ResponseWriter, f http.Flusher) {
	defer b.onceDone.Do(func() { close(b.writeDone) })
	for {
		select {
		case <-ctx.Done():
			b.dropQueued()
			return
		case e, ok := <-b.ch:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, f, e); err != nil {
				b.dropQueued()
				return
			}
		}
	}
}

// dropQueued discards currently buffered events without writing.
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

func writeSSEEvent(w http.ResponseWriter, f http.Flusher, e RunEvent) error {
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
