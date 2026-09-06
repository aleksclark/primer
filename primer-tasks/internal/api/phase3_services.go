package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/domain/parent"
)

// phase3Services adapt the ordinary P2 handlers inside a PostgreSQL transaction.
// Neither Fantasy nor its tools have an independent task/schedule SQL write path.
type phase3Services struct{ s *Server }

func (s *Server) parentTools() *parent.Tools {
	return &parent.Tools{Students: phase3Services{s}, Tasks: phase3Services{s}, Schedules: phase3Services{s}, Occurrences: phase3Services{s}, Confirmations: &parent.SQLConfirmationStore{DB: s.DB}, ConfirmationTTL: 5 * time.Minute}
}
func scopeTenant(c parent.ServiceContext) string { return c.TenantID }

var errToolEffectInProgress = errors.New("parent tool effect is already in progress")

func effectDigest(tool string, input any) string {
	b, _ := json.Marshal(input)
	h := sha256.Sum256(append([]byte(tool+"\x00"), b...))
	return hex.EncodeToString(h[:])
}
func (p phase3Services) beginToolEffect(ctx context.Context, c parent.ServiceContext, tool string, input any) ([]byte, error) {
	if c.RunID == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(c.RunID); err != nil {
		return nil, parent.ErrInvalidInput
	}
	if c.ToolStep < 1 {
		return nil, parent.ErrInvalidInput
	}
	digest := effectDigest(tool, input)
	tag, err := p.s.DB.Exec(ctx, `INSERT INTO agent_tool_effects(tenant_id,run_id,step,tool_name,action_digest,status,result) VALUES($1,$2,$3,$4,$5,'reserved','{}'::jsonb) ON CONFLICT (tenant_id,run_id,step) DO NOTHING`, c.TenantID, c.RunID, c.ToolStep, tool, digest)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		var status, name, stored string
		var result []byte
		if err = p.s.DB.QueryRow(ctx, `SELECT status,tool_name,action_digest,result FROM agent_tool_effects WHERE tenant_id=$1 AND run_id=$2 AND step=$3`, c.TenantID, c.RunID, c.ToolStep).Scan(&status, &name, &stored, &result); err != nil {
			return nil, err
		}
		if name != tool || stored != digest || status != "applied" {
			return nil, errToolEffectInProgress
		}
		return result, nil
	}
	return nil, nil
}
func (p phase3Services) completeToolEffect(ctx context.Context, c parent.ServiceContext, tool string, input, result any) error {
	if c.RunID == "" {
		return nil
	}
	if _, err := uuid.Parse(c.RunID); err != nil {
		return parent.ErrInvalidInput
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tag, err := p.s.DB.Exec(ctx, `UPDATE agent_tool_effects SET status='applied',result=$6,updated_at=now() WHERE tenant_id=$1 AND run_id=$2 AND step=$3 AND tool_name=$4 AND action_digest=$5 AND status='reserved'`, c.TenantID, c.RunID, c.ToolStep, tool, effectDigest(tool, input), b)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errToolEffectInProgress
	}
	_, err = p.s.DB.Exec(ctx, `UPDATE agent_runs SET durable_step=GREATEST(durable_step,$3),updated_at=now() WHERE tenant_id=$1 AND id=$2`, c.TenantID, c.RunID, c.ToolStep)
	return err
}

func (p phase3Services) mutate(ctx context.Context, c parent.ServiceContext, tool string, input any, out any, apply func(phase3Services) error) error {
	tx, err := p.s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	bound := *p.s
	bound.DB = tx
	local := phase3Services{&bound}
	if c.RunID != "" {
		if err = lockAgentAuthority(ctx, tx, c.TenantID, c.ActorID, c.RunID); err != nil {
			return err
		}
	}
	replay, err := local.beginToolEffect(ctx, c, tool, input)
	if err != nil {
		return err
	}
	if replay != nil {
		return json.Unmarshal(replay, out)
	}
	if err = apply(local); err != nil {
		return err
	}
	if err = local.completeToolEffect(ctx, c, tool, input, out); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// invokeP2 uses the production request decoder, validation, tenant predicates,
// publication checks, materializer and transactions. pgx nested Begin creates
// savepoints, so an effect reservation/result and every domain write are atomic.
func (p phase3Services) invokeP2(ctx context.Context, c parent.ServiceContext, method, id string, input, out any, handler parentHandler) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, "/", bytes.NewReader(b))
	if err != nil {
		return err
	}
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rc))
	rec := &capturedResponse{header: make(http.Header)}
	handler(rec, req, scope{Tenant: c.TenantID, Subject: c.ActorID})
	if rec.status < 200 || rec.status >= 300 {
		return fmt.Errorf("%w: Tasks domain operation rejected (%d)", parent.ErrInvalidInput, rec.status)
	}
	if out != nil {
		return json.Unmarshal(rec.body.Bytes(), out)
	}
	return nil
}
func (p phase3Services) ListStudents(ctx context.Context, c parent.ServiceContext, q parent.StudentQuery) ([]parent.Student, error) {
	if err := checkAgentAuthorization(ctx); err != nil {
		return nil, err
	}
	rows, err := p.s.DB.Query(ctx, `SELECT id,display_name,created_at FROM students WHERE tenant_id=$1 AND archived_at IS NULL AND ($2='' OR display_name ILIKE '%'||$2||'%') ORDER BY display_name,id LIMIT $3 OFFSET $4`, c.TenantID, strings.TrimSpace(q.Name), boundedLimit(q.Limit), maxInt(q.Offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []parent.Student{}
	for rows.Next() {
		var x parent.Student
		if err = rows.Scan(&x.ID, &x.DisplayName, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func boundedLimit(n int) int {
	if n < 1 || n > 100 {
		return 20
	}
	return n
}
func (p phase3Services) GetStudent(ctx context.Context, c parent.ServiceContext, id string) (x parent.Student, err error) {
	if err = checkAgentAuthorization(ctx); err != nil {
		return
	}
	err = p.s.DB.QueryRow(ctx, `SELECT id,display_name,created_at FROM students WHERE tenant_id=$1 AND id=$2 AND archived_at IS NULL`, c.TenantID, id).Scan(&x.ID, &x.DisplayName, &x.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = parent.ErrServiceMissing
	}
	return
}
func (p phase3Services) ListTasks(ctx context.Context, c parent.ServiceContext, q parent.TaskQuery) ([]parent.Task, error) {
	if err := checkAgentAuthorization(ctx); err != nil {
		return nil, err
	}
	rows, err := p.s.DB.Query(ctx, `SELECT r.id,r.template_id,r.version,r.title,r.instructions,r.status FROM task_revisions r JOIN task_templates t ON t.id=r.template_id AND t.tenant_id=r.tenant_id WHERE r.tenant_id=$1 AND t.status<>'retired' AND r.version=(SELECT max(latest.version) FROM task_revisions latest WHERE latest.tenant_id=r.tenant_id AND latest.template_id=r.template_id) AND ($2='' OR r.title ILIKE '%'||$2||'%') ORDER BY r.title,r.id LIMIT $3 OFFSET $4`, c.TenantID, q.Query, boundedLimit(q.Limit), maxInt(q.Offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []parent.Task{}
	for rows.Next() {
		var x parent.Task
		if err = rows.Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (p phase3Services) GetTask(ctx context.Context, c parent.ServiceContext, id string) (x parent.Task, err error) {
	if err = checkAgentAuthorization(ctx); err != nil {
		return
	}
	err = p.s.DB.QueryRow(ctx, `SELECT id,template_id,version,title,instructions,status,COALESCE((SELECT jsonb_agg(v.config->0 ORDER BY v.ordinal) FROM verification_requirements v WHERE v.tenant_id=r.tenant_id AND v.revision_id=r.id),'[]'::jsonb) FROM task_revisions r WHERE tenant_id=$1 AND id=$2`, c.TenantID, id).Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status, &x.Requirements)
	if errors.Is(err, pgx.ErrNoRows) {
		err = parent.ErrServiceMissing
	}
	return
}
func (p phase3Services) DraftTask(ctx context.Context, c parent.ServiceContext, in parent.TaskDraftInput) (x parent.Task, err error) {
	var rs []Requirement
	b, e := json.Marshal(in.Requirements)
	if e != nil {
		return x, e
	}
	if e = json.Unmarshal(b, &rs); e != nil {
		return x, e
	}
	input := TaskInput2{Title: in.Title, Instructions: in.Instructions, Requirements: rs}
	err = p.mutate(ctx, c, parent.ToolDraftTask, input, &x, func(p phase3Services) error {
		return p.invokeP2(ctx, c, http.MethodPost, "", input, &x, p.s.createTask2)
	})
	return
}
func (p phase3Services) UpdateTask(ctx context.Context, c parent.ServiceContext, in parent.TaskUpdateInput) (x parent.Task, err error) {
	if _, e := uuid.Parse(in.TaskID); e != nil || in.ExpectedVersion < 1 {
		return x, parent.ErrInvalidInput
	}
	err = p.mutate(ctx, c, parent.ToolUpdateTask, in, &x, func(p phase3Services) error {
		row, e := p.GetTask(ctx, c, in.TaskID)
		if e != nil {
			return e
		}
		var version int
		if e = p.s.DB.QueryRow(ctx, `SELECT (SELECT max(r.version) FROM task_revisions r WHERE r.tenant_id=t.tenant_id AND r.template_id=t.id) FROM task_templates t WHERE t.tenant_id=$1 AND t.id=$2 AND t.status<>'retired' FOR UPDATE`, c.TenantID, row.TemplateID).Scan(&version); e != nil || in.ExpectedVersion < 1 || in.ExpectedVersion != version || row.Version != version {
			return parent.ErrConfirmationExpired
		}
		var requirements []Requirement
		b, e := json.Marshal(in.Requirements)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &requirements); e != nil {
			return e
		}
		return p.invokeP2(ctx, c, http.MethodPost, row.TemplateID, TaskInput2{Title: in.Title, Instructions: in.Instructions, Requirements: requirements}, &x, p.s.reviseTask2)
	})
	return
}
func (p phase3Services) PublishTask(ctx context.Context, c parent.ServiceContext, id string) (x parent.Task, err error) {
	err = p.mutate(ctx, c, parent.ToolPublishTask, id, &x, func(p phase3Services) error {
		return p.invokeP2(ctx, c, http.MethodPost, id, nil, &x, p.s.publishTask2)
	})
	return
}
func (p phase3Services) RetireTask(ctx context.Context, c parent.ServiceContext, id string, version int) (x parent.Task, err error) {
	err = p.mutate(ctx, c, parent.ActionRetireTask, []any{id, version}, &x, func(p phase3Services) error {
		if e := p.s.DB.QueryRow(ctx, `SELECT r.id,r.template_id,r.version,r.title,r.instructions,r.status FROM task_revisions r JOIN task_templates t ON t.id=r.template_id AND t.tenant_id=r.tenant_id WHERE r.tenant_id=$1 AND r.id=$2 AND r.version=$3 AND (SELECT max(latest.version) FROM task_revisions latest WHERE latest.tenant_id=t.tenant_id AND latest.template_id=t.id)=$3 AND t.status<>'retired' FOR UPDATE OF t`, c.TenantID, id, version).Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status); e != nil {
			return parent.ErrConfirmationExpired
		}
		return p.invokeP2(ctx, c, http.MethodPost, x.TemplateID, nil, nil, p.s.retireTask2)
	})
	return
}

const scheduleColumns = `id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes,enabled,version`

func scanParentSchedule(row pgx.Row) (x parent.Schedule, err error) {
	err = row.Scan(&x.ID, &x.StudentID, &x.TemplateID, &x.RevisionID, &x.Kind, &x.Timezone, &x.StartAt, &x.EndAt, &x.RRULE, &x.DueOffsetMinutes, &x.Enabled, &x.Version)
	return
}
func (p phase3Services) ListSchedules(ctx context.Context, c parent.ServiceContext, q parent.ScheduleQuery) ([]parent.Schedule, error) {
	if err := checkAgentAuthorization(ctx); err != nil {
		return nil, err
	}
	rows, err := p.s.DB.Query(ctx, `SELECT `+scheduleColumns+` FROM task_schedules WHERE tenant_id=$1 AND ($2 OR enabled) ORDER BY start_local,id LIMIT $3 OFFSET $4`, c.TenantID, q.IncludeDisabled, boundedLimit(q.Limit), maxInt(q.Offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []parent.Schedule{}
	for rows.Next() {
		x, e := scanParentSchedule(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (p phase3Services) GetSchedule(ctx context.Context, c parent.ServiceContext, id string) (parent.Schedule, error) {
	if err := checkAgentAuthorization(ctx); err != nil {
		return parent.Schedule{}, err
	}
	x, err := scanParentSchedule(p.s.DB.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM task_schedules WHERE tenant_id=$1 AND id=$2`, c.TenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		err = parent.ErrServiceMissing
	}
	return x, err
}
func scheduleInput(in parent.ScheduleInput) ScheduleInput2 {
	return ScheduleInput2{StudentID: in.StudentID, TemplateID: in.TemplateID, RevisionID: in.RevisionID, Kind: in.Kind, Timezone: in.Timezone, StartAt: in.StartAt, EndAt: in.EndAt, RRULE: in.RRULE, DueOffsetMinutes: in.DueOffsetMinutes}
}
func (p phase3Services) CreateSchedule(ctx context.Context, c parent.ServiceContext, in parent.ScheduleInput) (x parent.Schedule, err error) {
	err = p.mutate(ctx, c, parent.ToolCreateSchedule, in, &x, func(p phase3Services) error {
		return p.invokeP2(ctx, c, http.MethodPost, "", scheduleInput(in), &x, p.s.createSchedule2)
	})
	return
}
func (p phase3Services) UpdateSchedule(ctx context.Context, c parent.ServiceContext, in parent.ScheduleUpdateInput) (x parent.Schedule, err error) {
	err = p.mutate(ctx, c, parent.ToolUpdateSchedule, in, &x, func(p phase3Services) error {
		var version int
		if e := p.s.DB.QueryRow(ctx, `SELECT version FROM task_schedules WHERE tenant_id=$1 AND id=$2 AND enabled FOR UPDATE`, c.TenantID, in.ScheduleID).Scan(&version); e != nil {
			return parent.ErrServiceMissing
		}
		if in.ExpectedVersion < 1 || version != in.ExpectedVersion {
			return parent.ErrConfirmationExpired
		}
		return p.invokeP2(ctx, c, http.MethodPatch, in.ScheduleID, scheduleInput(in.ScheduleInput), &x, p.s.updateSchedule2)
	})
	return
}
func (p phase3Services) DisableSchedule(ctx context.Context, c parent.ServiceContext, id string, version int) (x parent.Schedule, err error) {
	err = p.mutate(ctx, c, parent.ActionDisableSchedule, []any{id, version}, &x, func(p phase3Services) error {
		var current int
		if e := p.s.DB.QueryRow(ctx, `SELECT version FROM task_schedules WHERE tenant_id=$1 AND id=$2 AND enabled FOR UPDATE`, c.TenantID, id).Scan(&current); e != nil || current != version {
			return parent.ErrConfirmationExpired
		}
		if e := p.invokeP2(ctx, c, http.MethodDelete, id, nil, nil, p.s.retireSchedule2); e != nil {
			return e
		}
		var e error
		x, e = p.GetSchedule(ctx, c, id)
		return e
	})
	return
}
func (p phase3Services) BulkDisableSchedules(context.Context, parent.ServiceContext, []string) ([]parent.Schedule, error) {
	return nil, parent.ErrConfirmationRejected
} // no versionless bulk effects
func (p phase3Services) ListOccurrences(ctx context.Context, c parent.ServiceContext, q parent.OccurrenceQuery) ([]parent.Occurrence, error) {
	if err := checkAgentAuthorization(ctx); err != nil {
		return nil, err
	}
	rows, err := p.s.DB.Query(ctx, `SELECT id,student_id,schedule_id,revision_id,revision_snapshot->>'title',status,nominal_at,due_at FROM task_occurrences WHERE tenant_id=$1 AND ($2='' OR status=$2) ORDER BY nominal_at,id LIMIT $3 OFFSET $4`, c.TenantID, q.Status, boundedLimit(q.Limit), maxInt(q.Offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []parent.Occurrence{}
	for rows.Next() {
		var x parent.Occurrence
		if err = rows.Scan(&x.ID, &x.StudentID, &x.ScheduleID, &x.RevisionID, &x.Title, &x.Status, &x.NominalAt, &x.DueAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
