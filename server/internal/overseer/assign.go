// Package overseer selects the next learning assignment for a student.
package overseer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Options controls AssignNext selection.
type Options struct {
	// Slug, when set, assigns the latest published revision of that activity.
	Slug string
	// PreferReinforcement tries reinforcement-due mastery before open curriculum.
	// When Slug is empty, defaults to true.
	PreferReinforcement *bool
	// AssignedBy is stored on the assignment when non-nil.
	AssignedBy *string
	// Priority for the created assignment.
	Priority int
	// Reason stored on the assignment.
	Reason string
	// Now overrides the clock (tests).
	Now time.Time
}

// Result is the assignment created (or an existing open one when duplicate).
type Result struct {
	Assignment *domain.StudentAssignment
	Reason     string
	Created    bool
}

// AssignNext picks a published activity revision and creates an assignment.
//
// Selection order when reinforcement is preferred (default without slug):
//  1. Mastery due for reinforcement (next_reinforcement_at <= now or
//     status approaching/in_progress), preferring a published activity that
//     links that standard with role=reinforcement. Typing activities are
//     preferred when a TYPE* standard is due.
//  2. Otherwise the next published activity whose latest revision is not
//     already completed or open for the student.
func AssignNext(ctx context.Context, q repo.Querier, studentID string, opts Options) (*Result, error) {
	if _, err := repo.Students.Get(ctx, q, studentID); err != nil {
		return nil, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	preferReinf := true
	if opts.PreferReinforcement != nil {
		preferReinf = *opts.PreferReinforcement
	}
	if opts.Slug != "" {
		preferReinf = false
	}

	reason := opts.Reason
	var revID string
	var pickReason string

	switch {
	case opts.Slug != "":
		id, err := latestPublishedRevisionBySlug(ctx, q, opts.Slug)
		if err != nil {
			return nil, err
		}
		revID = id
		pickReason = "slug:" + opts.Slug
	case preferReinf:
		id, why, err := pickReinforcementRevision(ctx, q, studentID, now)
		if err != nil {
			return nil, err
		}
		revID, pickReason = id, why
	}

	if revID == "" {
		id, why, err := pickNextCurriculumRevision(ctx, q, studentID)
		if err != nil {
			return nil, err
		}
		if id == "" {
			return nil, repo.ErrBadRequest{Msg: "no assignable published activity found"}
		}
		revID, pickReason = id, why
	}
	if reason == "" {
		reason = pickReason
	}

	if existing, err := openAssignmentForRevision(ctx, q, studentID, revID); err != nil {
		return nil, err
	} else if existing != nil {
		return &Result{Assignment: existing, Reason: "already-open", Created: false}, nil
	}

	asg, err := repo.CreateAssignment(ctx, q, studentID, revID, opts.AssignedBy, opts.Priority, reason)
	if err != nil {
		return nil, err
	}
	return &Result{Assignment: asg, Reason: pickReason, Created: true}, nil
}

func latestPublishedRevisionBySlug(ctx context.Context, q repo.Querier, slug string) (string, error) {
	const sqlStr = `
SELECT r.id
FROM learning_activity_revisions r
JOIN learning_activities a ON a.id = r.activity_id
WHERE a.slug = $1
  AND a.status = 'published'
  AND r.published_at IS NOT NULL
ORDER BY r.revision DESC
LIMIT 1`
	var id string
	err := q.QueryRow(ctx, sqlStr, slug).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%w: activity slug %q", repo.ErrNotFound, slug)
		}
		return "", fmt.Errorf("latest revision by slug: %w", err)
	}
	return id, nil
}

func pickReinforcementRevision(ctx context.Context, q repo.Querier, studentID string, now time.Time) (revisionID, reason string, err error) {
	due, err := listDueMastery(ctx, q, studentID, now)
	if err != nil {
		return "", "", err
	}
	if len(due) == 0 {
		return "", "", nil
	}

	preferTyping := false
	for _, d := range due {
		if standardLooksLikeTyping(d.Code) {
			preferTyping = true
			break
		}
	}

	for _, d := range due {
		id, err := findRevisionForStandard(ctx, q, studentID, d.StandardID, contracts.StandardRoleReinforcement, preferTyping)
		if err != nil {
			return "", "", err
		}
		if id != "" {
			return id, fmt.Sprintf("reinforcement:%s", d.Code), nil
		}
	}
	for _, d := range due {
		id, err := findRevisionForStandard(ctx, q, studentID, d.StandardID, "", preferTyping)
		if err != nil {
			return "", "", err
		}
		if id != "" {
			return id, fmt.Sprintf("reinforcement-any:%s", d.Code), nil
		}
	}
	return "", "", nil
}

type dueMastery struct {
	StandardID string
	Code       string
	Status     string
}

func listDueMastery(ctx context.Context, q repo.Querier, studentID string, now time.Time) ([]dueMastery, error) {
	const sqlStr = `
SELECT mr.standard_id, s.code, mr.status
FROM mastery_records mr
JOIN standards s ON s.id = mr.standard_id
WHERE mr.student_id = $1
  AND (
    (mr.next_reinforcement_at IS NOT NULL AND mr.next_reinforcement_at <= $2)
    OR mr.status IN ('approaching', 'in_progress')
  )
ORDER BY
  CASE WHEN mr.next_reinforcement_at IS NOT NULL AND mr.next_reinforcement_at <= $2 THEN 0 ELSE 1 END,
  mr.next_reinforcement_at NULLS LAST,
  CASE mr.status WHEN 'approaching' THEN 0 WHEN 'in_progress' THEN 1 ELSE 2 END,
  s.code`
	rows, err := q.Query(ctx, sqlStr, studentID, now)
	if err != nil {
		return nil, fmt.Errorf("list due mastery: %w", err)
	}
	defer rows.Close()
	var out []dueMastery
	for rows.Next() {
		var d dueMastery
		if err := rows.Scan(&d.StandardID, &d.Code, &d.Status); err != nil {
			return nil, fmt.Errorf("scan due mastery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func standardLooksLikeTyping(code string) bool {
	return strings.Contains(code, "TYPE")
}

func findRevisionForStandard(ctx context.Context, q repo.Querier, studentID, standardID, role string, preferTyping bool) (string, error) {
	kindOrder := `CASE a.kind WHEN 'terminal' THEN 0 ELSE 1 END`
	if preferTyping {
		kindOrder = `CASE a.kind WHEN 'typing' THEN 0 ELSE 1 END`
	}
	roleFilter := ""
	args := []any{studentID, standardID}
	if role != "" {
		roleFilter = "AND lars.role = $3"
		args = append(args, role)
	}
	sqlStr := fmt.Sprintf(`
SELECT r.id
FROM learning_activity_revisions r
JOIN learning_activities a ON a.id = r.activity_id
JOIN learning_activity_revision_standards lars ON lars.activity_revision_id = r.id
WHERE a.status = 'published'
  AND r.published_at IS NOT NULL
  AND lars.standard_id = $2
  %s
  AND r.id = (
    SELECT r2.id FROM learning_activity_revisions r2
    WHERE r2.activity_id = a.id AND r2.published_at IS NOT NULL
    ORDER BY r2.revision DESC LIMIT 1
  )
  AND NOT EXISTS (
    SELECT 1 FROM student_assignments sa
    WHERE sa.student_id = $1
      AND sa.activity_revision_id = r.id
      AND sa.state = 'completed'
  )
ORDER BY %s, a.slug
LIMIT 1`, roleFilter, kindOrder)

	var id string
	err := q.QueryRow(ctx, sqlStr, args...).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("find revision for standard: %w", err)
	}
	return id, nil
}

func pickNextCurriculumRevision(ctx context.Context, q repo.Querier, studentID string) (revisionID, reason string, err error) {
	const sqlStr = `
SELECT r.id, a.slug
FROM learning_activities a
JOIN learning_activity_revisions r ON r.activity_id = a.id
WHERE a.status = 'published'
  AND r.published_at IS NOT NULL
  AND r.revision = (
    SELECT MAX(r2.revision) FROM learning_activity_revisions r2
    WHERE r2.activity_id = a.id AND r2.published_at IS NOT NULL
  )
  AND NOT EXISTS (
    SELECT 1 FROM student_assignments sa
    WHERE sa.student_id = $1
      AND sa.activity_revision_id = r.id
      AND sa.state = 'completed'
  )
  AND NOT EXISTS (
    SELECT 1 FROM student_assignments sa
    WHERE sa.student_id = $1
      AND sa.activity_revision_id = r.id
      AND sa.state IN ('available', 'in_progress')
  )
ORDER BY a.slug ASC
LIMIT 1`
	var id, slug string
	err = q.QueryRow(ctx, sqlStr, studentID).Scan(&id, &slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("pick curriculum revision: %w", err)
	}
	return id, "curriculum:" + slug, nil
}

func openAssignmentForRevision(ctx context.Context, q repo.Querier, studentID, revisionID string) (*domain.StudentAssignment, error) {
	const sqlStr = `
SELECT id, student_id, activity_revision_id, enrollment_id, state, priority,
       available_at, due_at, assigned_by, reason, constraints, created_at, updated_at
FROM student_assignments
WHERE student_id = $1
  AND activity_revision_id = $2
  AND state IN ('available', 'in_progress')
ORDER BY created_at DESC
LIMIT 1`
	rows, err := q.Query(ctx, sqlStr, studentID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("query open assignment: %w", err)
	}
	asg, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.StudentAssignment])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan open assignment: %w", err)
	}
	return &asg, nil
}
