package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/bedrock"
	"charm.land/fantasy/providers/openrouter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	tasksdb "primer-tasks/internal/db"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

const agentProtocolVersion = 1

type agentHub struct {
	mu          sync.Mutex
	subscribers map[*agentSubscriber]struct{}
	credentials map[string]*agentAuthorization
}
type agentSubscriber struct {
	mu                   sync.Mutex
	cursor               int64
	acked                int64
	lastAck              time.Time
	sessionHash          []byte
	authorization        *agentAuthorization
	wake                 chan struct{}
	tenant, conversation string
	queue                chan wireAgentEvent
	done                 chan struct{}
	cancel               context.CancelFunc
	once                 sync.Once
}

func newAgentHub() *agentHub {
	return &agentHub{subscribers: make(map[*agentSubscriber]struct{}), credentials: make(map[string]*agentAuthorization)}
}
func (h *agentHub) add(s *agentSubscriber) { h.mu.Lock(); h.subscribers[s] = struct{}{}; h.mu.Unlock() }
func (h *agentHub) remove(s *agentSubscriber) {
	h.mu.Lock()
	delete(h.subscribers, s)
	h.mu.Unlock()
	s.close()
}
func (s *agentSubscriber) close() {
	s.once.Do(func() {
		close(s.done)
		if s.cancel != nil {
			s.cancel()
		}
		// The writer observes done and exits; leave the queue allocated until
		// that goroutine has selected its terminal branch. Closing a queue while
		// publish is racing with eviction can turn bounded backpressure into a
		// send-on-closed-channel panic.
	})
}

// enqueue is the sole path after upgrade for a socket-bound event. The writer
// goroutine owns websocket writes; command handling and worker fan-out never
// call wsjson.Write concurrently with it.
func (s *agentSubscriber) enqueue(event wireAgentEvent) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.queue <- event:
		return true
	case <-s.done:
		return false
	default:
		return false
	}
}
func (h *agentHub) publish(e wireAgentEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subscribers {
		sub.mu.Lock()
		matches := sub.tenant == e.TenantID && sub.conversation != "" && sub.conversation == e.ConversationID
		sub.mu.Unlock()
		if matches {
			// Wake only. PostgreSQL, not publisher arrival order, supplies events.
			select {
			case sub.wake <- struct{}{}:
			default:
			}
		}
	}
}

type wireAgentEvent struct {
	Type             string    `json:"kind"`
	ProtocolVersion  int       `json:"protocol"`
	ConversationID   string    `json:"conversationId"`
	Sequence         int64     `json:"sequence"`
	Cursor           int64     `json:"cursor"`
	RunID            string    `json:"runId,omitempty"`
	TenantID         string    `json:"-"`
	ConnectionID     string    `json:"connectionId,omitempty"`
	HeartbeatSeconds int       `json:"heartbeatSeconds,omitempty"`
	MessageID        string    `json:"messageId,omitempty"`
	ClientMessageID  string    `json:"clientMessageId,omitempty"`
	Text             string    `json:"text,omitempty"`
	Delta            string    `json:"-"`
	Tool             string    `json:"label,omitempty"`
	ToolStatus       string    `json:"-"`
	Phase            string    `json:"phase,omitempty"`
	Status           string    `json:"status,omitempty"`
	Code             string    `json:"code,omitempty"`
	Message          string    `json:"message,omitempty"`
	Retryable        bool      `json:"retryable,omitempty"`
	Time             time.Time `json:"time"`
	Attempt          int       `json:"retry,omitempty"`
	MaxAttempts      int       `json:"maxAttempts,omitempty"`
	RetryAfterMS     int       `json:"retryAfterMs"`
	ConfirmationID   string    `json:"confirmationId,omitempty"`
	ActionLabel      string    `json:"actionLabel,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
}

type agentCommand struct {
	RequestID       string `json:"requestId,omitempty"`
	Type            string `json:"kind"`
	ProtocolVersion int    `json:"protocol"`
	ConversationID  string `json:"conversationId"`
	Cursor          int64  `json:"cursor"`
	RunID           string `json:"runId"`
	ClientMessageID string `json:"clientMessageId"`
	Text            string `json:"text"`
	ConfirmationID  string `json:"confirmationId"`
}

func (s *Server) agentOriginAllowed(r *http.Request) bool {
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	if origin == "" {
		return false
	}
	publicOrigin := strings.TrimRight(s.Auth.PublicOrigin, "/")
	redirectOrigin := strings.TrimSuffix(strings.TrimRight(s.Auth.RedirectURL, "/"), "/auth/callback")
	allowed := map[string]bool{publicOrigin: true, redirectOrigin: true}
	for _, value := range strings.Split(os.Getenv("TASKS_ALLOWED_ORIGINS"), ",") {
		if value = strings.TrimRight(strings.TrimSpace(value), "/"); value != "" {
			allowed[value] = true
		}
	}
	if allowed[origin] {
		return true
	}
	if s.Env != "production" && (strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "http://localhost:")) {
		return true
	}
	return false
}

func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie("tasks_csrf"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	token := hex.EncodeToString(bytes)
	http.SetCookie(w, &http.Cookie{Name: "tasks_csrf", Value: token, Path: "/", HttpOnly: false, Secure: s.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: 28800})
	return token
}
func (s *Server) validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie("tasks_csrf")
	if err != nil || cookie.Value == "" {
		return false
	}
	for _, offered := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		offered = strings.TrimSpace(offered)
		if offered == "primer-tasks.v1.csrf."+cookie.Value {
			return true
		}
	}
	return false
}

func (s *Server) createAgentConversation(w http.ResponseWriter, r *http.Request, sc scope) {
	id := uuid.NewString()
	if _, err := s.DB.Exec(r.Context(), `INSERT INTO agent_conversations(id,tenant_id,actor_id,status,policy_version) VALUES($1,$2,$3,'active','parent.v1')`, id, sc.Tenant, sc.Subject); err != nil {
		problem(w, http.StatusInternalServerError, "internal", "unable to create agent conversation")
		return
	}
	jsonStatus(w, AgentConversation{ID: id, TenantID: sc.Tenant, PolicyVersion: "parent.v1", CreatedAt: time.Now().UTC()}, http.StatusCreated)
}

func (s *Server) agentWS(w http.ResponseWriter, r *http.Request) {
	if !s.agentOriginAllowed(r) || !s.validCSRF(r) {
		http.Error(w, "origin or csrf rejected", http.StatusForbidden)
		return
	}
	sc, err := s.parentScope(r)
	if err != nil {
		http.Error(w, "parent session required", http.StatusUnauthorized)
		return
	}
	authorization, err := s.socketAuthorization(r, sc)
	if err != nil {
		http.Error(w, "parent session required", http.StatusUnauthorized)
		return
	}
	// Origin and CSRF are checked above using the authenticated parent session.
	// nhooyr's default same-host check is intentionally bypassed here because
	// Stacklane and loopback are both legitimate public origins in development.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"primer-tasks.v1"}, InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "closed")
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), agentAuthKey{}, authorization))
	defer cancel()
	conn.SetReadLimit(16 * 1024)
	sub := &agentSubscriber{tenant: sc.Tenant, authorization: authorization, queue: make(chan wireAgentEvent, 64), wake: make(chan struct{}, 1), done: make(chan struct{}), cancel: cancel}
	if cookie, e := r.Cookie("tasks_parent"); e == nil {
		sub.sessionHash = hash(cookie.Value)
	}
	s.agentHub.add(sub)
	defer s.agentHub.remove(sub)
	connectionID := uuid.NewString()
	if err := writeAgentFrame(ctx, conn, wireAgentEvent{Type: "hello", ProtocolVersion: agentProtocolVersion, ConnectionID: connectionID, HeartbeatSeconds: 30, Time: time.Now().UTC()}); err != nil {
		return
	}
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); s.tailAgentSocket(ctx, conn, sc, sub); cancel() }()
	defer func() { cancel(); <-writerDone }()
	for {
		var cmd agentCommand
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
			return
		}
		if authorization.check(ctx) != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "parent session revoked")
			return
		}
		if cmd.ProtocolVersion != agentProtocolVersion {
			s.sendAgentToSubscriber(sub, wireAgentEvent{Type: "error", ProtocolVersion: agentProtocolVersion, Code: "protocol_version", Message: "unsupported protocol version", Retryable: false})
			continue
		}
		switch cmd.Type {
		case "hello":
			// Connection hello is a protocol negotiation frame; it has no
			// durable side effect and is intentionally not replayed.
		case "subscribe":
			s.agentSubscribe(ctx, sc, sub, cmd)
		case "user_message":
			s.agentMessage(ctx, sc, cmd)
		case "cancel":
			s.agentCancel(ctx, sc, cmd)
		case "confirm":
			s.agentConfirm(ctx, sc, cmd)
		case "unsubscribe":
			sub.mu.Lock()
			sub.conversation = ""
			sub.mu.Unlock()
		case "ack":
			sub.mu.Lock()
			if cmd.Cursor > sub.acked && cmd.Cursor <= sub.cursor {
				sub.acked = cmd.Cursor
				sub.lastAck = time.Now()
			}
			sub.mu.Unlock()
		default:
			s.sendAgentToSubscriber(sub, wireAgentEvent{Type: "error", ProtocolVersion: agentProtocolVersion, ConversationID: cmd.ConversationID, Code: "unknown_command", Message: "unknown command", Retryable: false})
		}
	}
}

func (s *Server) sendAgentToSubscriber(sub *agentSubscriber, event wireAgentEvent) {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if !sub.enqueue(event) {
		s.agentHub.remove(sub)
	}
}

func (s *Server) agentSubscribe(ctx context.Context, sc scope, sub *agentSubscriber, cmd agentCommand) {
	if cmd.ConversationID == "" && cmd.RunID != "" {
		_ = s.DB.QueryRow(ctx, `SELECT conversation_id FROM agent_runs WHERE tenant_id=$1 AND id=$2`, sc.Tenant, cmd.RunID).Scan(&cmd.ConversationID)
	}
	if _, err := uuid.Parse(cmd.ConversationID); err != nil {
		return
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_conversations WHERE tenant_id=$1 AND id=$2 AND actor_id=$3)`, sc.Tenant, cmd.ConversationID, sc.Subject).Scan(&exists); err != nil || !exists {
		return
	}
	if cmd.Cursor < 0 {
		return
	}
	sub.mu.Lock()
	sub.conversation, sub.cursor, sub.acked, sub.lastAck = cmd.ConversationID, cmd.Cursor, cmd.Cursor, time.Now()
	sub.mu.Unlock()
	select {
	case sub.wake <- struct{}{}:
	default:
	}
}

func (s *Server) agentMessage(ctx context.Context, sc scope, cmd agentCommand) {
	if strings.TrimSpace(cmd.Text) == "" || cmd.ClientMessageID == "" {
		return
	}
	if _, err := uuid.Parse(cmd.ConversationID); err != nil {
		return
	}
	if len(cmd.Text) > 8192 || len(cmd.ClientMessageID) > 128 {
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if s.Auth.Mode == "clerk" && ctx.Value(agentAuthKey{}) == nil {
		return
	}
	if err = lockAgentParent(ctx, tx, sc.Tenant, sc.Subject); err != nil {
		return
	}
	var actor string
	if err := tx.QueryRow(ctx, `SELECT c.actor_id FROM agent_conversations c WHERE c.tenant_id=$1 AND c.id=$2 AND c.status='active' AND EXISTS(SELECT 1 FROM parent_memberships p WHERE p.tenant_id=c.tenant_id AND p.subject_ref=c.actor_id AND p.revoked_at IS NULL AND p.role='admin') FOR UPDATE`, sc.Tenant, cmd.ConversationID).Scan(&actor); err != nil || actor != sc.Subject {
		return
	}
	var sequence int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM agent_messages WHERE tenant_id=$1 AND conversation_id=$2`, sc.Tenant, cmd.ConversationID).Scan(&sequence); err != nil {
		return
	}
	repo := agent.NewPostgresRepository(tx)
	now := time.Now().UTC()
	message, inserted, err := repo.AppendUserMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: sc.Tenant, ConversationID: cmd.ConversationID, ClientMessageID: cmd.ClientMessageID, Content: strings.TrimSpace(cmd.Text), Sequence: sequence, CreatedAt: now})
	if err != nil || !inserted {
		return
	}
	var outstanding int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM agent_runs WHERE tenant_id=$1 AND conversation_id=$2 AND status IN ('queued','running','awaiting_confirmation','cancel_requested')`, sc.Tenant, cmd.ConversationID).Scan(&outstanding); err != nil || outstanding >= 16 {
		return
	}
	runID := uuid.NewString()
	cfg, cfgErr := parent.LoadProviderConfig()
	if cfgErr != nil {
		cfg = parent.ProviderConfig{Mode: parent.ProviderDisabled, MaxSteps: 12, MaxTokens: 4096, MaxDuration: 120 * time.Second, MaxRetries: 0}
	}
	digest := sha256.Sum256([]byte(message.Content))
	run := agent.Run{ID: runID, TenantID: sc.Tenant, ConversationID: cmd.ConversationID, UserMessageID: message.ID, Status: agent.RunQueued, MaxSteps: cfg.MaxSteps, MaxTokens: cfg.MaxTokens, Deadline: now.Add(cfg.MaxDuration), CreatedAt: now, Provenance: agent.Provenance{Provider: string(cfg.Mode), PolicyVersion: "parent.v1", PromptDigest: hex.EncodeToString(digest[:])}}
	if err := repo.CreateRun(ctx, run); err != nil {
		return
	}
	if err = bindAgentRunAuthorization(ctx, tx, sc.Tenant, runID); err != nil {
		return
	}
	// Register ephemeral authority before the committed job becomes visible.
	if a, ok := ctx.Value(agentAuthKey{}).(*agentAuthorization); ok {
		copy := *a
		copy.deadline = run.Deadline
		s.agentHub.mu.Lock()
		for id, previous := range s.agentHub.credentials {
			if !previous.deadline.After(now) {
				delete(s.agentHub.credentials, id)
			}
		}
		s.agentHub.credentials[runID] = &copy
		s.agentHub.mu.Unlock()
	}
	committed := false
	defer func() {
		if !committed {
			s.agentHub.mu.Lock()
			delete(s.agentHub.credentials, runID)
			s.agentHub.mu.Unlock()
		}
	}()
	if err = jobsRepository(tx).Enqueue(ctx, jobRecord(runID, sc.Tenant, cfg.MaxRetries+1)); err != nil {
		return
	}
	bound := *s
	bound.DB, bound.agentHub = tx, nil
	if err = bound.publishAgent(ctx, sc.Tenant, cmd.ConversationID, wireAgentEvent{Type: "user_message", RunID: runID, ClientMessageID: cmd.ClientMessageID, Text: message.Content}); err != nil {
		return
	}
	if err = tx.Commit(ctx); err != nil {
		return
	}
	committed = true
	s.agentHub.publish(wireAgentEvent{TenantID: sc.Tenant, ConversationID: cmd.ConversationID})
}

func jobRecord(runID, tenant string, attempts int) jobs.Job {
	return jobs.Job{ID: uuid.NewString(), TenantID: tenant, RunID: runID, MaxAttempts: maxInt(attempts, 1), AvailableAt: time.Now().UTC()}
}
func jobsRepository(db tasksdb.Database) *jobs.PostgresRepository {
	return jobs.NewPostgresRepository(db)
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Server) agentCancel(ctx context.Context, sc scope, cmd agentCommand) {
	if cmd.RunID == "" {
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if err = lockAgentParent(ctx, tx, sc.Tenant, sc.Subject); err != nil {
		return
	}
	if err = checkDurableAgentAuthorization(ctx, tx, sc.Tenant, sc.Subject, cmd.RunID); err != nil {
		return
	}
	var conversation string
	err = tx.QueryRow(ctx, `UPDATE agent_runs r SET cancel_requested=true,status='canceled',updated_at=now() WHERE r.tenant_id=$1 AND r.id=$2 AND ($4='' OR r.conversation_id::text=$4) AND EXISTS(SELECT 1 FROM agent_conversations c JOIN parent_memberships p ON p.tenant_id=c.tenant_id AND p.subject_ref=c.actor_id WHERE c.tenant_id=r.tenant_id AND c.id=r.conversation_id AND c.actor_id=$3 AND p.revoked_at IS NULL AND p.role='admin') AND r.status IN ('queued','running','awaiting_confirmation','cancel_requested') RETURNING r.conversation_id`, sc.Tenant, cmd.RunID, sc.Subject, cmd.ConversationID).Scan(&conversation)
	if err != nil {
		return
	}
	local := *s
	local.DB, local.agentHub = tx, nil
	if err = local.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "terminal", RunID: cmd.RunID, Status: "cancelled", Message: "The run was canceled. No further agent work will be performed."}); err != nil {
		return
	}
	_ = tx.Commit(ctx)
}
func (s *Server) agentConfirm(ctx context.Context, sc scope, cmd agentCommand) {
	if cmd.ConfirmationID == "" || cmd.RunID == "" {
		return
	}
	if err := s.confirmAgentAction(ctx, sc, cmd); err != nil {
		var conversation string
		if e := s.DB.QueryRow(ctx, `SELECT r.conversation_id FROM agent_runs r JOIN agent_conversations c ON c.tenant_id=r.tenant_id AND c.id=r.conversation_id WHERE r.tenant_id=$1 AND r.id=$2 AND c.actor_id=$3`, sc.Tenant, cmd.RunID, sc.Subject).Scan(&conversation); e == nil {
			_ = s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "error", RunID: cmd.RunID, Code: "confirmation_rejected", Message: "Confirmation was not applied."})
		}
	}
}

func parentHandleHash(handle string) []byte {
	h := sha256.Sum256(append([]byte("primer.tasks.parent.confirmation.handle.v1\x00"), []byte(handle)...))
	return h[:]
}

func (s *Server) publishAgent(ctx context.Context, tenant, conversation string, event wireAgentEvent) error {
	if event.Type != "terminal" && event.Type != "error" {
		if err := checkAgentAuthorization(ctx); err != nil {
			return err
		}
	}
	if event.RunID == "" || conversation == "" {
		return errors.New("durable agent events require a run and conversation")
	}
	switch agentprotocol.EventKind(event.Type) {
	case agentprotocol.EventThinkingStart, agentprotocol.EventThinkingEnd:
		event.Text, event.Message, event.Summary = "", "", ""
	case agentprotocol.EventUserMessage, agentprotocol.EventTextStart, agentprotocol.EventTextDelta, agentprotocol.EventTextEnd, agentprotocol.EventToolProgress, agentprotocol.EventRetry, agentprotocol.EventTerminal, agentprotocol.EventError:
	default:
		return errors.New("unsupported durable event variant")
	}
	if len(event.Text) > 32768 || len(event.Message) > 32768 || len(event.Summary) > 8192 {
		return errors.New("agent event exceeds size limit")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if job, ok := ctx.Value(agentLeaseKey{}).(jobs.Job); ok {
		if job.TenantID != tenant || job.RunID != event.RunID {
			return parent.ErrInvalidContext
		}
		if err = lockJobLease(ctx, tx, job); err != nil {
			return err
		}
	}
	// Serialize cursor allocation across all runs in a conversation. A second
	// command may start while the first terminal event is being persisted.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant+":"+conversation); err != nil {
		return err
	}
	var seq int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(e.sequence),0)+1 FROM agent_run_events e JOIN agent_runs r ON r.id=e.run_id AND r.tenant_id=e.tenant_id WHERE e.tenant_id=$1 AND r.conversation_id=$2`, tenant, conversation).Scan(&seq); err != nil {
		return err
	}
	var runStatus agent.RunStatus
	if err = tx.QueryRow(ctx, `SELECT status FROM agent_runs WHERE id=$1 AND tenant_id=$2 AND conversation_id=$3`, event.RunID, tenant, conversation).Scan(&runStatus); err != nil {
		return err
	}
	if runStatus.Terminal() && event.Type != "terminal" && event.Type != "error" {
		return agent.ErrAlreadyTerminal
	}
	if event.Type == "terminal" {
		var terminal bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_run_events WHERE run_id=$1 AND tenant_id=$2 AND event_type='terminal')`, event.RunID, tenant).Scan(&terminal); err != nil {
			return err
		}
		if terminal {
			return nil
		}
	}
	event.ProtocolVersion = agentProtocolVersion
	event.ConversationID = conversation
	if event.Text == "" && event.Message != "" {
		event.Text = event.Message
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	event.Sequence, event.Cursor, event.TenantID = seq, seq, tenant
	if err := validateSocketEvent(event); err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_run_events(run_id,tenant_id,sequence,event_type,payload) VALUES($1,$2,$3,$4,$5)`, event.RunID, tenant, seq, event.Type, payload); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if s.agentHub != nil {
		s.agentHub.publish(event)
	}
	return nil
}

func (s *Server) StartAgentWorker(ctx context.Context) {
	runRepo := agent.NewPostgresRepository(s.DB)
	jobRepo := jobsRepository(s.DB)
	worker := jobs.NewWorker(jobRepo, runRepo, func(ctx context.Context, job jobs.Job) error {
		err := s.executeAgentRun(ctx, job)
		if err != nil {
			slog.Error("parent agent job failed", "run_id", job.RunID, "code", "job_failed")
		}
		return err
	})
	if cfg, err := parent.LoadProviderConfig(); err == nil && cfg.MaxConcurrentTenants > 0 {
		worker.MaxConcurrentTenants = cfg.MaxConcurrentTenants
	}
	go worker.Run(ctx)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcileAgentOutcomes(ctx)
			}
		}
	}()
}

func (s *Server) executeAgentRun(ctx context.Context, job jobs.Job) error {
	if job.ID != "" {
		ctx = context.WithValue(ctx, agentLeaseKey{}, job)
	}
	repo := agent.NewPostgresRepository(s.DB)
	run, err := repo.GetRun(ctx, job.TenantID, job.RunID)
	if err != nil {
		return err
	}
	// A job may be re-delivered after its lease expires. A terminal run is
	// already durable work; never replay its tools or emit a second terminal.
	s.agentHub.mu.Lock()
	authority := s.agentHub.credentials[run.ID]
	s.agentHub.mu.Unlock()
	if authority != nil {
		ctx = context.WithValue(ctx, agentAuthKey{}, authority)
	}
	defer func() { s.agentHub.mu.Lock(); delete(s.agentHub.credentials, run.ID); s.agentHub.mu.Unlock() }()
	if run.Status.Terminal() || run.Status == agent.RunAwaitingConfirmation {
		return nil
	}
	if !run.Deadline.After(time.Now()) {
		return s.finishAgentRun(ctx, run, agent.RunFailed, "failed", "The run expired.", "run_expired", "", run.Usage)
	}
	var conversation, prompt, actor string
	if err := s.DB.QueryRow(ctx, `SELECT r.conversation_id,m.content,c.actor_id FROM agent_runs r JOIN agent_messages m ON m.id=r.user_message_id JOIN agent_conversations c ON c.id=r.conversation_id AND c.tenant_id=r.tenant_id WHERE r.tenant_id=$1 AND r.id=$2`, job.TenantID, job.RunID).Scan(&conversation, &prompt, &actor); err != nil {
		return err
	}
	tx, authErr := s.DB.Begin(ctx)
	if authErr == nil {
		authErr = lockAgentAuthority(ctx, tx, job.TenantID, actor, run.ID)
		_ = tx.Rollback(ctx)
	}
	if authErr != nil {
		return s.finishAgentRun(ctx, run, agent.RunFailed, "failed", "Parent authorization must be renewed. Send a new request.", "reauth_required", "", run.Usage)
	}
	cfg, cfgErr := parent.LoadProviderConfig()
	if cfgErr != nil {
		cfg = parent.ProviderConfig{Mode: parent.ProviderDisabled, MaxSteps: 12, MaxTokens: 4096, MaxDuration: 120 * time.Second, MaxRetries: 0}
	}
	if run.CancelRequested || run.Status == agent.RunCancelRequested {
		return s.finishAgentRun(ctx, run, agent.RunCanceled, "cancelled", "The run was canceled. No further agent work will be performed.", "", "", run.Usage)
	}
	if run.DurableStep > 0 {
		// A process can die after a mutation is committed but before the
		// provider reaches its next durable boundary. Replaying from prompt
		// zero is unsafe; fail closed at the recorded boundary instead.
		return s.finishAgentRun(ctx, run, agent.RunFailed, "failed", "The run stopped at a durable tool boundary; no mutation was replayed.", "durable_boundary", "", run.Usage)
	}
	if cfg.Mode == parent.ProviderDisabled {
		return s.finishAgentRun(ctx, run, agent.RunFailed, "disabled", "Agent mode is disabled. Use the ordinary Tasks and Schedules pages.", "", "", run.Usage)
	}
	model, err := s.agentModel(ctx, cfg, prompt)
	if err != nil {
		return s.finishAgentRun(ctx, run, agent.RunFailed, "failed", "The configured provider could not start.", "provider_unavailable", "", run.Usage)
	}
	_, _ = s.DB.Exec(ctx, `UPDATE agent_runs SET provider=$3,model=$4,updated_at=now() WHERE tenant_id=$1 AND id=$2`, job.TenantID, job.RunID, model.Provider(), model.Model())
	tools := s.fantasyTools(job.TenantID, actor, cfg.ActiveTools, run.ID)
	rt, err := agent.NewFantasyAgent(model, tools, agent.Limits{MaxSteps: min(cfg.MaxSteps, run.MaxSteps), MaxTokens: min(cfg.MaxTokens, run.MaxTokens), Deadline: min(cfg.MaxDuration, time.Until(run.Deadline)), MaxRetries: cfg.MaxRetries})
	if err != nil {
		return err
	}
	_ = repo.TransitionRun(ctx, job.TenantID, job.RunID, agent.RunRunning, run.DurableStep, run.Usage)
	runCtx, stopWatchingCancel, canceled := s.watchRunCancellation(ctx, job.TenantID, job.RunID)
	defer stopWatchingCancel()
	execution, err := rt.Execute(runCtx, job.RunID, prompt, func(event agentprotocol.Event) error {
		return s.emitProtocolEvent(ctx, job.TenantID, conversation, event)
	})
	if err != nil {
		// Worker lease loss cancels the handler context. Leave the run at its
		// durable boundary for the new owner; the stale worker must not publish
		// a competing terminal event.
		if ctx.Err() != nil {
			return err
		}
		select {
		case <-canceled:
			return s.finishAgentRun(ctx, run, agent.RunCanceled, "cancelled", "The run was canceled. No further agent work will be performed.", "", "", run.Usage)
		default:
		}
		return s.finishAgentRun(ctx, run, agent.RunFailed, "failed", "The provider or tool failed before a confirmed result.", "run_failed", "", run.Usage)
	}
	var pending bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parent_confirmation_previews WHERE tenant_id=$1 AND run_id=$2 AND consumed_at IS NULL)`, job.TenantID, job.RunID).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return nil
	}
	return s.finishAgentRun(ctx, run, agent.RunSucceeded, "completed", "The parent command completed.", "", execution.FinalText, execution.Usage)
}

// watchRunCancellation makes the database cancellation flag authoritative.
// The worker context is process-owned; no websocket request context can cancel
// a detached run. The returned cancellation is only a signal to Fantasy.
func (s *Server) watchRunCancellation(workerCtx context.Context, tenant, runID string) (context.Context, func(), <-chan struct{}) {
	watchCtx, stopWatch := context.WithCancel(workerCtx)
	runCtx, cancel := context.WithCancel(workerCtx)
	stop := make(chan struct{})
	done := make(chan struct{})
	canceled := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			if checkAgentAuthorization(watchCtx) != nil {
				cancel()
				return
			}
			var requested bool
			err := s.DB.QueryRow(watchCtx, `SELECT cancel_requested OR status='cancel_requested' FROM agent_runs WHERE tenant_id=$1 AND id=$2`, tenant, runID).Scan(&requested)
			if err == nil && requested {
				once.Do(func() { close(canceled); cancel() })
				return
			}
			select {
			case <-workerCtx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return runCtx, func() { close(stop); stopWatch(); cancel(); <-done }, canceled
}

func (s *Server) emitProtocolEvent(ctx context.Context, tenant, conversation string, event agentprotocol.Event) error {
	out := wireAgentEvent{RunID: event.RunID, Text: event.Text, Delta: event.Text, Code: event.Code, Status: event.Status, Summary: event.Text}
	switch event.Kind {
	case agentprotocol.EventTextStart:
		out.Type = "text_start"
	case agentprotocol.EventTextDelta:
		out.Type = "text_delta"
	case agentprotocol.EventTextEnd:
		out.Type = "text_end"
	case agentprotocol.EventThinkingStart:
		out.Type = "thinking_start"
	case agentprotocol.EventThinkingEnd:
		out.Type = "thinking_end"
	case agentprotocol.EventToolProgress:
		out.Type = "tool_progress"
		out.Tool = event.Label
		out.ToolStatus = event.Phase
		out.Phase = event.Phase
	case agentprotocol.EventRetry:
		out.Type = "retry"
		out.Attempt = event.Retry
		out.MaxAttempts = event.Retry
		out.RetryAfterMS = event.RetryAfterMS
	case agentprotocol.EventConfirmation:
		// The preview and its frame were committed together by stageAgentAction.
		return nil
	case agentprotocol.EventTerminal:
		out.Type = "terminal"
	case agentprotocol.EventError:
		out.Type = "error"
	default:
		return nil
	}
	return s.emitAgent(ctx, tenant, conversation, event.RunID, out)
}
func (s *Server) emitAgent(ctx context.Context, tenant, conversation, runID string, event wireAgentEvent) error {
	event.RunID = runID
	return s.publishAgent(ctx, tenant, conversation, event)
}

func defaultToolNames() []string {
	return []string{parent.ToolListStudents, parent.ToolListTasks, parent.ToolGetTask, parent.ToolDraftTask, parent.ToolUpdateTask, parent.ToolPublishTask, parent.ToolPreviewAction, parent.ToolConfirmAction, parent.ToolListSchedules, parent.ToolCreateSchedule, parent.ToolUpdateSchedule, parent.ToolListOccurrences}
}

func (s *Server) fantasyTools(tenant, actor string, active []string, runID string) []fantasy.AgentTool {
	tools := s.parentTools()
	set, err := parent.NewToolSet(active)
	if err != nil {
		return nil
	}
	// A retry of one durable run must keep the same idempotency identity;
	// generating a fresh key here would permit duplicate domain effects.
	ctx := parent.Context{TenantID: tenant, ActorID: actor, IdempotencyKey: runID, RunID: runID, Tools: set}
	toolStep := 0
	nextToolContext := func(toolName string) parent.Context {
		switch toolName {
		case parent.ToolListStudents, parent.ToolListTasks, parent.ToolGetTask, parent.ToolListSchedules, parent.ToolListOccurrences:
		default:
			toolStep++
		}
		toolCtx := ctx
		toolCtx.ToolStep = toolStep
		return toolCtx
	}
	// Actor is supplied by the authenticated run in production; this function
	// is called only after the run tenant has been verified.  The worker fills
	// actor from the durable conversation before invoking tools in later steps.
	all := []fantasy.AgentTool{
		fantasy.NewAgentTool("get_task", "Inspect a household task revision", func(c context.Context, in struct {
			ID string `json:"id"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.GetTask(c, nextToolContext(parent.ToolGetTask), in.ID)
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("update_schedule", "Prepare a version-bound schedule edit for parent confirmation", func(c context.Context, in agentScheduleEditInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if _, err := time.Parse(time.RFC3339, in.StartAt); err != nil {
				return safeToolJSON(nil, parent.ErrInvalidInput)
			}
			b, e := json.Marshal(in)
			if e != nil {
				return safeToolJSON(nil, e)
			}
			var payload map[string]any
			if e = json.Unmarshal(b, &payload); e != nil {
				return safeToolJSON(nil, e)
			}
			return s.stageAgentAction(c, nextToolContext(parent.ToolUpdateSchedule), parent.Action{Kind: parent.ToolUpdateSchedule, TargetIDs: []string{in.ScheduleID}, Payload: payload})
		}),
		fantasy.NewAgentTool("list_students", "List one bounded page of household students; use offset for subsequent pages", func(c context.Context, in struct {
			Query  string `json:"query"`
			Limit  int    `json:"limit"`
			Offset int    `json:"offset,omitempty"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListStudents(c, nextToolContext(parent.ToolListStudents), parent.StudentQuery{Name: in.Query, Limit: in.Limit, Offset: in.Offset})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("list_tasks", "List one bounded page of current household tasks; use offset for subsequent pages", func(c context.Context, in struct {
			Query  string `json:"query"`
			Limit  int    `json:"limit"`
			Offset int    `json:"offset,omitempty"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListTasks(c, nextToolContext(parent.ToolListTasks), parent.TaskQuery{Query: in.Query, Limit: in.Limit, Offset: in.Offset})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("draft_task", "Draft a task", func(c context.Context, in struct {
			Title        string `json:"title"`
			Instructions string `json:"instructions"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return s.stageAgentAction(c, nextToolContext(parent.ToolDraftTask), parent.Action{Kind: parent.ToolDraftTask, TargetIDs: []string{"new task"}, Payload: map[string]any{"Title": in.Title, "Instructions": in.Instructions}})
		}),
		fantasy.NewAgentTool("update_task", "Preview a new draft revision; published and assigned history stays unchanged", func(c context.Context, in struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Instructions string `json:"instructions"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return s.stageAgentAction(c, nextToolContext(parent.ToolUpdateTask), parent.Action{Kind: parent.ToolUpdateTask, TargetIDs: []string{in.ID}, Payload: map[string]any{"Title": in.Title, "Instructions": in.Instructions}})
		}),
		fantasy.NewAgentTool("publish_task", "Publish a drafted task", func(c context.Context, in struct {
			ID string `json:"id"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return s.stageAgentAction(c, nextToolContext(parent.ToolPublishTask), parent.Action{Kind: parent.ToolPublishTask, TargetIDs: []string{in.ID}})
		}),
		fantasy.NewAgentTool("create_schedule", "Schedule a published task for a student", func(c context.Context, in struct {
			StudentID        string `json:"studentId"`
			StudentName      string `json:"studentName"`
			TemplateID       string `json:"templateId"`
			RevisionID       string `json:"revisionId"`
			Kind             string `json:"kind"`
			Timezone         string `json:"timezone"`
			StartAt          string `json:"startAt"`
			RRULE            string `json:"rrule"`
			DueOffsetMinutes int    `json:"dueOffsetMinutes"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			startAt, parseErr := time.Parse(time.RFC3339, in.StartAt)
			if parseErr != nil {
				return safeToolJSON(nil, parent.ErrInvalidInput)
			}
			student, e := tools.ResolveStudent(c, nextToolContext(parent.ToolListStudents), in.StudentID, in.StudentName)
			if e != nil {
				return safeToolJSON(nil, e)
			}
			return s.stageAgentAction(c, nextToolContext(parent.ToolCreateSchedule), parent.Action{Kind: parent.ToolCreateSchedule, TargetIDs: []string{student.ID}, Payload: map[string]any{"StudentID": student.ID, "TemplateID": in.TemplateID, "RevisionID": in.RevisionID, "Kind": in.Kind, "Timezone": in.Timezone, "StartAt": startAt, "RRULE": in.RRULE, "DueOffsetMinutes": in.DueOffsetMinutes}})
		}),
		fantasy.NewAgentTool("list_schedules", "List one bounded page of schedules; use offset for subsequent pages", func(c context.Context, in struct {
			IncludeDisabled bool `json:"includeDisabled"`
			Limit           int  `json:"limit"`
			Offset          int  `json:"offset,omitempty"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListSchedules(c, nextToolContext(parent.ToolListSchedules), parent.ScheduleQuery{IncludeDisabled: in.IncludeDisabled, Limit: in.Limit, Offset: in.Offset})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("list_occurrences", "List one bounded page of occurrences; use offset for subsequent pages", func(c context.Context, in struct {
			Status string `json:"status"`
			Limit  int    `json:"limit"`
			Offset int    `json:"offset,omitempty"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListOccurrences(c, nextToolContext(parent.ToolListOccurrences), parent.OccurrenceQuery{Status: in.Status, Limit: in.Limit, Offset: in.Offset})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("preview_action", "Prepare a destructive change for explicit parent confirmation", func(c context.Context, in struct {
			Kind      string         `json:"kind"`
			TargetIDs []string       `json:"targetIds"`
			Payload   map[string]any `json:"payload"`
			Summary   string         `json:"summary"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return s.stageAgentAction(c, nextToolContext(parent.ToolPreviewAction), parent.Action{Kind: in.Kind, TargetIDs: in.TargetIDs, Payload: in.Payload})
		}),
	}
	// Do not merely reject a disabled tool at invocation time: omit it from
	// Fantasy's advertised schema. This leaves model input with no authority
	// widening path and makes an empty/unknown allowlist fail closed above.
	out := make([]fantasy.AgentTool, 0, len(all))
	for _, tool := range all {
		if set.Allows(tool.Info().Name) {
			out = append(out, tool)
		}
	}
	return out
}
func safeToolJSON(v any, err error) (fantasy.ToolResponse, error) {
	if err != nil {
		// The client receives only the bounded terminal failure. Returning the
		// underlying error to Fantasy stops a scripted chain before it can claim
		// later tool effects that never committed.
		return fantasy.NewTextErrorResponse("tool unavailable"), parent.ErrServiceMissing
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fantasy.NewTextErrorResponse("tool unavailable"), err
	}
	return fantasy.NewTextResponse(string(b)), nil
}

var secretPattern = regexp.MustCompile(`(?i)(reasoning|secret|api[_ -]?key|authorization|bearer)`)

// scriptedParentModel is deliberately test/dev-only and deterministic. It is
// a real Fantasy LanguageModel fixture used to qualify the loop, not a claim
// about live provider or pedagogical quality.
type scriptedParentModel struct {
	prompt string
	calls  int
}

func (m *scriptedParentModel) Provider() string { return "scripted" }
func (m *scriptedParentModel) Model() string    { return "primer-tasks-scripted" }
func (m *scriptedParentModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("scripted model only streams")
}
func (m *scriptedParentModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, errors.New("scripted object disabled")
}
func (m *scriptedParentModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, errors.New("scripted object disabled")
}
func (m *scriptedParentModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls++
	lower := strings.ToLower(m.prompt)
	if strings.Contains(lower, "ambiguous") || strings.Contains(lower, "alex") {
		if m.calls == 1 {
			return scriptedToolStream(ctx, "list_students", `{"query":"Alex","limit":20}`), nil
		}
		return scriptedStream(ctx, "Please clarify which student you mean; I will not guess."), nil
	}
	if strings.Contains(lower, "disable") || strings.Contains(lower, "cancel schedule") {
		if m.calls == 1 {
			return scriptedToolStream(ctx, "list_schedules", `{"includeDisabled":false,"limit":20}`), nil
		}
		if m.calls == 2 {
			return scriptedToolStream(ctx, "preview_action", scriptedSchedulePreviewInput(call)), nil
		}
		return scriptedStream(ctx, "I prepared a server preview. No destructive change was made without your confirmation."), nil
	}
	if strings.Contains(lower, "retire") {
		if m.calls == 1 {
			return scriptedToolStream(ctx, "list_tasks", `{"query":"","limit":20}`), nil
		}
		if m.calls == 2 {
			return scriptedToolStream(ctx, "preview_action", scriptedPreviewInput(call)), nil
		}
		return scriptedStream(ctx, "I prepared a server preview. No destructive change was made without your confirmation."), nil
	}
	if strings.Contains(lower, "create") || strings.Contains(lower, "schedule") {
		switch m.calls {
		case 1:
			query := ""
			if match := regexp.MustCompile(`student "([^"]+)"`).FindStringSubmatch(m.prompt); len(match) == 2 {
				query = match[1]
			}
			input, _ := json.Marshal(map[string]any{"query": query, "limit": 20})
			return scriptedToolStream(ctx, "list_students", string(input)), nil
		case 2:
			return scriptedToolStream(ctx, "preview_action", scriptedCreatePreview(call)), nil
		}
		return scriptedStream(ctx, "A server preview requires explicit parent confirmation."), nil
	}
	if strings.Contains(lower, "list") && m.calls == 1 {
		return scriptedToolStream(ctx, "list_students", `{"query":"","limit":20}`), nil
	}
	return scriptedStream(ctx, "I inspected the server-owned Tasks records."), nil
}

func toolResultJSON(call fantasy.Call, name string) map[string]any {
	// Fantasy projects a completed tool call into an assistant ToolCallPart and
	// its result into a later tool-role ToolResultPart. Match their opaque call
	// IDs rather than assuming result parts retain a model-controlled name.
	callNames := map[string]string{}
	for _, message := range call.Prompt {
		for _, part := range message.Content {
			if toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
				callNames[toolCall.ToolCallID] = toolCall.ToolName
			}
		}
	}
	for _, message := range call.Prompt {
		for _, part := range message.Content {
			result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok || callNames[result.ToolCallID] != name {
				continue
			}
			text, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output)
			if !ok {
				return map[string]any{}
			}
			var value map[string]any
			if json.Unmarshal([]byte(text.Text), &value) == nil {
				return value
			}
			var items []any
			if json.Unmarshal([]byte(text.Text), &items) == nil {
				return map[string]any{"items": items}
			}
		}
	}
	return map[string]any{}
}
func scriptedIDInput(call fantasy.Call, name string) string {
	value := toolResultJSON(call, name)
	id, _ := value["id"].(string)
	return fmt.Sprintf(`{"id":%q}`, id)
}
func scriptedScheduleInput(call fantasy.Call) string {
	task := toolResultJSON(call, "publish_task")
	students := toolResultJSON(call, "list_students")
	id, _ := students["id"].(string)
	if id == "" {
		if items, ok := students["items"].([]any); ok && len(items) > 0 {
			if row, ok := items[0].(map[string]any); ok {
				id, _ = row["id"].(string)
			}
		}
	}
	templateID, _ := task["templateId"].(string)
	revisionID, _ := task["id"].(string)
	return fmt.Sprintf(`{"studentId":%q,"studentName":"","templateId":%q,"revisionId":%q,"kind":"one_off","timezone":"UTC","startAt":%q,"rrule":"","dueOffsetMinutes":0}`, id, templateID, revisionID, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339))
}
func scriptedSchedulePreviewInput(call fantasy.Call) string {
	schedules := toolResultJSON(call, "list_schedules")
	id, version := "", 1
	if items, ok := schedules["items"].([]any); ok {
		for _, item := range items {
			row, rowOK := item.(map[string]any)
			if !rowOK {
				continue
			}
			if enabled, _ := row["enabled"].(bool); !enabled {
				continue
			}
			id, _ = row["id"].(string)
			if n, ok := row["version"].(float64); ok {
				version = int(n)
			}
			break
		}
	}
	return fmt.Sprintf(`{"kind":"disable_schedule","targetIds":[%q],"payload":{"expectedVersion":%d},"summary":"Disable schedule"}`, id, version)
}

func scriptedPreviewInput(call fantasy.Call) string {
	tasks := toolResultJSON(call, "list_tasks")
	id, _ := tasks["id"].(string)
	version := 1
	if items, ok := tasks["items"].([]any); ok && len(items) > 0 {
		var selected map[string]any
		for _, item := range items {
			row, rowOK := item.(map[string]any)
			if !rowOK {
				continue
			}
			if selected == nil {
				selected = row
			}
			if status, _ := row["status"].(string); status == "published" {
				selected = row
				break
			}
		}
		if selected != nil {
			id, _ = selected["id"].(string)
			if n, ok := selected["version"].(float64); ok {
				version = int(n)
			}
		}
	}
	return fmt.Sprintf(`{"kind":"retire_task","targetIds":[%q],"payload":{"expectedVersion":%d},"summary":"Retire task"}`, id, version)
}
func scriptedDelay(ctx context.Context) bool {
	raw := strings.TrimSpace(os.Getenv("TASKS_AGENT_SCRIPTED_DELAY_MS"))
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 || ms > 30000 {
		return true
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func scriptedToolStream(ctx context.Context, name, input string) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		// Fantasy associates a tool result with the tool-call ID. Reusing one ID
		// made later scripted steps misattribute results and falsely report a
		// schedule that never committed.
		toolID := "scripted-" + name
		parts := []fantasy.StreamPart{{Type: fantasy.StreamPartTypeReasoningStart, ID: "r"}, {Type: fantasy.StreamPartTypeReasoningDelta, ID: "r", Delta: "hidden"}, {Type: fantasy.StreamPartTypeReasoningEnd, ID: "r"}, {Type: fantasy.StreamPartTypeToolInputStart, ID: toolID, ToolCallName: name}, {Type: fantasy.StreamPartTypeToolInputDelta, ID: toolID, Delta: input}, {Type: fantasy.StreamPartTypeToolInputEnd, ID: toolID}, {Type: fantasy.StreamPartTypeToolCall, ID: toolID, ToolCallName: name, ToolCallInput: input}, {Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls}}
		for i, part := range parts {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(part) {
				return
			}
			if i == 0 && !scriptedDelay(ctx) {
				return
			}
		}
	}
}
func scriptedStream(ctx context.Context, text string) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		parts := []fantasy.StreamPart{{Type: fantasy.StreamPartTypeReasoningStart, ID: "r"}, {Type: fantasy.StreamPartTypeReasoningDelta, ID: "r", Delta: "hidden"}, {Type: fantasy.StreamPartTypeReasoningEnd, ID: "r"}, {Type: fantasy.StreamPartTypeTextStart, ID: "t"}, {Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: text}, {Type: fantasy.StreamPartTypeTextEnd, ID: "t"}, {Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 1, OutputTokens: int64(len(text)), TotalTokens: int64(len(text) + 1)}}}
		for i, p := range parts {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(p) {
				return
			}
			if i == 0 && !scriptedDelay(ctx) {
				return
			}
		}
	}
}

func (s *Server) agentModel(ctx context.Context, cfg parent.ProviderConfig, prompt string) (fantasy.LanguageModel, error) {
	switch cfg.Mode {
	case parent.ProviderScripted:
		if s.Env == "production" || cfg.Environment == "production" {
			return nil, agent.ErrProviderDisabled
		}
		return &scriptedParentModel{prompt: prompt}, nil
	case parent.ProviderDisabled:
		return nil, agent.ErrProviderDisabled
	}
	// Credentials are read only in this server-side factory. They are never
	// placed in prompts, tool inputs, durable events, or browser state.
	var primary fantasy.Provider
	var err error
	if cfg.Primary == parent.ProviderBedrock {
		primary, err = bedrock.New(bedrock.WithRegion(cfg.BedrockRegion))
	} else {
		primary, err = openrouter.New(openrouter.WithAPIKey(os.Getenv("TASKS_OPENROUTER_API_KEY")))
	}
	if err == nil {
		modelID := cfg.OpenRouterModel
		if cfg.Primary == parent.ProviderBedrock {
			modelID = cfg.BedrockModel
		}
		if modelID != "" {
			if model, modelErr := primary.LanguageModel(ctx, modelID); modelErr == nil {
				return model, nil
			} else {
				err = modelErr
			}
		}
	}
	if cfg.Fallback != parent.ProviderDisabled && cfg.Fallback != cfg.Primary {
		fallback, fallbackErr := openrouter.New(openrouter.WithAPIKey(os.Getenv("TASKS_OPENROUTER_API_KEY")))
		if fallbackErr == nil && cfg.OpenRouterModel != "" {
			return fallback.LanguageModel(ctx, cfg.OpenRouterModel)
		}
		if err == nil {
			err = fallbackErr
		}
	}
	if err == nil {
		err = errors.New("provider model is not configured")
	}
	return nil, fmt.Errorf("provider unavailable: %w", err)
}

var _ = pgx.ErrNoRows
