package schedule

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strconv"
	"time"
)

// Worker is the durable near-term materializer. Lease ownership lives in
// PostgreSQL, so restarts and concurrent instances converge on unique rows.
type Worker struct {
	DB       *pgxpool.Pool
	Owner    string
	Horizon  time.Duration
	Interval time.Duration
}

func NewWorker(db *pgxpool.Pool) *Worker {
	days := 45
	if n, e := strconv.Atoi(os.Getenv("TASKS_SCHEDULE_HORIZON_DAYS")); e == nil && n > 0 && n <= 730 {
		days = n
	}
	return &Worker{DB: db, Owner: uuid.NewString(), Horizon: time.Duration(days) * 24 * time.Hour, Interval: time.Minute}
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	_ = w.Materialize(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.Materialize(ctx)
		}
	}
}
func (w *Worker) Materialize(ctx context.Context) error {
	rows, e := w.DB.Query(ctx, `SELECT tenant_id,id FROM task_schedules WHERE enabled AND retired_at IS NULL`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var tenant, id string
		if e = rows.Scan(&tenant, &id); e != nil {
			return e
		}
		if ok, e := w.claim(ctx, tenant); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			return e
		} else if ok {
			if e = w.materializeSchedule(ctx, tenant, id); e != nil {
				return e
			}
		}
	}
	return rows.Err()
}
func (w *Worker) claim(ctx context.Context, tenant string) (bool, error) {
	var ok bool
	e := w.DB.QueryRow(ctx, `INSERT INTO task_materializer_leases(tenant_id,owner,lease_until) VALUES($1,$2,now()+interval '30 seconds') ON CONFLICT(tenant_id) DO UPDATE SET owner=EXCLUDED.owner,lease_until=EXCLUDED.lease_until,updated_at=now() WHERE task_materializer_leases.lease_until<now() OR task_materializer_leases.owner=$2 RETURNING true`, tenant, w.Owner).Scan(&ok)
	if e != nil {
		return false, e
	}
	return ok, nil
}
func (w *Worker) materializeSchedule(ctx context.Context, tenant, id string) error {
	var student, rev, kind, zone, rule string
	var start, end *time.Time
	var due, version int
	if e := w.DB.QueryRow(ctx, `SELECT student_id,revision_id,kind,timezone,rrule,start_local,end_local,due_offset_minutes,version FROM task_schedules WHERE tenant_id=$1 AND id=$2 AND enabled`, tenant, id).Scan(&student, &rev, &kind, &zone, &rule, &start, &end, &due, &version); e != nil {
		return e
	}
	ruleText := rule
	if kind == "one_off" {
		ruleText = ""
	}
	spec, e := Parse(ruleText, zone, *start, end)
	if e != nil {
		// A single corrupt schedule must not block the rest of the tenant.
		return nil
	}
	for _, at := range spec.Occurrences(time.Now().Add(w.Horizon)) {
		_, e = w.DB.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,revision_snapshot) VALUES($1,$2,$3,$4,$5::uuid,$6::timestamptz,$6::timestamptz+($8::int*interval '1 minute'),jsonb_build_object('revisionId',$5::uuid,'title',(SELECT title FROM task_revisions WHERE tenant_id=$2 AND id=$5),'instructions',(SELECT instructions FROM task_revisions WHERE tenant_id=$2 AND id=$5),'taskRevisionVersion',(SELECT version FROM task_revisions WHERE tenant_id=$2 AND id=$5),'timezone',$7::text,'dueOffsetMinutes',$8::int,'dueSemantics','offset_from_nominal','scheduleVersion',$9::int)) ON CONFLICT (tenant_id,schedule_id,nominal_at) DO NOTHING`, uuid.New(), tenant, id, student, rev, at, zone, due, version)
		if e != nil {
			return e
		}
	}
	return nil
}
