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
	// Slug, when set, assigns the latest published revision of that activity
	// (parent-authorized pin/force). When the student has active course
	// enrollments, the slug should belong to an enrolled revision.
	Slug string
	// PreferReinforcement tries reinforcement-due mastery before open curriculum.
	// When slug is empty, defaults to true.
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
	Assignment *domain.StudentAssignment `json:"assignment,omitempty"`
	Reason     string                    `json:"reason"`
	Created    bool                      `json:"created"`
	// BlockReason is set when no assignment could be created.
	BlockReason string `json:"blockReason,omitempty"`
	// Candidates lists deterministic reason codes considered during selection.
	Candidates []string `json:"candidates,omitempty"`
}

// AssignNext picks a published activity revision and creates an assignment.
//
// Selection order:
//  1. explicit authorized parent pin or slug
//  2. required remediation created by returned evidence (course branch)
//  3. due reinforcement eligible within the course (or unsequenced library)
//  4. next eligible course activity
//  5. no assignment, with a clear blocking reason
//
// Global alphabetical fallback is only used when the student has no active
// course enrollments (unsequenced practice library).
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

	courseEns, err := repo.ListCourseEnrollmentsForStudent(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	activeEns := make([]domain.Enrollment, 0, len(courseEns))
	pausedOnly := false
	for _, en := range courseEns {
		if en.Status == "active" {
			activeEns = append(activeEns, en)
		}
	}
	hasCourse := len(activeEns) > 0
	if !hasCourse && len(courseEns) > 0 {
		// Student is enrolled in a course but all such enrollments are paused.
		pausedOnly = true
	}

	var candidates []string
	reason := opts.Reason
	var revID string
	var pickReason string
	var enrollmentID *string
	var membershipID *string

	// Paused course enrollments block new automatic assignments (preserve evidence).
	// Explicit parent slug may still force work when authorized.
	if pausedOnly && opts.Slug == "" {
		return &Result{
			Reason:      BlockEnrollmentPaused,
			BlockReason: BlockEnrollmentPaused,
			Candidates:  []string{BlockEnrollmentPaused},
		}, nil
	}

	// 1. Explicit slug (API force) or enrollment pin.
	if opts.Slug != "" {
		candidates = append(candidates, "slug:"+opts.Slug)
		if hasCourse || pausedOnly {
			ok, en, mem, err := IsSlugInActiveEnrollments(ctx, q, studentID, opts.Slug)
			if err != nil {
				return nil, err
			}
			if !ok {
				return &Result{
					Reason:      "",
					BlockReason: fmt.Sprintf("%s: slug %q not in active course enrollments", BlockNotInRevision, opts.Slug),
					Candidates:  candidates,
				}, nil
			}
			if mem.ActivityRevisionID == nil {
				return nil, repo.ErrBadRequest{Msg: "course membership missing activity revision"}
			}
			revID = *mem.ActivityRevisionID
			pickReason = "slug:" + opts.Slug
			enrollmentID = &en.ID
			membershipID = &mem.ID
		} else {
			id, err := latestPublishedRevisionBySlug(ctx, q, opts.Slug)
			if err != nil {
				return nil, err
			}
			revID = id
			pickReason = "slug:" + opts.Slug
		}
	} else if hasCourse {
		// Parent pin on highest-priority enrollment.
		for i := range activeEns {
			en := activeEns[i]
			if en.PinnedActivitySlug == "" {
				continue
			}
			candidates = append(candidates, "pin:"+en.PinnedActivitySlug)
			ok, _, mem, err := IsSlugInActiveEnrollments(ctx, q, studentID, en.PinnedActivitySlug)
			if err != nil {
				return nil, err
			}
			if !ok || mem == nil || mem.ActivityRevisionID == nil {
				continue
			}
			// Skip if already completed; keep pin visible via block summary otherwise.
			completed, err := repo.CompletedActivitySlugsForStudent(ctx, q, studentID)
			if err != nil {
				return nil, err
			}
			if completed[en.PinnedActivitySlug] {
				continue
			}
			revID = *mem.ActivityRevisionID
			pickReason = "pin:" + en.PinnedActivitySlug
			enrollmentID = &en.ID
			membershipID = &mem.ID
			break
		}
	}

	// 2. Remediation branches for returned conceptual responses (MVP: pending
	// returned responses on course activities open their remediation slug).
	if revID == "" && hasCourse {
		slug, en, mem, why, err := pickRemediation(ctx, q, studentID, activeEns)
		if err != nil {
			return nil, err
		}
		if slug != "" && mem != nil && mem.ActivityRevisionID != nil {
			candidates = append(candidates, why)
			revID = *mem.ActivityRevisionID
			pickReason = why
			enrollmentID = &en.ID
			membershipID = &mem.ID
		}
	}

	// 3. Reinforcement due (constrained to course membership when enrolled).
	if revID == "" && preferReinf {
		id, why, err := pickReinforcementRevision(ctx, q, studentID, now, hasCourse)
		if err != nil {
			return nil, err
		}
		if id != "" {
			candidates = append(candidates, why)
			// Attach enrollment provenance when the reinforcement slug is in-course.
			if hasCourse {
				slug, err := activitySlugForRevision(ctx, q, id)
				if err != nil {
					return nil, err
				}
				ok, en, mem, err := IsSlugInActiveEnrollments(ctx, q, studentID, slug)
				if err != nil {
					return nil, err
				}
				if ok {
					enrollmentID = &en.ID
					membershipID = &mem.ID
					if mem.ActivityRevisionID != nil {
						id = *mem.ActivityRevisionID
					}
				} else {
					// Skip out-of-course reinforcement for enrolled students.
					id = ""
					why = ""
				}
			}
			if id != "" {
				revID = id
				pickReason = why
			}
		}
	}

	// 4. Next eligible course activity, or unsequenced library fallback.
	if revID == "" {
		if hasCourse {
			en, mem, why, err := FirstEligibleCourseActivity(ctx, q, studentID)
			if err != nil {
				return nil, err
			}
			if mem != nil && mem.ActivityRevisionID != nil {
				candidates = append(candidates, why)
				revID = *mem.ActivityRevisionID
				pickReason = why
				if en != nil {
					enrollmentID = &en.ID
				}
				membershipID = &mem.ID
			} else {
				block, err := CollectEnrollmentBlockSummary(ctx, q, studentID)
				if err != nil {
					return nil, err
				}
				if block == "" {
					block = BlockNoEligibleActivity
				}
				candidates = append(candidates, block)
				return &Result{
					Reason:      block,
					BlockReason: block,
					Candidates:  candidates,
				}, nil
			}
		} else {
			id, why, err := pickNextCurriculumRevision(ctx, q, studentID)
			if err != nil {
				return nil, err
			}
			if id == "" {
				return &Result{
					Reason:      BlockNoEligibleActivity,
					BlockReason: "no assignable published activity found",
					Candidates:  candidates,
				}, nil
			}
			candidates = append(candidates, why)
			revID, pickReason = id, why
		}
	}

	if reason == "" {
		reason = pickReason
	}

	if existing, err := openAssignmentForRevision(ctx, q, studentID, revID); err != nil {
		return nil, err
	} else if existing != nil {
		return &Result{Assignment: existing, Reason: "already-open", Created: false, Candidates: candidates}, nil
	}

	asg, err := repo.CreateAssignmentFull(ctx, q, repo.AssignmentCreate{
		StudentID:            studentID,
		ActivityRevisionID:   revID,
		EnrollmentID:         enrollmentID,
		CurriculumActivityID: membershipID,
		SelectionReason:      pickReason,
		AssignedBy:           opts.AssignedBy,
		Priority:             opts.Priority,
		Reason:               reason,
	})
	if err != nil {
		return nil, err
	}
	return &Result{Assignment: asg, Reason: pickReason, Created: true, Candidates: candidates}, nil
}

func pickRemediation(ctx context.Context, q repo.Querier, studentID string, ens []domain.Enrollment) (slug string, en *domain.Enrollment, mem *domain.CurriculumActivity, reason string, err error) {
	// Find returned conceptual responses and map to remediation branches.
	const sqlStr = `
SELECT DISTINCT a.slug
FROM student_responses sr
JOIN learning_activity_revisions r ON r.id = sr.activity_revision_id
JOIN learning_activities a ON a.id = r.activity_id
WHERE sr.student_id = $1 AND sr.status = 'returned'
ORDER BY a.slug`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return "", nil, nil, "", fmt.Errorf("list returned responses: %w", err)
	}
	defer rows.Close()
	var returned []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", nil, nil, "", err
		}
		returned = append(returned, s)
	}
	if err := rows.Err(); err != nil {
		return "", nil, nil, "", err
	}
	completed, err := repo.CompletedActivitySlugsForStudent(ctx, q, studentID)
	if err != nil {
		return "", nil, nil, "", err
	}
	openSlugs, err := repo.OpenAssignmentSlugsForStudent(ctx, q, studentID)
	if err != nil {
		return "", nil, nil, "", err
	}
	for i := range ens {
		if ens[i].CurriculumRevisionID == nil {
			continue
		}
		for _, forSlug := range returned {
			branch, err := RemediationBranchSlug(ctx, q, *ens[i].CurriculumRevisionID, forSlug)
			if err != nil {
				return "", nil, nil, "", err
			}
			if branch == "" || completed[branch] || openSlugs[branch] {
				continue
			}
			ok, enPtr, memPtr, err := IsSlugInActiveEnrollments(ctx, q, studentID, branch)
			if err != nil {
				return "", nil, nil, "", err
			}
			if ok && memPtr != nil {
				return branch, enPtr, memPtr, "remediation:" + branch, nil
			}
			// Branch may be outside membership; resolve latest published slug.
			// Only assign if published.
			_, err = latestPublishedRevisionBySlug(ctx, q, branch)
			if err != nil {
				continue
			}
			// Attach to enrollment even if not in membership list.
			enCopy := ens[i]
			return branch, &enCopy, nil, "remediation:" + branch, nil
		}
	}
	return "", nil, nil, "", nil
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

func pickReinforcementRevision(ctx context.Context, q repo.Querier, studentID string, now time.Time, constrainToCourse bool) (revisionID, reason string, err error) {
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

	try := func(role string) (string, string, error) {
		for _, d := range due {
			id, err := findRevisionForStandard(ctx, q, studentID, d.StandardID, role, preferTyping)
			if err != nil {
				return "", "", err
			}
			if id == "" {
				continue
			}
			if constrainToCourse {
				slug, err := activitySlugForRevision(ctx, q, id)
				if err != nil {
					return "", "", err
				}
				ok, err := slugAllowedForEnrolledStudent(ctx, q, studentID, slug)
				if err != nil {
					return "", "", err
				}
				if !ok {
					continue
				}
			}
			prefix := "reinforcement"
			if role == "" {
				prefix = "reinforcement-any"
			}
			return id, fmt.Sprintf("%s:%s", prefix, d.Code), nil
		}
		return "", "", nil
	}

	if id, why, err := try(contracts.StandardRoleReinforcement); err != nil || id != "" {
		return id, why, err
	}
	return try("")
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
	// Unsequenced practice library only — alphabetical among published activities.
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
	return id, "library:" + slug, nil
}

func openAssignmentForRevision(ctx context.Context, q repo.Querier, studentID, revisionID string) (*domain.StudentAssignment, error) {
	const sqlStr = `
SELECT id, student_id, activity_revision_id, enrollment_id, curriculum_activity_id,
       selection_reason, state, priority,
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
