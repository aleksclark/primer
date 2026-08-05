package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// TutorOffMarker is appended to student.notes to disable tutoring.
const TutorOffMarker = "tutor:off"

// CancelAssignment sets an open assignment to cancelled.
// Completed assignments cannot be cancelled.
func CancelAssignment(ctx context.Context, q Querier, assignmentID string) (*domain.StudentAssignment, error) {
	asg, err := StudentAssignments.Get(ctx, q, assignmentID)
	if err != nil {
		return nil, err
	}
	if asg.State == domain.AssignmentCancelled {
		return asg, nil
	}
	if asg.State == domain.AssignmentCompleted {
		return nil, ErrBadRequest{Msg: "completed assignment cannot be cancelled"}
	}
	return StudentAssignments.Update(ctx, q, assignmentID, map[string]any{
		"state": domain.AssignmentCancelled,
	})
}

// RetryAssignment creates a new available assignment for the same activity revision.
// The original assignment is left unchanged (typically cancelled first by the parent).
func RetryAssignment(ctx context.Context, q Querier, assignmentID string, assignedBy *string, reason string) (*domain.StudentAssignment, error) {
	asg, err := StudentAssignments.Get(ctx, q, assignmentID)
	if err != nil {
		return nil, err
	}
	if reason == "" {
		reason = "parent-retry"
	}
	priority := asg.Priority
	return CreateAssignment(ctx, q, asg.StudentID, asg.ActivityRevisionID, assignedBy, priority, reason)
}

// SetStudentTutorEnabled toggles the tutor:off marker in student notes.
// enabled=true removes the marker; enabled=false ensures it is present.
func SetStudentTutorEnabled(ctx context.Context, q Querier, studentID string, enabled bool) (*domain.Student, error) {
	st, err := Students.Get(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	notes := st.Notes
	hasOff := strings.Contains(strings.ToLower(notes), TutorOffMarker)
	switch {
	case enabled && hasOff:
		notes = removeTutorOffMarker(notes)
	case !enabled && !hasOff:
		notes = strings.TrimSpace(notes)
		if notes == "" {
			notes = TutorOffMarker
		} else {
			notes = notes + "\n" + TutorOffMarker
		}
	default:
		return st, nil
	}
	return Students.Update(ctx, q, studentID, map[string]any{"notes": notes})
}

func removeTutorOffMarker(notes string) string {
	lines := strings.Split(notes, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, TutorOffMarker) {
			continue
		}
		// Drop lines that become empty after removing the marker token.
		without := strings.ReplaceAll(strings.ToLower(line), TutorOffMarker, "")
		if strings.TrimSpace(without) == "" && strings.Contains(strings.ToLower(line), TutorOffMarker) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// SupersedeMasteryEvidence marks existing evidence as superseded and inserts a
// replacement evidence row on the same mastery record. Device event history is
// not rewritten.
func SupersedeMasteryEvidence(ctx context.Context, q Querier, evidenceID, note string, educatorID string, now time.Time) (original *domain.MasteryEvidence, replacement *domain.MasteryEvidence, err error) {
	err = WithTx(ctx, q, func(tx Querier) error {
		ev, err := MasteryEvidences.Get(ctx, tx, evidenceID)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(ev.Context), "superseded by parent") {
			return ErrBadRequest{Msg: "evidence already superseded"}
		}
		mark := fmt.Sprintf("superseded by parent at %s", now.UTC().Format(time.RFC3339))
		if educatorID != "" {
			mark += " educator=" + educatorID
		}
		if note != "" {
			mark += "; " + note
		}
		newCtx := strings.TrimSpace(ev.Context)
		if newCtx == "" {
			newCtx = mark
		} else {
			newCtx = newCtx + " | " + mark
		}
		updated, err := MasteryEvidences.Update(ctx, tx, evidenceID, map[string]any{
			"context": newCtx,
		})
		if err != nil {
			return err
		}
		original = updated

		repNote := "parent correction superseding " + evidenceID
		if note != "" {
			repNote = note + " (supersedes " + evidenceID + ")"
		}
		sourceRef := fmt.Sprintf("parent-supersede:%s:%d", evidenceID, now.UnixNano())
		class := ev.EvidenceClass
		if class == "" {
			class = domain.EvidenceProceduralContinuous
		}
		prov := map[string]any{
			"supersedes": evidenceID,
			"educatorId": educatorID,
			"note":       note,
		}
		rep, err := MasteryEvidences.Create(ctx, tx, map[string]any{
			"mastery_record_id": ev.MasteryRecordID,
			"kind":              ev.Kind,
			"evidence_class":    class,
			"provenance":        prov,
			"policy_version":    ev.PolicyVersion,
			"occurred_on":       now.UTC(),
			"context":           repNote,
			"source_ref":        sourceRef,
		})
		if err != nil {
			return err
		}
		replacement = rep
		return nil
	})
	return original, replacement, err
}

// EvidenceStatus values for parent-visible mastery evidence reporting.
const (
	EvidenceStatusNotIntroduced          = "not_introduced"
	EvidenceStatusActivityCompleted      = "activity_completed"
	EvidenceStatusProceduralAccepted     = "procedural_accepted"
	EvidenceStatusAdditionalEvidenceReq  = "additional_evidence_required"
	EvidenceStatusFormalMastery          = "formal_mastery"
)

// StandardEvidenceStatus is parent-visible evidence mix for one standard.
type StandardEvidenceStatus struct {
	StandardID            string   `json:"standardId"`
	StandardCode          string   `json:"standardCode"`
	MasteryRecordID       string   `json:"masteryRecordId,omitempty"`
	MasteryStatus         string   `json:"masteryStatus"`
	Confidence            float64  `json:"confidence"`
	AcceptedEvidenceClasses []string `json:"acceptedEvidenceClasses"`
	MissingEvidenceClasses  []string `json:"missingEvidenceClasses"`
	EvidenceStatus        string   `json:"evidenceStatus"`
	ActivityCompleted     bool     `json:"activityCompleted"`
	ProceduralAccepted    bool     `json:"proceduralAccepted"`
	AdditionalEvidenceRequired bool `json:"additionalEvidenceRequired"`
	FormalMastery         bool     `json:"formalMastery"`
}

// StudentLearningOverview aggregates parent-facing learning state for one student.
type StudentLearningOverview struct {
	Student            domain.Student             `json:"student"`
	Devices            []domain.StudentDevice     `json:"devices"`
	OpenAssignments    []domain.StudentAssignment `json:"openAssignments"`
	RecentSessions     []domain.LearningSession   `json:"recentSessions"`
	MasterySummary     []domain.MasteryRecord     `json:"masterySummary"`
	EvidenceStatuses   []StandardEvidenceStatus   `json:"evidenceStatuses"`
	TutorNotesDisable  bool                       `json:"tutorNotesDisable"`
}

// GetStudentLearningOverview loads devices, open work, recent sessions, and mastery.
func GetStudentLearningOverview(ctx context.Context, q Querier, studentID string, sessionLimit int) (*StudentLearningOverview, error) {
	if sessionLimit <= 0 || sessionLimit > 100 {
		sessionLimit = 20
	}
	st, err := Students.Get(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	devices, err := ListStudentDevices(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	open, err := ListOpenAssignmentsForStudent(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	sessions, err := ListRecentSessionsForStudent(ctx, q, studentID, sessionLimit)
	if err != nil {
		return nil, err
	}
	mastery, err := ListMasteryForStudent(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	evidence, err := ListEvidenceStatusesForStudent(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	if devices == nil {
		devices = []domain.StudentDevice{}
	}
	if open == nil {
		open = []domain.StudentAssignment{}
	}
	if sessions == nil {
		sessions = []domain.LearningSession{}
	}
	if mastery == nil {
		mastery = []domain.MasteryRecord{}
	}
	if evidence == nil {
		evidence = []StandardEvidenceStatus{}
	}
	return &StudentLearningOverview{
		Student:           *st,
		Devices:           devices,
		OpenAssignments:   open,
		RecentSessions:    sessions,
		MasterySummary:    mastery,
		EvidenceStatuses:  evidence,
		TutorNotesDisable: strings.Contains(strings.ToLower(st.Notes), TutorOffMarker),
	}, nil
}

// ListEvidenceStatusesForStudent builds parent-visible accepted/missing evidence classes.
func ListEvidenceStatusesForStudent(ctx context.Context, q Querier, studentID string) ([]StandardEvidenceStatus, error) {
	const sqlStr = `
SELECT mr.id, mr.standard_id, mr.status, mr.confidence,
       s.code AS standard_code,
       COALESCE((
           SELECT array_agg(DISTINCT me.evidence_class ORDER BY me.evidence_class)
           FROM mastery_evidence me
           WHERE me.mastery_record_id = mr.id
             AND me.context NOT ILIKE '%superseded by parent%'
       ), '{}') AS accepted
FROM mastery_records mr
JOIN standards s ON s.id = mr.standard_id
WHERE mr.student_id = $1
ORDER BY mr.updated_at DESC`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("list evidence statuses: %w", err)
	}

	type rowData struct {
		recID, stdID, status, code string
		conf                       float64
		accepted                   []string
	}
	// Collect first so nested policy lookups do not share the open row connection
	// (pgx/single-conn queriers return "conn busy" otherwise).
	var raw []rowData
	for rows.Next() {
		var r rowData
		if err := rows.Scan(&r.recID, &r.stdID, &r.status, &r.conf, &r.code, &r.accepted); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan evidence status: %w", err)
		}
		if r.accepted == nil {
			r.accepted = []string{}
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	// Default terminal policy for missing-class reporting when no link policy is available.
	defaultPol := contracts.DefaultTerminalEvidencePolicy()
	var out []StandardEvidenceStatus
	for _, r := range raw {
		pol := defaultPol
		if strings.Contains(r.code, ".TYPE.") {
			pol = contracts.DefaultTypingEvidencePolicy()
		}
		// Pull the newest revision-standard policy for this standard if present.
		if p, ok, err := latestPolicyForStandard(ctx, q, r.stdID); err == nil && ok {
			pol = p
		} else if err != nil {
			return nil, err
		}
		missing := missingClassesForNext(r.status, r.accepted, pol)
		procedural := containsString(r.accepted, domain.EvidenceProceduralContinuous)
		formal := r.status == "mastered" && len(missingClassesForStatus("mastered", r.accepted, pol)) == 0
		additional := len(missing) > 0 && procedural
		evStatus := EvidenceStatusNotIntroduced
		switch {
		case formal:
			evStatus = EvidenceStatusFormalMastery
		case additional:
			evStatus = EvidenceStatusAdditionalEvidenceReq
		case procedural:
			evStatus = EvidenceStatusProceduralAccepted
		case r.status != "not_introduced":
			evStatus = EvidenceStatusActivityCompleted
		}
		out = append(out, StandardEvidenceStatus{
			StandardID:                 r.stdID,
			StandardCode:               r.code,
			MasteryRecordID:            r.recID,
			MasteryStatus:              r.status,
			Confidence:                 r.conf,
			AcceptedEvidenceClasses:    r.accepted,
			MissingEvidenceClasses:     missing,
			EvidenceStatus:             evStatus,
			ActivityCompleted:          r.status != "not_introduced" || procedural,
			ProceduralAccepted:         procedural,
			AdditionalEvidenceRequired: additional,
			FormalMastery:              formal,
		})
	}
	return out, nil
}

func latestPolicyForStandard(ctx context.Context, q Querier, standardID string) (contracts.EvidencePolicy, bool, error) {
	const sqlStr = `
SELECT evidence_policy
FROM learning_activity_revision_standards
WHERE standard_id = $1
ORDER BY created_at DESC
LIMIT 1`
	var m map[string]any
	err := q.QueryRow(ctx, sqlStr, standardID).Scan(&m)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contracts.EvidencePolicy{}, false, nil
		}
		return contracts.EvidencePolicy{}, false, fmt.Errorf("latest policy: %w", err)
	}
	p, ok := ParseEvidencePolicy(m)
	return p, ok, nil
}

func missingClassesForNext(current string, accepted []string, pol contracts.EvidencePolicy) []string {
	order := []string{"in_progress", "approaching", "mastered"}
	rank := map[string]int{"not_introduced": 0, "": 0, "in_progress": 1, "approaching": 2, "mastered": 3}
	cur := rank[current]
	for _, st := range order {
		if rank[st] <= cur {
			continue
		}
		miss := missingClassesForStatus(st, accepted, pol)
		if len(miss) > 0 {
			return miss
		}
	}
	return []string{}
}

func missingClassesForStatus(status string, accepted []string, pol contracts.EvidencePolicy) []string {
	req := pol.StatusRequirements[status]
	have := map[string]bool{}
	for _, a := range accepted {
		have[a] = true
	}
	var missing []string
	for _, need := range req {
		if !have[need] {
			missing = append(missing, need)
		}
	}
	return missing
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// ListOpenAssignmentsForStudent returns available/in_progress assignments.
func ListOpenAssignmentsForStudent(ctx context.Context, q Querier, studentID string) ([]domain.StudentAssignment, error) {
	const sqlStr = `
SELECT id, student_id, activity_revision_id, enrollment_id, state, priority,
       available_at, due_at, assigned_by, reason, constraints, created_at, updated_at
FROM student_assignments
WHERE student_id = $1 AND state IN ('available', 'in_progress')
ORDER BY priority DESC, available_at ASC, id ASC`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("list open assignments: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.StudentAssignment])
	if err != nil {
		return nil, fmt.Errorf("scan open assignments: %w", err)
	}
	return items, nil
}

// ListRecentSessionsForStudent returns the newest sessions for a student.
func ListRecentSessionsForStudent(ctx context.Context, q Querier, studentID string, limit int) ([]domain.LearningSession, error) {
	const sqlStr = `
SELECT id, assignment_id, student_id, device_id, client_session_id, activity_revision_id,
       state, started_at, last_event_at, completed_at, duration_seconds, summary, created_at, updated_at
FROM learning_sessions
WHERE student_id = $1
ORDER BY started_at DESC
LIMIT $2`
	rows, err := q.Query(ctx, sqlStr, studentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent sessions: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.LearningSession])
	if err != nil {
		return nil, fmt.Errorf("scan recent sessions: %w", err)
	}
	return items, nil
}

// ListMasteryForStudent returns mastery records for a student.
func ListMasteryForStudent(ctx context.Context, q Querier, studentID string) ([]domain.MasteryRecord, error) {
	const sqlStr = `
SELECT id, student_id, standard_id, status, confidence, decay_rate,
       last_assessed_at, last_reinforced_at, next_reinforcement_at, created_at, updated_at
FROM mastery_records
WHERE student_id = $1
ORDER BY updated_at DESC`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("list mastery: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.MasteryRecord])
	if err != nil {
		return nil, fmt.Errorf("scan mastery: %w", err)
	}
	return items, nil
}

// StudentClientMetrics are simple household counters for ops dashboards.
type StudentClientMetrics struct {
	DevicesActive         int `json:"devicesActive"`
	DevicesRevoked        int `json:"devicesRevoked"`
	AssignmentsOpen       int `json:"assignmentsOpen"`
	SessionsActive        int `json:"sessionsActive"`
	CompletionsLast24h    int `json:"completionsLast24h"`
	TutorFailuresLast24h  int `json:"tutorFailuresLast24h"`
}

// GetStudentClientMetrics counts active devices, open work, and recent completions.
// TutorFailuresLast24h is filled by the API layer from the in-process tutor service
// when available; the SQL path always returns 0 for that field.
func GetStudentClientMetrics(ctx context.Context, q Querier, now time.Time) (*StudentClientMetrics, error) {
	since := now.Add(-24 * time.Hour)
	m := &StudentClientMetrics{}

	if err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM student_devices WHERE revoked_at IS NULL`).Scan(&m.DevicesActive); err != nil {
		return nil, fmt.Errorf("count active devices: %w", err)
	}
	if err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM student_devices WHERE revoked_at IS NOT NULL`).Scan(&m.DevicesRevoked); err != nil {
		return nil, fmt.Errorf("count revoked devices: %w", err)
	}
	if err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM student_assignments WHERE state IN ('available', 'in_progress')`).Scan(&m.AssignmentsOpen); err != nil {
		return nil, fmt.Errorf("count open assignments: %w", err)
	}
	if err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM learning_sessions WHERE state IN ('started', 'paused')`).Scan(&m.SessionsActive); err != nil {
		return nil, fmt.Errorf("count active sessions: %w", err)
	}
	if err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM learning_session_completions WHERE created_at >= $1`, since).Scan(&m.CompletionsLast24h); err != nil {
		return nil, fmt.Errorf("count completions: %w", err)
	}
	// Process-local tutor failures are not in SQL; API fills TutorFailuresLast24h.
	return m, nil
}
