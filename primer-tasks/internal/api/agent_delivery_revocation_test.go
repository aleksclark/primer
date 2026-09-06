package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// This gate wraps the REAL server TCP connection, below net/http and WebSocket.
// It never manufactures a frame or replaces the Tasks handler. A selected
// private frame can be paused at its actual network write to inspect PG locks.
// Close unblocks it, so the production WebSocket context deadline still owns
// the bound, including the deliberately stalled-reader regression.
type privateFrameGate struct {
	marker                     []byte
	pause                      bool
	armed                      atomic.Bool
	once                       sync.Once
	entered, release, finished chan struct{}
	wrote                      atomic.Bool
}

func newPrivateFrameGate(marker string, pause bool) *privateFrameGate {
	return &privateFrameGate{marker: []byte(marker), pause: pause, entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
}

type privateGateListener struct {
	net.Listener
	gate *privateFrameGate
}

func (l *privateGateListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e != nil {
		return nil, e
	}
	return &privateGateConn{Conn: c, gate: l.gate, closed: make(chan struct{})}, nil
}

type privateGateConn struct {
	net.Conn
	gate   *privateFrameGate
	closed chan struct{}
	once   sync.Once
}

func (c *privateGateConn) Close() error { c.once.Do(func() { close(c.closed) }); return c.Conn.Close() }
func (c *privateGateConn) Write(p []byte) (int, error) {
	selected := false
	if c.gate.armed.Load() && bytes.Contains(p, c.gate.marker) {
		c.gate.once.Do(func() { selected = true; close(c.gate.entered) })
	}
	if selected {
		defer close(c.gate.finished)
		if c.gate.pause {
			select {
			case <-c.gate.release:
			case <-c.closed:
				return 0, net.ErrClosed
			}
		}
	}
	n, err := c.Conn.Write(p)
	if selected && n == len(p) && err == nil {
		c.gate.wrote.Store(true)
	}
	return n, err
}
func deliveryBarrier(t *testing.T, ctx context.Context, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatalf("%s barrier not observed: %v", name, ctx.Err())
	}
}

// The observation is PostgreSQL's real blocking graph, not an arbitrary sleep.
// A timeout is a failed positive lock assertion, never proof of zero frames.
func awaitDeliveryLock(t *testing.T, h *agentWSTest, ctx context.Context, holderPID int, unexpected <-chan error) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0 AND ($1::int=0 OR $1=ANY(pg_blocking_pids(pid))))`, holderPID).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case err := <-unexpected:
			t.Fatalf("revocation completed before frame release: %v", err)
		case <-ctx.Done():
			t.Fatal("expected PostgreSQL lock wait was not observed")
		case <-ticker.C:
		}
	}
}

type deliveryRevocationCase struct {
	name                 string
	clerk                bool
	lock, change, assert string
	args                 []any
	publicLogout         bool
	queued               bool
}

func deliveryCases() []deliveryRevocationCase {
	return []deliveryRevocationCase{
		{name: "membership", lock: `SELECT subject_ref FROM parent_memberships WHERE tenant_id=$1 AND subject_ref='parent-a' FOR UPDATE`, change: `UPDATE parent_memberships SET revoked_at=now() WHERE tenant_id=$1 AND subject_ref='parent-a'`, assert: `SELECT count(*) FROM parent_memberships WHERE tenant_id=$1 AND subject_ref='parent-a' AND revoked_at IS NOT NULL`, args: []any{tenantA}},
		{name: "BFF session", lock: `SELECT subject_ref FROM bff_sessions WHERE handle_hash=$1 FOR UPDATE`, change: `UPDATE bff_sessions SET revoked_at=now() WHERE handle_hash=$1`, assert: `SELECT count(*) FROM bff_sessions WHERE handle_hash=$1 AND revoked_at IS NOT NULL`, args: []any{hash("parent-a")}, publicLogout: true},
		{name: "Clerk identity", clerk: true, lock: `SELECT subject FROM parent_identities WHERE issuer=$1 AND subject='parent-a' FOR UPDATE`, change: `UPDATE parent_identities SET revoked_at=now() WHERE issuer=$1 AND subject='parent-a'`, assert: `SELECT count(*) FROM parent_identities WHERE issuer=$1 AND subject='parent-a' AND revoked_at IS NOT NULL`, args: []any{agentFixtureIssuer}},
		{name: "Clerk session insertion", clerk: true, lock: `SELECT subject FROM parent_identities WHERE issuer=$1 AND subject='parent-a' FOR UPDATE`, change: `INSERT INTO parent_session_revocations(issuer,session_id) VALUES($1,'session-parent-a')`, assert: `SELECT count(*) FROM parent_session_revocations WHERE issuer=$1 AND session_id='session-parent-a'`, args: []any{agentFixtureIssuer}, publicLogout: true},
		{name: "conversation actor", lock: `SELECT id FROM agent_conversations WHERE id=$1 FOR UPDATE`, change: `UPDATE agent_conversations SET actor_id='parent-a2' WHERE id=$1`, assert: `SELECT count(*) FROM agent_conversations WHERE id=$1 AND actor_id='parent-a2'`},
		{name: "conversation archived", lock: `SELECT id FROM agent_conversations WHERE id=$1 FOR UPDATE`, change: `UPDATE agent_conversations SET status='archived' WHERE id=$1`, assert: `SELECT count(*) FROM agent_conversations WHERE id=$1 AND status='archived'`},
		{name: "queued private Clerk control", clerk: true, lock: `SELECT subject FROM parent_identities WHERE issuer=$1 AND subject='parent-a' FOR UPDATE`, change: `INSERT INTO parent_session_revocations(issuer,session_id) VALUES($1,'session-parent-a')`, assert: `SELECT count(*) FROM parent_session_revocations WHERE issuer=$1 AND session_id='session-parent-a'`, args: []any{agentFixtureIssuer}, publicLogout: true, queued: true},
	}
}

type deliveryFixture struct {
	h            *agentWSTest
	gate         *privateFrameGate
	client       *websocket.Conn
	conversation string
	seed         wireAgentEvent
	token        string
	scenario     deliveryRevocationCase
}

func newDeliveryFixture(t *testing.T, scenario deliveryRevocationCase, pause bool) *deliveryFixture {
	t.Helper()
	var h *agentWSTest
	var clerkFixture *clerkAgentFixture
	if scenario.clerk {
		clerkFixture = newClerkAgentFixture(t)
		h = clerkFixture.agentWSTest
	} else {
		h = newAgentWSTest(t)
		h.secondActor()
	}
	marker := "private-frame-" + uuid.NewString()
	gate := newPrivateFrameGate(marker, pause)
	h.http.Close()
	var handler http.Handler = h.s.Routes()
	if scenario.clerk {
		handler = Mount(handler, "/tasks", t.TempDir())
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = &privateGateListener{Listener: server.Listener, gate: gate}
	server.Start()
	h.http = server
	t.Cleanup(server.Close)
	var token, conv string
	var producer, client *websocket.Conn
	if scenario.clerk {
		token = clerkFixture.token("parent-a", nil)
		conv = clerkFixture.create(token)
		producer = clerkFixture.socketToken(token)
	} else {
		conv = h.conversation("parent-a")
		producer = h.socket("parent-a")
	}
	seed := h.run(producer, conv, "delivery-seed", marker)
	// Observe actual producer subscription teardown before selecting the new
	// socket. This avoids any test-only tenant-wide broadcast or subscriber race.
	h.s.agentHub.mu.Lock()
	var previous *agentSubscriber
	for sub := range h.s.agentHub.subscribers {
		previous = sub
	}
	h.s.agentHub.mu.Unlock()
	_ = producer.CloseNow()
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()
	deliveryBarrier(t, ctx, previous.done, "producer teardown")
	if scenario.clerk {
		client = clerkFixture.socketToken(token)
	} else {
		client = h.socket("parent-a")
	}
	if len(scenario.args) == 0 {
		scenario.args = []any{conv}
	}
	return &deliveryFixture{h: h, gate: gate, client: client, conversation: conv, seed: seed, token: token, scenario: scenario}
}
func (f *deliveryFixture) startFrame() {
	f.gate.armed.Store(true)
	if f.scenario.queued {
		f.h.s.agentHub.mu.Lock()
		var sub *agentSubscriber
		for current := range f.h.s.agentHub.subscribers {
			sub = current
		}
		f.h.s.agentHub.mu.Unlock()
		if sub == nil {
			f.h.t.Fatal("missing real subscriber")
		}
		f.h.s.sendAgentToSubscriber(sub, wireAgentEvent{ProtocolVersion: 1, Type: "error", ConversationID: f.conversation, RunID: f.seed.RunID, Code: "private_control", Text: string(f.gate.marker), Time: time.Now().UTC()})
	} else {
		f.h.send(f.client, agentCommand{Type: "subscribe", ConversationID: f.conversation})
	}
}
func (f *deliveryFixture) assertRevoked() {
	if f.h.count(f.scenario.assert, f.scenario.args...) != 1 {
		f.h.t.Fatal("revocation did not commit durably")
	}
}
func (f *deliveryFixture) readDenied(ctx context.Context) {
	f.h.t.Helper()
	var frame wireAgentEvent
	err := wsjson.Read(ctx, f.client, &frame)
	if err == nil {
		f.h.t.Fatal("protected frame was delivered after revocation committed")
	}
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		f.h.t.Fatalf("expected explicit policy close, got status %d", websocket.CloseStatus(err))
	}
	if f.gate.wrote.Load() {
		f.h.t.Fatal("private network write occurred after revocation")
	}
}
func (f *deliveryFixture) beginHolder(ctx context.Context) (pgx.Tx, int) {
	f.h.t.Helper()
	tx, err := f.h.db.Begin(ctx)
	if err != nil {
		f.h.t.Fatal(err)
	}
	f.h.t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	var pid int
	if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		f.h.t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, f.scenario.lock, f.scenario.args...); err != nil {
		f.h.t.Fatal(err)
	}
	return tx, pid
}
func (f *deliveryFixture) revoke(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() {
		if f.scenario.publicLogout {
			path := "/auth/logout"
			if f.scenario.clerk {
				path = "/tasks/api/auth/logout"
			}
			req, err := http.NewRequestWithContext(ctx, "POST", f.h.http.URL+path, nil)
			if err != nil {
				done <- err
				return
			}
			if f.scenario.clerk {
				req.Header.Set("Authorization", "Bearer "+f.token)
			} else {
				req.AddCookie(&http.Cookie{Name: "tasks_parent", Value: "parent-a"})
			}
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = res.Body.Close()
				if res.StatusCode != 200 {
					err = fmt.Errorf("logout status %d", res.StatusCode)
				}
			}
			done <- err
			return
		}
		tx, err := f.h.db.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, f.scenario.lock, f.scenario.args...); err == nil {
			_, err = tx.Exec(ctx, f.scenario.change, f.scenario.args...)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		done <- err
	}()
	return done
}

func TestPublicAgentRevocationBeforePrivateWriteUsesFreshPostWaitState(t *testing.T) {
	for _, scenario := range deliveryCases() {
		t.Run(scenario.name, func(t *testing.T) {
			f := newDeliveryFixture(t, scenario, false)
			ctx, cancel := context.WithTimeout(f.h.ctx, 6*time.Second)
			defer cancel()
			holder, pid := f.beginHolder(ctx)
			f.startFrame()
			awaitDeliveryLock(t, f.h, ctx, pid, nil)
			// For Clerk session insertion the waiting identity row is unchanged. The
			// INSERT happens only after the tail has already started its locking SELECT;
			// a NOT EXISTS evaluated from that pre-wait snapshot would incorrectly pass.
			if _, err := holder.Exec(ctx, scenario.change, f.scenario.args...); err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			f.assertRevoked()
			f.readDenied(ctx) // explicit 1008 close, not a no-frame timeout assertion
		})
	}
}

func TestPublicAgentPrivateWriteBeforeRevocationHoldsLocksUntilFrameReturns(t *testing.T) {
	for _, scenario := range deliveryCases() {
		t.Run(scenario.name, func(t *testing.T) {
			f := newDeliveryFixture(t, scenario, true)
			ctx, cancel := context.WithTimeout(f.h.ctx, 6*time.Second)
			defer cancel()
			f.startFrame()
			deliveryBarrier(t, ctx, f.gate.entered, "real private TCP write")
			revoked := f.revoke(ctx)
			awaitDeliveryLock(t, f.h, ctx, 0, revoked)
			select {
			case err := <-revoked:
				t.Fatalf("revocation committed while private write held: %v", err)
			default:
			}
			close(f.gate.release)
			deliveryBarrier(t, ctx, f.gate.finished, "real frame write return")
			if !f.gate.wrote.Load() {
				t.Fatal("real private frame was not written")
			}
			select {
			case err := <-revoked:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("revocation did not unblock after frame")
			}
			f.assertRevoked()
			var frame wireAgentEvent
			if err := wsjson.Read(ctx, f.client, &frame); err != nil {
				t.Fatal("serialized private frame missing")
			}
			if !strings.Contains(frame.Text, string(f.gate.marker)) {
				t.Fatal("wrong real frame observed")
			}
		})
	}
}

func TestPublicAgentStalledPrivateWriteBoundsRevocationLockLifetime(t *testing.T) {
	scenario := deliveryCases()[3] // real Clerk HTTP logout, not an alternate revoker
	f := newDeliveryFixture(t, scenario, true)
	ctx, cancel := context.WithTimeout(f.h.ctx, 7*time.Second)
	defer cancel()
	f.startFrame()
	deliveryBarrier(t, ctx, f.gate.entered, "stalled real private TCP write")
	revoked := f.revoke(ctx)
	awaitDeliveryLock(t, f.h, ctx, 0, revoked)
	// Do NOT release the writer. The production per-frame hard deadline must
	// close its transport and release PG locks, independent of this test context.
	deliveryBarrier(t, ctx, f.gate.finished, "production bounded write termination")
	if f.gate.wrote.Load() {
		t.Fatal("stalled private bytes were unexpectedly written")
	}
	select {
	case err := <-revoked:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("stalled subscriber retained revocation locks")
	}
	f.assertRevoked()
	var frame wireAgentEvent
	err := wsjson.Read(ctx, f.client, &frame)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("stalled connection did not reach an actual terminal transport outcome")
	}
}
