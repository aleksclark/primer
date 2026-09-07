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
	"primer-tasks/internal/repo"
	"primer-tasks/internal/schedule"
	"primer-tasks/internal/verification"
)

type TaskPage2 struct {
	Items      []TaskRevision `json:"items" nullable:"false"`
	TotalCount int            `json:"totalCount"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
}
type TaskRevision struct {
	ID             string        `json:"id"`
	RevisionID     string        `json:"revisionId,omitempty"`
	TemplateID     string        `json:"templateId"`
	Version        int           `json:"version"`
	Title          string        `json:"title"`
	Instructions   string        `json:"instructions"`
	Status         string        `json:"status"`
	TemplateStatus string        `json:"templateStatus,omitempty"`
	Requirements   []Requirement `json:"requirements"`
	CreatedAt      time.Time     `json:"createdAt"`
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
type SchedulePage2 struct {
	Items      []Schedule2 `json:"items" nullable:"false"`
	TotalCount int         `json:"totalCount"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}
type Schedule2 struct {
	Title            string     `json:"title,omitempty"`
	StudentName      string     `json:"studentName,omitempty"`
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
// StudentRequirement is the student-safe occurrence summary. It names the
// issued requirement without config, source, rubric, or other payloads.
type StudentRequirement struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ConfigVersion int    `json:"configVersion"`
	Interaction   string `json:"interaction"`
	Executor      string `json:"executor"`
}

type Occurrence2 struct {
	StudentName         string               `json:"studentName,omitempty"`
	ID                  string               `json:"id"`
	StudentID           string               `json:"studentId"`
	ScheduleID          string               `json:"scheduleId"`
	RevisionID          string               `json:"revisionId"`
	Title               string               `json:"title"`
	Instructions        string               `json:"instructions"`
	Status              string               `json:"status"`
	NominalAt           time.Time            `json:"nominalAt"`
	DueAt               time.Time            `json:"dueAt"`
	Timezone            string               `json:"timezone"`
	DueOffsetMinutes    int                  `json:"dueOffsetMinutes"`
	DueSemantics        string               `json:"dueSemantics"`
	TaskRevisionVersion int                  `json:"taskRevisionVersion"`
	ScheduleVersion     int                  `json:"scheduleVersion"`
	AttemptNumber       int                  `json:"attemptNumber"`
	Requirements        []StudentRequirement `json:"requirements,omitempty" nullable:"false"`
	StudentCapability   string               `json:"studentCapability,omitempty" enum:"parent_approval,unsupported"`
}
type OccurrencePage2 struct {
	Items      []Occurrence2 `json:"items" nullable:"false"`
	TotalCount int           `json:"totalCount"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}
type DecisionInput2 struct {
	Accepted      bool   `json:"accepted"`
	Reason        string `json:"reason"`
	RequirementID string `json:"requirementId,omitempty"`
}

type OccurrenceAction2 struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type OccurrenceDecision2 struct {
	OccurrenceID string `json:"occurrenceId"`
	DecisionID   string `json:"decisionId"`
	Accepted     bool   `json:"accepted"`
	Status       string `json:"status"`
}

type OccurrenceRetry2 struct {
	OccurrenceID      string `json:"occurrenceId"`
	RequirementID     string `json:"requirementId"`
	PreviousAttemptID string `json:"previousAttemptId"`
	AttemptNumber     int    `json:"attemptNumber"`
	Status            string `json:"status"`
}

type IDInput2 struct {
	ID string `path:"id"`
}

type taskRow struct{ rev TaskRevision }

func requirementJSON(rs []Requirement) []byte { b, _ := json.Marshal(rs); return b }

func studentCapabilityFor(reqs []StudentRequirement) string {
	if len(reqs) == 1 &&
		reqs[0].Kind == "parent_approval" &&
		reqs[0].ConfigVersion == 1 &&
		reqs[0].Interaction == "parent_action" &&
		reqs[0].Executor == "human" {
		return "parent_approval"
	}
	return "unsupported"
}

func (s *Server) loadStudentRequirements(ctx context.Context, revisionID string) ([]StudentRequirement, error) {
	rows, e := s.DB.Query(ctx, `SELECT id::text,kind,config_version,interaction,executor FROM verification_requirements WHERE revision_id=$1 ORDER BY ordinal,id`, revisionID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []StudentRequirement{}
	for rows.Next() {
		var req StudentRequirement
		if e = rows.Scan(&req.ID, &req.Kind, &req.ConfigVersion, &req.Interaction, &req.Executor); e != nil {
			return nil, e
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (s *Server) attachStudentRequirements(ctx context.Context, items ...*Occurrence2) error {
	cache := map[string][]StudentRequirement{}
	for _, item := range items {
		if item == nil || item.RevisionID == "" {
			continue
		}
		reqs, ok := cache[item.RevisionID]
		if !ok {
			loaded, e := s.loadStudentRequirements(ctx, item.RevisionID)
			if e != nil {
				return e
			}
			reqs = loaded
			cache[item.RevisionID] = reqs
		}
		item.Requirements = reqs
		item.StudentCapability = studentCapabilityFor(reqs)
	}
	return nil
}

func parentApprovalRequirement(reqs []StudentRequirement) (StudentRequirement, bool) {
	if studentCapabilityFor(reqs) != "parent_approval" {
		return StudentRequirement{}, false
	}
	return reqs[0], true
}
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
	var created time.Time
	if e = tx.QueryRow(ctx, `INSERT INTO task_templates(id,tenant_id,title) VALUES($1,$2,$3) RETURNING id`, tid, sc.Tenant, strings.TrimSpace(in.Title)).Scan(&tid); e != nil {
		problem(w, 409, "conflict", "task could not be created")
		return
	}
	if e = tx.QueryRow(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions) VALUES($1,$2,$3,1,$4,$5) RETURNING created_at`, rid, sc.Tenant, tid, in.Title, in.Instructions).Scan(&created); e != nil {
		problem(w, 500, "internal", "revision could not be created")
		return
	}
	for i, x := range rs {
		if _, e = tx.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), sc.Tenant, rid, i, x.Kind, x.ConfigVersion, requirementJSON([]Requirement{x}), x.Interaction, x.Executor); e != nil {
			problem(w, 400, "invalid_request", "unsupported verification requirement")
			return
		}
	}
	if e = tx.Commit(ctx); e != nil {
		problem(w, 500, "internal", "unable to commit task")
		return
	}
	jsonStatus(w, TaskRevision{ID: rid.String(), TemplateID: tid.String(), Version: 1, Title: in.Title, Instructions: in.Instructions, Status: "draft", Requirements: rs, CreatedAt: created}, 201)
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
	// Select one revision per template BEFORE search and pagination. The default
	// remains the revision collection for existing consumers. Published pickers
	// select the newest published revision even when a newer draft exists.
	status := r.URL.Query().Get("status")
	base := `SELECT r.*,t.status AS template_status FROM task_revisions r JOIN task_templates t ON t.tenant_id=r.tenant_id AND t.id=r.template_id WHERE r.tenant_id=$1 AND ($3='' OR $3='all' OR ($3='active' AND t.status<>'retired') OR ($3='published' AND r.status='published' AND t.status<>'retired'))`
	if r.URL.Query().Get("view") == "templates" {
		base = `SELECT DISTINCT ON (template_id) * FROM (` + base + `) revisions ORDER BY template_id,version DESC`
	}
	filtered := ` FROM (` + base + `) tasks WHERE ($2='' OR title ILIKE '%'||$2||'%')`
	var total int
	if e := s.DB.QueryRow(r.Context(), `SELECT count(*)`+filtered, sc.Tenant, q, status).Scan(&total); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	sql := `SELECT id,template_id,version,title,instructions,status,created_at,template_status,COALESCE((SELECT jsonb_agg(v.config->0 ORDER BY v.ordinal) FROM verification_requirements v WHERE v.tenant_id=tasks.tenant_id AND v.revision_id=tasks.id),'[]'::jsonb)` + filtered + fmt.Sprintf(` ORDER BY %s %s,id LIMIT $4 OFFSET $5`, sortCol, dir)
	rows, e := s.DB.Query(r.Context(), sql, sc.Tenant, q, status, limit, offset)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer rows.Close()
	out := []TaskRevision{}
	for rows.Next() {
		var x TaskRevision
		if e = rows.Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status, &x.CreatedAt, &x.TemplateStatus, &x.Requirements); e != nil {
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
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		problem(w, 500, "internal", "unable to publish task")
		return
	}
	defer tx.Rollback(ctx)
	var x TaskRevision
	e = tx.QueryRow(ctx, `UPDATE task_revisions SET status='published',published_at=now() WHERE tenant_id=$1 AND id=$2 AND status='draft' AND EXISTS(SELECT 1 FROM task_templates t WHERE t.tenant_id=task_revisions.tenant_id AND t.id=task_revisions.template_id AND t.status<>'retired') RETURNING id,template_id,version,title,instructions,status,created_at`, sc.Tenant, id).Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status, &x.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "draft revision not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	// Validate and resolve source from the stored canonical requirement array
	// in this same publication transaction. Failure rolls back the draft->
	// published transition; no client-authored snapshot is trusted.
	if e = repo.NewDialogueRepository(tx).PublishRevisionPolicies(ctx, sc.Tenant, id); e != nil {
		if errors.Is(e, domain.ErrInvalidDialogueConfig) || errors.Is(e, domain.ErrDialogueSourceMissing) {
			problem(w, 400, "invalid_request", "verification requirement configuration is invalid")
		} else {
			problem(w, 500, "internal", "unable to snapshot verification policy")
		}
		return
	}
	if _, e = tx.Exec(ctx, `UPDATE task_templates SET title=$1,status='published',current_revision=$2 WHERE tenant_id=$3 AND id=$4`, x.Title, x.Version, sc.Tenant, x.TemplateID); e != nil {
		problem(w, 500, "internal", "unable to publish task")
		return
	}
	if e = tx.Commit(ctx); e != nil {
		problem(w, 500, "internal", "unable to publish task")
		return
	}
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
	inserted, e := s.DB.Exec(r.Context(), `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 WHERE EXISTS(SELECT 1 FROM task_revisions r JOIN task_templates t ON t.tenant_id=r.tenant_id AND t.id=r.template_id WHERE r.tenant_id=$2 AND r.id=$5 AND r.template_id=$4 AND r.status='published' AND t.status<>'retired')`, id, sc.Tenant, in.StudentID, in.TemplateID, in.RevisionID, in.Kind, in.Timezone, in.StartAt, in.EndAt, in.RRULE, in.DueOffsetMinutes)
	if e != nil || inserted.RowsAffected() != 1 {
		problem(w, 409, "conflict", "schedule references an unavailable task or student")
		return
	}
	if err := s.materializeSchedule(r.Context(), sc.Tenant, id.String(), "request"); err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	jsonStatus(w, Schedule2{ID: id.String(), StudentID: in.StudentID, TemplateID: in.TemplateID, RevisionID: in.RevisionID, Kind: in.Kind, Timezone: in.Timezone, StartAt: in.StartAt, EndAt: in.EndAt, RRULE: in.RRULE, DueOffsetMinutes: in.DueOffsetMinutes, Enabled: true, Version: 1}, 201)
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
	dir := "ASC"
	if r.URL.Query().Get("dir") == "desc" {
		dir = "DESC"
	}
	rows, e := s.DB.Query(r.Context(), `SELECT o.id,o.student_id,o.schedule_id,o.revision_id,o.revision_snapshot->>'title',o.revision_snapshot->>'instructions',o.status,o.nominal_at,o.due_at,o.revision_snapshot->>'timezone',(o.revision_snapshot->>'dueOffsetMinutes')::int,o.revision_snapshot->>'dueSemantics',(o.revision_snapshot->>'taskRevisionVersion')::int,(o.revision_snapshot->>'scheduleVersion')::int,COALESCE((SELECT max(number) FROM verification_attempts a WHERE a.occurrence_id=o.id),0),st.display_name FROM task_occurrences o JOIN students st ON st.tenant_id=o.tenant_id AND st.id=o.student_id WHERE o.tenant_id=$1 AND ($2='' OR o.status=$2) ORDER BY o.nominal_at `+dir+`,o.id LIMIT $3 OFFSET $4`, sc.Tenant, q, limit, offset)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer rows.Close()
	out := []Occurrence2{}
	for rows.Next() {
		var x Occurrence2
		if e = rows.Scan(&x.ID, &x.StudentID, &x.ScheduleID, &x.RevisionID, &x.Title, &x.Instructions, &x.Status, &x.NominalAt, &x.DueAt, &x.Timezone, &x.DueOffsetMinutes, &x.DueSemantics, &x.TaskRevisionVersion, &x.ScheduleVersion, &x.AttemptNumber, &x.StudentName); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		out = append(out, x)
	}
	ptrs := make([]*Occurrence2, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	if e = s.attachStudentRequirements(r.Context(), ptrs...); e != nil {
		problem(w, 500, "internal", e.Error())
		return
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
	ptrs := make([]*Occurrence2, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	if e = s.attachStudentRequirements(r.Context(), ptrs...); e != nil {
		problem(w, 500, "internal", e.Error())
		return
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
	var current string
	if e = tx.QueryRow(r.Context(), `SELECT status FROM task_occurrences WHERE id=$1 AND student_id=$2 FOR UPDATE`, oid, id).Scan(&current); e != nil {
		problem(w, 404, "not_found", "occurrence unavailable")
		return
	}
	if current == string(domain.OccurrenceInProgress) {
		if e = tx.Commit(r.Context()); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		jsonOK(w, OccurrenceAction2{ID: oid, Status: current})
		return
	}
	if !domain.CanTransition(domain.OccurrenceStatus(current), domain.OccurrenceInProgress) {
		problem(w, 409, "conflict", "occurrence cannot be started")
		return
	}
	var n int
	if e = tx.QueryRow(r.Context(), `UPDATE task_occurrences SET status='in_progress' WHERE id=$1 AND student_id=$2 AND status=$3 RETURNING 1`, oid, id, current).Scan(&n); e != nil {
		problem(w, 409, "conflict", "occurrence changed concurrently")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, OccurrenceAction2{ID: oid, Status: "in_progress"})
}

func (s *Server) submitOccurrence2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	oid := chi.URLParam(r, "id")
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var tenant, rev, current string
	if e = tx.QueryRow(r.Context(), `SELECT tenant_id,revision_id,status FROM task_occurrences WHERE id=$1 AND student_id=$2 FOR UPDATE`, oid, id).Scan(&tenant, &rev, &current); e != nil {
		problem(w, 404, "not_found", "occurrence unavailable")
		return
	}
	if current == string(domain.OccurrenceAwaitingVerification) {
		if e = tx.Commit(r.Context()); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		jsonOK(w, OccurrenceAction2{ID: oid, Status: current})
		return
	}
	if !domain.CanTransition(domain.OccurrenceStatus(current), domain.OccurrenceAwaitingVerification) {
		problem(w, 409, "conflict", "occurrence cannot be submitted")
		return
	}
	var n int
	if e = tx.QueryRow(r.Context(), `UPDATE task_occurrences SET status='awaiting_verification' WHERE id=$1 AND student_id=$2 AND status=$3 RETURNING 1`, oid, id, current).Scan(&n); e != nil {
		problem(w, 409, "conflict", "occurrence changed concurrently")
		return
	}
	reqs, e := s.loadStudentRequirements(r.Context(), rev)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	manual, ok := parentApprovalRequirement(reqs)
	if !ok {
		problem(w, 409, "unsupported_task", "this assigned work cannot be submitted as parent approval")
		return
	}
	req := manual.ID
	var next int
	if e = tx.QueryRow(r.Context(), `SELECT COALESCE(max(number),0)+1 FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2`, tenant, oid).Scan(&next); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,$5)`, uuid.New(), tenant, oid, req, next); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, OccurrenceAction2{ID: oid, Status: "awaiting_verification"})
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
	var current string
	if e = tx.QueryRow(r.Context(), `SELECT status FROM task_occurrences WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, sc.Tenant, oid).Scan(&current); e != nil {
		problem(w, 404, "not_found", "occurrence not found")
		return
	}
	rows, e := tx.Query(r.Context(), `SELECT vr.id FROM verification_requirements vr JOIN task_occurrences o ON o.tenant_id=vr.tenant_id AND o.revision_id=vr.revision_id WHERE o.tenant_id=$1 AND o.id=$2 AND vr.kind='parent_approval' ORDER BY vr.ordinal`, sc.Tenant, oid)
	if e != nil {
		problem(w, 500, "internal", "unable to read manual requirements")
		return
	}
	var manual []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		manual = append(manual, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		problem(w, 500, "internal", "unable to read manual requirements")
		return
	}
	requirement := in.RequirementID
	if requirement == "" && len(manual) == 1 {
		requirement = manual[0]
	}
	found := false
	for _, id := range manual {
		found = found || id == requirement
	}
	if !found {
		problem(w, 409, "conflict", "select a manual requirement; dialogue requires its evidence or a separate audited override")
		return
	}
	var attempt, attemptStatus string
	e = tx.QueryRow(r.Context(), `SELECT id,status FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2 AND requirement_id=$3 ORDER BY number DESC LIMIT 1 FOR UPDATE`, sc.Tenant, oid, requirement).Scan(&attempt, &attemptStatus)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		problem(w, 500, "internal", e.Error())
		return
	}
	if !errors.Is(e, pgx.ErrNoRows) && attemptStatus != "open" {
		var decisionID string
		var accepted bool
		if lookup := tx.QueryRow(r.Context(), `SELECT id,accepted FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, sc.Tenant, attempt).Scan(&decisionID, &accepted); lookup != nil {
			problem(w, 409, "conflict", "occurrence is not awaiting a parent decision")
			return
		}
		if e = tx.Commit(r.Context()); e != nil {
			problem(w, 500, "internal", e.Error())
			return
		}
		jsonOK(w, OccurrenceDecision2{OccurrenceID: oid, DecisionID: decisionID, Accepted: accepted, Status: current})
		return
	}
	if current != string(domain.DecisionExpectedStatus()) || errors.Is(e, pgx.ErrNoRows) || attemptStatus != "open" {
		problem(w, 409, "conflict", "occurrence is not awaiting a parent decision")
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
	next := domain.OccurrencePending
	attemptNext := "rejected"
	if accepted {
		allAccepted, policyErr := verification.RequirementPolicySatisfied(r.Context(), tx, sc.Tenant, oid)
		if policyErr != nil {
			problem(w, 500, "internal", "unable to evaluate issued verification policy")
			return
		}
		next = domain.OccurrenceAwaitingVerification
		if allAccepted {
			next = domain.OccurrenceCompleted
		}
		attemptNext = "accepted"
	}
	if !domain.CanTransition(domain.OccurrenceStatus(current), next) {
		problem(w, 409, "conflict", "occurrence cannot accept this decision")
		return
	}
	var n int
	if e = tx.QueryRow(r.Context(), `UPDATE task_occurrences SET status=$1 WHERE tenant_id=$2 AND id=$3 AND status=$4 RETURNING 1`, string(next), sc.Tenant, oid, current).Scan(&n); e != nil {
		problem(w, 409, "conflict", "occurrence changed concurrently")
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE verification_attempts SET status=$1 WHERE tenant_id=$2 AND id=$3 AND status='open'`, attemptNext, sc.Tenant, attempt); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if next == domain.OccurrenceCompleted {
		if e = verification.PublishDialogueOccurrenceCompletion(r.Context(), tx, sc.Tenant, oid); e != nil {
			problem(w, 500, "internal", "unable to record verification completion")
			return
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, OccurrenceDecision2{OccurrenceID: oid, DecisionID: decisionID, Accepted: accepted, Status: string(next)})
}

type OccurrenceRetryInput2 struct {
	ID            string `path:"id"`
	RequirementID string `query:"requirementId"`
	AttemptID     string `query:"attemptId"`
}

func (s *Server) retryOccurrence2(w http.ResponseWriter, r *http.Request, sc scope) {
	oid := chi.URLParam(r, "id")
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var occurrenceStatus string
	if e = tx.QueryRow(r.Context(), `SELECT status FROM task_occurrences WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, sc.Tenant, oid).Scan(&occurrenceStatus); e != nil {
		problem(w, 404, "not_found", "occurrence not found")
		return
	}
	requestedRequirement, requestedAttempt := r.URL.Query().Get("requirementId"), r.URL.Query().Get("attemptId")
	explicit := requestedRequirement != "" || requestedAttempt != ""
	if occurrenceStatus != string(domain.OccurrencePending) && !(explicit && occurrenceStatus == string(domain.OccurrenceAwaitingVerification)) {
		problem(w, 409, "conflict", "retry requires a pending occurrence or explicit failed requirement")
		return
	}
	rows, e := tx.Query(r.Context(), `SELECT a.id,a.requirement_id,r.kind,a.status,a.number FROM verification_attempts a JOIN verification_requirements r ON r.tenant_id=a.tenant_id AND r.id=a.requirement_id JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id AND o.revision_id=r.revision_id
 WHERE a.tenant_id=$1 AND a.occurrence_id=$2 AND a.number=(SELECT max(number) FROM verification_attempts WHERE tenant_id=a.tenant_id AND occurrence_id=a.occurrence_id AND requirement_id=a.requirement_id)
 AND ($3='' OR a.requirement_id::text=$3) AND ($4='' OR a.id::text=$4) ORDER BY r.ordinal FOR UPDATE OF a`, sc.Tenant, oid, requestedRequirement, requestedAttempt)
	if e != nil {
		problem(w, 500, "internal", "unable to select retry attempt")
		return
	}
	type retryCandidate struct {
		id, requirement, kind, status string
		number                        int
	}
	var candidates []retryCandidate
	matched := 0
	for rows.Next() {
		var candidate retryCandidate
		if e = rows.Scan(&candidate.id, &candidate.requirement, &candidate.kind, &candidate.status, &candidate.number); e != nil {
			break
		}
		matched++
		if candidate.status == "rejected" || candidate.status == "exhausted" {
			candidates = append(candidates, candidate)
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		problem(w, 500, "internal", "unable to read retry attempt")
		return
	}
	if explicit && matched == 0 {
		problem(w, 404, "not_found", "retry selection unavailable")
		return
	}
	if len(candidates) != 1 {
		problem(w, 409, "conflict", "select exactly one rejected or exhausted requirement attempt")
		return
	}
	selected := candidates[0]
	req, kind, n := selected.requirement, selected.kind, selected.number+1
	if kind == domain.AgentDialogueKind {
		var maximum int
		if e = tx.QueryRow(r.Context(), `SELECT (p.snapshot->'config'->>'maxAttempts')::int FROM dialogue_revision_policies p JOIN task_occurrences o ON o.tenant_id=p.tenant_id AND o.revision_id=p.revision_id WHERE p.tenant_id=$1 AND o.id=$2 AND p.requirement_id=$3`, sc.Tenant, oid, req).Scan(&maximum); e != nil || n > maximum {
			problem(w, 409, "conflict", "dialogue attempt policy exhausted")
			return
		}
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,$5)`, uuid.New(), sc.Tenant, oid, req, n); e != nil {
		problem(w, 409, "conflict", "retry is not permitted")
		return
	}
	var updated int
	if e = tx.QueryRow(r.Context(), `UPDATE task_occurrences SET status='awaiting_verification' WHERE tenant_id=$1 AND id=$2 AND status=$3 RETURNING 1`, sc.Tenant, oid, occurrenceStatus).Scan(&updated); e != nil {
		problem(w, 409, "conflict", "occurrence changed concurrently")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, OccurrenceRetry2{OccurrenceID: oid, RequirementID: req, PreviousAttemptID: selected.id, AttemptNumber: n, Status: "awaiting_verification"})
}
func (s *Server) setOccurrenceStatus2(w http.ResponseWriter, r *http.Request, sc scope, status string) {
	oid := chi.URLParam(r, "id")
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var current string
	if e = tx.QueryRow(r.Context(), `SELECT status FROM task_occurrences WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, sc.Tenant, oid).Scan(&current); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			problem(w, 404, "not_found", "occurrence not found")
			return
		}
		problem(w, 500, "internal", e.Error())
		return
	}
	if current == status || !domain.CanTransition(domain.OccurrenceStatus(current), domain.OccurrenceStatus(status)) {
		problem(w, 409, "conflict", "occurrence is terminal or unavailable")
		return
	}
	var n int
	e = tx.QueryRow(r.Context(), `UPDATE task_occurrences SET status=$1 WHERE tenant_id=$2 AND id=$3 AND status=$4 RETURNING 1`, status, sc.Tenant, oid, current).Scan(&n)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 409, "conflict", "occurrence changed concurrently")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, OccurrenceAction2{ID: oid, Status: status})
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
	if e = s.attachStudentRequirements(r.Context(), &x); e != nil {
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
	if e = s.attachStudentRequirements(r.Context(), &x); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, x)
}
func (s *Server) studentStart2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.startOccurrence2(w, r, id)
}
func (s *Server) studentSubmit2(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.submitOccurrence2(w, r, id)
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
func (s *Server) deviceSubmitWrapper(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.submitOccurrence2(w, r, id)
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
		r := domain.VerificationRequirement{Kind: req.Kind, ConfigVersion: req.ConfigVersion, Config: req.Config, Interaction: req.Interaction, Executor: req.Executor}
		if domain.ValidateRevision(in.Title, in.Instructions, []domain.VerificationRequirement{r}) != nil {
			problem(w, 400, "invalid_request", "unsupported verification requirement configuration")
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
		if _, err = tx.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), sc.Tenant, rid, i, req.Kind, req.ConfigVersion, requirementJSON([]Requirement{req}), req.Interaction, req.Executor); err != nil {
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
	rows, err := s.DB.Query(r.Context(), `SELECT s.id,s.student_id,s.template_id,s.revision_id,s.kind,s.timezone,s.start_local,s.end_local,s.rrule,s.due_offset_minutes,s.enabled,s.version,r.title,st.display_name FROM task_schedules s JOIN task_revisions r ON r.tenant_id=s.tenant_id AND r.id=s.revision_id JOIN students st ON st.tenant_id=s.tenant_id AND st.id=s.student_id WHERE s.tenant_id=$1 AND ($2 OR s.enabled) ORDER BY s.start_local,s.id LIMIT $3 OFFSET $4`, sc.Tenant, all, limit, offset)
	if err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	items := []Schedule2{}
	for rows.Next() {
		var x Schedule2
		if err := rows.Scan(&x.ID, &x.StudentID, &x.TemplateID, &x.RevisionID, &x.Kind, &x.Timezone, &x.StartAt, &x.EndAt, &x.RRULE, &x.DueOffsetMinutes, &x.Enabled, &x.Version, &x.Title, &x.StudentName); err != nil {
			problem(w, 500, "internal", err.Error())
			return
		}
		items = append(items, x)
	}
	jsonOK(w, SchedulePage2{items, total, limit, offset})
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
