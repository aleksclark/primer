package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	rows, err := s.DB.Query(r.Context(), `SELECT m.role,m.content,m.sequence,m.created_at,COALESCE(q.id::text,''),COALESCE(e.message_id::text,''),e.accepted,COALESCE(e.rationale,''),COALESCE(e.provider,''),COALESCE(e.model,''),COALESCE(e.policy_version,''),COALESCE(e.usage,'{}'::jsonb) FROM verification_messages m JOIN verification_attempts a ON a.tenant_id=m.tenant_id AND a.id=m.attempt_id LEFT JOIN dialogue_questions q ON q.tenant_id=a.tenant_id AND q.attempt_id=a.id LEFT JOIN verification_evaluations e ON e.tenant_id=m.tenant_id AND e.message_id=m.id WHERE m.tenant_id=$1 AND a.occurrence_id=$2 ORDER BY m.sequence,e.created_at`, sc.Tenant, id)
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
	jsonOK(w, map[string]any{"occurrenceId": occurrence, "studentId": student, "status": status, "timeline": items})
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
}
