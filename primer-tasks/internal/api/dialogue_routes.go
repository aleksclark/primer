package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"git.clark.team/aleksclark/authstack/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/domain"
	"primer-tasks/internal/verification"
)

type DialogueOccurrenceInput struct {
	ID            string `path:"id"`
	RequirementID string `query:"requirementId"`
}
type DialogueStartBody struct {
	RequirementID string `json:"requirementId,omitempty"`
}
type DialogueStartInput struct {
	ID   string `path:"id"`
	Body DialogueStartBody
}
type DialogueStateOutput struct {
	ResponseHeaders
	Body verification.DialogueEvent
}

func (s *Server) studentOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	if origin == s.Auth.PublicOrigin {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("TASKS_ALLOWED_ORIGINS"), ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}
func (s *Server) studentIdentityFromRequest(r *http.Request) (verification.StudentAuthority, error) {
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return verification.StudentAuthority{}, verification.ErrDialogueRevoked
	}
	cookie, err := r.Cookie("tasks_student")
	if err != nil || cookie.Value == "" || len(cookie.Value) > 512 {
		return verification.StudentAuthority{}, verification.ErrDialogueRevoked
	}
	for _, key := range []string{"token", "access_token", "authorization", "auth", "cookie", "session"} {
		if _, present := r.URL.Query()[key]; present {
			return verification.StudentAuthority{}, verification.ErrDialogueRevoked
		}
	}
	return verification.ResolveStudentAuthority(r.Context(), s.DB, hash(cookie.Value))
}
func (s *Server) studentMutationAllowed(r *http.Request) bool {
	cookie, err := r.Cookie("tasks_csrf")
	return err == nil && cookie.Value != "" && r.Header.Get("X-CSRF-Token") == cookie.Value && s.studentOriginAllowed(r)
}
func dialogueProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, verification.ErrDialogueRevoked):
		problem(w, 401, "revoked", "Student pairing is no longer active.")
	case errors.Is(err, verification.ErrDialogueContext):
		problem(w, 404, "not_found", "This dialogue is unavailable.")
	case errors.Is(err, verification.ErrDialogueConflict), errors.Is(err, verification.ErrDialogueTerminal), errors.Is(err, verification.ErrDialogueLimit):
		problem(w, 409, "conflict", "Refresh the current dialogue state before continuing.")
	default:
		problem(w, 503, "unavailable", "Dialogue is temporarily unavailable; saved evidence is retained.")
	}
}

func (s *Server) studentDialogueStart(w http.ResponseWriter, r *http.Request) {
	if !s.studentMutationAllowed(r) {
		problem(w, 403, "denied", "Origin or CSRF rejected.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	a, err := s.studentIdentityFromRequest(r)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	var body DialogueStartBody
	if !decode(w, r, &body) {
		return
	}
	state, err := (verification.DialogueEngine{DB: s.DB}).Start(ctx, a, chi.URLParam(r, "id"), body.RequirementID)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	event, err := verification.DialogueStateEvent(state)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	jsonOK(w, event)
}
func (s *Server) studentDialogueState(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	a, err := s.studentIdentityFromRequest(r)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	occurrence := chi.URLParam(r, "id")
	requirement := r.URL.Query().Get("requirementId")
	var attempt string
	err = s.DB.QueryRow(ctx, `SELECT d.attempt_id FROM dialogue_attempts d JOIN verification_attempts v ON v.tenant_id=d.tenant_id AND v.id=d.attempt_id WHERE d.tenant_id=$1 AND d.student_id=$2 AND d.occurrence_id=$3 AND ($4='' OR d.requirement_id::text=$4) ORDER BY v.number DESC,d.requirement_id LIMIT 1`, a.TenantID, a.StudentID, occurrence, requirement).Scan(&attempt)
	if err != nil {
		dialogueProblem(w, verification.ErrDialogueContext)
		return
	}
	event, err := s.readStudentDialogueState(ctx, a, occurrence, attempt)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	jsonOK(w, event)
}
func (s *Server) readStudentDialogueState(ctx context.Context, a verification.StudentAuthority, occurrence, attempt string) (verification.DialogueEvent, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return verification.DialogueEvent{}, err
	}
	defer tx.Rollback(ctx)
	if err = verification.LockDialogueAccess(ctx, tx, a, occurrence, attempt, false); err != nil {
		return verification.DialogueEvent{}, err
	}
	// Assemble the version/questions/messages/evaluations from one stable
	// progress state. This short DB-only lock ends BEFORE any network write;
	// writeStudentFrame still holds only revocation/immutable-binding authority.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT attempt_id FROM dialogue_attempts WHERE tenant_id=$1 AND occurrence_id=$2 AND attempt_id=$3 FOR SHARE`, a.TenantID, occurrence, attempt).Scan(&locked); err != nil {
		return verification.DialogueEvent{}, verification.ErrDialogueContext
	}
	if err = verification.CheckLockedStudentAuthority(ctx, tx, a); err != nil {
		return verification.DialogueEvent{}, err
	}
	state, err := verification.LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return verification.DialogueEvent{}, err
	}
	event, err := verification.DialogueStateEvent(state)
	if err != nil {
		return event, err
	}
	var failed, retryable bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2 AND status='failed'),EXISTS(SELECT 1 FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2 AND status='failed' AND attempts<max_attempts AND deadline>clock_timestamp() AND session_id=$3)`, a.TenantID, attempt, a.SessionID).Scan(&failed, &retryable); err != nil {
		return event, err
	}
	if failed && !state.Terminal {
		event.Phase, event.Code, event.Retryable = "failed", "provider_unavailable", retryable
	}
	if err = tx.QueryRow(ctx, `SELECT CASE WHEN EXISTS(SELECT 1 FROM verification_overrides WHERE tenant_id=$1 AND attempt_id=$2) THEN 'parent_override' WHEN EXISTS(SELECT 1 FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2) THEN 'verification_engine' ELSE '' END`, a.TenantID, attempt).Scan(&event.DecisionSource); err != nil {
		return event, err
	}
	return event, nil
}

type DialogueOverrideRequest struct {
	AttemptID       string `json:"attemptId"`
	ClientRequestID string `json:"clientRequestId" minLength:"1" maxLength:"128"`
	ExpectedVersion int64  `json:"expectedVersion" minimum:"1"`
	Accepted        bool   `json:"accepted"`
	Reason          string `json:"reason" minLength:"8" maxLength:"1000"`
}
type DialogueOverrideInput struct {
	ID   string `path:"id"`
	Body DialogueOverrideRequest
}
type DialogueOverrideOutput struct {
	ResponseHeaders
	Body verification.DialogueOverrideReceipt
}
type DialogueInspectInput struct {
	ID            string `path:"id"`
	AttemptID     string `query:"attemptId"`
	After         int64  `query:"after" minimum:"0"`
	Limit         int    `query:"limit" default:"20" minimum:"1" maximum:"50"`
	AttemptOffset int    `query:"attemptOffset" minimum:"0"`
}
type DialogueInspectEntry struct {
	Sequence       int64     `json:"sequence"`
	Kind           string    `json:"kind"`
	Author         string    `json:"author"`
	At             time.Time `json:"at"`
	Text           string    `json:"text"`
	QuestionID     string    `json:"questionId,omitempty"`
	MessageID      string    `json:"messageId,omitempty"`
	DecisionID     string    `json:"decisionId,omitempty"`
	DecisionSource string    `json:"decisionSource,omitempty"`
	Status         string    `json:"status,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	Model          string    `json:"model,omitempty"`
	PolicyVersion  string    `json:"policyVersion"`
	InputTokens    int64     `json:"inputTokens"`
	OutputTokens   int64     `json:"outputTokens"`
	Criteria       []string  `json:"criteria"`
}
type DialogueAttemptInfo struct {
	ID            string `json:"id"`
	RequirementID string `json:"requirementId"`
	Number        int    `json:"number"`
	Status        string `json:"status"`
}
type DialogueInspect struct {
	OccurrenceID  string                 `json:"occurrenceId"`
	Title         string                 `json:"title"`
	StudentName   string                 `json:"studentName"`
	Status        string                 `json:"status"`
	AttemptID     string                 `json:"attemptId"`
	RequirementID string                 `json:"requirementId"`
	Version       int64                  `json:"version"`
	AcceptedCount int                    `json:"acceptedCount"`
	RequiredCount int                    `json:"requiredCount"`
	SourceVersion string                 `json:"sourceVersion"`
	SourceSHA256  string                 `json:"sourceSha256"`
	Entries       []DialogueInspectEntry `json:"entries"`
	NextCursor    int64                  `json:"nextCursor"`
	HasMore       bool                   `json:"hasMore"`
	Attempts      []DialogueAttemptInfo  `json:"attempts"`
	AttemptTotal  int                    `json:"attemptTotal"`
	AttemptOffset int                    `json:"attemptOffset"`
	AttemptLimit  int                    `json:"attemptLimit"`
}
type DialogueInspectOutput struct {
	ResponseHeaders
	Body DialogueInspect
}

// HTTP parent credentials use the same already-established Authstack/local
// membership locks as P3. No provider token is persisted or sent to a tool.
func (s *Server) dialogueParentContext(r *http.Request, sc scope) (context.Context, error) {
	a := &agentAuthorization{}
	if s.Auth.Mode == "clerk" {
		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok || p.SessionID == "" {
			return nil, auth.ErrUnauthenticated
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		a.issuer, a.subject, a.session = p.Issuer, string(p.Subject), p.SessionID
		a.verifyProvider = func(ctx context.Context) error {
			current, err := s.ParentAuthenticator.Authenticate(ctx, auth.Credential{Token: token}, s.ParentPolicy)
			if err != nil || current.Kind != auth.PrincipalHuman || current.Credential != auth.CredentialSession || current.Issuer != a.issuer || string(current.Subject) != a.subject || current.SessionID != a.session {
				return auth.ErrUnauthenticated
			}
			return nil
		}
	} else {
		cookie, err := r.Cookie("tasks_parent")
		if err != nil {
			return nil, err
		}
		a.bffHash = hash(cookie.Value)
	}
	return context.WithValue(r.Context(), agentAuthKey{}, a), nil
}
func (s *Server) dialogueParentTransaction(r *http.Request, sc scope) (context.Context, pgx.Tx, context.CancelFunc, error) {
	ctx, err := s.dialogueParentContext(r, sc)
	if err != nil {
		return nil, nil, func() {}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		cancel()
		return ctx, nil, func() {}, err
	}
	if err = lockAgentParent(ctx, tx, sc.Tenant, sc.Subject); err != nil {
		_ = tx.Rollback(ctx)
		cancel()
		return ctx, nil, func() {}, err
	}
	return ctx, tx, cancel, nil
}
func (s *Server) dialogueOverride(w http.ResponseWriter, r *http.Request, sc scope) {
	ctx, tx, cancel, err := s.dialogueParentTransaction(r, sc)
	if err != nil {
		s.parentError(w, err)
		return
	}
	defer cancel()
	defer tx.Rollback(ctx)
	var in DialogueOverrideRequest
	if !decode(w, r, &in) {
		return
	}
	occurrence := chi.URLParam(r, "id")
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM task_occurrences WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, sc.Tenant, occurrence).Scan(&id); err != nil {
		dialogueProblem(w, verification.ErrDialogueContext)
		return
	}
	if err = checkLockedAgentParent(ctx, tx, sc.Tenant, sc.Subject); err != nil {
		s.parentError(w, err)
		return
	}
	receipt, err := verification.OverrideDialogue(ctx, tx, sc.Tenant, sc.Subject, occurrence, in.AttemptID, in.ClientRequestID, in.Reason, in.ExpectedVersion, in.Accepted)
	if errors.Is(err, verification.ErrDialogueReversal) {
		problem(w, 409, "completed_reversal_unsupported", "Completed work cannot be reversed by this override.")
		return
	}
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		dialogueProblem(w, err)
		return
	}
	jsonOK(w, receipt)
}
func (s *Server) dialogueInspect(w http.ResponseWriter, r *http.Request, sc scope) {
	ctx, tx, cancel, err := s.dialogueParentTransaction(r, sc)
	if err != nil {
		s.parentError(w, err)
		return
	}
	defer cancel()
	defer tx.Rollback(ctx)
	out := DialogueInspect{OccurrenceID: chi.URLParam(r, "id"), Entries: []DialogueInspectEntry{}, Attempts: []DialogueAttemptInfo{}, AttemptLimit: 10}
	var student string
	if err = tx.QueryRow(ctx, `SELECT o.revision_snapshot->>'title',st.display_name,o.status,o.student_id FROM task_occurrences o JOIN students st ON st.tenant_id=o.tenant_id AND st.id=o.student_id WHERE o.tenant_id=$1 AND o.id=$2`, sc.Tenant, out.OccurrenceID).Scan(&out.Title, &out.StudentName, &out.Status, &student); err != nil {
		dialogueProblem(w, verification.ErrDialogueContext)
		return
	}
	out.AttemptID = r.URL.Query().Get("attemptId")
	if out.AttemptID == "" {
		if err = tx.QueryRow(ctx, `SELECT d.attempt_id FROM dialogue_attempts d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE d.tenant_id=$1 AND d.occurrence_id=$2 ORDER BY a.number DESC,d.attempt_id LIMIT 1`, sc.Tenant, out.OccurrenceID).Scan(&out.AttemptID); err != nil {
			dialogueProblem(w, verification.ErrDialogueContext)
			return
		}
	}
	// Parent history includes earlier attempts; it is not restricted to the
	// student's current writable attempt. The published snapshot stays bound.
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT requirement_id,version,config_snapshot FROM dialogue_attempts WHERE tenant_id=$1 AND occurrence_id=$2 AND attempt_id=$3`, sc.Tenant, out.OccurrenceID, out.AttemptID).Scan(&out.RequirementID, &out.Version, &raw); err != nil {
		dialogueProblem(w, verification.ErrDialogueContext)
		return
	}
	var snapshot domain.DialogueSnapshot
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.Validate() != nil || snapshot.RequirementID != out.RequirementID {
		dialogueProblem(w, verification.ErrDialogueContext)
		return
	}
	out.SourceVersion, out.SourceSHA256 = snapshot.Source.Version, snapshot.Source.SHA256
	out.RequiredCount = 3
	if err = tx.QueryRow(ctx, `SELECT count(DISTINCT question_id) FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2 AND accepted`, sc.Tenant, out.AttemptID).Scan(&out.AcceptedCount); err != nil {
		dialogueProblem(w, err)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit := 20
	if value, _ := strconv.Atoi(r.URL.Query().Get("limit")); value > 0 && value <= 50 {
		limit = value
	}
	var maxCursor int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2`, sc.Tenant, out.AttemptID).Scan(&maxCursor); err != nil {
		dialogueProblem(w, err)
		return
	}
	if after > maxCursor {
		dialogueProblem(w, verification.ErrDialogueConflict)
		return
	}
	rows, err := tx.Query(ctx, `SELECT ev.sequence,ev.payload,COALESCE(v.provider,''),COALESCE(v.model,''),COALESCE(v.input_tokens,0),COALESCE(v.output_tokens,0),COALESCE(v.criteria,'[]'::jsonb),COALESCE(ov.reason,'') FROM verification_events ev LEFT JOIN verification_evaluations v ON ev.kind='answer_evaluation' AND v.tenant_id=ev.tenant_id AND v.attempt_id=ev.attempt_id AND v.message_id::text=ev.payload->>'messageId' LEFT JOIN verification_overrides ov ON ev.kind='override' AND ov.tenant_id=ev.tenant_id AND ov.attempt_id=ev.attempt_id AND ov.id::text=ev.payload->>'decisionId' WHERE ev.tenant_id=$1 AND ev.attempt_id=$2 AND ev.sequence>$3 ORDER BY ev.sequence LIMIT $4`, sc.Tenant, out.AttemptID, after, limit+1)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	for rows.Next() {
		var entry DialogueInspectEntry
		var event wireStudentEvent
		var payload []byte
		var overrideReason string
		if err = rows.Scan(&entry.Sequence, &payload, &entry.Provider, &entry.Model, &entry.InputTokens, &entry.OutputTokens, &entry.Criteria, &overrideReason); err != nil {
			break
		}
		if len(out.Entries) == limit {
			out.HasMore = true
			break
		}
		if err = json.Unmarshal(payload, &event); err != nil {
			break
		}
		entry.Kind, entry.At, entry.Text, entry.QuestionID, entry.MessageID, entry.DecisionID, entry.DecisionSource, entry.Status, entry.PolicyVersion = event.Kind, event.Time, event.Text, event.QuestionID, event.MessageID, event.DecisionID, event.DecisionSource, event.Status, event.PolicyVersion
		entry.Author = "system"
		if event.Kind == "message_ack" {
			entry.Author = "student"
		} else if event.Kind == "question" || event.Kind == "answer_evaluation" {
			entry.Author = "agent"
		} else if event.Kind == "override" {
			entry.Author, entry.Text = "parent", overrideReason
		}
		out.Entries = append(out.Entries, entry)
		out.NextCursor = entry.Sequence
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	out.AttemptOffset, _ = strconv.Atoi(r.URL.Query().Get("attemptOffset"))
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dialogue_attempts WHERE tenant_id=$1 AND occurrence_id=$2`, sc.Tenant, out.OccurrenceID).Scan(&out.AttemptTotal); err != nil {
		dialogueProblem(w, err)
		return
	}
	rows, err = tx.Query(ctx, `SELECT a.id,a.requirement_id,a.number,a.status FROM verification_attempts a JOIN dialogue_attempts d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id WHERE a.tenant_id=$1 AND a.occurrence_id=$2 ORDER BY a.number DESC,a.id LIMIT $3 OFFSET $4`, sc.Tenant, out.OccurrenceID, out.AttemptLimit, out.AttemptOffset)
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	for rows.Next() {
		var attempt DialogueAttemptInfo
		if err = rows.Scan(&attempt.ID, &attempt.RequirementID, &attempt.Number, &attempt.Status); err != nil {
			break
		}
		out.Attempts = append(out.Attempts, attempt)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		dialogueProblem(w, err)
		return
	}
	if err = checkLockedAgentParent(ctx, tx, sc.Tenant, sc.Subject); err != nil {
		s.parentError(w, err)
		return
	}
	jsonOK(w, out)
}

func (s *Server) registerDialogueRoutes(api huma.API) {
	register(api, huma.Operation{OperationID: "occurrence-dialogue-inspect", Method: http.MethodGet, Path: "/occurrences/{id}/inspect", Errors: []int{401, 403, 404, 409, 503}}, func(ctx context.Context, _ *DialogueInspectInput) (*DialogueInspectOutput, error) {
		body, headers, err := legacyJSON[DialogueInspect](ctx, s.requireParent(s.dialogueInspect), nil)
		return &DialogueInspectOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "occurrence-dialogue-override", Method: http.MethodPost, Path: "/occurrences/{id}/override", MaxBodyBytes: 16384, Errors: []int{401, 403, 404, 409, 422, 503}}, func(ctx context.Context, in *DialogueOverrideInput) (*DialogueOverrideOutput, error) {
		body, headers, err := legacyJSON[verification.DialogueOverrideReceipt](ctx, s.requireParent(s.dialogueOverride), in.Body)
		return &DialogueOverrideOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "student-occurrence-dialogue-start", Method: http.MethodPost, Path: "/student/occurrences/{id}/dialogue", MaxBodyBytes: 16384, Errors: []int{401, 403, 404, 409, 422, 503}}, func(ctx context.Context, in *DialogueStartInput) (*DialogueStateOutput, error) {
		body, headers, err := legacyJSON[verification.DialogueEvent](ctx, http.HandlerFunc(s.studentDialogueStart), in.Body)
		return &DialogueStateOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "student-occurrence-dialogue-state", Method: http.MethodGet, Path: "/student/occurrences/{id}/dialogue", Errors: []int{401, 404, 409, 503}}, func(ctx context.Context, _ *DialogueOccurrenceInput) (*DialogueStateOutput, error) {
		body, headers, err := legacyJSON[verification.DialogueEvent](ctx, http.HandlerFunc(s.studentDialogueState), nil)
		return &DialogueStateOutput{ResponseHeaders: headers, Body: body}, err
	})
}
