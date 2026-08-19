package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/domain/parent"
)

// phase3Services are domain-service adapters over the Phase 2 transactional
// operations. Parent tools depend on these interfaces, never on repositories
// or model-supplied tenant/actor fields.
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

// beginToolEffect reserves a mutation before the domain write. If a process
// dies after the write but before completion, a retry sees the reservation and
// fails safely instead of replaying the mutation. Applied effects replay their
// sanitized domain result. Read-only/unit callers without a run ID bypass the
// ledger because they are not durable worker executions.
func (p phase3Services) beginToolEffect(ctx context.Context, c parent.ServiceContext, tool string, input any) ([]byte, error) {
	if c.RunID == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(c.RunID); err != nil {
		// Unit adapters may use a descriptive test identity; only durable
		// worker runs carry a UUID-backed ledger key.
		return nil, nil
	}
	digest := effectDigest(tool, input)
	tag, err := p.s.DB.Exec(ctx, `INSERT INTO agent_tool_effects(tenant_id,run_id,step,tool_name,action_digest,status,result) VALUES($1,$2,$3,$4,$5,'reserved','{}'::jsonb) ON CONFLICT (tenant_id,run_id,step) DO NOTHING`, c.TenantID, c.RunID, c.ToolStep, tool, digest)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		var status, existingTool, existingDigest string
		var result []byte
		if err := p.s.DB.QueryRow(ctx, `SELECT status,tool_name,action_digest,result FROM agent_tool_effects WHERE tenant_id=$1 AND run_id=$2 AND step=$3`, c.TenantID, c.RunID, c.ToolStep).Scan(&status, &existingTool, &existingDigest, &result); err != nil {
			return nil, err
		}
		if existingTool != tool || existingDigest != digest {
			return nil, fmt.Errorf("%w: durable tool boundary changed", errToolEffectInProgress)
		}
		if status == "applied" {
			return result, nil
		}
		return nil, errToolEffectInProgress
	}
	return nil, nil
}

func (p phase3Services) completeToolEffect(ctx context.Context, c parent.ServiceContext, tool string, input, result any) error {
	if c.RunID == "" {
		return nil
	}
	if _, err := uuid.Parse(c.RunID); err != nil {
		return nil
	}
	digest := effectDigest(tool, input)
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tx, err := p.s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE agent_tool_effects SET status='applied',result=$6,updated_at=now() WHERE tenant_id=$1 AND run_id=$2 AND step=$3 AND tool_name=$4 AND action_digest=$5 AND status='reserved'`, c.TenantID, c.RunID, c.ToolStep, tool, digest, b); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_runs SET durable_step=GREATEST(durable_step,$3),updated_at=now() WHERE tenant_id=$1 AND id=$2`, c.TenantID, c.RunID, c.ToolStep); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p phase3Services) ListStudents(ctx context.Context, c parent.ServiceContext, q parent.StudentQuery) ([]parent.Student, error) {
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := p.s.DB.Query(ctx, `SELECT id,display_name,created_at FROM students WHERE tenant_id=$1 AND archived_at IS NULL AND ($2='' OR display_name ILIKE '%'||$2||'%') ORDER BY display_name LIMIT $3 OFFSET $4`, scopeTenant(c), strings.TrimSpace(q.Name), limit, offset)
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
func (p phase3Services) GetStudent(ctx context.Context, c parent.ServiceContext, id string) (parent.Student, error) {
	var x parent.Student
	err := p.s.DB.QueryRow(ctx, `SELECT id,display_name,created_at FROM students WHERE tenant_id=$1 AND id=$2 AND archived_at IS NULL`, scopeTenant(c), id).Scan(&x.ID, &x.DisplayName, &x.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, parent.ErrServiceMissing
	}
	return x, err
}
func (p phase3Services) ListTasks(ctx context.Context, c parent.ServiceContext, q parent.TaskQuery) ([]parent.Task, error) {
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := p.s.DB.Query(ctx, `SELECT r.id,r.template_id,r.version,r.title,r.instructions,r.status FROM task_revisions r JOIN task_templates t ON t.id=r.template_id AND t.tenant_id=r.tenant_id WHERE r.tenant_id=$1 AND t.status<>'retired' AND ($2='' OR r.title ILIKE '%'||$2||'%') ORDER BY r.title LIMIT $3 OFFSET $4`, scopeTenant(c), q.Query, limit, offset)
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
func (p phase3Services) GetTask(ctx context.Context, c parent.ServiceContext, id string) (parent.Task, error) {
	var x parent.Task
	err := p.s.DB.QueryRow(ctx, `SELECT id,template_id,version,title,instructions,status FROM task_revisions WHERE tenant_id=$1 AND id=$2`, scopeTenant(c), id).Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, parent.ErrServiceMissing
	}
	return x, err
}
func (p phase3Services) DraftTask(ctx context.Context, c parent.ServiceContext, in parent.TaskDraftInput) (parent.Task, error) {
	if strings.TrimSpace(in.Title) == "" {
		return parent.Task{}, parent.ErrInvalidInput
	}
	input := struct {
		Title        string           `json:"title"`
		Instructions string           `json:"instructions"`
		Requirements []map[string]any `json:"requirements"`
	}{strings.TrimSpace(in.Title), in.Instructions, in.Requirements}
	if replay, err := p.beginToolEffect(ctx, c, parent.ToolDraftTask, input); err != nil {
		return parent.Task{}, err
	} else if replay != nil {
		var task parent.Task
		if err := json.Unmarshal(replay, &task); err != nil {
			return parent.Task{}, err
		}
		return task, nil
	}
	tx, err := p.s.DB.Begin(ctx)
	if err != nil {
		return parent.Task{}, err
	}
	defer tx.Rollback(ctx)
	tid, rid := uuid.New(), uuid.New()
	var created time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO task_templates(id,tenant_id,title) VALUES($1,$2,$3) RETURNING created_at`, tid, scopeTenant(c), strings.TrimSpace(in.Title)).Scan(&created); err != nil {
		return parent.Task{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions) VALUES($1,$2,$3,1,$4,$5) RETURNING created_at`, rid, scopeTenant(c), tid, strings.TrimSpace(in.Title), in.Instructions).Scan(&created); err != nil {
		return parent.Task{}, err
	}
	req := in.Requirements
	if len(req) == 0 {
		req = []map[string]any{{"kind": "parent_approval", "configVersion": 1, "interaction": "parent_action", "executor": "human", "config": map[string]any{}}}
	}
	for i, r := range req {
		kind, _ := r["kind"].(string)
		if kind == "" {
			kind = "parent_approval"
		}
		cv := 1
		if n, ok := r["configVersion"].(float64); ok {
			cv = int(n)
		}
		b, _ := json.Marshal(r)
		if _, err = tx.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,$4,$5,$6,$7,'parent_action','human')`, uuid.New(), scopeTenant(c), rid, i, kind, cv, b); err != nil {
			return parent.Task{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return parent.Task{}, err
	}
	task := parent.Task{ID: rid.String(), TemplateID: tid.String(), Version: 1, Title: strings.TrimSpace(in.Title), Instructions: in.Instructions, Status: "draft"}
	if err := p.completeToolEffect(ctx, c, parent.ToolDraftTask, input, task); err != nil {
		return parent.Task{}, err
	}
	return task, nil
}
func (p phase3Services) UpdateTask(context.Context, parent.ServiceContext, parent.TaskUpdateInput) (parent.Task, error) {
	return parent.Task{}, errors.New("task updates require a new revision in Phase 3")
}
func (p phase3Services) PublishTask(ctx context.Context, c parent.ServiceContext, id string) (parent.Task, error) {
	if replay, err := p.beginToolEffect(ctx, c, parent.ToolPublishTask, map[string]string{"id": strings.TrimSpace(id)}); err != nil {
		return parent.Task{}, err
	} else if replay != nil {
		var task parent.Task
		if err := json.Unmarshal(replay, &task); err != nil {
			return parent.Task{}, err
		}
		return task, nil
	}
	var x parent.Task
	err := p.s.DB.QueryRow(ctx, `UPDATE task_revisions SET status='published',published_at=now() WHERE tenant_id=$1 AND id=$2 AND status='draft' RETURNING id,template_id,version,title,instructions,status`, scopeTenant(c), id).Scan(&x.ID, &x.TemplateID, &x.Version, &x.Title, &x.Instructions, &x.Status)
	if err != nil {
		return x, err
	}
	_, err = p.s.DB.Exec(ctx, `UPDATE task_templates SET status='published',current_revision=$1,title=$2 WHERE tenant_id=$3 AND id=$4`, x.Version, x.Title, scopeTenant(c), x.TemplateID)
	if err == nil {
		err = p.completeToolEffect(ctx, c, parent.ToolPublishTask, map[string]string{"id": strings.TrimSpace(id)}, x)
	}
	return x, err
}
func (p phase3Services) RetireTask(ctx context.Context, c parent.ServiceContext, id string, version int) (parent.Task, error) {
	var x parent.Task
	err := p.s.DB.QueryRow(ctx, `UPDATE task_templates SET status='retired',retired_at=now() WHERE tenant_id=$1 AND id=(SELECT template_id FROM task_revisions WHERE tenant_id=$1 AND id=$2 AND version=$3) AND status<>'retired' RETURNING id`, scopeTenant(c), id, version).Scan(&x.TemplateID)
	if err != nil {
		return x, err
	}
	return p.GetTask(ctx, c, id)
}
func (p phase3Services) ListSchedules(ctx context.Context, c parent.ServiceContext, q parent.ScheduleQuery) ([]parent.Schedule, error) {
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	where := ""
	if !q.IncludeDisabled {
		where = " AND enabled"
	}
	rows, err := p.s.DB.Query(ctx, `SELECT id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes,enabled,version FROM task_schedules WHERE tenant_id=$1`+where+` ORDER BY start_local LIMIT $2 OFFSET $3`, scopeTenant(c), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []parent.Schedule{}
	for rows.Next() {
		var x parent.Schedule
		if err = rows.Scan(&x.ID, &x.StudentID, &x.TemplateID, &x.RevisionID, &x.Kind, &x.Timezone, &x.StartAt, &x.EndAt, &x.RRULE, &x.DueOffsetMinutes, &x.Enabled, &x.Version); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (p phase3Services) GetSchedule(ctx context.Context, c parent.ServiceContext, id string) (parent.Schedule, error) {
	xs, err := p.ListSchedules(ctx, c, parent.ScheduleQuery{IncludeDisabled: true, Limit: 100})
	if err != nil {
		return parent.Schedule{}, err
	}
	for _, x := range xs {
		if x.ID == id {
			return x, nil
		}
	}
	return parent.Schedule{}, parent.ErrServiceMissing
}
func (p phase3Services) CreateSchedule(ctx context.Context, c parent.ServiceContext, in parent.ScheduleInput) (parent.Schedule, error) {
	if replay, err := p.beginToolEffect(ctx, c, parent.ToolCreateSchedule, in); err != nil {
		return parent.Schedule{}, err
	} else if replay != nil {
		var schedule parent.Schedule
		if err := json.Unmarshal(replay, &schedule); err != nil {
			return parent.Schedule{}, err
		}
		return schedule, nil
	}
	id := uuid.New()
	_, err := p.s.DB.Exec(ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local,end_local,rrule,due_offset_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, scopeTenant(c), in.StudentID, in.TemplateID, in.RevisionID, in.Kind, in.Timezone, in.StartAt, in.EndAt, in.RRULE, in.DueOffsetMinutes)
	if err != nil {
		return parent.Schedule{}, err
	}
	schedule, err := p.GetSchedule(ctx, c, id.String())
	if err != nil {
		return parent.Schedule{}, err
	}
	if err := p.completeToolEffect(ctx, c, parent.ToolCreateSchedule, in, schedule); err != nil {
		return parent.Schedule{}, err
	}
	return schedule, nil
}
func (p phase3Services) UpdateSchedule(ctx context.Context, c parent.ServiceContext, in parent.ScheduleUpdateInput) (parent.Schedule, error) {
	result, err := p.s.DB.Exec(ctx, `UPDATE task_schedules SET timezone=$3,start_local=$4,end_local=$5,rrule=$6,due_offset_minutes=$7,version=version+1 WHERE tenant_id=$1 AND id=$2 AND enabled`, scopeTenant(c), in.ScheduleID, in.Timezone, in.StartAt, in.EndAt, in.RRULE, in.DueOffsetMinutes)
	if err != nil {
		return parent.Schedule{}, err
	}
	if result.RowsAffected() != 1 {
		return parent.Schedule{}, parent.ErrServiceMissing
	}
	return p.GetSchedule(ctx, c, in.ScheduleID)
}
func (p phase3Services) DisableSchedule(ctx context.Context, c parent.ServiceContext, id string, version int) (parent.Schedule, error) {
	result, err := p.s.DB.Exec(ctx, `UPDATE task_schedules SET enabled=false,retired_at=now(),version=version+1 WHERE tenant_id=$1 AND id=$2 AND version=$3 AND enabled`, scopeTenant(c), id, version)
	if err != nil {
		return parent.Schedule{}, err
	}
	if result.RowsAffected() != 1 {
		return parent.Schedule{}, parent.ErrConfirmationExpired
	}
	return p.GetSchedule(ctx, c, id)
}
func (p phase3Services) BulkDisableSchedules(ctx context.Context, c parent.ServiceContext, ids []string) ([]parent.Schedule, error) {
	out := []parent.Schedule{}
	for _, id := range ids {
		x, err := p.GetSchedule(ctx, c, id)
		if err != nil {
			return nil, err
		}
		x, err = p.DisableSchedule(ctx, c, id, x.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}
func (p phase3Services) ListOccurrences(ctx context.Context, c parent.ServiceContext, q parent.OccurrenceQuery) ([]parent.Occurrence, error) {
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := p.s.DB.Query(ctx, `SELECT id,student_id,schedule_id,revision_id,revision_snapshot->>'title',status,nominal_at,due_at FROM task_occurrences WHERE tenant_id=$1 AND ($2='' OR status=$2) ORDER BY nominal_at LIMIT $3`, scopeTenant(c), q.Status, limit)
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
