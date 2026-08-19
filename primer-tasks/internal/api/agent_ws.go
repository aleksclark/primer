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
	"github.com/jackc/pgx/v5/pgxpool"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

const agentProtocolVersion = 1

type agentHub struct {
	mu          sync.Mutex
	subscribers map[*agentSubscriber]struct{}
}
type agentSubscriber struct {
	tenant, conversation string
	queue                chan wireAgentEvent
	done                 chan struct{}
	once                 sync.Once
}

func newAgentHub() *agentHub               { return &agentHub{subscribers: make(map[*agentSubscriber]struct{})} }
func (h *agentHub) add(s *agentSubscriber) { h.mu.Lock(); h.subscribers[s] = struct{}{}; h.mu.Unlock() }
func (h *agentHub) remove(s *agentSubscriber) {
	h.mu.Lock()
	delete(h.subscribers, s)
	h.mu.Unlock()
	s.close()
}
func (s *agentSubscriber) close() { s.once.Do(func() { close(s.done); close(s.queue) }) }

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
		if sub.tenant != e.TenantID || (sub.conversation != "" && sub.conversation != e.ConversationID) {
			continue
		}
		if !sub.enqueue(e) {
			sub.close()
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
	ConfirmationID   string    `json:"confirmationId,omitempty"`
	ActionLabel      string    `json:"actionLabel,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
}

type agentCommand struct {
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
	// Origin and CSRF are checked above using the authenticated BFF session.
	// nhooyr's default same-host check is intentionally bypassed here because
	// Stacklane and loopback are both legitimate public origins in development.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"primer-tasks.v1"}, InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "closed")
	sub := &agentSubscriber{tenant: sc.Tenant, queue: make(chan wireAgentEvent, 64), done: make(chan struct{})}
	s.agentHub.add(sub)
	defer s.agentHub.remove(sub)
	connectionID := uuid.NewString()
	_ = wsjson.Write(r.Context(), conn, wireAgentEvent{Type: "hello", ProtocolVersion: agentProtocolVersion, ConnectionID: connectionID, HeartbeatSeconds: 30, ConversationID: "", Sequence: 0, Cursor: 0, Time: time.Now().UTC()})
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sub.done:
				return
			case event, ok := <-sub.queue:
				if !ok {
					return
				}
				_ = wsjson.Write(ctx, conn, event)
			}
		}
	}()
	for {
		var cmd agentCommand
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
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
			sub.conversation = ""
		case "ack":
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
	sub.conversation = cmd.ConversationID
	repo := agent.NewPostgresRepository(s.DB)
	events, err := repo.ReplayEvents(ctx, sc.Tenant, cmd.ConversationID, cmd.Cursor, 200)
	if err == nil {
		for _, item := range events {
			var event wireAgentEvent
			if json.Unmarshal(item.Payload, &event) == nil {
				event.TenantID = sc.Tenant
				if !sub.enqueue(event) {
					s.agentHub.remove(sub)
					return
				}
			}
		}
	}
}

func (s *Server) agentMessage(ctx context.Context, sc scope, cmd agentCommand) {
	if strings.TrimSpace(cmd.Text) == "" || cmd.ClientMessageID == "" {
		return
	}
	if _, err := uuid.Parse(cmd.ConversationID); err != nil {
		return
	}
	var actor string
	if err := s.DB.QueryRow(ctx, `SELECT actor_id FROM agent_conversations WHERE tenant_id=$1 AND id=$2`, sc.Tenant, cmd.ConversationID).Scan(&actor); err != nil || actor != sc.Subject {
		return
	}
	var sequence int64
	_ = s.DB.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM agent_messages WHERE tenant_id=$1 AND conversation_id=$2`, sc.Tenant, cmd.ConversationID).Scan(&sequence)
	repo := agent.NewPostgresRepository(s.DB)
	now := time.Now().UTC()
	message, inserted, err := repo.AppendUserMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: sc.Tenant, ConversationID: cmd.ConversationID, ClientMessageID: cmd.ClientMessageID, Content: strings.TrimSpace(cmd.Text), Sequence: sequence, CreatedAt: now})
	if err != nil {
		return
	}
	runID := uuid.NewString()
	if !inserted {
		return
	}
	cfg, cfgErr := parent.LoadProviderConfig()
	if cfgErr != nil {
		cfg = parent.ProviderConfig{Mode: parent.ProviderDisabled, MaxSteps: 12, MaxTokens: 4096, MaxDuration: 120 * time.Second, MaxRetries: 0}
	}
	digest := sha256.Sum256([]byte(message.Content))
	run := agent.Run{ID: runID, TenantID: sc.Tenant, ConversationID: cmd.ConversationID, UserMessageID: message.ID, Status: agent.RunQueued, MaxSteps: cfg.MaxSteps, MaxTokens: cfg.MaxTokens, Deadline: now.Add(cfg.MaxDuration), CreatedAt: now, Provenance: agent.Provenance{Provider: string(cfg.Mode), PolicyVersion: "parent.v1", PromptDigest: hex.EncodeToString(digest[:])}}
	if err := repo.CreateRun(ctx, run); err != nil {
		return
	}
	jobRepo := jobsRepository(s.DB)
	_ = jobRepo.Enqueue(ctx, jobRecord(runID, sc.Tenant, cfg.MaxRetries+1))
}

func jobRecord(runID, tenant string, attempts int) jobs.Job {
	return jobs.Job{ID: uuid.NewString(), TenantID: tenant, RunID: runID, MaxAttempts: maxInt(attempts, 1), AvailableAt: time.Now().UTC()}
}
func jobsRepository(db *pgxpool.Pool) *jobs.PostgresRepository { return jobs.NewPostgresRepository(db) }
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
	_ = agent.NewPostgresRepository(s.DB).RequestCancel(ctx, sc.Tenant, cmd.RunID)
}
func (s *Server) agentConfirm(ctx context.Context, sc scope, cmd agentCommand) {
	if cmd.ConfirmationID == "" || cmd.RunID == "" {
		return
	}
	if cmd.ConversationID == "" {
		_ = s.DB.QueryRow(ctx, `SELECT conversation_id FROM agent_runs WHERE tenant_id=$1 AND id=$2`, sc.Tenant, cmd.RunID).Scan(&cmd.ConversationID)
	}
	if cmd.ConversationID == "" {
		return
	}
	handle := cmd.ConfirmationID
	var actionJSON []byte
	var digest []byte
	err := s.DB.QueryRow(ctx, `SELECT action,action_digest FROM parent_confirmation_previews WHERE handle_hash=$1 AND tenant_id=$2 AND actor_id=$3`, parentHandleHash(handle), sc.Tenant, sc.Subject).Scan(&actionJSON, &digest)
	if err != nil {
		s.publishAgent(ctx, sc.Tenant, cmd.ConversationID, wireAgentEvent{Type: "error", ProtocolVersion: 1, ConversationID: cmd.ConversationID, RunID: cmd.RunID, Code: "confirmation_rejected", Message: "confirmation is expired, altered, or unavailable", Retryable: false, TenantID: sc.Tenant})
		return
	}
	var action parent.Action
	if json.Unmarshal(actionJSON, &action) != nil {
		return
	}
	cfg, cfgErr := parent.LoadProviderConfig()
	if cfgErr != nil {
		return
	}
	toolSet, err := parent.NewToolSet(cfg.ActiveTools)
	if err != nil {
		return
	}
	tools := s.parentTools()
	_, err = tools.ConfirmAction(ctx, parent.Context{TenantID: sc.Tenant, ActorID: sc.Subject, IdempotencyKey: uuid.NewString(), Tools: toolSet}, parent.ConfirmActionInput{Handle: handle, Action: action})
	if err != nil {
		s.publishAgent(ctx, sc.Tenant, cmd.ConversationID, wireAgentEvent{Type: "error", ProtocolVersion: 1, ConversationID: cmd.ConversationID, RunID: cmd.RunID, Code: "confirmation_rejected", Message: "confirmation was not applied", Retryable: false, TenantID: sc.Tenant})
		return
	}
	s.publishAgent(ctx, sc.Tenant, cmd.ConversationID, wireAgentEvent{Type: "tool_progress", ProtocolVersion: 1, ConversationID: cmd.ConversationID, RunID: cmd.RunID, Tool: "Confirm change", ToolStatus: "completed", Phase: "completed", Summary: "Confirmed change applied through the Tasks service.", TenantID: sc.Tenant})
}

func parentHandleHash(handle string) []byte {
	h := sha256.Sum256(append([]byte("primer.tasks.parent.confirmation.handle.v1\x00"), []byte(handle)...))
	return h[:]
}

func (s *Server) publishAgent(ctx context.Context, tenant, conversation string, event wireAgentEvent) error {
	if event.RunID == "" || conversation == "" {
		return errors.New("durable agent events require a run and conversation")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serialize cursor allocation across all runs in a conversation. A second
	// command may start while the first terminal event is being persisted.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant+":"+conversation); err != nil {
		return err
	}
	var seq int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(e.sequence),0)+1 FROM agent_run_events e JOIN agent_runs r ON r.id=e.run_id AND r.tenant_id=e.tenant_id WHERE e.tenant_id=$1 AND r.conversation_id=$2`, tenant, conversation).Scan(&seq); err != nil {
		return err
	}
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE id=$1 AND tenant_id=$2 AND conversation_id=$3)`, event.RunID, tenant, conversation).Scan(&owned); err != nil || !owned {
		if err == nil {
			err = errors.New("agent run does not belong to conversation")
		}
		return err
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
			slog.Error("parent agent job failed", "run_id", job.RunID, "error", err)
		}
		return err
	})
	go worker.Run(ctx)
}

func (s *Server) executeAgentRun(ctx context.Context, job jobs.Job) error {
	repo := agent.NewPostgresRepository(s.DB)
	run, err := repo.GetRun(ctx, job.TenantID, job.RunID)
	if err != nil {
		return err
	}
	var conversation, prompt, actor string
	if err := s.DB.QueryRow(ctx, `SELECT r.conversation_id,m.content,c.actor_id FROM agent_runs r JOIN agent_messages m ON m.id=r.user_message_id JOIN agent_conversations c ON c.id=r.conversation_id AND c.tenant_id=r.tenant_id WHERE r.tenant_id=$1 AND r.id=$2`, job.TenantID, job.RunID).Scan(&conversation, &prompt, &actor); err != nil {
		return err
	}
	cfg, cfgErr := parent.LoadProviderConfig()
	if cfgErr != nil {
		cfg = parent.ProviderConfig{Mode: parent.ProviderDisabled, MaxSteps: 12, MaxTokens: 4096, MaxDuration: 120 * time.Second, MaxRetries: 0}
	}
	if run.CancelRequested || run.Status == agent.RunCancelRequested {
		_ = repo.TransitionRun(ctx, job.TenantID, job.RunID, agent.RunCanceled, run.DurableStep, run.Usage)
		_ = s.emitAgent(ctx, job.TenantID, conversation, job.RunID, wireAgentEvent{Type: "terminal", Status: "cancelled", Message: "The run was canceled. No further agent work will be performed."})
		return nil
	}
	if cfg.Mode == parent.ProviderDisabled {
		_, _ = s.DB.Exec(ctx, `UPDATE agent_runs SET status='failed',updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status IN ('queued','running')`, job.TenantID, job.RunID)
		_ = s.publishAgent(ctx, job.TenantID, conversation, wireAgentEvent{Type: "terminal", RunID: job.RunID, Status: "disabled", Text: "Agent mode is disabled. Use the ordinary Tasks and Schedules pages.", Message: "Agent mode is disabled. Use the ordinary Tasks and Schedules pages.", TenantID: job.TenantID})
		return nil
	}
	model, err := s.agentModel(ctx, cfg, prompt)
	if err != nil {
		s.publishAgent(ctx, job.TenantID, conversation, wireAgentEvent{Type: "terminal", RunID: job.RunID, Status: "failed", Message: "The configured provider could not start.", Code: "provider_unavailable", TenantID: job.TenantID})
		return err
	}
	_, _ = s.DB.Exec(ctx, `UPDATE agent_runs SET provider=$3,model=$4,updated_at=now() WHERE tenant_id=$1 AND id=$2`, job.TenantID, job.RunID, model.Provider(), model.Model())
	tools := s.fantasyTools(job.TenantID, actor, cfg.ActiveTools, run.Provenance.PromptDigest)
	rt, err := agent.NewFantasyAgent(model, tools, agent.Limits{MaxSteps: cfg.MaxSteps, MaxTokens: cfg.MaxTokens, Deadline: cfg.MaxDuration, MaxRetries: cfg.MaxRetries})
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
		select {
		case <-canceled:
			_ = repo.TransitionRun(ctx, job.TenantID, job.RunID, agent.RunCanceled, run.DurableStep, run.Usage)
			_ = s.emitAgent(ctx, job.TenantID, conversation, job.RunID, wireAgentEvent{Type: "terminal", Status: "cancelled", Message: "The run was canceled. No further agent work will be performed."})
			return nil
		default:
		}
		_ = repo.TransitionRun(ctx, job.TenantID, job.RunID, agent.RunFailed, run.DurableStep, run.Usage)
		_ = s.emitAgent(ctx, job.TenantID, conversation, job.RunID, wireAgentEvent{Type: "terminal", Status: "failed", Message: "The provider or tool failed before a confirmed result.", Code: "run_failed"})
		return nil
	}
	if execution.FinalText != "" {
		_ = repo.AppendMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: job.TenantID, ConversationID: conversation, Role: agent.RoleAssistant, Content: execution.FinalText, Sequence: time.Now().UnixNano()})
	}
	_ = repo.TransitionRun(ctx, job.TenantID, job.RunID, agent.RunSucceeded, run.DurableStep+1, execution.Usage)
	_ = s.emitAgent(ctx, job.TenantID, conversation, job.RunID, wireAgentEvent{Type: "terminal", Status: "completed", Message: "The parent command completed."})
	return nil
}

// watchRunCancellation makes the database cancellation flag authoritative.
// The worker context is process-owned; no websocket request context can cancel
// a detached run. The returned cancellation is only a signal to Fantasy.
func (s *Server) watchRunCancellation(workerCtx context.Context, tenant, runID string) (context.Context, func(), <-chan struct{}) {
	runCtx, cancel := context.WithCancel(workerCtx)
	stop := make(chan struct{})
	canceled := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			var requested bool
			err := s.DB.QueryRow(workerCtx, `SELECT cancel_requested OR status='cancel_requested' FROM agent_runs WHERE tenant_id=$1 AND id=$2`, tenant, runID).Scan(&requested)
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
	return runCtx, func() { close(stop); cancel() }, canceled
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
	case agentprotocol.EventConfirmation:
		out.Type = "tool_progress"
		out.Tool = "Prepare change"
		out.Phase = "awaiting_confirmation"
		out.ConfirmationID = event.ConfirmationID
		out.Summary = event.Summary
		out.ExpiresAt = event.ExpiresAt
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

func (s *Server) fantasyTools(tenant, actor string, active []string, _ string) []fantasy.AgentTool {
	tools := s.parentTools()
	set, err := parent.NewToolSet(active)
	if err != nil {
		return nil
	}
	ctx := parent.Context{TenantID: tenant, ActorID: actor, IdempotencyKey: uuid.NewString(), Tools: set}
	// Actor is supplied by the authenticated run in production; this function
	// is called only after the run tenant has been verified.  The worker fills
	// actor from the durable conversation before invoking tools in later steps.
	all := []fantasy.AgentTool{
		fantasy.NewAgentTool("list_students", "List household students", func(c context.Context, in struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListStudents(c, ctx, parent.StudentQuery{Name: in.Query, Limit: in.Limit})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("list_tasks", "List household tasks", func(c context.Context, in struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListTasks(c, ctx, parent.TaskQuery{Query: in.Query, Limit: in.Limit})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("draft_task", "Draft a task", func(c context.Context, in struct {
			Title        string `json:"title"`
			Instructions string `json:"instructions"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.DraftTask(c, ctx, parent.TaskDraftInput{Title: in.Title, Instructions: in.Instructions})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("publish_task", "Publish a drafted task", func(c context.Context, in struct {
			ID string `json:"id"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.PublishTask(c, ctx, in.ID)
			return safeToolJSON(v, e)
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
			v, e := tools.CreateSchedule(c, ctx, parent.ScheduleInput{StudentID: in.StudentID, TemplateID: in.TemplateID, RevisionID: in.RevisionID, Kind: in.Kind, Timezone: in.Timezone, StartAt: startAt, RRULE: in.RRULE, DueOffsetMinutes: in.DueOffsetMinutes}, in.StudentName)
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("list_schedules", "List schedules", func(c context.Context, in struct {
			IncludeDisabled bool `json:"includeDisabled"`
			Limit           int  `json:"limit"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListSchedules(c, ctx, parent.ScheduleQuery{IncludeDisabled: in.IncludeDisabled, Limit: in.Limit})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("list_occurrences", "List occurrences", func(c context.Context, in struct {
			Status string `json:"status"`
			Limit  int    `json:"limit"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.ListOccurrences(c, ctx, parent.OccurrenceQuery{Status: in.Status, Limit: in.Limit})
			return safeToolJSON(v, e)
		}),
		fantasy.NewAgentTool("preview_action", "Prepare a destructive change for explicit parent confirmation", func(c context.Context, in struct {
			Kind      string         `json:"kind"`
			TargetIDs []string       `json:"targetIds"`
			Payload   map[string]any `json:"payload"`
			Summary   string         `json:"summary"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			v, e := tools.PreviewAction(c, ctx, parent.Action{Kind: in.Kind, TargetIDs: in.TargetIDs, Payload: in.Payload}, in.Summary)
			if e == nil {
				b, _ := json.Marshal(v)
				return fantasy.NewTextResponse(string(b)), nil
			}
			return safeToolJSON(v, e)
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
		return fantasy.NewTextErrorResponse("tool unavailable"), err
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
			return scriptedToolStream(ctx, "draft_task", `{"title":"Scripted parent task","instructions":"Complete the task, then ask a parent to check it."}`), nil
		case 2:
			return scriptedToolStream(ctx, "publish_task", scriptedIDInput(call, "draft_task")), nil
		case 3:
			return scriptedToolStream(ctx, "list_students", `{"query":"","limit":20}`), nil
		case 4:
			return scriptedToolStream(ctx, "create_schedule", scriptedScheduleInput(call)), nil
		}
		return scriptedStream(ctx, "The task was created and scheduled through the server-owned Tasks service."), nil
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
