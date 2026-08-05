package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// PublishCourseResult summarizes a course revision publication.
type PublishCourseResult struct {
	Curriculum *domain.Curriculum
	Revision   *domain.CurriculumRevision
	Activities int
	Created    bool
}

// PublishCourseDocument upserts a curriculum identity and publishes an immutable
// revision with ordered activity membership, prerequisites, gates, and remediation.
// Activity revisions must already be published; membership pins the latest published
// revision for each slug when revisionPolicy is latest_published.
func PublishCourseDocument(ctx context.Context, q Querier, doc *contracts.CourseDocument, now time.Time) (*PublishCourseResult, error) {
	if doc == nil {
		return nil, ErrBadRequest{Msg: "course document is required"}
	}
	if err := contracts.ValidateCourseDocument(doc); err != nil {
		return nil, ErrBadRequest{Msg: err.Error()}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	curr, err := ensureCurriculumIdentity(ctx, q, doc)
	if err != nil {
		return nil, err
	}

	// Resolve each activity slug to its latest published revision.
	resolved := make(map[string]string, len(doc.Activities))
	for _, a := range doc.Activities {
		revID, err := latestPublishedActivityRevisionID(ctx, q, a.Slug)
		if err != nil {
			return nil, err
		}
		if revID == "" {
			return nil, ErrBadRequest{Msg: fmt.Sprintf("activity %q has no published revision", a.Slug)}
		}
		resolved[a.Slug] = revID
	}

	nextRev, err := nextCurriculumRevisionNumber(ctx, q, curr.ID)
	if err != nil {
		return nil, err
	}

	docMap, err := courseDocumentMap(doc)
	if err != nil {
		return nil, err
	}
	policy := doc.RevisionPolicy
	if policy == "" {
		policy = contracts.RevisionPolicyLatestPublished
	}
	rev, err := CurriculumRevisions.Create(ctx, q, map[string]any{
		"curriculum_id":   curr.ID,
		"revision":        nextRev,
		"title":           doc.Title,
		"description":     doc.ParentDescription,
		"subject_code":    doc.SubjectCode,
		"version":         firstNonEmpty(doc.Version, "1"),
		"revision_policy": policy,
		"document":        docMap,
		"published_at":    now,
	})
	if err != nil {
		return nil, fmt.Errorf("create curriculum revision: %w", err)
	}

	// Stable order by CourseActivityRef.Order.
	acts := append([]contracts.CourseActivityRef(nil), doc.Activities...)
	sort.Slice(acts, func(i, j int) bool { return acts[i].Order < acts[j].Order })
	for _, a := range acts {
		revID := resolved[a.Slug]
		meta := stringMapAny(a.Metadata)
		cont := ""
		if a.Continuity != nil {
			cont = a.Continuity.Mode
		} else if doc.ContinuityDefaults != nil {
			cont = doc.ContinuityDefaults.Mode
		}
		if _, err := CurriculumActivities.Create(ctx, q, map[string]any{
			"curriculum_revision_id": rev.ID,
			"position":               a.Order,
			"activity_slug":          a.Slug,
			"activity_revision_id":   revID,
			"module":                 a.Module,
			"capstone":               a.Capstone,
			"continuity_mode":        cont,
			"metadata":               meta,
		}); err != nil {
			return nil, fmt.Errorf("create curriculum activity %s: %w", a.Slug, err)
		}
	}

	for _, p := range doc.Prerequisites {
		req := p.Requirement
		if req == "" {
			req = contracts.PrereqCompleted
		}
		for _, reqSlug := range p.Requires {
			if _, err := CurriculumActivityPrerequisites.Create(ctx, q, map[string]any{
				"curriculum_revision_id": rev.ID,
				"activity_slug":          p.Activity,
				"requires_slug":          reqSlug,
				"requirement":            req,
				"description":            p.Description,
			}); err != nil {
				return nil, fmt.Errorf("create prerequisite %s->%s: %w", p.Activity, reqSlug, err)
			}
		}
	}

	for _, g := range doc.Gates {
		stds := g.Standards
		if stds == nil {
			stds = []string{}
		}
		if _, err := CurriculumActivityGates.Create(ctx, q, map[string]any{
			"curriculum_revision_id": rev.ID,
			"activity_slug":          g.Activity,
			"kind":                   g.Kind,
			"standards":              stds,
			"description":            g.Description,
		}); err != nil {
			return nil, fmt.Errorf("create gate %s: %w", g.Activity, err)
		}
	}

	for _, r := range doc.Remediation {
		kind := r.Kind
		if kind == "" {
			kind = "remediation"
		}
		if _, err := CurriculumActivityRemediations.Create(ctx, q, map[string]any{
			"curriculum_revision_id": rev.ID,
			"for_activity_slug":      r.ForActivity,
			"branch_slug":            r.BranchSlug,
			"kind":                   kind,
			"description":            r.Description,
		}); err != nil {
			return nil, fmt.Errorf("create remediation %s: %w", r.ForActivity, err)
		}
	}

	// Keep identity row aligned with latest published course metadata.
	if _, err := Curricula.Update(ctx, q, curr.ID, map[string]any{
		"name":         doc.Title,
		"description":  doc.ParentDescription,
		"subject_code": doc.SubjectCode,
		"status":       "published",
		"approach":     "mastery_based",
	}); err != nil {
		return nil, err
	}
	curr, err = Curricula.Get(ctx, q, curr.ID)
	if err != nil {
		return nil, err
	}

	return &PublishCourseResult{
		Curriculum: curr,
		Revision:   rev,
		Activities: len(acts),
		Created:    true,
	}, nil
}

func ensureCurriculumIdentity(ctx context.Context, q Querier, doc *contracts.CourseDocument) (*domain.Curriculum, error) {
	page, err := Curricula.List(ctx, q, ListParams{
		Limit:   1,
		Filters: map[string]any{"slug": doc.Slug},
	})
	if err != nil {
		return nil, err
	}
	if page.TotalCount > 0 {
		return &page.Items[0], nil
	}
	return Curricula.Create(ctx, q, map[string]any{
		"slug":         doc.Slug,
		"name":         doc.Title,
		"description":  doc.ParentDescription,
		"approach":     "mastery_based",
		"subject_code": doc.SubjectCode,
		"status":       "published",
		"metadata":     map[string]any{"source": "course-document"},
	})
}

func nextCurriculumRevisionNumber(ctx context.Context, q Querier, curriculumID string) (int, error) {
	const sqlStr = `SELECT COALESCE(MAX(revision), 0) + 1 FROM curriculum_revisions WHERE curriculum_id = $1`
	var n int
	if err := q.QueryRow(ctx, sqlStr, curriculumID).Scan(&n); err != nil {
		return 0, fmt.Errorf("next curriculum revision: %w", err)
	}
	return n, nil
}

func latestPublishedActivityRevisionID(ctx context.Context, q Querier, slug string) (string, error) {
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
			return "", nil
		}
		return "", fmt.Errorf("latest activity revision %s: %w", slug, err)
	}
	return id, nil
}

func courseDocumentMap(doc *contracts.CourseDocument) (map[string]any, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal course document: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode course document map: %w", err)
	}
	return m, nil
}

func stringMapAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// LatestCurriculumRevision returns the highest published revision for a curriculum.
func LatestCurriculumRevision(ctx context.Context, q Querier, curriculumID string) (*domain.CurriculumRevision, error) {
	const sqlStr = `
SELECT id, curriculum_id, revision, title, description, subject_code, version,
       revision_policy, document, published_at, created_at
FROM curriculum_revisions
WHERE curriculum_id = $1 AND published_at IS NOT NULL
ORDER BY revision DESC
LIMIT 1`
	rows, err := q.Query(ctx, sqlStr, curriculumID)
	if err != nil {
		return nil, fmt.Errorf("latest curriculum revision: %w", err)
	}
	rev, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.CurriculumRevision])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: curriculum revision", ErrNotFound)
		}
		return nil, fmt.Errorf("scan curriculum revision: %w", err)
	}
	return &rev, nil
}

// GetCurriculumBySlug returns a curriculum identity by slug.
func GetCurriculumBySlug(ctx context.Context, q Querier, slug string) (*domain.Curriculum, error) {
	page, err := Curricula.List(ctx, q, ListParams{
		Limit:   1,
		Filters: map[string]any{"slug": slug},
	})
	if err != nil {
		return nil, err
	}
	if page.TotalCount == 0 {
		return nil, fmt.Errorf("%w: curriculum slug %q", ErrNotFound, slug)
	}
	return &page.Items[0], nil
}

// ListCurriculumActivities returns ordered membership for a revision.
func ListCurriculumActivities(ctx context.Context, q Querier, revisionID string) ([]domain.CurriculumActivity, error) {
	page, err := CurriculumActivities.List(ctx, q, ListParams{
		Limit:   500,
		Sort:    "position",
		Dir:     SortAsc,
		Filters: map[string]any{"curriculum_revision_id": revisionID},
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListCurriculumPrerequisites returns prerequisite edges for a revision.
func ListCurriculumPrerequisites(ctx context.Context, q Querier, revisionID string) ([]domain.CurriculumActivityPrerequisite, error) {
	page, err := CurriculumActivityPrerequisites.List(ctx, q, ListParams{
		Limit:   1000,
		Filters: map[string]any{"curriculum_revision_id": revisionID},
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListCurriculumGates returns gates for a revision.
func ListCurriculumGates(ctx context.Context, q Querier, revisionID string) ([]domain.CurriculumActivityGate, error) {
	page, err := CurriculumActivityGates.List(ctx, q, ListParams{
		Limit:   1000,
		Filters: map[string]any{"curriculum_revision_id": revisionID},
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListCurriculumRemediations returns remediation branches for a revision.
func ListCurriculumRemediations(ctx context.Context, q Querier, revisionID string) ([]domain.CurriculumActivityRemediation, error) {
	page, err := CurriculumActivityRemediations.List(ctx, q, ListParams{
		Limit:   1000,
		Filters: map[string]any{"curriculum_revision_id": revisionID},
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// EnrollStudentInCurriculumRevision creates or resumes an enrollment on a revision.
func EnrollStudentInCurriculumRevision(ctx context.Context, q Querier, studentID, curriculumID, revisionID string, educatorID *string, priority int) (*domain.Enrollment, error) {
	if _, err := Students.Get(ctx, q, studentID); err != nil {
		return nil, err
	}
	if _, err := Curricula.Get(ctx, q, curriculumID); err != nil {
		return nil, err
	}
	rev, err := CurriculumRevisions.Get(ctx, q, revisionID)
	if err != nil {
		return nil, err
	}
	if rev.CurriculumID != curriculumID {
		return nil, ErrBadRequest{Msg: "revision does not belong to curriculum"}
	}
	if rev.PublishedAt == nil {
		return nil, ErrBadRequest{Msg: "curriculum revision is not published"}
	}

	page, err := Enrollments.List(ctx, q, ListParams{
		Limit: 1,
		Filters: map[string]any{
			"student_id":    studentID,
			"curriculum_id": curriculumID,
		},
	})
	if err != nil {
		return nil, err
	}
	if page.TotalCount > 0 {
		en := page.Items[0]
		vals := map[string]any{
			"status":                 "active",
			"curriculum_revision_id": revisionID,
			"priority":               priority,
			"ended_on":               nil,
		}
		updated, err := Enrollments.Update(ctx, q, en.ID, vals)
		if err != nil {
			return nil, err
		}
		if err := AppendEnrollmentAudit(ctx, q, updated.ID, educatorID, "enroll", "resume or re-enroll", map[string]any{
			"curriculumRevisionId": revisionID,
		}); err != nil {
			return nil, err
		}
		return updated, nil
	}

	en, err := Enrollments.Create(ctx, q, map[string]any{
		"student_id":             studentID,
		"curriculum_id":          curriculumID,
		"curriculum_revision_id": revisionID,
		"status":                 "active",
		"priority":               priority,
		"override_slugs":         []string{},
		"blocking_reasons":       []any{},
		"constraints":            map[string]any{},
	})
	if err != nil {
		return nil, err
	}
	if err := AppendEnrollmentAudit(ctx, q, en.ID, educatorID, "enroll", "", map[string]any{
		"curriculumRevisionId": revisionID,
	}); err != nil {
		return nil, err
	}
	return en, nil
}

// SetEnrollmentStatus pauses, resumes, completes, or withdraws an enrollment.
func SetEnrollmentStatus(ctx context.Context, q Querier, enrollmentID, status string, educatorID *string, reason string) (*domain.Enrollment, error) {
	switch status {
	case "active", "paused", "completed", "withdrawn":
	default:
		return nil, ErrBadRequest{Msg: "invalid enrollment status"}
	}
	vals := map[string]any{"status": status}
	if status == "completed" || status == "withdrawn" {
		now := time.Now().UTC()
		vals["ended_on"] = now
	} else {
		vals["ended_on"] = nil
	}
	en, err := Enrollments.Update(ctx, q, enrollmentID, vals)
	if err != nil {
		return nil, err
	}
	action := status
	if status == "active" {
		action = "resume"
	} else if status == "paused" {
		action = "pause"
	} else if status == "completed" {
		action = "complete"
	} else if status == "withdrawn" {
		action = "withdraw"
	}
	if err := AppendEnrollmentAudit(ctx, q, en.ID, educatorID, action, reason, nil); err != nil {
		return nil, err
	}
	return en, nil
}

// PinEnrollmentActivity sets or clears the parent pin on an enrollment.
func PinEnrollmentActivity(ctx context.Context, q Querier, enrollmentID, slug, reason string, educatorID *string) (*domain.Enrollment, error) {
	en, err := Enrollments.Get(ctx, q, enrollmentID)
	if err != nil {
		return nil, err
	}
	if slug != "" {
		if en.CurriculumRevisionID == nil || *en.CurriculumRevisionID == "" {
			return nil, ErrBadRequest{Msg: "enrollment has no curriculum revision"}
		}
		ok, err := membershipExists(ctx, q, *en.CurriculumRevisionID, slug)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrBadRequest{Msg: fmt.Sprintf("activity slug %q is not in enrolled revision", slug)}
		}
	}
	now := time.Now().UTC()
	vals := map[string]any{
		"pinned_activity_slug": slug,
		"pinned_reason":        reason,
	}
	if slug == "" {
		vals["pinned_by"] = nil
		vals["pinned_at"] = nil
		vals["pinned_reason"] = ""
	} else {
		if educatorID != nil {
			vals["pinned_by"] = *educatorID
		}
		vals["pinned_at"] = now
	}
	updated, err := Enrollments.Update(ctx, q, enrollmentID, vals)
	if err != nil {
		return nil, err
	}
	action := "pin"
	if slug == "" {
		action = "unpin"
	}
	if err := AppendEnrollmentAudit(ctx, q, updated.ID, educatorID, action, reason, map[string]any{
		"slug": slug,
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

// OverrideEnrollmentPrereq records an auditable prerequisite override for a slug.
func OverrideEnrollmentPrereq(ctx context.Context, q Querier, enrollmentID, slug, reason string, educatorID *string) (*domain.Enrollment, error) {
	if slug == "" {
		return nil, ErrBadRequest{Msg: "slug is required"}
	}
	if reason == "" {
		return nil, ErrBadRequest{Msg: "override reason is required"}
	}
	en, err := Enrollments.Get(ctx, q, enrollmentID)
	if err != nil {
		return nil, err
	}
	if en.CurriculumRevisionID == nil || *en.CurriculumRevisionID == "" {
		return nil, ErrBadRequest{Msg: "enrollment has no curriculum revision"}
	}
	ok, err := membershipExists(ctx, q, *en.CurriculumRevisionID, slug)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBadRequest{Msg: fmt.Sprintf("activity slug %q is not in enrolled revision", slug)}
	}
	slugs := append([]string{}, en.OverrideSlugs...)
	if !containsString(slugs, slug) {
		slugs = append(slugs, slug)
	}
	updated, err := Enrollments.Update(ctx, q, enrollmentID, map[string]any{
		"override_slugs":  slugs,
		"override_reason": reason,
	})
	if err != nil {
		return nil, err
	}
	if err := AppendEnrollmentAudit(ctx, q, updated.ID, educatorID, "override_prereq", reason, map[string]any{
		"slug": slug,
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

func membershipExists(ctx context.Context, q Querier, revisionID, slug string) (bool, error) {
	const sqlStr = `
SELECT 1 FROM curriculum_activities
WHERE curriculum_revision_id = $1 AND activity_slug = $2
LIMIT 1`
	var one int
	err := q.QueryRow(ctx, sqlStr, revisionID, slug).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// AppendEnrollmentAudit writes an enrollment audit event.
func AppendEnrollmentAudit(ctx context.Context, q Querier, enrollmentID string, educatorID *string, action, reason string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	values := map[string]any{
		"enrollment_id": enrollmentID,
		"action":        action,
		"reason":        reason,
		"detail":        detail,
	}
	if educatorID != nil {
		values["educator_id"] = *educatorID
	}
	_, err := EnrollmentAuditEvents.Create(ctx, q, values)
	return err
}

// ListActiveEnrollmentsForStudent returns active course-backed enrollments ordered by priority.
func ListActiveEnrollmentsForStudent(ctx context.Context, q Querier, studentID string) ([]domain.Enrollment, error) {
	return listCourseEnrollments(ctx, q, studentID, "active")
}

// ListCourseEnrollmentsForStudent returns course-backed enrollments in active or paused state.
// Used by AssignNext to avoid leaking global library work while a course is paused.
func ListCourseEnrollmentsForStudent(ctx context.Context, q Querier, studentID string) ([]domain.Enrollment, error) {
	page, err := Enrollments.List(ctx, q, ListParams{
		Limit:   100,
		Sort:    "priority",
		Dir:     SortDesc,
		Filters: map[string]any{"student_id": studentID},
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Enrollment, 0, len(page.Items))
	for _, en := range page.Items {
		if en.CurriculumRevisionID == nil || *en.CurriculumRevisionID == "" {
			continue
		}
		if en.Status != "active" && en.Status != "paused" {
			continue
		}
		out = append(out, en)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func listCourseEnrollments(ctx context.Context, q Querier, studentID, status string) ([]domain.Enrollment, error) {
	page, err := Enrollments.List(ctx, q, ListParams{
		Limit:   100,
		Sort:    "priority",
		Dir:     SortDesc,
		Filters: map[string]any{"student_id": studentID, "status": status},
	})
	if err != nil {
		return nil, err
	}
	// Prefer enrollments that have an explicit revision (course-backed).
	out := make([]domain.Enrollment, 0, len(page.Items))
	for _, en := range page.Items {
		if en.CurriculumRevisionID != nil && *en.CurriculumRevisionID != "" {
			out = append(out, en)
		}
	}
	// Keep deterministic secondary order by created_at when priorities tie.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// UpdateEnrollmentBlockingReasons stores the latest eligibility block summary.
func UpdateEnrollmentBlockingReasons(ctx context.Context, q Querier, enrollmentID string, reasons []any) error {
	if reasons == nil {
		reasons = []any{}
	}
	_, err := Enrollments.Update(ctx, q, enrollmentID, map[string]any{
		"blocking_reasons": reasons,
	})
	return err
}

// CompletedActivitySlugsForStudent returns slugs with a completed assignment for the student.
func CompletedActivitySlugsForStudent(ctx context.Context, q Querier, studentID string) (map[string]bool, error) {
	const sqlStr = `
SELECT DISTINCT a.slug
FROM student_assignments sa
JOIN learning_activity_revisions r ON r.id = sa.activity_revision_id
JOIN learning_activities a ON a.id = r.activity_id
WHERE sa.student_id = $1 AND sa.state = 'completed'`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("completed activity slugs: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out[slug] = true
	}
	return out, rows.Err()
}

// OpenAssignmentSlugsForStudent returns slugs with an open assignment.
func OpenAssignmentSlugsForStudent(ctx context.Context, q Querier, studentID string) (map[string]bool, error) {
	const sqlStr = `
SELECT DISTINCT a.slug
FROM student_assignments sa
JOIN learning_activity_revisions r ON r.id = sa.activity_revision_id
JOIN learning_activities a ON a.id = r.activity_id
WHERE sa.student_id = $1 AND sa.state IN ('available', 'in_progress')`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("open assignment slugs: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out[slug] = true
	}
	return out, rows.Err()
}

// PendingParentReviewSlugs returns activity slugs with submitted conceptual responses awaiting review.
func PendingParentReviewSlugs(ctx context.Context, q Querier, studentID string) (map[string]bool, error) {
	const sqlStr = `
SELECT DISTINCT a.slug
FROM student_responses sr
JOIN learning_activity_revisions r ON r.id = sr.activity_revision_id
JOIN learning_activities a ON a.id = r.activity_id
WHERE sr.student_id = $1
  AND sr.status = 'submitted'
  AND sr.parent_review_required = TRUE`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("pending parent reviews: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out[slug] = true
	}
	return out, rows.Err()
}

// MasteryStatusByStandardCode returns student mastery status keyed by standard code.
func MasteryStatusByStandardCode(ctx context.Context, q Querier, studentID string) (map[string]string, error) {
	const sqlStr = `
SELECT s.code, mr.status
FROM mastery_records mr
JOIN standards s ON s.id = mr.standard_id
WHERE mr.student_id = $1`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("mastery by code: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var code, status string
		if err := rows.Scan(&code, &status); err != nil {
			return nil, err
		}
		out[code] = status
	}
	return out, rows.Err()
}

// HasOpenAssignmentForRevision reports whether student already has open work on a revision.
func HasOpenAssignmentForRevision(ctx context.Context, q Querier, studentID, revisionID string) (bool, error) {
	const sqlStr = `
SELECT 1 FROM student_assignments
WHERE student_id = $1 AND activity_revision_id = $2 AND state IN ('available', 'in_progress')
LIMIT 1`
	var one int
	err := q.QueryRow(ctx, sqlStr, studentID, revisionID).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListEnrollmentAudit returns recent audit events for an enrollment.
func ListEnrollmentAudit(ctx context.Context, q Querier, enrollmentID string, limit int) ([]domain.EnrollmentAuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	page, err := EnrollmentAuditEvents.List(ctx, q, ListParams{
		Limit:   limit,
		Sort:    "created_at",
		Dir:     SortDesc,
		Filters: map[string]any{"enrollment_id": enrollmentID},
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}
