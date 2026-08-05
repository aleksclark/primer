package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// ResponseListItem is a parent-queue row for conceptual responses.
type ResponseListItem struct {
	Response       domain.StudentResponse `json:"response"`
	StudentName    string                 `json:"studentName"`
	ActivitySlug   string                 `json:"activitySlug"`
	ActivityTitle  string                 `json:"activityTitle"`
	TaskTitle      string                 `json:"taskTitle,omitempty"`
	ReviewRequired bool                   `json:"reviewRequired"`
}

// ResponseDetail is the parent review surface for one submission.
type ResponseDetail struct {
	Response      domain.StudentResponse        `json:"response"`
	Reviews       []domain.StudentResponseReview `json:"reviews"`
	Student       domain.Student                `json:"student"`
	ActivitySlug  string                        `json:"activitySlug"`
	ActivityTitle string                        `json:"activityTitle"`
	Task          *contracts.Task               `json:"task,omitempty"`
	// Blocks excludes parent_note (parents still get the prompt/rubric/student body).
	StudentBlocks []contracts.InstructionBlock `json:"studentBlocks,omitempty"`
	// ParentNotes are authoring parent_note blocks for the reviewer only.
	ParentNotes []contracts.InstructionBlock `json:"parentNotes,omitempty"`
}

// SubmitStudentResponse stores an idempotent conceptual response and returns the row.
// Mastery evidence is applied by the caller (mastery package) after insert.
func SubmitStudentResponse(ctx context.Context, q Querier, device *domain.StudentDevice, sessionID string, req contracts.ResponseSubmission, now time.Time) (*domain.StudentResponse, bool, error) {
	if device == nil {
		return nil, false, ErrBadRequest{Msg: "device required"}
	}
	if req.SubmissionID == "" {
		return nil, false, ErrBadRequest{Msg: "submissionId is required"}
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return nil, false, ErrBadRequest{Msg: "taskId is required"}
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, false, ErrBadRequest{Msg: "body is required"}
	}
	if utf8.RuneCountInString(body) > contracts.MaxResponseMaxChars {
		return nil, false, ErrBadRequest{Msg: fmt.Sprintf("body exceeds %d characters", contracts.MaxResponseMaxChars)}
	}
	// Never accept obvious tutor/system paste markers as student evidence.
	if looksLikeTutorAuthored(body) {
		return nil, false, ErrBadRequest{Msg: "body must be the student's own response"}
	}

	var out *domain.StudentResponse
	var created bool
	err := WithTx(ctx, q, func(tx Querier) error {
		// Idempotent by submission UUID.
		if existing, err := GetResponseBySubmissionID(ctx, tx, req.SubmissionID); err == nil {
			digest := responseDigest(req.TaskID, body)
			if existing.BodySHA256 != hashBody(body) && existing.RequestDigest != digest {
				return fmt.Errorf("%w: submission id reused with different body", ErrConflict)
			}
			out = existing
			created = false
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}

		sess, err := GetSession(ctx, tx, sessionID)
		if err != nil {
			return err
		}
		if sess.StudentID != device.StudentID {
			return ErrNotFound
		}
		if sess.State == domain.SessionAbandoned {
			return ErrBadRequest{Msg: "session is abandoned"}
		}

		asg, err := StudentAssignments.Get(ctx, tx, sess.AssignmentID)
		if err != nil {
			return err
		}
		if asg.State == domain.AssignmentCancelled {
			return ErrBadRequest{Msg: "assignment is cancelled"}
		}

		rev, err := GetRevision(ctx, tx, sess.ActivityRevisionID)
		if err != nil {
			return err
		}
		content, err := DecodeActivityContent(rev.Content)
		if err != nil {
			return err
		}
		task, ok := findTask(content.Tasks, req.TaskID)
		if !ok {
			return ErrBadRequest{Msg: "unknown taskId for this revision"}
		}
		if contracts.TaskKindOrDefault(task) != contracts.TaskKindShortResponse || task.Response == nil {
			return ErrBadRequest{Msg: "task is not a short_response task"}
		}
		maxChars := task.Response.MaxChars
		if maxChars <= 0 {
			maxChars = contracts.DefaultResponseMaxChars
		}
		if utf8.RuneCountInString(body) > maxChars {
			return ErrBadRequest{Msg: fmt.Sprintf("body exceeds task maxChars (%d)", maxChars)}
		}

		attempt, err := nextResponseAttempt(ctx, tx, sess.AssignmentID, req.TaskID)
		if err != nil {
			return err
		}
		// Prior submitted (unreviewed) response for same task blocks a new attempt.
		if open, err := latestOpenResponse(ctx, tx, sess.AssignmentID, req.TaskID); err == nil && open != nil {
			return ErrBadRequest{Msg: "a response is already awaiting review; revise only after return"}
		} else if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}

		rubric := rubricSnapshot(task.Response.Rubric)
		digest := req.RequestDigest
		if digest == "" {
			digest = responseDigest(req.TaskID, body)
		}
		bodyHash := hashBody(body)

		const ins = `
INSERT INTO student_responses (
    submission_id, student_id, session_id, assignment_id, activity_revision_id,
    task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
    parent_review_required, submitted_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
)
RETURNING id, submission_id, student_id, session_id, assignment_id, activity_revision_id,
          task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
          parent_review_required, submitted_at, reviewed_at, reviewed_by, return_reason,
          created_at, updated_at`
		row := tx.QueryRow(ctx, ins,
			req.SubmissionID, device.StudentID, sess.ID, sess.AssignmentID, sess.ActivityRevisionID,
			req.TaskID, body, bodyHash, domain.ResponseSubmitted, digest, attempt, rubric,
			task.Response.ParentReviewRequired, now.UTC(),
		)
		rec, err := scanStudentResponse(row)
		if err != nil {
			return fmt.Errorf("insert student response: %w", err)
		}
		out = rec
		created = true

		// Audit event (server-side).
		_ = InsertServerSessionEvent(ctx, tx, sess.ID, contracts.EventResponseSubmitted, map[string]any{
			"submissionId": req.SubmissionID,
			"responseId":   rec.ID,
			"taskId":       req.TaskID,
			"attempt":      attempt,
			"bodySha256":   bodyHash,
			"source":       "student",
		}, now)
		return nil
	})
	return out, created, err
}

// GetResponseBySubmissionID loads by client submission UUID.
func GetResponseBySubmissionID(ctx context.Context, q Querier, submissionID string) (*domain.StudentResponse, error) {
	const sqlStr = `
SELECT id, submission_id, student_id, session_id, assignment_id, activity_revision_id,
       task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
       parent_review_required, submitted_at, reviewed_at, reviewed_by, return_reason,
       created_at, updated_at
FROM student_responses WHERE submission_id = $1`
	rec, err := scanStudentResponse(q.QueryRow(ctx, sqlStr, submissionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rec, err
}

// GetResponse loads by primary key.
func GetResponse(ctx context.Context, q Querier, id string) (*domain.StudentResponse, error) {
	const sqlStr = `
SELECT id, submission_id, student_id, session_id, assignment_id, activity_revision_id,
       task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
       parent_review_required, submitted_at, reviewed_at, reviewed_by, return_reason,
       created_at, updated_at
FROM student_responses WHERE id = $1`
	rec, err := scanStudentResponse(q.QueryRow(ctx, sqlStr, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rec, err
}

// ListResponsesForReview lists responses needing parent attention (submitted) or filtered by student/status.
func ListResponsesForReview(ctx context.Context, q Querier, studentID, status string, limit int) ([]ResponseListItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := []string{"1=1"}
	if studentID != "" {
		args = append(args, studentID)
		where = append(where, fmt.Sprintf("r.student_id = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("r.status = $%d", len(args)))
	} else {
		// Default queue: awaiting decision.
		where = append(where, "r.status = 'submitted'")
	}
	args = append(args, limit)
	sqlStr := fmt.Sprintf(`
SELECT r.id, r.submission_id, r.student_id, r.session_id, r.assignment_id, r.activity_revision_id,
       r.task_id, r.body, r.body_sha256, r.status, r.request_digest, r.attempt, r.rubric_snapshot,
       r.parent_review_required, r.submitted_at, r.reviewed_at, r.reviewed_by, r.return_reason,
       r.created_at, r.updated_at,
       s.first_name || ' ' || s.last_name AS student_name,
       COALESCE(a.slug, '') AS activity_slug,
       COALESCE(a.title, '') AS activity_title
FROM student_responses r
JOIN students s ON s.id = r.student_id
JOIN learning_activity_revisions rev ON rev.id = r.activity_revision_id
JOIN learning_activities a ON a.id = rev.activity_id
WHERE %s
ORDER BY r.submitted_at ASC
LIMIT $%d`, strings.Join(where, " AND "), len(args))

	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list responses: %w", err)
	}
	defer rows.Close()
	var out []ResponseListItem
	for rows.Next() {
		var item ResponseListItem
		var rubric []byte
		if err := rows.Scan(
			&item.Response.ID, &item.Response.SubmissionID, &item.Response.StudentID, &item.Response.SessionID,
			&item.Response.AssignmentID, &item.Response.ActivityRevisionID, &item.Response.TaskID, &item.Response.Body,
			&item.Response.BodySHA256, &item.Response.Status, &item.Response.RequestDigest, &item.Response.Attempt,
			&rubric, &item.Response.ParentReviewRequired, &item.Response.SubmittedAt, &item.Response.ReviewedAt,
			&item.Response.ReviewedBy, &item.Response.ReturnReason, &item.Response.CreatedAt, &item.Response.UpdatedAt,
			&item.StudentName, &item.ActivitySlug, &item.ActivityTitle,
		); err != nil {
			return nil, err
		}
		item.Response.RubricSnapshot = decodeRubric(rubric)
		item.ReviewRequired = item.Response.ParentReviewRequired || item.Response.Status == domain.ResponseSubmitted
		// Task title best-effort from revision content.
		if rev, err := GetRevision(ctx, q, item.Response.ActivityRevisionID); err == nil {
			if content, err := DecodeActivityContent(rev.Content); err == nil {
				if t, ok := findTask(content.Tasks, item.Response.TaskID); ok {
					item.TaskTitle = t.Title
				}
			}
		}
		out = append(out, item)
	}
	if out == nil {
		out = []ResponseListItem{}
	}
	return out, rows.Err()
}

// GetResponseDetail loads response + reviews + activity context for parent review.
func GetResponseDetail(ctx context.Context, q Querier, responseID string) (*ResponseDetail, error) {
	resp, err := GetResponse(ctx, q, responseID)
	if err != nil {
		return nil, err
	}
	st, err := Students.Get(ctx, q, resp.StudentID)
	if err != nil {
		return nil, err
	}
	rev, err := GetRevision(ctx, q, resp.ActivityRevisionID)
	if err != nil {
		return nil, err
	}
	act, err := LearningActivities.Get(ctx, q, rev.ActivityID)
	if err != nil {
		return nil, err
	}
	content, err := DecodeActivityContent(rev.Content)
	if err != nil {
		return nil, err
	}
	detail := &ResponseDetail{
		Response:      *resp,
		Student:       *st,
		ActivitySlug:  act.Slug,
		ActivityTitle: act.Title,
		StudentBlocks: contracts.StudentBlocks(content.Blocks),
	}
	if t, ok := findTask(content.Tasks, resp.TaskID); ok {
		tt := t
		detail.Task = &tt
	}
	for _, b := range content.Blocks {
		if b.Kind == contracts.BlockParentNote {
			detail.ParentNotes = append(detail.ParentNotes, b)
		}
	}
	reviews, err := ListResponseReviews(ctx, q, responseID)
	if err != nil {
		return nil, err
	}
	detail.Reviews = reviews
	return detail, nil
}

// ListResponseReviews returns review decisions oldest-first.
func ListResponseReviews(ctx context.Context, q Querier, responseID string) ([]domain.StudentResponseReview, error) {
	const sqlStr = `
SELECT id, response_id, educator_id, decision, reason, criteria, created_at
FROM student_response_reviews
WHERE response_id = $1
ORDER BY created_at ASC`
	rows, err := q.Query(ctx, sqlStr, responseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StudentResponseReview
	for rows.Next() {
		var r domain.StudentResponseReview
		var crit []byte
		if err := rows.Scan(&r.ID, &r.ResponseID, &r.EducatorID, &r.Decision, &r.Reason, &crit, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Criteria = decodeRubric(crit)
		out = append(out, r)
	}
	if out == nil {
		out = []domain.StudentResponseReview{}
	}
	return out, rows.Err()
}

// ReviewDecisionInput is the parent accept/return payload.
type ReviewDecisionInput struct {
	Decision string           `json:"decision"`
	Reason   string           `json:"reason,omitempty"`
	Criteria []map[string]any `json:"criteria,omitempty"`
}

// ApplyResponseReview records a parent decision. Idempotent when the response
// is already in the target status with the same decision on the latest review.
func ApplyResponseReview(ctx context.Context, q Querier, educatorID, responseID string, in ReviewDecisionInput, now time.Time) (*domain.StudentResponse, *domain.StudentResponseReview, error) {
	decision := strings.TrimSpace(in.Decision)
	if decision != domain.ReviewAccept && decision != domain.ReviewReturn {
		return nil, nil, ErrBadRequest{Msg: "decision must be accept or return"}
	}
	if decision == domain.ReviewReturn && strings.TrimSpace(in.Reason) == "" {
		return nil, nil, ErrBadRequest{Msg: "return requires a reason"}
	}

	var resp *domain.StudentResponse
	var review *domain.StudentResponseReview
	err := WithTx(ctx, q, func(tx Querier) error {
		cur, err := GetResponse(ctx, tx, responseID)
		if err != nil {
			return err
		}
		// Idempotent: already decided the same way.
		if cur.Status == domain.ResponseAccepted && decision == domain.ReviewAccept {
			reviews, err := ListResponseReviews(ctx, tx, responseID)
			if err != nil {
				return err
			}
			resp = cur
			if len(reviews) > 0 {
				r := reviews[len(reviews)-1]
				review = &r
			}
			return nil
		}
		if cur.Status == domain.ResponseReturned && decision == domain.ReviewReturn {
			reviews, err := ListResponseReviews(ctx, tx, responseID)
			if err != nil {
				return err
			}
			resp = cur
			if len(reviews) > 0 {
				r := reviews[len(reviews)-1]
				review = &r
			}
			return nil
		}
		if cur.Status != domain.ResponseSubmitted {
			return ErrBadRequest{Msg: "response is not awaiting review"}
		}

		newStatus := domain.ResponseAccepted
		if decision == domain.ReviewReturn {
			newStatus = domain.ResponseReturned
		}
		critJSON, _ := json.Marshal(in.Criteria)
		if in.Criteria == nil {
			critJSON = []byte("[]")
		}
		const ins = `
INSERT INTO student_response_reviews (response_id, educator_id, decision, reason, criteria)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, response_id, educator_id, decision, reason, criteria, created_at`
		var rev domain.StudentResponseReview
		var critBytes []byte
		if err := tx.QueryRow(ctx, ins, responseID, educatorID, decision, strings.TrimSpace(in.Reason), critJSON).
			Scan(&rev.ID, &rev.ResponseID, &rev.EducatorID, &rev.Decision, &rev.Reason, &critBytes, &rev.CreatedAt); err != nil {
			return fmt.Errorf("insert review: %w", err)
		}
		rev.Criteria = decodeRubric(critBytes)
		review = &rev

		const upd = `
UPDATE student_responses
SET status = $2, reviewed_at = $3, reviewed_by = $4, return_reason = $5, updated_at = now()
WHERE id = $1
RETURNING id, submission_id, student_id, session_id, assignment_id, activity_revision_id,
          task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
          parent_review_required, submitted_at, reviewed_at, reviewed_by, return_reason,
          created_at, updated_at`
		reason := ""
		if decision == domain.ReviewReturn {
			reason = strings.TrimSpace(in.Reason)
		}
		updated, err := scanStudentResponse(tx.QueryRow(ctx, upd, responseID, newStatus, now.UTC(), educatorID, reason))
		if err != nil {
			return err
		}
		resp = updated

		evType := contracts.EventResponseReturned
		if decision == domain.ReviewAccept {
			// Reuse submitted event type with decision metadata until a dedicated
			// accepted audit type exists; never mis-label accept as a new submission.
			evType = contracts.EventResponseSubmitted
		}
		_ = InsertServerSessionEvent(ctx, tx, cur.SessionID, evType, map[string]any{
			"responseId": responseID,
			"decision":   decision,
			"educatorId": educatorID,
			"reason":     reason,
			"source":     "parent",
		}, now)
		return nil
	})
	return resp, review, err
}

// ListSubmittedTaskIDs returns task IDs with a non-returned submitted/accepted response for a session.
func ListSubmittedTaskIDs(ctx context.Context, q Querier, sessionID string) (map[string]bool, error) {
	const sqlStr = `
SELECT DISTINCT task_id FROM student_responses
WHERE session_id = $1 AND status IN ('submitted', 'accepted')`
	rows, err := q.Query(ctx, sqlStr, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// DecodeActivityContent unmarshals revision content JSONB into ActivityContent.
func DecodeActivityContent(m map[string]any) (contracts.ActivityContent, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return contracts.ActivityContent{}, err
	}
	var c contracts.ActivityContent
	if err := json.Unmarshal(raw, &c); err != nil {
		return contracts.ActivityContent{}, err
	}
	return c, nil
}

func findTask(tasks []contracts.Task, id string) (contracts.Task, bool) {
	for _, t := range tasks {
		if t.ID == id {
			return t, true
		}
	}
	return contracts.Task{}, false
}

func nextResponseAttempt(ctx context.Context, q Querier, assignmentID, taskID string) (int, error) {
	const sqlStr = `SELECT COALESCE(MAX(attempt), 0) FROM student_responses WHERE assignment_id = $1 AND task_id = $2`
	var max int
	if err := q.QueryRow(ctx, sqlStr, assignmentID, taskID).Scan(&max); err != nil {
		return 0, err
	}
	return max + 1, nil
}

func latestOpenResponse(ctx context.Context, q Querier, assignmentID, taskID string) (*domain.StudentResponse, error) {
	const sqlStr = `
SELECT id, submission_id, student_id, session_id, assignment_id, activity_revision_id,
       task_id, body, body_sha256, status, request_digest, attempt, rubric_snapshot,
       parent_review_required, submitted_at, reviewed_at, reviewed_by, return_reason,
       created_at, updated_at
FROM student_responses
WHERE assignment_id = $1 AND task_id = $2 AND status = 'submitted'
ORDER BY attempt DESC LIMIT 1`
	rec, err := scanStudentResponse(q.QueryRow(ctx, sqlStr, assignmentID, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rec, err
}

func scanStudentResponse(row pgx.Row) (*domain.StudentResponse, error) {
	var r domain.StudentResponse
	var rubric []byte
	err := row.Scan(
		&r.ID, &r.SubmissionID, &r.StudentID, &r.SessionID, &r.AssignmentID, &r.ActivityRevisionID,
		&r.TaskID, &r.Body, &r.BodySHA256, &r.Status, &r.RequestDigest, &r.Attempt, &rubric,
		&r.ParentReviewRequired, &r.SubmittedAt, &r.ReviewedAt, &r.ReviewedBy, &r.ReturnReason,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	r.RubricSnapshot = decodeRubric(rubric)
	return &r, nil
}

func rubricSnapshot(criteria []contracts.RubricCriterion) []byte {
	type row struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Required    bool   `json:"required,omitempty"`
	}
	rows := make([]row, 0, len(criteria))
	for _, c := range criteria {
		rows = append(rows, row{ID: c.ID, Description: c.Description, Required: c.Required})
	}
	b, _ := json.Marshal(rows)
	return b
}

func decodeRubric(b []byte) []map[string]any {
	if len(b) == 0 {
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func hashBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func responseDigest(taskID, body string) string {
	sum := sha256.Sum256([]byte(taskID + "\n" + body))
	return hex.EncodeToString(sum[:])
}

func looksLikeTutorAuthored(body string) bool {
	lower := strings.ToLower(body)
	// Lightweight guardrails: explicit markers that coaching text was pasted.
	markers := []string{
		"[tutor]",
		"as your tutor",
		"i am an ai",
		"generated by the tutor",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
