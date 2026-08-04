package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// LearningActivities is the activity authoring repository.
var LearningActivities = NewResource[domain.LearningActivity](ListConfig{
	Table:             "learning_activities",
	SearchColumns:     []string{"slug", "title", "summary"},
	SortableColumns:   []string{"slug", "title", "kind", "status", "created_at", "updated_at"},
	FilterableColumns: []string{"kind", "status", "subject_id", "slug"},
})

// LearningActivityRevisions is the immutable revision repository.
var LearningActivityRevisions = NewResource[domain.LearningActivityRevision](ListConfig{
	Table:             "learning_activity_revisions",
	SortableColumns:   []string{"revision", "published_at", "created_at"},
	FilterableColumns: []string{"activity_id", "content_sha256", "schema_version"},
})

// StudentAssignments is the assignment repository.
var StudentAssignments = NewResource[domain.StudentAssignment](ListConfig{
	Table:             "student_assignments",
	SortableColumns:   []string{"priority", "available_at", "due_at", "state", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id", "activity_revision_id", "state", "assigned_by"},
})

// StudentDevices is the student device repository.
var StudentDevices = NewResource[domain.StudentDevice](ListConfig{
	Table:             "student_devices",
	SearchColumns:     []string{"name"},
	SortableColumns:   []string{"name", "last_seen_at", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id"},
})

// LearningSessions is the learning session repository.
var LearningSessions = NewResource[domain.LearningSession](ListConfig{
	Table:             "learning_sessions",
	SortableColumns:   []string{"started_at", "last_event_at", "completed_at", "state", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id", "assignment_id", "device_id", "state"},
})

// ContentSHA256 returns the hex SHA-256 of canonical JSON for content.
func ContentSHA256(content any) (string, []byte, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return "", nil, fmt.Errorf("marshal content: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

// PublishActivityRevision upserts an activity by slug and appends a revision
// when the content digest is new. Idempotent on (slug, content_sha256).
func PublishActivityRevision(ctx context.Context, q Querier, doc *contracts.ActivityDocument, subjectID *string, standardIDs map[string]string, now time.Time) (*domain.LearningActivity, *domain.LearningActivityRevision, error) {
	if doc == nil {
		return nil, nil, ErrBadRequest{Msg: "activity document is required"}
	}
	digest, contentRaw, err := ContentSHA256(doc.Content)
	if err != nil {
		return nil, nil, err
	}
	var contentMap map[string]any
	if err := json.Unmarshal(contentRaw, &contentMap); err != nil {
		return nil, nil, fmt.Errorf("decode content map: %w", err)
	}

	var activity *domain.LearningActivity
	var revision *domain.LearningActivityRevision
	err = WithTx(ctx, q, func(tx Querier) error {
		act, err := activityBySlug(ctx, tx, doc.Slug)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if errors.Is(err, ErrNotFound) {
			values := map[string]any{
				"slug":    doc.Slug,
				"title":   doc.Title,
				"summary": doc.Summary,
				"kind":    doc.Kind,
				"status":  domain.ActivityStatusPublished,
			}
			if subjectID != nil {
				values["subject_id"] = *subjectID
			}
			act, err = LearningActivities.Create(ctx, tx, values)
			if err != nil {
				return err
			}
		} else {
			act, err = LearningActivities.Update(ctx, tx, act.ID, map[string]any{
				"title":   doc.Title,
				"summary": doc.Summary,
				"kind":    doc.Kind,
				"status":  domain.ActivityStatusPublished,
				"subject_id": subjectID,
			})
			if err != nil {
				return err
			}
		}
		activity = act

		existing, err := revisionByDigest(ctx, tx, act.ID, digest)
		if err == nil {
			revision = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		next, err := nextRevisionNumber(ctx, tx, act.ID)
		if err != nil {
			return err
		}
		rev, err := LearningActivityRevisions.Create(ctx, tx, map[string]any{
			"activity_id":     act.ID,
			"revision":        next,
			"schema_version":  doc.SchemaVersion,
			"content":         contentMap,
			"content_sha256":  digest,
			"published_at":    now,
		})
		if err != nil {
			return err
		}
		for _, ref := range doc.Standards {
			stdID, ok := standardIDs[ref.Code]
			if !ok {
				return ErrBadRequest{Msg: "unknown standard code: " + ref.Code}
			}
			weight := ref.Weight
			if weight == 0 {
				weight = 1
			}
			role := ref.Role
			if role == "" {
				role = contracts.StandardRolePrimary
			}
			if _, err := qCreateRevisionStandard(ctx, tx, rev.ID, stdID, role, weight); err != nil {
				return err
			}
		}
		revision = rev
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return activity, revision, nil
}

func activityBySlug(ctx context.Context, q Querier, slug string) (*domain.LearningActivity, error) {
	const sqlStr = `
SELECT id, slug, title, summary, kind, subject_id, status, created_at, updated_at
FROM learning_activities WHERE slug = $1`
	rows, err := q.Query(ctx, sqlStr, slug)
	if err != nil {
		return nil, fmt.Errorf("query activity by slug: %w", err)
	}
	act, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningActivity])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan activity: %w", err)
	}
	return &act, nil
}

func revisionByDigest(ctx context.Context, q Querier, activityID, digest string) (*domain.LearningActivityRevision, error) {
	const sqlStr = `
SELECT id, activity_id, revision, schema_version, content, content_sha256, published_at, created_at
FROM learning_activity_revisions
WHERE activity_id = $1 AND content_sha256 = $2`
	rows, err := q.Query(ctx, sqlStr, activityID, digest)
	if err != nil {
		return nil, fmt.Errorf("query revision by digest: %w", err)
	}
	rev, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningActivityRevision])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan revision: %w", err)
	}
	return &rev, nil
}

func nextRevisionNumber(ctx context.Context, q Querier, activityID string) (int, error) {
	var max *int
	err := q.QueryRow(ctx,
		`SELECT MAX(revision) FROM learning_activity_revisions WHERE activity_id = $1`,
		activityID).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("max revision: %w", err)
	}
	if max == nil {
		return 1, nil
	}
	return *max + 1, nil
}

func qCreateRevisionStandard(ctx context.Context, q Querier, revID, standardID, role string, weight float64) (*domain.LearningActivityRevisionStandard, error) {
	const sqlStr = `
INSERT INTO learning_activity_revision_standards
    (activity_revision_id, standard_id, role, weight)
VALUES ($1, $2, $3, $4)
RETURNING id, activity_revision_id, standard_id, role, weight, mastery_criterion, created_at`
	rows, err := q.Query(ctx, sqlStr, revID, standardID, role, weight)
	if err != nil {
		return nil, fmt.Errorf("insert revision standard: %w", err)
	}
	link, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningActivityRevisionStandard])
	if err != nil {
		return nil, fmt.Errorf("scan revision standard: %w", err)
	}
	return &link, nil
}

// ListRevisionStandards returns standards linked to a revision.
func ListRevisionStandards(ctx context.Context, q Querier, revisionID string) ([]domain.LearningActivityRevisionStandard, error) {
	const sqlStr = `
SELECT id, activity_revision_id, standard_id, role, weight, mastery_criterion, created_at
FROM learning_activity_revision_standards
WHERE activity_revision_id = $1
ORDER BY role, created_at`
	rows, err := q.Query(ctx, sqlStr, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list revision standards: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.LearningActivityRevisionStandard])
	if err != nil {
		return nil, fmt.Errorf("scan revision standards: %w", err)
	}
	return items, nil
}

// CreateDraftActivity inserts a draft activity shell.
func CreateDraftActivity(ctx context.Context, q Querier, slug, title, summary, kind string, subjectID *string) (*domain.LearningActivity, error) {
	values := map[string]any{
		"slug":    slug,
		"title":   title,
		"summary": summary,
		"kind":    kind,
		"status":  domain.ActivityStatusDraft,
	}
	if subjectID != nil {
		values["subject_id"] = *subjectID
	}
	return LearningActivities.Create(ctx, q, values)
}

// PublishDraftRevision publishes content for an existing activity as a new revision.
func PublishDraftRevision(ctx context.Context, q Querier, activityID string, content contracts.ActivityContent, schemaVersion string, standards []contracts.StandardRef, standardIDs map[string]string, now time.Time) (*domain.LearningActivityRevision, error) {
	doc := &contracts.ActivityDocument{
		SchemaVersion: schemaVersion,
		Content:       content,
		Standards:     standards,
	}
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = contracts.SchemaVersion
	}
	digest, contentRaw, err := ContentSHA256(content)
	if err != nil {
		return nil, err
	}
	var contentMap map[string]any
	if err := json.Unmarshal(contentRaw, &contentMap); err != nil {
		return nil, err
	}

	var revision *domain.LearningActivityRevision
	err = WithTx(ctx, q, func(tx Querier) error {
		act, err := LearningActivities.Get(ctx, tx, activityID)
		if err != nil {
			return err
		}
		if existing, err := revisionByDigest(ctx, tx, act.ID, digest); err == nil {
			revision = existing
			_, _ = LearningActivities.Update(ctx, tx, act.ID, map[string]any{"status": domain.ActivityStatusPublished})
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		next, err := nextRevisionNumber(ctx, tx, act.ID)
		if err != nil {
			return err
		}
		rev, err := LearningActivityRevisions.Create(ctx, tx, map[string]any{
			"activity_id":    act.ID,
			"revision":       next,
			"schema_version": doc.SchemaVersion,
			"content":        contentMap,
			"content_sha256": digest,
			"published_at":   now,
		})
		if err != nil {
			return err
		}
		for _, ref := range standards {
			stdID, ok := standardIDs[ref.Code]
			if !ok {
				return ErrBadRequest{Msg: "unknown standard code: " + ref.Code}
			}
			weight := ref.Weight
			if weight == 0 {
				weight = 1
			}
			role := ref.Role
			if role == "" {
				role = contracts.StandardRolePrimary
			}
			if _, err := qCreateRevisionStandard(ctx, tx, rev.ID, stdID, role, weight); err != nil {
				return err
			}
		}
		if _, err := LearningActivities.Update(ctx, tx, act.ID, map[string]any{"status": domain.ActivityStatusPublished}); err != nil {
			return err
		}
		revision = rev
		return nil
	})
	return revision, err
}

// CreateAssignment assigns a published revision to a student.
func CreateAssignment(ctx context.Context, q Querier, studentID, revisionID string, assignedBy *string, priority int, reason string) (*domain.StudentAssignment, error) {
	values := map[string]any{
		"student_id":           studentID,
		"activity_revision_id": revisionID,
		"state":                domain.AssignmentAvailable,
		"priority":             priority,
		"reason":               reason,
		"constraints":          map[string]any{},
	}
	if assignedBy != nil {
		values["assigned_by"] = *assignedBy
	}
	return StudentAssignments.Create(ctx, q, values)
}

// ListAssignmentsForStudent returns assignments ordered for the work queue.
func ListAssignmentsForStudent(ctx context.Context, q Querier, studentID string) ([]domain.StudentAssignment, error) {
	const sqlStr = `
SELECT id, student_id, activity_revision_id, enrollment_id, state, priority,
       available_at, due_at, assigned_by, reason, constraints, created_at, updated_at
FROM student_assignments
WHERE student_id = $1 AND state <> 'cancelled'
ORDER BY priority DESC, available_at ASC, id ASC`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.StudentAssignment])
	if err != nil {
		return nil, fmt.Errorf("scan assignments: %w", err)
	}
	return items, nil
}

// StudentWorkItem is one assignment plus its immutable revision for the work queue.
type StudentWorkItem struct {
	Assignment domain.StudentAssignment        `json:"assignment"`
	Revision   domain.LearningActivityRevision `json:"revision"`
	Activity   domain.LearningActivity         `json:"activity"`
}

// ListStudentWork returns assignment upserts after an optional cursor (updated_at|id).
func ListStudentWork(ctx context.Context, q Querier, studentID, after string, limit int) ([]StudentWorkItem, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var afterTime time.Time
	var afterID string
	if after != "" {
		// cursor format: RFC3339Nano|uuid
		parts := splitCursor(after)
		if len(parts) != 2 {
			return nil, "", ErrBadRequest{Msg: "invalid after cursor"}
		}
		t, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return nil, "", ErrBadRequest{Msg: "invalid after cursor time"}
		}
		afterTime, afterID = t, parts[1]
	}

	const sqlStr = `
SELECT a.id, a.student_id, a.activity_revision_id, a.enrollment_id, a.state, a.priority,
       a.available_at, a.due_at, a.assigned_by, a.reason, a.constraints, a.created_at, a.updated_at,
       r.id, r.activity_id, r.revision, r.schema_version, r.content, r.content_sha256, r.published_at, r.created_at,
       la.id, la.slug, la.title, la.summary, la.kind, la.subject_id, la.status, la.created_at, la.updated_at
FROM student_assignments a
JOIN learning_activity_revisions r ON r.id = a.activity_revision_id
JOIN learning_activities la ON la.id = r.activity_id
WHERE a.student_id = $1
  AND (
    $2::timestamptz IS NULL
    OR (a.updated_at, a.id) > ($2::timestamptz, $3::uuid)
  )
ORDER BY a.updated_at ASC, a.id ASC
LIMIT $4`

	var tArg any
	var idArg any
	if after == "" {
		tArg = nil
		idArg = nil
	} else {
		tArg = afterTime
		idArg = afterID
	}

	rows, err := q.Query(ctx, sqlStr, studentID, tArg, idArg, limit)
	if err != nil {
		return nil, "", fmt.Errorf("list student work: %w", err)
	}
	defer rows.Close()

	var items []StudentWorkItem
	var cursor string
	for rows.Next() {
		var a domain.StudentAssignment
		var r domain.LearningActivityRevision
		var la domain.LearningActivity
		if err := rows.Scan(
			&a.ID, &a.StudentID, &a.ActivityRevisionID, &a.EnrollmentID, &a.State, &a.Priority,
			&a.AvailableAt, &a.DueAt, &a.AssignedBy, &a.Reason, &a.Constraints, &a.CreatedAt, &a.UpdatedAt,
			&r.ID, &r.ActivityID, &r.Revision, &r.SchemaVersion, &r.Content, &r.ContentSHA256, &r.PublishedAt, &r.CreatedAt,
			&la.ID, &la.Slug, &la.Title, &la.Summary, &la.Kind, &la.SubjectID, &la.Status, &la.CreatedAt, &la.UpdatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan student work: %w", err)
		}
		items = append(items, StudentWorkItem{Assignment: a, Revision: r, Activity: la})
		cursor = a.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + a.ID
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return items, cursor, nil
}

func splitCursor(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}

// GetRevision returns a revision by id.
func GetRevision(ctx context.Context, q Querier, id string) (*domain.LearningActivityRevision, error) {
	return LearningActivityRevisions.Get(ctx, q, id)
}
