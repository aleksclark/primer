package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
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

// writePrivateAgentFrame linearizes each private frame with local revocation.
// The transaction covers exactly ONE bounded frame, not a replay page. Locks
// are acquired before fresh READ COMMITTED authorization statements, including
// after waiting for a conversation or a Clerk logout's identity lock.
func (s *Server) writePrivateAgentFrame(ctx context.Context, conn *websocket.Conn, sc scope, authorization *agentAuthorization, event wireAgentEvent) error {
	if authorization == nil {
		return parent.ErrInvalidContext
	}
	frameCtx, cancel := context.WithTimeout(context.WithValue(ctx, agentAuthKey{}, authorization), agentWriteTimeout+time.Second)
	defer cancel()
	tx, err := s.DB.Begin(frameCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(frameCtx)
	if _, err = tx.Exec(frameCtx, `SET TRANSACTION ISOLATION LEVEL READ COMMITTED`); err != nil {
		return err
	}
	if err = lockAgentParentRows(frameCtx, tx, sc.Tenant, sc.Subject); err != nil {
		return err
	}
	conversation := event.ConversationID
	if conversation == "" && event.RunID != "" {
		if err = tx.QueryRow(frameCtx, `SELECT conversation_id FROM agent_runs WHERE tenant_id=$1 AND id=$2`, sc.Tenant, event.RunID).Scan(&conversation); err != nil {
			return parent.ErrInvalidContext
		}
	}
	if conversation != "" {
		var locked string
		if err = tx.QueryRow(frameCtx, `SELECT id FROM agent_conversations WHERE tenant_id=$1 AND id=$2 FOR SHARE`, sc.Tenant, conversation).Scan(&locked); err != nil {
			return parent.ErrInvalidContext
		}
	}
	// Fresh statements AFTER all potentially waiting locks. In particular, do
	// not combine Clerk session NOT EXISTS with the identity-locking SELECT.
	if err = checkLockedAgentParent(frameCtx, tx, sc.Tenant, sc.Subject); err != nil {
		return err
	}
	if conversation != "" {
		var allowed bool
		if err = tx.QueryRow(frameCtx, `SELECT EXISTS(SELECT 1 FROM agent_conversations WHERE tenant_id=$1 AND id=$2 AND actor_id=$3 AND status='active')`, sc.Tenant, conversation, sc.Subject).Scan(&allowed); err != nil || !allowed {
			return parent.ErrInvalidContext
		}
	}
	return writeAgentFrame(frameCtx, conn, event)
}

func closeAgentDelivery(conn *websocket.Conn, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		_ = conn.Close(websocket.StatusTryAgainLater, "bounded private delivery timed out; reconnect using durable cursor")
		return
	}
	_ = conn.Close(websocket.StatusPolicyViolation, "private delivery authorization unavailable or revoked")
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
			// Socket-bound control frames can also contain private run data.
			if err := s.writePrivateAgentFrame(ctx, conn, sc, sub.authorization, event); err != nil {
				closeAgentDelivery(conn, err)
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
				if err = s.writePrivateAgentFrame(ctx, conn, sc, sub.authorization, event); err != nil {
					break
				}
				sub.cursor = item.Sequence
			}
		}
		sub.mu.Unlock()
		if err != nil {
			closeAgentDelivery(conn, err)
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
