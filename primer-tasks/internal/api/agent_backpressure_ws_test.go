package api

import (
	"context"
	"fmt"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"testing"
	"time"
)

func TestPublicAgentSlowSubscriberCloses1013WithoutBlockingHealthyTenant(t *testing.T) {
	h := newAgentWSTest(t)
	conv := h.conversation("parent-a")
	creator := h.socket("parent-a")
	run := h.run(creator, conv, "slow", "List students.")
	_ = creator.CloseNow()
	for i := 0; i < 100; i++ {
		if err := h.s.publishAgent(h.ctx, tenantA, conv, wireAgentEvent{Type: "text_delta", RunID: run.RunID, Text: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	slow := h.socket("parent-a")
	h.send(slow, agentCommand{Type: "subscribe", ConversationID: conv})
	// Do not read or acknowledge slow. The application window, not arbitrarily
	// large kernel buffers, bounds this connection's outstanding data.
	healthy := h.socket("parent-b")
	healthyConv := h.conversation("parent-b")
	h.s.StartAgentWorker(h.ctx)
	h.run(healthy, healthyConv, "healthy", "List students.")
	terminal := h.until(healthy, "terminal", "")
	if terminal.Status != "completed" {
		t.Fatalf("healthy tenant %+v", terminal)
	}
	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()
	received := 0
	for {
		var v wireAgentEvent
		err := wsjson.Read(ctx, slow, &v)
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusTryAgainLater {
				t.Fatalf("slow subscriber close=%d err=%v", websocket.CloseStatus(err), err)
			}
			break
		}
		received++
	}
	if received != 64 {
		t.Fatalf("unacknowledged frames=%d want bounded window 64", received)
	}
	h.awaitCount(`SELECT count(*) FROM agent_jobs WHERE status='running'`, 0)
	// The connection handler and its writer/tailer are removed rather than left
	// blocked after the slow peer is evicted.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if h.s.agentHub.subscriberCountForTest() == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subscriber leak: %d", h.s.agentHub.subscriberCountForTest())
}
