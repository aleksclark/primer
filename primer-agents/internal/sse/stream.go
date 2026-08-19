// Package sse implements the primer-agents replay-first SSE event stream.
//
// Design invariants:
//  - PostgreSQL run_events is the source of truth; no process-local ring buffer.
//  - Last-Event-ID (or ?afterSeq=N) is the resume cursor; sequence IDs are
//    durable DB sequence numbers.
//  - The request (stream) context controls only the writer goroutine.
//    The worker/run context is never derived from or cancelled by a stream.
//  - Slow/disconnected writers exit without calling run cancel or blocking
//    the durable event writer.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
)

const (
	// DefaultPageSize is the number of events fetched per DB poll round.
	DefaultPageSize = 50
	// DefaultPollInterval is how often the stream re-queries when caught up.
	DefaultPollInterval = 100 * time.Millisecond
	// DefaultHeartbeatInterval is how often a keep-alive comment is sent.
	DefaultHeartbeatInterval = 15 * time.Second
	// DefaultWriteTimeout is the deadline for writing one SSE frame.
	DefaultWriteTimeout = 5 * time.Second
	// DefaultMaxLifetime caps a single subscriber connection.
	DefaultMaxLifetime = 30 * time.Minute
)

// Config controls SSE stream behaviour.
type Config struct {
	PageSize          int
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	WriteTimeout      time.Duration
	MaxLifetime       time.Duration
}

// DefaultConfig returns safe production defaults.
func DefaultConfig() Config {
	return Config{
		PageSize:          DefaultPageSize,
		PollInterval:      DefaultPollInterval,
		HeartbeatInterval: DefaultHeartbeatInterval,
		WriteTimeout:      DefaultWriteTimeout,
		MaxLifetime:       DefaultMaxLifetime,
	}
}

// EventFrame is the SSE-serialisable form of a durable run event.
// Schema version and sequence are embedded for reconnect/idempotency.
type EventFrame struct {
	SchemaVersion int    `json:"schemaVersion"`
	Sequence      int64  `json:"sequence"`
	RunID         string `json:"runId"`
	RootRunID     string `json:"rootRunId,omitempty"`
	ParentRunID   string `json:"parentRunId,omitempty"`
	AgentID       string `json:"agentId,omitempty"`
	AgentType     string `json:"agentType,omitempty"`
	AgentDepth    int    `json:"agentDepth,omitempty"`
	Kind          string `json:"kind"`
	// Payload is the bounded diagnostic payload from run_events.
	// Raw model text is excluded per content policy.
	Payload   *string `json:"payload,omitempty"`
	CreatedAt string  `json:"createdAt"`
}

func frameFromEvent(e *domain.RunEvent) EventFrame {
	f := EventFrame{
		SchemaVersion: int(e.SchemaVersion),
		Sequence:      e.Sequence,
		RunID:         e.RunID,
		Kind:          e.Kind,
		Payload:       e.Payload,
		CreatedAt:     e.CreatedAt.UTC().Format(time.RFC3339),
	}
	if e.RootRunID != nil {
		f.RootRunID = *e.RootRunID
	}
	if e.ParentRunID != nil {
		f.ParentRunID = *e.ParentRunID
	}
	if e.AgentID != nil {
		f.AgentID = *e.AgentID
	}
	if e.AgentType != nil {
		f.AgentType = *e.AgentType
	}
	if e.AgentDepth != nil {
		f.AgentDepth = *e.AgentDepth
	}
	return f
}

// Stream serves a replay-first SSE stream for a single run.
// The stream context (r.Context()) controls the writer only.
// The run lifecycle is managed by the worker; disconnect does NOT cancel the run.
func Stream(
	streamCtx context.Context,
	w http.ResponseWriter,
	pool *pgxpool.Pool,
	runID, namespace string,
	afterSeq int64,
	cfg Config,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Bound the connection to MaxLifetime.
	ctx, cancel := context.WithTimeout(streamCtx, cfg.MaxLifetime)
	defer cancel()

	// Subscribe to LISTEN/NOTIFY for live wake-ups (optimization; polling is fallback).
	notifyCh, err := repo.SubscribeRunEvents(ctx, pool, runID)
	if err != nil {
		slog.Error("sse: subscribe listen failed, falling back to poll-only",
			"run_id", runID, "error", err)
		// Create a stub channel that never fires so polling still works.
		stub := make(chan struct{})
		defer close(stub)
		notifyCh = stub
	}

	cursor := afterSeq
	heartbeat := time.NewTicker(cfg.HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		// Page committed events after cursor.
		events, err := repo.ListRunEventsByCursor(ctx, pool, runID, namespace, cursor, cfg.PageSize)
		if err != nil {
			if ctx.Err() != nil {
				return // client disconnected or lifetime expired
			}
			slog.Error("sse: event page error", "run_id", runID, "error", err)
			return
		}

		for _, e := range events {
			if err := writeFrame(w, flusher, e, cfg.WriteTimeout); err != nil {
				return // client gone; do not cancel the run
			}
			cursor = e.Sequence
		}
		flusher.Flush()

		// Check whether the run is terminal — if so, drain one more page and close.
		if len(events) == 0 || int64(len(events)) < int64(cfg.PageSize) {
			run, err := repo.GetRunWithOwnership(ctx, pool, runID, namespace)
			if err != nil {
				return
			}
			if run.Status.IsTerminal() {
				// One final catch-up to pick up any terminal event just written.
				final, err := repo.ListRunEventsByCursor(ctx, pool, runID, namespace, cursor, cfg.PageSize)
				if err == nil {
					for _, e := range final {
						if err := writeFrame(w, flusher, e, cfg.WriteTimeout); err != nil {
							return
						}
						cursor = e.Sequence
					}
					flusher.Flush()
				}
				return // stream complete
			}
		}

		// Wait for a wake-up, heartbeat tick, or client disconnect.
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			// Send keep-alive comment; do not log content.
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case _, ok := <-notifyCh:
			if !ok {
				// LISTEN channel closed; fall back to poll-only.
				notifyCh = nil
			}
			// New event ready — loop back to page.
		case <-time.After(cfg.PollInterval):
			// Poll timeout; loop back to page.
		}
	}
}

// ParseCursor extracts the afterSeq cursor from Last-Event-ID header or ?afterSeq= query param.
// Returns 0 (replay all) when neither is set.
func ParseCursor(r *http.Request) int64 {
	if h := strings.TrimSpace(r.Header.Get("Last-Event-ID")); h != "" {
		if n, err := strconv.ParseInt(h, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	if q := r.URL.Query().Get("afterSeq"); q != "" {
		if n, err := strconv.ParseInt(q, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return 0
}

func writeFrame(w http.ResponseWriter, f http.Flusher, e *domain.RunEvent, timeout time.Duration) error {
	frame := frameFromEvent(e)
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	// SSE wire format: id, event type, data, blank line.
	// The id is the durable sequence so Last-Event-ID works for reconnect.
	// Do not log data content.
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n",
		e.Sequence, e.Kind, data)
	if err != nil {
		return err
	}
	_ = timeout // WriteTimeout enforcement can be added by wrapping w
	return nil
}
