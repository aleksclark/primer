package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

type DialogueOccurrenceInput struct {
	ID string `path:"id"`
}
type DialogueOverrideInput struct {
	ID   string           `path:"id"`
	Body DialogueOverride `required:"true"`
}
type DialogueOverride struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
}
type DialogueJSONOutput struct {
	ResponseHeaders
	Body map[string]any
}

func (s *Server) dialogueInspect(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	var occurrence, student, status string
	if err := s.DB.QueryRow(r.Context(), `SELECT id::text,student_id::text,status FROM task_occurrences WHERE tenant_id=$1 AND id=$2`, sc.Tenant, id).Scan(&occurrence, &student, &status); err != nil {
		if err == pgx.ErrNoRows {
			problem(w, 404, "not_found", "occurrence not found")
		} else {
			problem(w, 500, "internal", "unable to inspect occurrence")
		}
		return
	}
	type item struct {
		Kind, Role, Content, QuestionID, MessageID, Rationale, Provider, Model, Policy string
		Sequence                                                                       int64
		Accepted                                                                       *bool
		Usage                                                                          map[string]any
		CreatedAt                                                                      time.Time
	}
	rows, err := s.DB.Query(r.Context(), `SELECT m.role,m.content,m.sequence,m.created_at,COALESCE(e.question_id::text,''),COALESCE(e.message_id::text,''),e.accepted,COALESCE(e.rationale,''),COALESCE(e.provider,''),COALESCE(e.model,''),COALESCE(e.policy_version,''),COALESCE(e.usage,'{}'::jsonb) FROM verification_messages m JOIN verification_attempts a ON a.tenant_id=m.tenant_id AND a.id=m.attempt_id LEFT JOIN verification_evaluations e ON e.tenant_id=m.tenant_id AND e.message_id=m.id WHERE m.tenant_id=$1 AND a.occurrence_id=$2 ORDER BY m.sequence,e.created_at`, sc.Tenant, id)
	if err != nil {
		problem(w, 500, "internal", "unable to inspect dialogue")
		return
	}
	defer rows.Close()
	items := make([]item, 0)
	for rows.Next() {
		var x item
		var raw []byte
		if err := rows.Scan(&x.Role, &x.Content, &x.Sequence, &x.CreatedAt, &x.QuestionID, &x.MessageID, &x.Accepted, &x.Rationale, &x.Provider, &x.Model, &x.Policy, &raw); err != nil {
			problem(w, 500, "internal", "unable to read dialogue")
			return
		}
		_ = json.Unmarshal(raw, &x.Usage)
		x.Kind = "message"
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		problem(w, 500, "internal", "unable to read dialogue")
		return
	}
	entries := make([]map[string]any, 0, len(items)*2)
	for _, x := range items {
		messageID := x.MessageID
		if messageID == "" {
			messageID = fmt.Sprintf("message-%d", x.Sequence)
		}
		if x.Role == "student" {
			// Keep student-authored text and agent evaluation evidence as separate
			// immutable records. A shared message ID made the old UI duplicate or
			// merge rows when the evaluation join was replayed.
			entries = append(entries, map[string]any{"id": "message-" + messageID, "kind": "student_answer", "at": x.CreatedAt, "author": "student", "title": "Student answer", "body": x.Content})
		} else {
			entries = append(entries, map[string]any{"id": "message-" + messageID, "kind": "status", "at": x.CreatedAt, "author": "agent", "title": "Agent message", "body": x.Content})
		}
		if x.MessageID != "" && x.Accepted != nil {
			entry := map[string]any{"id": "evaluation-" + x.MessageID, "kind": "evaluation", "at": x.CreatedAt, "author": "agent", "title": "Evaluation", "body": "The answer was evaluated against the parent-authored rubric."}
			if *x.Accepted {
				entry["outcome"] = "accepted"
			} else {
				entry["outcome"] = "rejected"
			}
			if x.Rationale != "" {
				entry["rationale"] = x.Rationale
			}
			if x.Provider != "" {
				entry["provider"] = x.Provider
			}
			if x.Model != "" {
				entry["model"] = x.Model
			}
			if x.Policy != "" {
				entry["policyVersion"] = x.Policy
			}
			if x.Usage != nil {
				entry["usage"] = x.Usage
			}
			entries = append(entries, entry)
		}
	}
	// Questions are agent-authored evidence, stored independently from student
	// messages so an evaluation join can never multiply transcript rows.
	qrows, qerr := s.DB.Query(r.Context(), `SELECT q.id::text,q.prompt,q.created_at FROM dialogue_questions q JOIN verification_attempts a ON a.tenant_id=q.tenant_id AND a.id=q.attempt_id WHERE q.tenant_id=$1 AND a.occurrence_id=$2 ORDER BY q.ordinal`, sc.Tenant, id)
	if qerr != nil {
		problem(w, 500, "internal", "unable to inspect dialogue questions")
		return
	}
	defer qrows.Close()
	for qrows.Next() {
		var qid, prompt string
		var at time.Time
		if err := qrows.Scan(&qid, &prompt, &at); err != nil {
			problem(w, 500, "internal", "unable to read dialogue question")
			return
		}
		entries = append(entries, map[string]any{"id": "question-" + qid, "kind": "question", "at": at, "author": "agent", "title": "Question", "body": prompt})
	}
	if err := qrows.Err(); err != nil {
		problem(w, 500, "internal", "unable to read dialogue questions")
		return
	}
	overrides := make([]map[string]any, 0)
	orows, orr := s.DB.Query(r.Context(), `SELECT id::text,accepted,reason,actor_id,created_at FROM verification_overrides WHERE tenant_id=$1 AND occurrence_id=$2 ORDER BY created_at`, sc.Tenant, id)
	if orr != nil {
		problem(w, 500, "internal", "unable to inspect dialogue overrides")
		return
	}
	defer orows.Close()
	for orows.Next() {
		var oid, reason, actor string
		var accepted bool
		var at time.Time
		if err := orows.Scan(&oid, &accepted, &reason, &actor, &at); err != nil {
			problem(w, 500, "internal", "unable to read dialogue override")
			return
		}
		overrides = append(overrides, map[string]any{"id": oid, "accepted": accepted, "reason": reason, "actorId": actor, "createdAt": at})
	}
	if err := orows.Err(); err != nil {
		problem(w, 500, "internal", "unable to read dialogue overrides")
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, lok := entries[i]["at"].(time.Time)
		right, rok := entries[j]["at"].(time.Time)
		return lok && rok && left.Before(right)
	})
	var acceptedCount, requiredCount int
	var provider, policy string
	if err := s.DB.QueryRow(r.Context(), `SELECT COALESCE(d.accepted_count,0),COALESCE((d.config_snapshot->>'requiredQuestions')::int,0),COALESCE(e.provider,''),COALESCE(e.policy_version,'') FROM dialogue_attempts d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id LEFT JOIN LATERAL (SELECT provider,policy_version FROM verification_evaluations WHERE tenant_id=d.tenant_id AND attempt_id=d.attempt_id ORDER BY created_at DESC,id DESC LIMIT 1) e ON true WHERE d.tenant_id=$1 AND a.occurrence_id=$2 ORDER BY a.number DESC LIMIT 1`, sc.Tenant, id).Scan(&acceptedCount, &requiredCount, &provider, &policy); err != nil && err != pgx.ErrNoRows {
		problem(w, 500, "internal", "unable to inspect dialogue counts")
		return
	}
	response := map[string]any{"occurrenceId": occurrence, "studentId": student, "status": status, "acceptedCount": acceptedCount, "requiredCount": requiredCount, "timeline": entries, "entries": entries, "overrides": overrides}
	if provider != "" {
		response["provider"] = provider
	}
	if policy != "" {
		response["policyVersion"] = policy
	}
	jsonOK(w, response)
}

func (s *Server) dialogueOverride(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	var in DialogueOverride
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len([]rune(in.Reason)) > 1000 {
		problem(w, 400, "invalid_request", "override reason is required and bounded")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		problem(w, 500, "internal", "unable to start override")
		return
	}
	defer tx.Rollback(r.Context())
	var attempt, req string
	if err = tx.QueryRow(r.Context(), `SELECT a.id::text,a.requirement_id::text FROM verification_attempts a WHERE a.tenant_id=$1 AND a.occurrence_id=$2 ORDER BY a.number DESC LIMIT 1 FOR UPDATE`, sc.Tenant, id).Scan(&attempt, &req); err != nil {
		problem(w, 409, "blocked", "dialogue attempt is unavailable")
		return
	}
	var decisionID string
	err = tx.QueryRow(r.Context(), `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,attempt_id) DO NOTHING RETURNING id`, uuid.NewString(), sc.Tenant, attempt, in.Accepted, in.Reason, sc.Subject).Scan(&decisionID)
	if err == pgx.ErrNoRows {
		if err = tx.QueryRow(r.Context(), `SELECT id::text FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, sc.Tenant, attempt).Scan(&decisionID); err != nil {
			problem(w, 500, "internal", "unable to read decision")
			return
		}
	} else if err != nil {
		problem(w, 500, "internal", "unable to commit decision")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO verification_overrides(id,tenant_id,occurrence_id,requirement_id,attempt_id,accepted,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.NewString(), sc.Tenant, id, req, attempt, in.Accepted, in.Reason, sc.Subject); err != nil {
		problem(w, 500, "internal", "unable to audit override")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE verification_attempts SET status=$1 WHERE tenant_id=$2 AND id=$3`, map[bool]string{true: "accepted", false: "rejected"}[in.Accepted], sc.Tenant, attempt); err != nil {
		problem(w, 500, "internal", "unable to update attempt")
		return
	}
	occStatus := map[bool]string{true: "completed", false: "pending"}[in.Accepted]
	if _, err = tx.Exec(r.Context(), `UPDATE task_occurrences SET status=$1 WHERE tenant_id=$2 AND id=$3 AND status NOT IN ('canceled','completed')`, occStatus, sc.Tenant, id); err != nil {
		problem(w, 500, "internal", "unable to update occurrence")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id,metadata) VALUES($1,$2,'verification_override',$3,$4)`, sc.Tenant, sc.Subject, id, []byte(`{"appendOnly":true}`)); err != nil {
		problem(w, 500, "internal", "unable to audit override")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		problem(w, 500, "internal", "unable to commit override")
		return
	}
	jsonOK(w, map[string]any{"occurrenceId": id, "decisionId": decisionID, "accepted": in.Accepted, "status": occStatus, "appendOnly": true})
}

func (s *Server) studentDialogueState(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	occurrenceID := chi.URLParam(r, "id")
	var tenant, attempt, requirement, policy, kind string
	if err := s.DB.QueryRow(r.Context(), `SELECT a.tenant_id,a.id::text,a.requirement_id::text,COALESCE(d.policy_version,'dialogue.v1'),r.kind FROM verification_attempts a JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id JOIN verification_requirements r ON r.tenant_id=a.tenant_id AND r.id=a.requirement_id LEFT JOIN dialogue_attempts d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id WHERE o.id=$1 AND o.student_id=$2 ORDER BY a.number DESC LIMIT 1`, occurrenceID, id).Scan(&tenant, &attempt, &requirement, &policy, &kind); err != nil {
		if err == pgx.ErrNoRows {
			problem(w, 404, "not_found", "dialogue attempt unavailable")
		} else {
			problem(w, 500, "internal", "unable to read dialogue state")
		}
		return
	}
	if kind != "agent_dialogue" {
		problem(w, 404, "not_found", "dialogue attempt unavailable")
		return
	}
	scope := verification.DialogueContext{TenantID: tenant, StudentID: id.String(), OccurrenceID: occurrenceID, RequirementID: requirement, AttemptID: attempt, PolicyVersion: policy}
	state, err := repo.NewDialogueRepository(s.DB).GetDialogueState(r.Context(), scope)
	if err != nil {
		problem(w, 500, "internal", "unable to read dialogue state")
		return
	}
	current := ""
	if len(state.Questions) > 0 {
		current = state.Questions[len(state.Questions)-1].Prompt
	} else {
		current = "What is one specific fact from the parent-assigned chapter?"
	}
	var failed bool
	if err := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2 AND status='failed')`, tenant, attempt).Scan(&failed); err != nil {
		problem(w, 500, "internal", "unable to read dialogue job state")
		return
	}
	name := "in_progress"
	if failed {
		name = "error"
	} else if state.Terminal {
		if state.TerminalStatus == "accepted" || state.AcceptedCount >= state.Config.RequiredQuestions {
			name = "complete"
		} else {
			name = "error"
		}
	} else if len(state.Evaluations) > 0 && !state.Evaluations[len(state.Evaluations)-1].Accepted {
		name = "retry"
	}
	response := map[string]any{"occurrenceId": occurrenceID, "attemptId": attempt, "conversationId": attempt, "status": name, "acceptedCount": state.AcceptedCount, "requiredCount": state.Config.RequiredQuestions, "currentQuestion": current}
	if failed {
		response["errorExplanation"] = "The verifier could not finish this turn. Your answer is saved. Retry when ready."
	}
	jsonOK(w, response)
}

func (s *Server) registerDialogueRoutes(api huma.API) {
	register(api, huma.Operation{OperationID: "occurrence-dialogue-inspect", Method: http.MethodGet, Path: "/occurrences/{id}/inspect", Errors: []int{401, 404, 500}}, func(ctx context.Context, _ *DialogueOccurrenceInput) (*DialogueJSONOutput, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(s.dialogueInspect), nil)
		return &DialogueJSONOutput{h, b}, e
	})
	register(api, huma.Operation{OperationID: "occurrence-dialogue-override", Method: http.MethodPost, Path: "/occurrences/{id}/override", Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true}, func(ctx context.Context, in *DialogueOverrideInput) (*DialogueJSONOutput, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(s.dialogueOverride), in.Body)
		return &DialogueJSONOutput{h, b}, e
	})
	register(api, huma.Operation{OperationID: "student-occurrence-dialogue", Method: http.MethodPost, Path: "/student/occurrences/{id}/dialogue", Errors: []int{401, 404, 409}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *DialogueOccurrenceInput) (*DialogueJSONOutput, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireStudent(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) { s.studentStart2(w, r, id) }), nil)
		return &DialogueJSONOutput{h, b}, e
	})
	register(api, huma.Operation{OperationID: "student-occurrence-dialogue-state", Method: http.MethodGet, Path: "/student/occurrences/{id}/dialogue", Errors: []int{401, 404, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *DialogueOccurrenceInput) (*DialogueJSONOutput, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireStudent(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) { s.studentDialogueState(w, r, id) }), nil)
		return &DialogueJSONOutput{h, b}, e
	})
}
