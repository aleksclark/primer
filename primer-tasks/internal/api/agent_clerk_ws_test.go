package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.clark.team/aleksclark/authstack/auth"
	"git.clark.team/aleksclark/authstack/clerk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	tasksdb "primer-tasks/internal/db"
	"primer-tasks/internal/jobs"
)

// Cryptographically signed fixture credentials go through the published
// Authstack verifier. Its public clock injection exercises expiry without a
// timing sleep or a Tasks-side JWT parser. Credentials never appear in errors.
type clerkAgentFixture struct {
	*agentWSTest
	clock atomic.Int64
	key   *rsa.PrivateKey
}

const agentFixtureIssuer = "https://clerk.p3.invalid"

func newClerkAgentFixture(t *testing.T) *clerkAgentFixture {
	h := newAgentWSTest(t)
	h.secondActor()
	f := &clerkAgentFixture{agentWSTest: h}
	f.clock.Store(time.Now().Unix())
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.key = key
	v, err := clerk.NewAuthenticator(clerk.Config{Issuer: agentFixtureIssuer, JWKS: &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "p3-fixture", Algorithm: "RS256", Use: "sig"}}}, Now: func() time.Time { return time.Unix(f.clock.Load(), 0) }})
	if err != nil {
		t.Fatal(err)
	}
	h.s.Auth.Mode = "clerk"
	h.s.BasePath = "/tasks"
	h.s.ParentAuthenticator = v
	h.s.ParentPolicy = auth.AuthenticationPolicy{AcceptedCredentials: []auth.CredentialKind{auth.CredentialSession}, AuthorizedParties: []string{h.s.Auth.PublicOrigin}}
	for _, link := range []tasksdb.ParentLink{{Issuer: agentFixtureIssuer, Subject: "parent-a", TenantID: tenantA, ActorRef: "parent-a"}, {Issuer: agentFixtureIssuer, Subject: "parent-a2", TenantID: tenantA, ActorRef: "parent-a2"}, {Issuer: agentFixtureIssuer, Subject: "parent-b", TenantID: tenantB, ActorRef: "parent-b"}} {
		if err = tasksdb.BootstrapParent(h.ctx, h.db, link); err != nil {
			t.Fatal(err)
		}
	}
	h.http.Close()
	h.http = httptest.NewServer(Mount(h.s.Routes(), "/tasks", t.TempDir()))
	t.Cleanup(h.http.Close)
	return f
}
func (f *clerkAgentFixture) token(actor string, changes map[string]any) string {
	f.t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: f.key, KeyID: "p3-fixture"}}, nil)
	if err != nil {
		f.t.Fatal("fixture signer")
	}
	claims := map[string]any{"iss": agentFixtureIssuer, "sub": actor, "sid": "session-" + actor, "azp": f.s.Auth.PublicOrigin, "nbf": f.clock.Load() - 60, "exp": f.clock.Load() + 30}
	for k, v := range changes {
		claims[k] = v
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		f.t.Fatal("fixture signing")
	}
	return token
}
func (f *clerkAgentFixture) call(method, path, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	r := httptest.NewRequest(method, "/tasks/api"+path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	Mount(f.s.Routes(), "/tasks", "").ServeHTTP(w, r)
	return w
}
func (f *clerkAgentFixture) create(token string) string {
	f.t.Helper()
	w := f.call("POST", "/agent/conversations", token)
	if w.Code != 201 {
		f.t.Fatalf("conversation status %d", w.Code)
	}
	var c AgentConversation
	if json.Unmarshal(w.Body.Bytes(), &c) != nil {
		f.t.Fatal("conversation decode")
	}
	return c.ID
}
func (f *clerkAgentFixture) dial(token, origin string) (*websocket.Conn, *http.Response, error) {
	protocols := []string{"primer-tasks.v1", "primer-tasks.v1.csrf.fixture"}
	if token != "" {
		protocols = append(protocols, agentBearerProtocol+token)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.http.URL, "http")+"/tasks/api/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}, "Cookie": []string{"tasks_csrf=fixture"}}, Subprotocols: protocols})
}
func (f *clerkAgentFixture) socketToken(token string) *websocket.Conn {
	f.t.Helper()
	c, resp, err := f.dial(token, f.s.Auth.PublicOrigin)
	if err != nil {
		f.t.Fatal("authenticated fixture socket rejected")
	}
	f.t.Cleanup(func() { _ = c.CloseNow() })
	if c.Subprotocol() != "primer-tasks.v1" || resp.Header.Get("Sec-WebSocket-Protocol") != "primer-tasks.v1" {
		f.t.Fatal("credential protocol was echoed")
	}
	if f.read(c).Type != "hello" {
		f.t.Fatal("missing application hello")
	}
	return c
}
func (f *clerkAgentFixture) closed(c *websocket.Conn) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	var event wireAgentEvent
	err := wsjson.Read(ctx, c, &event)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		f.t.Fatal("expired/revoked socket did not close with policy violation")
	}
}
func (f *clerkAgentFixture) execute(s *Server, run wireAgentEvent) {
	f.t.Helper()
	if err := s.executeAgentRun(f.ctx, jobs.Job{TenantID: tenantA, RunID: run.RunID}); err != nil {
		f.t.Fatalf("fixture job failed: %v", err)
	}
}

func TestClerkAgentUpgradeUsesAuthstackWithoutCredentialEcho(t *testing.T) {
	f := newClerkAgentFixture(t)
	for name, token := range map[string]string{"missing": "", "malformed": "not-a-jwt", "expired": f.token("parent-a", map[string]any{"exp": f.clock.Load() - 1}), "issuer": f.token("parent-a", map[string]any{"iss": "https://foreign.invalid"}), "party": f.token("parent-a", map[string]any{"azp": "https://foreign.invalid"}), "machine": f.token("parent-a", map[string]any{"machine_id": "machine"}), "opaque student": "paired-student-secret", "unknown parent": f.token("unknown", nil)} {
		t.Run(name, func(t *testing.T) {
			c, resp, err := f.dial(token, f.s.Auth.PublicOrigin)
			if c != nil {
				_ = c.CloseNow()
			}
			if err == nil || resp == nil || (resp.StatusCode != 401 && resp.StatusCode != 403) {
				t.Fatal("invalid parent credential accepted")
			}
		})
	}
	token := f.token("parent-a", nil)
	if c, _, err := f.dial(token, "https://foreign.invalid"); err == nil {
		_ = c.CloseNow()
		t.Fatal("bearer bypassed origin policy")
	}
	c := f.socketToken(token)
	conv := f.create(token)
	f.send(c, agentCommand{Type: "subscribe", ConversationID: conv})
	// Expiry closes even an idle authenticated socket, before any replay frame.
	f.clock.Add(31)
	f.closed(c)
	if w := f.call("GET", "/device/profile", token); w.Code != 401 {
		t.Fatal("parent credential crossed device boundary")
	}
}

func TestClerkAgentQueuedAuthorityExpiresAndRestartFailsClosed(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "credential expires", true: "process credential lost"}[restart], func(t *testing.T) {
			f := newClerkAgentFixture(t)
			token := f.token("parent-a", nil)
			c := f.socketToken(token)
			event := f.run(c, f.create(token), "queued", "Create and schedule a task for Ada tomorrow")
			server := f.s
			if restart {
				server = NewWithAuth(f.db, "test", f.s.Auth)
				server.ParentAuthenticator = f.s.ParentAuthenticator
				server.ParentPolicy = f.s.ParentPolicy
			} else {
				f.clock.Add(31)
			}
			f.execute(server, event)
			if f.count(`SELECT count(*) FROM agent_runs WHERE id=$1 AND status='failed'`, event.RunID) != 1 {
				t.Fatal("unverified queued run executed")
			}
			if f.count(`SELECT count(*) FROM task_templates`)+f.count(`SELECT count(*) FROM parent_confirmation_previews`) != 0 {
				t.Fatal("expired/restarted authority produced a mutation or proposal")
			}
			var payload string
			if err := f.db.QueryRow(f.ctx, `SELECT payload::text FROM agent_run_events WHERE run_id=$1 AND event_type='terminal'`, event.RunID).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(payload, "renewed") || strings.Contains(payload, token) {
				t.Fatal("terminal authority outcome unsafe")
			}
		})
	}
}

func TestClerkAgentPendingPreviewRequiresFreshCurrentAuthority(t *testing.T) {
	f := newClerkAgentFixture(t)
	token := f.token("parent-a", nil)
	conv := f.create(token)
	c := f.socketToken(token)
	event := f.run(c, conv, "preview", "Create and schedule a task for Alice tomorrow")
	f.execute(f.s, event)
	preview := f.until(c, "tool_progress", "awaiting_confirmation")
	if f.count(`SELECT count(*) FROM task_templates`) != 0 {
		t.Fatal("preview mutated tasks")
	}
	// Same-tenant other actor cannot confirm even with the exact leaked handle.
	other := f.socketToken(f.token("parent-a2", nil))
	f.send(other, agentCommand{Type: "confirm", ConversationID: conv, RunID: event.RunID, ConfirmationID: preview.ConfirmationID})
	f.clock.Add(31)
	// Drain prior committed frames until the original socket is closed by expiry.
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	for {
		var v wireAgentEvent
		err := wsjson.Read(ctx, c, &v)
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatal("preview socket expiry")
			}
			break
		}
	}
	if f.count(`SELECT count(*) FROM task_templates`) != 0 {
		t.Fatal("foreign/expired confirmation produced effects")
	}
	// A new process lacks all ephemeral credentials; a pending preview itself is
	// inert. Only a freshly verified same-session parent can explicitly confirm.
	server := NewWithAuth(f.db, "test", f.s.Auth)
	server.ParentAuthenticator = f.s.ParentAuthenticator
	server.ParentPolicy = f.s.ParentPolicy
	f.http.Close()
	f.s = server
	f.http = httptest.NewServer(Mount(server.Routes(), "/tasks", t.TempDir()))
	t.Cleanup(f.http.Close)
	fresh := f.socketToken(f.token("parent-a", nil))
	f.send(fresh, agentCommand{Type: "subscribe", ConversationID: conv, Cursor: preview.Cursor})
	f.send(fresh, agentCommand{Type: "confirm", ConversationID: conv, RunID: event.RunID, ConfirmationID: preview.ConfirmationID})
	f.until(fresh, "terminal", "")
	f.send(fresh, agentCommand{Type: "confirm", ConversationID: conv, RunID: event.RunID, ConfirmationID: preview.ConfirmationID})
	if f.count(`SELECT count(*) FROM task_templates`) != 1 || f.count(`SELECT count(*) FROM task_schedules`) != 1 {
		t.Fatal("fresh confirmation did not apply exactly one domain effect")
	}
}

func TestLegacyAgentQueuedCredentialLossAlsoFailsClosed(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "restart"}[restart], func(t *testing.T) {
			h := newAgentWSTest(t)
			c := h.socket("parent-a")
			event := h.run(c, h.conversation("parent-a"), "legacy-queued", "Create and schedule a task.")
			s := h.s
			if restart {
				s = New(h.db, "test")
			} else {
				if _, err := h.db.Exec(h.ctx, `UPDATE bff_sessions SET expires_at=now()-interval '1 second' WHERE handle_hash=$1`, hash("parent-a")); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.executeAgentRun(h.ctx, jobs.Job{TenantID: tenantA, RunID: event.RunID}); err != nil {
				t.Fatal(err)
			}
			if h.count(`SELECT count(*) FROM agent_runs WHERE id=$1 AND status='failed'`, event.RunID) != 1 || h.count(`SELECT count(*) FROM task_templates`) != 0 {
				t.Fatal("unverified legacy authority executed queued work")
			}
		})
	}
}

func TestClerkAgentLocalRevocationClosesReplayAndRejectsEffects(t *testing.T) {
	for _, revoke := range []string{"membership", "identity", "session"} {
		t.Run(revoke, func(t *testing.T) {
			f := newClerkAgentFixture(t)
			token := f.token("parent-a", nil)
			c := f.socketToken(token)
			conv := f.create(token)
			f.send(c, agentCommand{Type: "subscribe", ConversationID: conv})
			var err error
			switch revoke {
			case "membership":
				_, err = f.db.Exec(f.ctx, `UPDATE parent_memberships SET revoked_at=now() WHERE tenant_id=$1 AND subject_ref='parent-a'`, tenantA)
			case "identity":
				_, err = f.db.Exec(f.ctx, `UPDATE parent_identities SET revoked_at=now() WHERE subject='parent-a'`)
			case "session":
				w := f.call("POST", "/auth/logout", token)
				if w.Code != 200 {
					t.Fatal("local logout failed")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			f.closed(c)
			if w := f.call("POST", "/agent/conversations", token); w.Code != 403 {
				t.Fatal("revoked parent admitted a conversation")
			}
		})
	}
}
