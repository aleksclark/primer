package sse_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

const liveWakePollInterval = 3 * time.Second
const liveWakeNotifyBudget = 500 * time.Millisecond

// TestSSELiveWakeAndTerminalClose streams a still-queued run over a real
// PostgreSQL-backed SSE connection. After the first heartbeat, it appends a
// durable event and requires delivery well before PollInterval so LISTEN/NOTIFY
// (not the poll fallback) must be the wake path. The terminal run.succeeded
// SSE frame must be parsed before reader EOF, then the writer must complete.
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
	cfg.PollInterval = liveWakePollInterval
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
	liveCh := make(chan sseFrame, 1)
	terminalCh := make(chan sseFrame, 1)
	eofCh := make(chan struct{})
	parseErr := make(chan error, 1)
	go func() {
		defer close(eofCh)
		var cur sseFrame
		for {
			line, err := reader.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "id: "):
				fmt.Sscanf(line[4:], "%d", &cur.ID)
			case strings.HasPrefix(line, "event: "):
				cur.Event = line[7:]
			case strings.HasPrefix(line, "data: "):
				if err2 := json.Unmarshal([]byte(line[6:]), &cur.Data); err2 != nil {
					parseErr <- fmt.Errorf("unmarshal frame: %w", err2)
					return
				}
			case line == "":
				if cur.Event != "" || cur.Data.RunID != "" {
					frame := cur
					cur = sseFrame{}
					switch frame.Data.Kind {
					case "run.text":
						select {
						case liveCh <- frame:
						default:
						}
					case "run.succeeded":
						select {
						case terminalCh <- frame:
						default:
						}
					}
				}
			case strings.HasPrefix(line, ": keepalive"):
				select {
				case <-sawKeepalive:
				default:
					close(sawKeepalive)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-sawKeepalive:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE did not emit a heartbeat keep-alive before the live event")
	}

	payload := `{"i":1}`
	appended, err := svc.AppendEvent(ctx, appservice.AppendEventCmd{
		RunID:          run.ID,
		OwnerNamespace: ns,
		Kind:           "run.text",
		Payload:        &payload,
	})
	require.NoError(t, err)
	notifiedAt := time.Now()

	var live sseFrame
	select {
	case err := <-parseErr:
		t.Fatal(err)
	case live = <-liveCh:
	case <-time.After(liveWakeNotifyBudget):
		t.Fatalf("SSE did not deliver the concurrently appended durable event via LISTEN/NOTIFY within %s (poll interval %s)", liveWakeNotifyBudget, liveWakePollInterval)
	}
	elapsed := time.Since(notifiedAt)
	if elapsed >= liveWakePollInterval {
		t.Fatalf("live SSE delivery took %s, which is not sub-poll (poll=%s)", elapsed, liveWakePollInterval)
	}
	require.Equal(t, run.ID, live.Data.RunID)
	require.Equal(t, appended.Sequence, live.ID)
	require.Equal(t, appended.Sequence, live.Data.Sequence)
	require.Equal(t, "run.text", live.Event)
	require.Equal(t, "run.text", live.Data.Kind)
	require.NotNil(t, live.Data.Payload)
	require.Equal(t, payload, *live.Data.Payload)

	running, err := svc.ClaimRun(ctx, run.ID, ns, run.StateVersion, time.Now().Add(time.Minute))
	require.NoError(t, err)
	terminated, err := svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID: run.ID, OwnerNamespace: ns, StateVersion: running.StateVersion,
		FromStatus: domain.RunStatusRunning, TermStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)
	require.Equal(t, domain.RunStatusSucceeded, terminated.Status)

	var terminal sseFrame
	select {
	case err := <-parseErr:
		t.Fatal(err)
	case terminal = <-terminalCh:
	case <-eofCh:
		t.Fatal("reader reached EOF before a parsed run.succeeded SSE frame")
	case <-time.After(2 * time.Second):
		t.Fatal("SSE did not deliver a parsed run.succeeded frame")
	}
	require.Equal(t, run.ID, terminal.Data.RunID)
	require.Equal(t, "run.succeeded", terminal.Event)
	require.Equal(t, "run.succeeded", terminal.Data.Kind)
	require.Greater(t, terminal.ID, live.ID)
	require.Equal(t, terminal.ID, terminal.Data.Sequence)

	select {
	case <-eofCh:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE reader did not complete after the terminal frame")
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
