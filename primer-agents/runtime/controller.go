package agentruntime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	mafagent "github.com/microsoft/agent-framework-go/agent"
	maftool "github.com/microsoft/agent-framework-go/tool"
)

// RunState is the process-local lifecycle state of a runtime run.
type RunState string

const (
	RunStarting  RunState = "starting"
	RunRunning   RunState = "running"
	RunSucceeded RunState = "succeeded"
	RunFailed    RunState = "failed"
	RunCanceled  RunState = "canceled"
)

// RunSnapshot is safe to expose to an API caller. Run records are process-local
// and are not a durable control plane.
type RunSnapshot struct {
	ID         string    `json:"runId"`
	RootRunID  string    `json:"rootRunId"`
	State      RunState  `json:"state"`
	ErrorClass string    `json:"errorClass,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt,omitempty"`
}

// Controller owns runtime contexts and detached run lifetimes. Request
// contexts are used only by Stream; a client disconnect therefore cannot
// cancel a run. Explicit Cancel is the only caller-owned cancellation path.
type Controller struct {
	mu       sync.Mutex
	runs     map[string]*controlledRun
	maxRuns  int
	budget   time.Duration
	capacity int

	agent     *mafagent.Agent
	spec      AgentSpec
	buildTool func(context.Context, *Runner, EventSink) maftool.FuncTool
}

type controlledRun struct {
	mu     sync.RWMutex
	state  RunState
	status RunSnapshot
	handle *RunHandle
	bridge *SSEBridge
	done   chan struct{}
}

// ControllerConfig configures a process-local runtime controller.
type ControllerConfig struct {
	Agent           *mafagent.Agent
	Spec            AgentSpec
	RunBudget       time.Duration
	BridgeCapacity  int
	MaxRetainedRuns int
	BuildChildTool  func(context.Context, *Runner, EventSink) maftool.FuncTool
}

// NewController constructs a controller. A nil agent disables starting runs.
func NewController(cfg ControllerConfig) *Controller {
	budget := cfg.RunBudget
	if budget <= 0 {
		budget = DefaultRunBudget
	}
	capacity := cfg.BridgeCapacity
	if capacity < 1 {
		capacity = DefaultSSEBridgeCapacity
	}
	maxRuns := cfg.MaxRetainedRuns
	if maxRuns < 1 {
		maxRuns = 256
	}
	return &Controller{
		runs:      make(map[string]*controlledRun),
		maxRuns:   maxRuns,
		budget:    budget,
		capacity:  capacity,
		agent:     cfg.Agent,
		spec:      cfg.Spec,
		buildTool: cfg.BuildChildTool,
	}
}

// Start detaches a run from any HTTP request and returns its server-generated
// ID immediately. The run owns a background context bounded by RunBudget.
func (c *Controller) Start(text string) (RunSnapshot, error) {
	if c == nil || c.agent == nil {
		return RunSnapshot{}, errors.New("agent runtime is disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.budget)
	handle := &RunHandle{ctx: ctx, cancel: cancel}
	bridge := NewSSEBridge(c.capacity)
	runner := NewRunner(c.spec, c.agent, bridge)
	id := runner.PrepareRun()
	rootID := runner.Orchestration().RootRunID
	if rootID == "" {
		rootID = id
	}
	now := time.Now().UTC()
	run := &controlledRun{
		state:  RunStarting,
		handle: handle,
		bridge: bridge,
		done:   make(chan struct{}),
		status: RunSnapshot{ID: id, RootRunID: rootID, State: RunStarting, StartedAt: now},
	}

	c.mu.Lock()
	c.retainLocked()
	c.runs[id] = run
	c.mu.Unlock()

	go func() {
		run.setState(RunRunning, "", "")
		var opts []mafagent.Option
		if c.buildTool != nil {
			if child := c.buildTool(handle.Context(), runner, bridge); child != nil {
				opts = append(opts, mafagent.WithTool(child))
			}
		}
		err := runner.Run(handle.Context(), text, opts...)
		state := RunSucceeded
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(handle.Err(), context.Canceled) {
				state = RunCanceled
			} else {
				state = RunFailed
			}
		}
		if err == nil && errors.Is(handle.Err(), context.Canceled) {
			state = RunCanceled
			if err = handle.Err(); err == nil {
				err = context.Canceled
			}
		}
		run.setState(state, publicErrorClass(err), publicErrorText(err))
		cancel()
		bridge.Close()
		close(run.done)
	}()
	return run.snapshot(), nil
}

func publicErrorClass(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	return "provider_error"
}

// publicErrorText deliberately does not expose provider, prompt, tool, or
// credential details in the HTTP-visible run record.
func publicErrorText(err error) string {
	switch publicErrorClass(err) {
	case "canceled":
		return "agent run canceled"
	case "deadline_exceeded":
		return "agent run exceeded its time budget"
	case "provider_error":
		return "agent provider failed"
	default:
		return ""
	}
}

// Get returns a process-local run snapshot.
func (c *Controller) Get(id string) (RunSnapshot, bool) {
	c.mu.Lock()
	run, ok := c.runs[id]
	c.mu.Unlock()
	if !ok {
		return RunSnapshot{}, false
	}
	return run.snapshot(), true
}

// Cancel explicitly cancels a run. It is idempotent after terminal completion.
func (c *Controller) Cancel(id string) (RunSnapshot, bool) {
	c.mu.Lock()
	run, ok := c.runs[id]
	c.mu.Unlock()
	if !ok {
		return RunSnapshot{}, false
	}
	run.handle.Cancel()
	return run.snapshot(), true
}

// Stream writes the run's buffered/live events to a request stream. The
// request context controls delivery only; it is never passed to the run.
func (c *Controller) Stream(ctx context.Context, id string, w http.ResponseWriter) error {
	if _, ok := c.Get(id); !ok {
		return ErrRunNotFound
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("streaming unsupported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()
	return c.StreamWriter(ctx, id, w, flusher)
}

// StreamWriter is the adapter used by Huma's StreamResponse, whose body
// writer is intentionally exposed as io.Writer rather than ResponseWriter.
func (c *Controller) StreamWriter(ctx context.Context, id string, w io.Writer, f http.Flusher) error {
	c.mu.Lock()
	run, ok := c.runs[id]
	c.mu.Unlock()
	if !ok {
		return ErrRunNotFound
	}
	run.bridge.WriteToWriter(ctx, w, f)
	return nil
}

// Wait waits for terminal completion without exposing the run context.
func (c *Controller) Wait(ctx context.Context, id string) (RunSnapshot, error) {
	c.mu.Lock()
	run, ok := c.runs[id]
	c.mu.Unlock()
	if !ok {
		return RunSnapshot{}, ErrRunNotFound
	}
	select {
	case <-run.done:
		return run.snapshot(), nil
	case <-ctx.Done():
		return RunSnapshot{}, ctx.Err()
	}
}

// Shutdown cancels active runs and waits up to ctx's deadline. Runs are
// process-local; shutdown does not persist or resume them.
func (c *Controller) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	active := make([]*controlledRun, 0, len(c.runs))
	for _, run := range c.runs {
		if !isTerminal(run.snapshot().State) {
			active = append(active, run)
		}
	}
	c.mu.Unlock()
	for _, run := range active {
		run.handle.Cancel()
	}
	for _, run := range active {
		select {
		case <-run.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *Controller) retainLocked() {
	if len(c.runs) < c.maxRuns {
		return
	}
	for id, run := range c.runs {
		if isTerminal(run.snapshot().State) {
			delete(c.runs, id)
			if len(c.runs) < c.maxRuns {
				return
			}
		}
	}
}

func (r *controlledRun) setState(state RunState, errorClass, errText string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
	r.status.State = state
	r.status.ErrorClass = errorClass
	r.status.Error = errText
	if isTerminal(state) {
		r.status.EndedAt = time.Now().UTC()
	}
}

func (r *controlledRun) snapshot() RunSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func isTerminal(state RunState) bool {
	return state == RunSucceeded || state == RunFailed || state == RunCanceled
}

// ErrRunNotFound is returned when a status, cancel, wait, or stream ID is
// unknown. Callers should map it to the API's not-found response.
var ErrRunNotFound = errors.New("agent run not found")
