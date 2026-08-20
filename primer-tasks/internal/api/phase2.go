package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/domain"
	"primer-tasks/internal/schedule"
	"primer-tasks/internal/verification"
)

type TaskPage2 struct {
	Items      []TaskRevision `json:"items"`
	TotalCount int            `json:"totalCount"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
}
type TaskRevision struct {
	ID           string        `json:"id"`
	RevisionID   string        `json:"revisionId,omitempty"`
	TemplateID   string        `json:"templateId"`
	Version      int           `json:"version"`
	Title        string        `json:"title"`
	Instructions string        `json:"instructions"`
	Status       string        `json:"status"`
	Requirements []Requirement `json:"requirements"`
	CreatedAt    time.Time     `json:"createdAt"`
}
type Requirement struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	ConfigVersion int            `json:"configVersion"`
	Config        map[string]any `json:"config"`
	Interaction   string         `json:"interaction"`
	Executor      string         `json:"executor"`
}
type TaskInput2 struct {
	Title        string        `json:"title"`
	Instructions string        `json:"instructions"`
	Requirements []Requirement `json:"requirements"`
}
type Schedule2 struct {
	ID               string     `json:"id"`
	StudentID        string     `json:"studentId"`
	TemplateID       string     `json:"templateId"`
	RevisionID       string     `json:"revisionId"`
	Kind             string     `json:"kind"`
	Timezone         string     `json:"timezone"`
	StartAt          time.Time  `json:"startAt"`
	EndAt            *time.Time `json:"endAt,omitempty"`
	RRULE            string     `json:"rrule,omitempty"`
	DueOffsetMinutes int        `json:"dueOffsetMinutes"`
	Enabled          bool       `json:"enabled"`
	Version          int        `json:"version"`
}
type ScheduleInput2 struct {
	StudentID        string     `json:"studentId"`
	TemplateID       string     `json:"templateId"`
	RevisionID       string     `json:"revisionId"`
	Kind             string     `json:"kind"`
	Timezone         string     `json:"timezone"`
	StartAt          time.Time  `json:"startAt"`
	EndAt            *time.Time `json:"endAt,omitempty"`
	RRULE            string     `json:"rrule,omitempty"`
	DueOffsetMinutes int        `json:"dueOffsetMinutes"`
}
type Occurrence2 struct {
	ID                  string    `json:"id"`
	StudentID           string    `json:"studentId"`
	ScheduleID          string    `json:"scheduleId"`
	RevisionID          string    `json:"revisionId"`
	Title               string    `json:"title"`
	Instructions        string    `json:"instructions"`
	Status              string    `json:"status"`
	NominalAt           time.Time `json:"nominalAt"`
	DueAt               time.Time `json:"dueAt"`
	Timezone            string    `json:"timezone"`
	DueOffsetMinutes    int       `json:"dueOffsetMinutes"`
	DueSemantics        string    `json:"dueSemantics"`
	TaskRevisionVersion int       `json:"taskRevisionVersion"`
	ScheduleVersion     int       `json:"scheduleVersion"`
	AttemptNumber       int       `json:"attemptNumber"`
}
type OccurrencePage2 struct {
	Items      []Occurrence2 `json:"items"`
	TotalCount int           `json:"totalCount"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}
type DecisionInput2 struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
}
type IDInput2 struct {
	ID string `path:"id"`
}

type taskRow struct{ rev TaskRevision }

func requirementConfigJSON(config map[string]any) []byte { b, _ := json.Marshal(config); return b }
func parsePage(r *http.Request) (int, int) {
	limit, offset := 20, 0
	if n, e := strconv.Atoi(r.URL.Query().Get("limit")); e == nil && n > 0 && n <= 100 {
		limit = n
	}
	if n, e := strconv.Atoi(r.URL.Query().Get("offset")); e == nil && n >= 0 {
		offset = n
	}
	return limit, offset
}
func (s *Server) createTask2(w http.ResponseWriter, r *http.Request, sc scope) {
	var in TaskInput2
	if !decode(w, r, &in) {
		return
	}
	rs := in.Requirements
	if len(rs) == 0 {
		rs = []Requirement{{Kind: "parent_approval", ConfigVersion: 1, Interaction: "parent_action", Executor: "human", Config: map[string]any{}}}
	}
	dr := make([]domain.VerificationRequirement, len(rs))
	for i, x := range rs {
		dr[i] = domain.VerificationRequirement{Kind: x.Kind, ConfigVersion: x.ConfigVersion, Config: x.Config, Interaction: x.Interaction, Executor: x.Executor}
		if x.Kind == domain.AgentDialogueKind {
			if err := domain.ValidateDialogueRequirement(domain.VerificationRequirement{Kind: x.Kind, ConfigVersion: x.ConfigVersion, Config: x.Config, Interaction: x.Interaction, Executor: x.Executor}); err != nil {
				problem(w, 400, "invalid_request", "dialogue requirement configuration is invalid")
				return
			}
		}
		if x.Kind == verification.ExternalCallbackKind && s.validateExternalConfig(r.Context(), x.Config) != nil {
			problem(w, 400, "invalid_request", "external verifier capability or schema is unavailable")
			return
		}
	}
	if errors.Is(domain.ValidateRevision(in.Title, in.Instructions, dr), domain.ErrInvalidTask) {
		problem(w, 400, "invalid_request", "a task requires a title and supported verification requirement")
		return
	}
	ctx := r.Context()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		problem(w, 500, "internal", "unable to create task")
		return
	}
	defer tx.Rollback(ctx)
	tid, rid := uuid.New(), uuid.New()
	if e = tx.QueryRow(ctx, `INSERT INTO task_templates(id,tenant_id,title) VALUES($1,$2,$3) RETURNING id`, tid, sc.Tenant, strings.TrimSpace(in.Title)).Scan(&tid); e != nil {
		problem(w, 409, "conflict", "task could not be created")
		return
	}
	if e = tx.QueryRow(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions) VALUES($1,$2,$3,1,$4,$5) RETURNING created_at`, rid, sc.Tenant, tid, in.Title, in.Instructions).Scan(new(time.Time)); e != nil {
		problem(w, 500, "internal", "revision could not be created")
		return
	}
	for i, x := range rs {
		if _, e = tx.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), sc.Tenant, rid, i, x.Kind, x.ConfigVersion, requirementConfigJSON(x.Config), x.Interaction, x.Executor); e != nil {
			problem(w, 400, "invalid_request", "unsupported verification requirement")
			return
		}
	}
	if e = tx.Commit(ctx); e != nil {
		problem(w, 500, "internal", "unable to commit task")
		return
	}
	jsonStatus(w, map[string]any{"id": rid.String(), "templateId": tid.String(), "version": 1, "title": in.Title, "instructions": in.Instructions, "status": "draft", "requirements": rs}, 201)
}
func (s *Server) listTasks2(w http.ResponseWriter, r *http.Request, sc scope) {
	limit, offset := parsePage(r)
	q := r.URL.Query().Get("q")
	sortCol := "created_at"
	if v := r.URL.Query().Get("sort"); v == "title" {
		sortCol = "title"
	}
	dir := "DESC"
	if strings.EqualFold(r.URL.Query().Get("dir"), "asc") {
		dir = "ASC"
	}
	var total int
	if e := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM task_revisions WHERE tenant_id=$1 AND ($2='' OR title ILIKE '%'||$2||'%')`, sc.Tenant, q).Scan(&total); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	sql := fmt.Sprintf(`SELECT id,template_id,version,title,instructions,status,created_at FROM task_revisions WHERE tenant_id=$1 AND ($2='' OR title ILIKE '%%'||$2||'%%') ORDER BY %s %s LIMIT $3 OFFSET $4`, sortCol, dir)
	rows, e := s.DB.Query(r.Context(), sql, sc.Tenant, q, limit, offset)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer rows.Close()
	out := []TaskRevision{}
	for rows.Next() {
		var x TaskRevision
		if e = rows.Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status, &x.CreatedAt); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		out = append(out, x)
	}
	jsonOK(w, TaskPage2{out, total, limit, offset})
}
func (s *Server) publishTask2(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	rows, queryErr := s.DB.Query(ctx, `SELECT kind,config FROM verification_requirements WHERE tenant_id=$1 AND revision_id=$2`, sc.Tenant, id)
	if queryErr != nil {
		problem(w, 500, "internal", "unable to validate task requirements")
		return
	}
	for rows.Next() {
		var kind string
		var raw []byte
		if err := rows.Scan(&kind, &raw); err != nil {
			rows.Close()
			problem(w, 500, "internal", "unable to validate task requirements")
			return
		}
		if kind == verification.ExternalCallbackKind {
			var config map[string]any
			if json.Unmarshal(raw, &config) != nil || s.validateExternalConfig(ctx, config) != nil {
				rows.Close()
				problem(w, 409, "blocked", "external verifier capability or schema is unavailable")
				return
			}
		}
	}
	rows.Close()
	var x TaskRevision
	e := s.DB.QueryRow(ctx, `UPDATE task_revisions SET status='published',published_at=now() WHERE tenant_id=$1 AND id=$2 AND status='draft' AND EXISTS(SELECT 1 FROM task_templates t WHERE t.tenant_id=task_revisions.tenant_id AND t.id=task_revisions.template_id AND t.status<>'retired') RETURNING id,template_id,version,title,instructions,status,created_at`, sc.Tenant, id).Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status, &x.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "draft revision not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	_, _ = s.DB.Exec(ctx, `UPDATE task_templates SET title=$1,status='published',current_revision=$2 WHERE tenant_id=$3 AND id=$4`, x.Title, x.Version, sc.Tenant, x.TemplateID)
	jsonOK(w, x)
}
func (s *Server) retireTask2(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	var n int
	e := s.DB.QueryRow(r.Context(), `UPDATE task_templates SET status='retired',retired_at=now() WHERE tenant_id=$1 AND id=$2 AND status<>'retired' RETURNING 1`, sc.Tenant, id).Scan(&n)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "task not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) createSchedule2(w http.ResponseWriter, r *http.Request, sc scope) {
	var in ScheduleInput2
	if !decode(w, r, &in) {
		return
	}
	if in.Kind != "one_off" && in.Kind != "recurrence" {
		problem(w, 400, "invalid_request", "schedule kind is invalid")
		return
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		problem(w, 400, "invalid_request", "explicit IANA timezone is required")
		return
	}
	if in.Kind == "recurrence" {
		if _, e := schedule.Parse(in.RRULE, in.Timezone, in.StartAt, in.EndAt); e != nil {
			problem(w, 400, "invalid_request", e.Error())
			return
		}
	}
	id := uuid.New()
	_, e := s.DB.Exec(r.Context(), `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 WHERE EXISTS(SELECT 1 FROM task_revisions r JOIN task_templates t ON t.tenant_id=r.tenant_id AND t.id=r.template_id WHERE r.tenant_id=$2 AND r.id=$5 AND r.template_id=$4 AND r.status='published' AND t.status<>'retired')`, id, sc.Tenant, in.StudentID, in.TemplateID, in.RevisionID, in.Kind, in.Timezone, in.StartAt, in.EndAt, in.RRULE, in.DueOffsetMinutes)
	if e != nil {
		problem(w, 409, "conflict", "schedule references an unavailable task or student")
		return
	}
	if err := s.materializeSchedule(r.Context(), sc.Tenant, id.String(), "request"); err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	jsonStatus(w, map[string]any{"id": id.String(), "studentId": in.StudentID, "templateId": in.TemplateID, "revisionId": in.RevisionID, "kind": in.Kind, "timezone": in.Timezone, "startAt": in.StartAt, "rrule": in.RRULE, "dueOffsetMinutes": in.DueOffsetMinutes, "enabled": true, "version": 1}, 201)
}
func (s *Server) materializeSchedule(ctx context.Context, tenant, id, owner string) error {
	var student, rev, kind, zone, rule string
	var start, end *time.Time
	var due, version int
	if e := s.DB.QueryRow(ctx, `SELECT student_id,revision_id,kind,timezone,rrule,start_local,end_local,due_offset_minutes,version FROM task_schedules WHERE tenant_id=$1 AND id=$2 AND enabled`, tenant, id).Scan(&student, &rev, &kind, &zone, &rule, &start, &end, &due, &version); e != nil {
		return e
	}
	if start == nil {
		return fmt.Errorf("missing start")
	}
	spec, e := schedule.Parse(func() string {
		if kind == "one_off" {
			return ""
		}
		return rule
	}(), zone, *start, end)
	if e != nil {
		return e
	}
	horizonDays := 45
	if n, err := strconv.Atoi(envOr("TASKS_SCHEDULE_HORIZON_DAYS", "45")); err == nil && n > 0 && n <= 730 {
		horizonDays = n
	}
	h := time.Now().Add(time.Duration(horizonDays) * 24 * time.Hour)
	for _, at := range spec.Occurrences(h) {
		oid := uuid.New()
		_, e = s.DB.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,revision_snapshot) SELECT $1,$2,$3,$4,$5,$6::timestamptz,$6::timestamptz+($8::int*interval '1 minute'),jsonb_build_object('revisionId',$5::uuid,'title',(SELECT title FROM task_revisions WHERE tenant_id=$2 AND id=$5),'instructions',(SELECT instructions FROM task_revisions WHERE tenant_id=$2 AND id=$5),'taskRevisionVersion',(SELECT version FROM task_revisions WHERE tenant_id=$2 AND id=$5),'timezone',$7::text,'dueOffsetMinutes',$8::int,'dueSemantics','offset_from_nominal','scheduleVersion',$9::int) ON CONFLICT (tenant_id,schedule_id,nominal_at) DO NOTHING`, oid, tenant, id, student, rev, at, zone, due, version)
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) listOccurrences2(w http.ResponseWriter, r *http.Request, sc scope) {
	limit, offset := parsePage(r)
	q := r.URL.Query().Get("status")
	var total int
	if e := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM task_occurrences WHERE tenant_id=$1 AND ($2='' OR status=$2)`, sc.Tenant, q).Scan(&total); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	rows, e := s.DB.Query(r.Context(), `SELECT o.id,o.student_id,o.schedule_id,o.revision_id,o.revision_snapshot->>'title',o.revision_snapshot->>'instructions',o.status,o.nominal_at,o.due_at,o.revision_snapshot->>'timezone',(o.revision_snapshot->>'dueOffsetMinutes')::int,o.revision_snapshot->>'dueSemantics',(o.revision_snapshot->>'taskRevisionVersion')::int,(o.revision_snapshot->>'scheduleVersion')::int,COALESCE((SELECT max(number) FROM verification_attempts a WHERE a.occurrence_id=o.id),0) FROM task_occurrences o WHERE o.tenant_id=$1 AND ($2='' OR o.status=$2) ORDER BY o.nominal_at LIMIT $3 OFFSET $4`, sc.Tenant, q, limit, offset)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer rows.Close()
	out := []Occurrence2{}
	for rows.Next() {
		var x Occurrence2
		if e = rows.Scan(&x.ID, &x.StudentID, &x.ScheduleID, &x.RevisionID, &x.Title, &x.Instructions, &x.Status, &x.NominalAt, &x.DueAt, &x.Timezone, &x.DueOffsetMinutes, &x.DueSemantics, &x.TaskRevisionVersion, &x.ScheduleVersion, &x.AttemptNumber); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		out = append(out, x)
	}
	jsonOK(w, OccurrencePage2{out, total, limit, offset})
}
func (s *Server) studentOccurrences2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	rows, e := s.DB.Query(r.Context(), `SELECT o.id,o.student_id,o.schedule_id,o.revision_id,o.revision_snapshot->>'title',o.revision_snapshot->>'instructions',o.status,o.nominal_at,o.due_at,o.revision_snapshot->>'timezone',(o.revision_snapshot->>'dueOffsetMinutes')::int,o.revision_snapshot->>'dueSemantics',(o.revision_snapshot->>'taskRevisionVersion')::int,(o.revision_snapshot->>'scheduleVersion')::int,COALESCE((SELECT max(number) FROM verification_attempts a WHERE a.occurrence_id=o.id),0) FROM task_occurrences o WHERE o.student_id=$1 AND o.status<>'canceled' AND o.nominal_at>=now()-interval '1 day' ORDER BY o.nominal_at`, id)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer rows.Close()
	out := []Occurrence2{}
	for rows.Next() {
		var x Occurrence2
		if e = rows.Scan(&x.ID, &x.StudentID, &x.ScheduleID, &x.RevisionID, &x.Title, &x.Instructions, &x.Status, &x.NominalAt, &x.DueAt, &x.Timezone, &x.DueOffsetMinutes, &x.DueSemantics, &x.TaskRevisionVersion, &x.ScheduleVersion, &x.AttemptNumber); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		out = append(out, x)
	}
	jsonOK(w, OccurrencePage2{Items: out, TotalCount: len(out), Limit: 100, Offset: 0})
}
func (s *Server) startOccurrence2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	oid := chi.URLParam(r, "id")
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var tenant, rev string
	if e = tx.QueryRow(r.Context(), `UPDATE task_occurrences SET status='awaiting_verification' WHERE id=$1 AND student_id=$2 AND status IN ('pending','in_progress') RETURNING tenant_id,revision_id`, oid, id).Scan(&tenant, &rev); e != nil {
		problem(w, 404, "not_found", "occurrence unavailable")
		return
	}
	var req, reqKind string
	var rawConfig []byte
	if e = tx.QueryRow(r.Context(), `SELECT id,kind,config FROM verification_requirements WHERE tenant_id=$1 AND revision_id=$2 ORDER BY ordinal LIMIT 1`, tenant, rev).Scan(&req, &reqKind, &rawConfig); e != nil {
		problem(w, 409, "blocked", "verification requirement unavailable")
		return
	}
	attemptID := uuid.New()
	_, e = tx.Exec(r.Context(), `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1) ON CONFLICT DO NOTHING`, attemptID, tenant, oid, req)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if reqKind == domain.AgentDialogueKind {
		var raw map[string]any
		if e = json.Unmarshal(rawConfig, &raw); e != nil {
			problem(w, 409, "blocked", "dialogue requirement configuration is invalid")
			return
		}
		config, parseErr := domain.ParseDialogueConfig(raw)
		if parseErr != nil {
			problem(w, 409, "blocked", "dialogue requirement configuration is invalid")
			return
		}
		snapshot, snapshotErr := domain.SnapshotDialogueConfig(config)
		if snapshotErr != nil {
			problem(w, 409, "blocked", "dialogue requirement configuration is invalid")
			return
		}
		snapshotJSON, marshalErr := json.Marshal(snapshot)
		if marshalErr != nil {
			problem(w, 500, "internal", marshalErr.Error())
			return
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,next_sequence) VALUES($1,$2,$3,$4,'dialogue.v1',$5,1) ON CONFLICT (tenant_id,attempt_id) DO NOTHING`, tenant, attemptID, oid, req, snapshotJSON)
		if e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
	}
	if reqKind == verification.ExternalCallbackKind {
		config, configErr := externalConfig(rawConfig)
		if configErr != nil || snapshotExternalAttempt(r.Context(), tx, tenant, attemptID.String(), req, config) != nil {
			problem(w, 409, "blocked", "external verifier capability is unavailable")
			return
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, map[string]string{"id": oid, "status": "awaiting_verification"})
}
func (s *Server) decideOccurrence2(w http.ResponseWriter, r *http.Request, sc scope) {
	oid := chi.URLParam(r, "id")
	var in DecisionInput2
	if !decode(w, r, &in) {
		return
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var tenant, attempt string
	if e = tx.QueryRow(r.Context(), `SELECT tenant_id,id FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2 ORDER BY number DESC LIMIT 1 FOR UPDATE`, sc.Tenant, oid).Scan(&tenant, &attempt); e != nil {
		problem(w, 409, "blocked", "student must start the occurrence first")
		return
	}
	var decisionID string
	var accepted bool
	e = tx.QueryRow(r.Context(), `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,attempt_id) DO NOTHING RETURNING id,accepted`, uuid.New(), sc.Tenant, attempt, in.Accepted, in.Reason, sc.Subject).Scan(&decisionID, &accepted)
	if errors.Is(e, pgx.ErrNoRows) {
		e = tx.QueryRow(r.Context(), `SELECT id,accepted FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, sc.Tenant, attempt).Scan(&decisionID, &accepted)
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	status := "pending"
	if accepted {
		status = "completed"
	}
	attemptStatus := "rejected"
	if accepted {
		attemptStatus = "accepted"
	}
	if _, e = tx.Exec(r.Context(), `UPDATE verification_attempts SET status=$1 WHERE tenant_id=$2 AND id=$3`, attemptStatus, sc.Tenant, attempt); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE task_occurrences SET status=$1 WHERE tenant_id=$2 AND id=$3 AND status<>'canceled'`, status, sc.Tenant, oid); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, map[string]any{"occurrenceId": oid, "decisionId": decisionID, "accepted": accepted, "status": status})
}
func (s *Server) retryOccurrence2(w http.ResponseWriter, r *http.Request, sc scope) {
	oid := chi.URLParam(r, "id")
	var occurrenceStatus string
	if e := s.DB.QueryRow(r.Context(), `SELECT status FROM task_occurrences WHERE tenant_id=$1 AND id=$2`, sc.Tenant, oid).Scan(&occurrenceStatus); e != nil {
		problem(w, 404, "not_found", "occurrence not found")
		return
	}
	if occurrenceStatus != "pending" {
		problem(w, 409, "conflict", "retry requires a pending occurrence")
		return
	}
	var latestStatus string
	if e := s.DB.QueryRow(r.Context(), `SELECT status FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2 ORDER BY number DESC LIMIT 1`, sc.Tenant, oid).Scan(&latestStatus); e != nil || (latestStatus != "rejected" && latestStatus != "exhausted") {
		problem(w, 409, "conflict", "retry requires a rejected attempt")
		return
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var previousNumber int
	var req, kind string
	var snapshot []byte
	e = tx.QueryRow(r.Context(), `SELECT a.number,a.requirement_id,r.kind,d.config_snapshot
		FROM verification_attempts a
		JOIN verification_requirements r ON r.tenant_id=a.tenant_id AND r.id=a.requirement_id
		LEFT JOIN dialogue_attempts d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id
		WHERE a.tenant_id=$1 AND a.occurrence_id=$2
		ORDER BY a.number DESC LIMIT 1 FOR UPDATE OF a`, sc.Tenant, oid).Scan(&previousNumber, &req, &kind, &snapshot)
	if e != nil {
		problem(w, 404, "not_found", "occurrence not found")
		return
	}
	n := previousNumber + 1
	var previousExternalAttempt string
	if kind == verification.ExternalCallbackKind {
		if err := s.DB.QueryRow(r.Context(), `SELECT id FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2 ORDER BY number DESC LIMIT 1`, sc.Tenant, oid).Scan(&previousExternalAttempt); err != nil {
			problem(w, 409, "blocked", "external verifier snapshot is unavailable")
			return
		}
	}
	if kind == domain.AgentDialogueKind {
		var raw map[string]any
		if len(snapshot) == 0 || json.Unmarshal(snapshot, &raw) != nil {
			problem(w, 409, "blocked", "dialogue policy snapshot is unavailable")
			return
		}
		config, parseErr := domain.ParseDialogueConfig(raw)
		if parseErr != nil {
			problem(w, 409, "blocked", "dialogue policy snapshot is invalid")
			return
		}
		if n > config.MaxAttempts {
			problem(w, 409, "conflict", "dialogue attempt limit reached")
			return
		}
	}
	attemptID := uuid.New()
	if _, e = tx.Exec(r.Context(), `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,$5)`, attemptID, sc.Tenant, oid, req, n); e != nil {
		problem(w, 409, "conflict", "retry is not permitted")
		return
	}
	if kind == domain.AgentDialogueKind {
		if _, e = tx.Exec(r.Context(), `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,next_sequence) VALUES($1,$2,$3,$4,'dialogue.v1',$5,1)`, sc.Tenant, attemptID, oid, req, snapshot); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
	}
	if kind == verification.ExternalCallbackKind {
		if _, e = tx.Exec(r.Context(), `INSERT INTO external_verifier_attempts(tenant_id,attempt_id,verifier_id,capability,schema_version,public_options,manifest_snapshot) SELECT tenant_id,$1,verifier_id,capability,schema_version,public_options,manifest_snapshot FROM external_verifier_attempts WHERE tenant_id=$2 AND attempt_id=$3`, attemptID, sc.Tenant, previousExternalAttempt); e != nil {
			problem(w, 500, "internal", "external verifier snapshot could not be copied")
			return
		}
	}
	if _, e = tx.Exec(r.Context(), `UPDATE task_occurrences SET status='awaiting_verification' WHERE tenant_id=$1 AND id=$2 AND status='pending'`, sc.Tenant, oid); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, map[string]any{"occurrenceId": oid, "attemptNumber": n, "status": "awaiting_verification"})
}
func (s *Server) setOccurrenceStatus2(w http.ResponseWriter, r *http.Request, sc scope, status string) {
	oid := chi.URLParam(r, "id")
	var n int
	e := s.DB.QueryRow(r.Context(), `UPDATE task_occurrences SET status=$1 WHERE tenant_id=$2 AND id=$3 AND status NOT IN ('completed','canceled') RETURNING 1`, status, sc.Tenant, oid).Scan(&n)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 409, "conflict", "occurrence is terminal or unavailable")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, map[string]string{"id": oid, "status": status})
}
func (s *Server) parentGetOccurrence2(w http.ResponseWriter, r *http.Request, sc scope) {
	var x Occurrence2
	e := s.DB.QueryRow(r.Context(), `SELECT o.id,o.student_id,o.schedule_id,o.revision_id,o.revision_snapshot->>'title',o.revision_snapshot->>'instructions',o.status,o.nominal_at,o.due_at,o.revision_snapshot->>'timezone',(o.revision_snapshot->>'dueOffsetMinutes')::int,o.revision_snapshot->>'dueSemantics',(o.revision_snapshot->>'taskRevisionVersion')::int,(o.revision_snapshot->>'scheduleVersion')::int,COALESCE((SELECT max(number) FROM verification_attempts a WHERE a.occurrence_id=o.id),0) FROM task_occurrences o WHERE o.tenant_id=$1 AND o.id=$2`, sc.Tenant, chi.URLParam(r, "id")).Scan(&x.ID, &x.StudentID, &x.ScheduleID, &x.RevisionID, &x.Title, &x.Instructions, &x.Status, &x.NominalAt, &x.DueAt, &x.Timezone, &x.DueOffsetMinutes, &x.DueSemantics, &x.TaskRevisionVersion, &x.ScheduleVersion, &x.AttemptNumber)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "occurrence not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, x)
}
func (s *Server) studentDetail2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var x Occurrence2
	e := s.DB.QueryRow(r.Context(), `SELECT o.id,o.student_id,o.schedule_id,o.revision_id,o.revision_snapshot->>'title',o.revision_snapshot->>'instructions',o.status,o.nominal_at,o.due_at,o.revision_snapshot->>'timezone',(o.revision_snapshot->>'dueOffsetMinutes')::int,o.revision_snapshot->>'dueSemantics',(o.revision_snapshot->>'taskRevisionVersion')::int,(o.revision_snapshot->>'scheduleVersion')::int,COALESCE((SELECT max(number) FROM verification_attempts a WHERE a.occurrence_id=o.id),0) FROM task_occurrences o WHERE o.student_id=$1 AND o.id=$2`, id, chi.URLParam(r, "id")).Scan(&x.ID, &x.StudentID, &x.ScheduleID, &x.RevisionID, &x.Title, &x.Instructions, &x.Status, &x.NominalAt, &x.DueAt, &x.Timezone, &x.DueOffsetMinutes, &x.DueSemantics, &x.TaskRevisionVersion, &x.ScheduleVersion, &x.AttemptNumber)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "occurrence not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, x)
}
func (s *Server) studentStart2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.startOccurrence2(w, r, id)
}
func (s *Server) studentListWrapper(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.studentOccurrences2(w, r, id)
}
func (s *Server) deviceListWrapper(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.studentOccurrences2(w, r, id)
}
func (s *Server) deviceDetailWrapper(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.studentDetail2(w, r, id)
}
func (s *Server) deviceStartWrapper(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.startOccurrence2(w, r, id)
}

// reviseTask2 appends a revision; it never mutates the published revision.
func (s *Server) reviseTask2(w http.ResponseWriter, r *http.Request, sc scope) {
	var in TaskInput2
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Requirements) == 0 {
		problem(w, 400, "invalid_request", "title and requirements are required")
		return
	}
	for _, req := range in.Requirements {
		if req.Kind != "parent_approval" || req.ConfigVersion != 1 {
			problem(w, 400, "invalid_request", "unsupported verification requirement")
			return
		}
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 500, "internal", "unable to revise task")
		return
	}
	defer tx.Rollback(ctx)
	var templateID uuid.UUID
	var version int
	if err = tx.QueryRow(ctx, `SELECT r.template_id,COALESCE(max(r.version),0)+1 FROM task_revisions r JOIN task_templates t ON t.tenant_id=r.tenant_id AND t.id=r.template_id WHERE r.tenant_id=$1 AND r.template_id=$2 AND t.status<>'retired' GROUP BY r.template_id`, sc.Tenant, chi.URLParam(r, "id")).Scan(&templateID, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, 404, "not_found", "task not found")
		} else {
			problem(w, 500, "internal", err.Error())
		}
		return
	}
	rid := uuid.New()
	var created time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, rid, sc.Tenant, templateID, version, strings.TrimSpace(in.Title), in.Instructions).Scan(&created); err != nil {
		problem(w, 409, "conflict", "revision could not be created")
		return
	}
	for i, req := range in.Requirements {
		if _, err = tx.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), sc.Tenant, rid, i, req.Kind, req.ConfigVersion, requirementConfigJSON(req.Config), req.Interaction, req.Executor); err != nil {
			problem(w, 400, "invalid_request", "unsupported verification requirement")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 500, "internal", "unable to commit revision")
		return
	}
	jsonStatus(w, TaskRevision{ID: rid.String(), TemplateID: templateID.String(), Version: version, Title: strings.TrimSpace(in.Title), Instructions: in.Instructions, Status: "draft", Requirements: in.Requirements, CreatedAt: created}, 201)
}

func (s *Server) listSchedules2(w http.ResponseWriter, r *http.Request, sc scope) {
	limit, offset := parsePage(r)
	all := r.URL.Query().Get("status") == "all"
	var total int
	if err := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM task_schedules WHERE tenant_id=$1 AND ($2 OR enabled)`, sc.Tenant, all).Scan(&total); err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes,enabled,version FROM task_schedules WHERE tenant_id=$1 AND ($2 OR enabled) ORDER BY start_local LIMIT $3 OFFSET $4`, sc.Tenant, all, limit, offset)
	if err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	items := []Schedule2{}
	for rows.Next() {
		var x Schedule2
		if err := rows.Scan(&x.ID, &x.StudentID, &x.TemplateID, &x.RevisionID, &x.Kind, &x.Timezone, &x.StartAt, &x.EndAt, &x.RRULE, &x.DueOffsetMinutes, &x.Enabled, &x.Version); err != nil {
			problem(w, 500, "internal", err.Error())
			return
		}
		items = append(items, x)
	}
	jsonOK(w, map[string]any{"items": items, "totalCount": total, "limit": limit, "offset": offset})
}

func (s *Server) updateSchedule2(w http.ResponseWriter, r *http.Request, sc scope) {
	var in ScheduleInput2
	if !decode(w, r, &in) {
		return
	}
	if in.Kind != "one_off" && in.Kind != "recurrence" {
		problem(w, 400, "invalid_request", "schedule kind is invalid")
		return
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		problem(w, 400, "invalid_request", "explicit IANA timezone is required")
		return
	}
	if in.Kind == "recurrence" {
		if _, err := schedule.Parse(in.RRULE, in.Timezone, in.StartAt, in.EndAt); err != nil {
			problem(w, 400, "invalid_request", err.Error())
			return
		}
	}
	var x Schedule2
	var valid bool
	if e := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM task_revisions r JOIN task_templates t ON t.tenant_id=r.tenant_id AND t.id=r.template_id WHERE r.tenant_id=$1 AND r.id=$2 AND r.template_id=$3 AND r.status='published' AND t.status<>'retired')`, sc.Tenant, in.RevisionID, in.TemplateID).Scan(&valid); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if !valid {
		problem(w, 409, "conflict", "schedule references an unavailable task revision")
		return
	}
	err := s.DB.QueryRow(r.Context(), `UPDATE task_schedules SET student_id=$1,template_id=$2,revision_id=$3,kind=$4,timezone=$5,start_local=$6,end_local=$7,rrule=$8,due_offset_minutes=$9,version=version+1 WHERE tenant_id=$10 AND id=$11 AND enabled RETURNING id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes,enabled,version`, in.StudentID, in.TemplateID, in.RevisionID, in.Kind, in.Timezone, in.StartAt, in.EndAt, in.RRULE, in.DueOffsetMinutes, sc.Tenant, chi.URLParam(r, "id")).Scan(&x.ID, &x.StudentID, &x.TemplateID, &x.RevisionID, &x.Kind, &x.Timezone, &x.StartAt, &x.EndAt, &x.RRULE, &x.DueOffsetMinutes, &x.Enabled, &x.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "schedule not found")
		return
	}
	if err != nil {
		problem(w, 409, "conflict", "schedule could not be updated")
		return
	}
	if err := s.materializeSchedule(r.Context(), sc.Tenant, x.ID, "update"); err != nil {
		problem(w, 500, "internal", "schedule was updated but could not be materialized")
		return
	}
	jsonOK(w, x)
}

func (s *Server) retireSchedule2(w http.ResponseWriter, r *http.Request, sc scope) {
	res, err := s.DB.Exec(r.Context(), `UPDATE task_schedules SET enabled=false,retired_at=now(),version=version+1 WHERE tenant_id=$1 AND id=$2 AND enabled`, sc.Tenant, chi.URLParam(r, "id"))
	if err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		problem(w, 404, "not_found", "schedule not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
