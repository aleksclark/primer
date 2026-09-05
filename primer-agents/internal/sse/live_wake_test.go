package sse_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/sse"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

// TestSSELiveWakeAndTerminalClose streams a still-queued run over a real
// PostgreSQL-backed SSE connection, then concurrently appends a durable event
// and terminates the run. The subscriber must observe the live event and the
// terminal close without the stream mutating run status. Heartbeats are
// required so the wait loop is not poll-only.
func TestSSELiveWakeAndTerminalClose(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	svc := appservice.New(pool)

	ns := "ns-sse-live-" + uuid.NewString()[:8]
	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: ns,
		IdempotencyKey: uuid.NewString(),
		Profile:        "tutor",
	})
	require.NoError(t, err)

	cfg := testConfig()
	cfg.PollInterval = 2 * time.Second
	cfg.HeartbeatInterval = 15 * time.Millisecond
	cfg.MaxLifetime = 15 * time.Second

	pr, pw := io.Pipe()
	rec := &pipeRecorder{w: pw, header: make(http.Header)}
	streamCtx, streamCancel := context.WithTimeout(ctx, 12*time.Second)
	defer streamCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer pw.Close()
		sse.Stream(streamCtx, rec, pool, run.ID, ns, 0, cfg)
	}()

	reader := bufio.NewReader(pr)
	sawKeepalive := make(chan struct{})
	sawLive := make(chan struct{})
	sawTerminal := make(chan struct{})
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				select {
				case <-sawTerminal:
				default:
					close(sawTerminal)
				}
				return
			}
			switch {
			case strings.HasPrefix(line, ": keepalive"):
				select {
				case <-sawKeepalive:
				default:
					close(sawKeepalive)
				}
			case strings.Contains(line, `"kind":"run.text"`):
				select {
				case <-sawLive:
				default:
					close(sawLive)
				}
			case strings.Contains(line, `"kind":"run.succeeded"`) || strings.Contains(line, "run.succeeded"):
				select {
				case <-sawTerminal:
				default:
					close(sawTerminal)
				}
			}
		}
	}()

	select {
	case <-sawKeepalive:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE did not emit a heartbeat keep-alive before the live event")
	}

	payload := `{"i":1}`
	_, err = svc.AppendEvent(ctx, appservice.AppendEventCmd{
		RunID:          run.ID,
		OwnerNamespace: ns,
		Kind:           "run.text",
		Payload:        &payload,
	})
	require.NoError(t, err)

	select {
	case <-sawLive:
	case <-time.After(3 * time.Second):
		t.Fatal("SSE did not deliver the concurrently appended durable event")
	}

	running, err := svc.ClaimRun(ctx, run.ID, ns, run.StateVersion, time.Now().Add(time.Minute))
	require.NoError(t, err)
	terminated, err := svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID: run.ID, OwnerNamespace: ns, StateVersion: running.StateVersion,
		FromStatus: domain.RunStatusRunning, TermStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)
	require.Equal(t, domain.RunStatusSucceeded, terminated.Status)

	select {
	case <-sawTerminal:
	case <-time.After(3 * time.Second):
		t.Fatal("SSE did not close after the durable terminal event")
	}

	wg.Wait()
	final, err := svc.GetRun(ctx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusSucceeded, final.Status)
}

type pipeRecorder struct {
	w      *io.PipeWriter
	header http.Header
}

func (r *pipeRecorder) Header() http.Header         { return r.header }
func (r *pipeRecorder) Write(p []byte) (int, error) { return r.w.Write(p) }
func (r *pipeRecorder) WriteHeader(int)             {}
func (r *pipeRecorder) Flush()                      {}
