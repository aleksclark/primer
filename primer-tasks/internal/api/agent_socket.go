package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"primer-tasks/internal/agent"
)

const agentWriteTimeout = 2 * time.Second

func writeAgentFrame(ctx context.Context, conn *websocket.Conn, event wireAgentEvent) error {
	if err := validateSocketEvent(event); err != nil {
		return err
	}
	// Close carries the diagnosable 1013 outcome before the write's hard deadline.
	slow := time.AfterFunc(agentWriteTimeout, func() {
		_ = conn.Close(websocket.StatusTryAgainLater, "slow subscriber; reconnect using durable cursor")
	})
	defer slow.Stop()
	writeCtx, cancel := context.WithTimeout(ctx, agentWriteTimeout+time.Second)
	defer cancel()
	return wsjson.Write(writeCtx, conn, event)
}

// tailAgentSocket is the sole writer after hello. Live delivery and historical
// replay use the same cursor query, with one bounded page resident at a time.
// Notifications only shorten latency; polling observes other server processes
// and closes the replay/live gap even when a notification is lost.
func (s *Server) tailAgentSocket(ctx context.Context, conn *websocket.Conn, sc scope, sub *agentSubscriber) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.done:
			return
		case event := <-sub.queue:
			if writeAgentFrame(ctx, conn, event) != nil {
				return
			}
		case <-ticker.C:
		case <-sub.wake:
		}
		if sub.authorization == nil || sub.authorization.check(ctx) != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "parent authorization expired or revoked")
			return
		}
		sub.mu.Lock()
		if sub.conversation == "" {
			sub.mu.Unlock()
			continue
		}
		// A removed parent cannot retain read authority by keeping a socket open.
		var allowed bool
		err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_conversations c JOIN parent_memberships p ON p.tenant_id=c.tenant_id AND p.subject_ref=c.actor_id WHERE c.tenant_id=$1 AND c.id=$2 AND c.actor_id=$3 AND c.status='active' AND p.revoked_at IS NULL AND p.role='admin')`, sc.Tenant, sub.conversation, sc.Subject).Scan(&allowed)
		if err != nil || !allowed {
			sub.mu.Unlock()
			_ = conn.Close(websocket.StatusPolicyViolation, "conversation access revoked")
			return
		}
		capacity := 64 - (sub.cursor - sub.acked)
		if capacity <= 0 {
			expired := time.Since(sub.lastAck) > agentWriteTimeout
			sub.mu.Unlock()
			if expired {
				_ = conn.Close(websocket.StatusTryAgainLater, "slow subscriber; acknowledgement window exhausted")
				return
			}
			continue
		}
		events, err := agent.NewPostgresRepository(s.DB).ReplayEvents(ctx, sc.Tenant, sub.conversation, sub.cursor, int(min(32, capacity)))
		if err == nil {
			for _, item := range events {
				if err = sub.authorization.check(ctx); err != nil {
					break
				}
				var event wireAgentEvent
				if json.Unmarshal(item.Payload, &event) != nil {
					err = errors.New("invalid durable event")
					break
				}
				if err = writeAgentFrame(ctx, conn, event); err != nil {
					break
				}
				sub.cursor = item.Sequence
			}
		}
		sub.mu.Unlock()
		if err != nil {
			return
		}
		if len(events) == 32 {
			select {
			case sub.wake <- struct{}{}:
			default:
			}
		}
	}
}
