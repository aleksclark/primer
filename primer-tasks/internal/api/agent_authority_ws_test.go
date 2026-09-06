package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type agentWSTest struct {
	t    *testing.T
	db   *pgxpool.Pool
	s    *Server
	http *httptest.Server
	ctx  context.Context
}

func newAgentWSTest(t *testing.T) *agentWSTest {
	t.Helper()
	t.Setenv("TASKS_ENV", "test")
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", strings.Join(defaultToolNames(), ","))
	t.Setenv("TASKS_AGENT_SCRIPTED_DELAY_MS", "0")
	db := integrationPool(t)
	seedIntegration(t, db)
	s := New(db, "test")
	h := httptest.NewServer(s.Routes())
	t.Cleanup(h.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &agentWSTest{t: t, db: db, s: s, http: h, ctx: ctx}
}
func (h *agentWSTest) conversation(actor string) string {
	h.t.Helper()
	r := requestJSON(h.t, h.s.Routes(), http.MethodPost, "/agent/conversations", actor, "")
	if r.Code != 201 {
		h.t.Fatalf("create conversation: %d %s", r.Code, r.Body.String())
	}
	var c AgentConversation
	if json.Unmarshal(r.Body.Bytes(), &c) != nil {
		h.t.Fatal("conversation JSON")
	}
	return c.ID
}
func (h *agentWSTest) socket(actor string) *websocket.Conn {
	h.t.Helper()
	r := requestJSON(h.t, h.s.Routes(), http.MethodGet, "/auth/session", actor, "")
	var csrf string
	for _, c := range r.Result().Cookies() {
		if c.Name == "tasks_csrf" {
			csrf = c.Value
		}
	}
	headers := http.Header{"Origin": []string{h.s.Auth.PublicOrigin}, "Cookie": []string{"tasks_parent=" + actor + "; tasks_csrf=" + csrf}}
	c, _, e := websocket.Dial(h.ctx, "ws"+strings.TrimPrefix(h.http.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: headers, Subprotocols: []string{"primer-tasks.v1.csrf." + csrf, "primer-tasks.v1"}})
	if e != nil {
		h.t.Fatal(e)
	}
	h.t.Cleanup(func() { _ = c.CloseNow() })
	if v := h.read(c); v.Type != "hello" {
		h.t.Fatalf("hello: %+v", v)
	}
	return c
}
func (h *agentWSTest) send(c *websocket.Conn, cmd agentCommand) {
	h.t.Helper()
	cmd.ProtocolVersion = 1
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()
	if e := wsjson.Write(ctx, c, cmd); e != nil {
		h.t.Fatal(e)
	}
}
func (h *agentWSTest) read(c *websocket.Conn) wireAgentEvent {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	var v wireAgentEvent
	if e := wsjson.Read(ctx, c, &v); e != nil {
		h.t.Fatal(e)
	}
	if v.Cursor > 0 {
		h.send(c, agentCommand{Type: "ack", Cursor: v.Cursor})
	}
	return v
}
func (h *agentWSTest) until(c *websocket.Conn, kind, phase string) wireAgentEvent {
	h.t.Helper()
	for i := 0; i < 1000; i++ {
		v := h.read(c)
		if v.Type == kind && (phase == "" || v.Phase == phase) {
			return v
		}
		if v.Type == "terminal" && v.Status == "failed" {
			h.t.Fatalf("unexpected failed run: %+v", v)
		}
	}
	h.t.Fatal("event not reached")
	return wireAgentEvent{}
}
func (h *agentWSTest) run(c *websocket.Conn, conv, key, text string) wireAgentEvent {
	h.send(c, agentCommand{Type: "subscribe", ConversationID: conv})
	h.send(c, agentCommand{Type: "user_message", ConversationID: conv, ClientMessageID: key, Text: text})
	for i := 0; i < 1000; i++ {
		event := h.read(c)
		if event.Type == "user_message" && event.ClientMessageID == key {
			return event
		}
	}
	h.t.Fatal("message acknowledgement missing")
	return wireAgentEvent{}
}
func (h *agentWSTest) count(query string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRow(h.ctx, query, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}
func (h *agentWSTest) awaitCount(query string, want int, args ...any) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if h.count(query, args...) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("count did not reach %d: %s", want, query)
}
func (h *agentWSTest) secondActor() {
	h.t.Helper()
	_, err := h.db.Exec(h.ctx, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES($1,'parent-a2','admin')`, tenantA)
	if err != nil {
		h.t.Fatal(err)
	}
	_, err = h.db.Exec(h.ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($2,$1,'parent-a2','parent',now()+interval '1 hour')`, tenantA, hash("parent-a2"))
	if err != nil {
		h.t.Fatal(err)
	}
}

func TestPublicAgentAtomicAdmissionAndActorIsolation(t *testing.T) {
	h := newAgentWSTest(t)
	h.secondActor()
	conv := h.conversation("parent-a")
	a, b := h.socket("parent-a"), h.socket("parent-a2")
	// A database failure at the final admission step must roll back the message
	// and run, leaving the same client id retryable.
	_, err := h.db.Exec(h.ctx, `CREATE FUNCTION reject_agent_job() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected job insert failure'; END $$; CREATE TRIGGER reject_agent_job BEFORE INSERT ON agent_jobs FOR EACH ROW EXECUTE FUNCTION reject_agent_job()`)
	if err != nil {
		t.Fatal(err)
	}
	h.send(a, agentCommand{Type: "user_message", ConversationID: conv, ClientMessageID: "atomic", Text: "List my tasks."})
	h.send(a, agentCommand{Type: "unknown"})
	h.until(a, "error", "")
	if n := h.count(`SELECT count(*) FROM agent_messages WHERE conversation_id=$1`, conv); n != 0 {
		t.Fatalf("orphan messages: %d", n)
	}
	if n := h.count(`SELECT count(*) FROM agent_runs WHERE conversation_id=$1`, conv); n != 0 {
		t.Fatalf("orphan runs: %d", n)
	}
	if _, err = h.db.Exec(h.ctx, `DROP TRIGGER reject_agent_job ON agent_jobs; DROP FUNCTION reject_agent_job()`); err != nil {
		t.Fatal(err)
	}
	accepted := h.run(a, conv, "atomic", "List my tasks.")
	// Duplicate delivery does not enqueue twice, and a household peer cannot
	// cancel or subscribe to this actor's conversation.
	h.send(a, agentCommand{Type: "user_message", ConversationID: conv, ClientMessageID: "atomic", Text: "changed prompt"})
	h.send(b, agentCommand{Type: "cancel", RunID: accepted.RunID})
	h.send(b, agentCommand{Type: "subscribe", ConversationID: conv})
	h.send(b, agentCommand{Type: "unknown"})
	if got := h.read(b); got.Type != "error" {
		t.Fatalf("foreign actor received: %+v", got)
	}
	if n := h.count(`SELECT count(*) FROM agent_jobs WHERE run_id=$1`, accepted.RunID); n != 1 {
		t.Fatalf("duplicate jobs: %d", n)
	}
	if n := h.count(`SELECT count(*) FROM agent_runs WHERE id=$1 AND cancel_requested`, accepted.RunID); n != 0 {
		t.Fatal("peer canceled run")
	}
	// Newly connected and unsubscribed sockets are never tenant-wide wildcards.
	h.s.StartAgentWorker(h.ctx)
	h.until(a, "terminal", "")
	h.send(b, agentCommand{Type: "unknown"})
	if got := h.read(b); got.Type != "error" {
		t.Fatalf("unsubscribed leak: %+v", got)
	}
	h.send(a, agentCommand{Type: "unsubscribe"})
	h.send(a, agentCommand{Type: "unknown"})
	h.until(a, "error", "")
	if err = h.s.publishAgent(h.ctx, tenantA, conv, wireAgentEvent{Type: "error", RunID: accepted.RunID, Code: "test_safe"}); err != nil {
		t.Fatal(err)
	}
	h.send(a, agentCommand{Type: "unknown"})
	if got := h.read(a); got.Type != "error" || got.RunID != "" {
		t.Fatalf("unsubscribed durable event: %+v", got)
	}
}

func TestPublicAgentReplayBeyondTwoHundredAndConcurrentTail(t *testing.T) {
	h := newAgentWSTest(t)
	conv := h.conversation("parent-a")
	c := h.socket("parent-a")
	run := h.run(c, conv, "replay", "List tasks.")
	_ = c.CloseNow()
	for i := 0; i < 250; i++ {
		if err := h.s.publishAgent(h.ctx, tenantA, conv, wireAgentEvent{Type: "text_delta", RunID: run.RunID, Text: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	c = h.socket("parent-a")
	h.send(c, agentCommand{Type: "subscribe", ConversationID: conv, Cursor: 0})
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 25; i++ {
			if err := h.s.publishAgent(h.ctx, tenantA, conv, wireAgentEvent{Type: "text_delta", RunID: run.RunID, Text: "live"}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for seq := int64(1); seq <= 276; seq++ {
		v := h.read(c)
		if v.Sequence != seq {
			t.Fatalf("replay gap/reorder got=%d want=%d", v.Sequence, seq)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = c.CloseNow()
	// A new Server object proves replay is not an in-memory history cache.
	h.s = New(h.db, "test")
	h.http.Close()
	h.http = httptest.NewServer(h.s.Routes())
	t.Cleanup(h.http.Close)
	c = h.socket("parent-a")
	h.send(c, agentCommand{Type: "subscribe", ConversationID: conv, Cursor: 200})
	for seq := int64(201); seq <= 276; seq++ {
		v := h.read(c)
		if v.Sequence != seq {
			t.Fatalf("restart cursor=%d want=%d", v.Sequence, seq)
		}
	}
}

func TestPublicAgentConfirmedEffectsCancelAndReplay(t *testing.T) {
	h := newAgentWSTest(t)
	h.secondActor()
	h.s.StartAgentWorker(h.ctx)
	conv := h.conversation("parent-a")
	c := h.socket("parent-a")
	h.run(c, conv, "preview-one", "Create and schedule a task.")
	preview := h.until(c, "tool_progress", "awaiting_confirmation")
	if n := h.count(`SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenantA); n != 0 {
		t.Fatal("schedule mutated before confirmation")
	}
	if n := h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenantA); n != 0 {
		t.Fatal("task mutated before confirmation")
	}
	// Peer actor, mismatched conversation and disallowed tools cannot acknowledge.
	b := h.socket("parent-a2")
	h.send(b, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	h.send(b, agentCommand{Type: "unknown"})
	h.until(b, "error", "")
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConversationID: h.conversation("parent-a"), ConfirmationID: preview.ConfirmationID})
	h.until(c, "error", "")
	if h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("foreign confirmation applied")
	}
	h.send(c, agentCommand{Type: "cancel", RunID: preview.RunID})
	h.until(c, "terminal", "")
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	h.until(c, "error", "")
	if h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("canceled effect applied")
	}
	conv = h.conversation("parent-a")
	h.run(c, conv, "preview-two", "Create and schedule a task.")
	preview = h.until(c, "tool_progress", "awaiting_confirmation")
	other := h.socket("parent-a")
	var wg sync.WaitGroup
	for _, socket := range []*websocket.Conn{c, other} {
		wg.Add(1)
		go func(socket *websocket.Conn) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
			defer cancel()
			_ = wsjson.Write(ctx, socket, agentCommand{ProtocolVersion: 1, Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
		}(socket)
	}
	wg.Wait()
	terminal := h.until(c, "terminal", "")
	if terminal.Status != "completed" {
		t.Fatalf("confirmation terminal %+v", terminal)
	}
	if n := h.count(`SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenantA); n != 1 {
		t.Fatalf("schedules=%d", n)
	}
	if n := h.count(`SELECT count(*) FROM agent_tool_effects WHERE run_id=$1 AND status='applied'`, preview.RunID); n != 3 {
		t.Fatalf("effects=%d", n)
	}
	if n := h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND event_type='terminal'`, preview.RunID); n != 1 {
		t.Fatalf("terminals=%d", n)
	}
	if n := h.count(`SELECT count(*) FROM agent_run_events WHERE tenant_id=$1 AND payload::text LIKE '%hidden%'`, tenantA); n != 0 {
		t.Fatal("raw reasoning in events")
	}
	// Issue a version-bound disabling preview, edit through the ordinary public
	// schedule API, and reject its now-stale CAS without consuming the handle.
	conv = h.conversation("parent-a")
	h.run(c, conv, "stale", "Disable schedule.")
	preview = h.until(c, "tool_progress", "awaiting_confirmation")
	listed := requestJSON(t, h.s.Routes(), http.MethodGet, "/schedules", "parent-a", "")
	var page struct {
		Items []Schedule2 `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatalf("schedule list: %s", listed.Body.String())
	}
	row := page.Items[0]
	body, _ := json.Marshal(ScheduleInput2{StudentID: row.StudentID, TemplateID: row.TemplateID, RevisionID: row.RevisionID, Kind: row.Kind, Timezone: row.Timezone, StartAt: row.StartAt, DueOffsetMinutes: 20})
	updated := requestJSON(t, h.s.Routes(), http.MethodPatch, "/schedules/"+row.ID, "parent-a", string(body))
	if updated.Code != 200 {
		t.Fatalf("public schedule edit: %d %s", updated.Code, updated.Body.String())
	}
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	h.until(c, "error", "")
	if n := h.count(`SELECT count(*) FROM task_schedules WHERE id=$1 AND enabled AND version=2`, row.ID); n != 1 {
		t.Fatal("stale CAS overwrote newer schedule")
	}
	if n := h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1 AND consumed_at IS NULL`, preview.RunID); n != 1 {
		t.Fatal("failed effect consumed its preview")
	}
}
