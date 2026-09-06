package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/google/uuid"
)

// CommentPageBounds is shared by storage and the HTTP page metadata.
func CommentPageBounds(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ListPage bounds both the returned rows and the database work retained in memory.
func (r *CommentRepo) ListPage(ctx context.Context, ws, revision uuid.UUID, node string, limit, offset int) ([]domain.PlanComment, int, error) {
	if r == nil || r.Q == nil {
		return nil, 0, ErrClosed
	}
	limit, offset = CommentPageBounds(limit, offset)
	const scope = ` FROM curriculum_studio.plan_comments p JOIN curriculum_studio.plan_revisions r ON r.id=p.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE p.workspace_id=$1 AND c.workspace_id=$1 AND p.plan_revision_id=$2 AND ($3='' OR p.node_id=$3)`
	var total int
	if err := r.Q.QueryRow(ctx, `SELECT count(*)`+scope, ws, revision, strings.TrimSpace(node)).Scan(&total); err != nil {
		return nil, 0, MapError(err)
	}
	items, err := listRows(ctx, r.Q, `SELECT p.id,p.workspace_id,p.plan_revision_id,p.node_id,p.author_subject_ref,p.author_display_name,p.body,p.created_at`+scope+` ORDER BY p.created_at,p.id LIMIT $4 OFFSET $5`, []any{ws, revision, strings.TrimSpace(node), limit, offset}, func(s scanner) (*domain.PlanComment, error) { return scanComment(s) })
	if items == nil {
		items = []domain.PlanComment{}
	}
	return items, total, err
}

func (r *ApprovalRepo) Fingerprint(ctx context.Context, ws, revision uuid.UUID) (string, error) {
	if r == nil || r.Q == nil {
		return "", ErrClosed
	}
	var fingerprint string
	err := r.Q.QueryRow(ctx, `SELECT curriculum_studio.plan_content_fingerprint(r.id) FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 AND c.workspace_id=$2`, revision, ws).Scan(&fingerprint)
	return fingerprint, MapError(err)
}

// Decide is a compare-and-set on the reviewed content, not just the revision ID.
// All locks are held through commit: revision -> membership -> approval row.
// Only then does a new statement check status, authority and fingerprint. Graph
// writes use the same revision lock via DB triggers, including direct SQL edits.
func (r *ApprovalRepo) Decide(ctx context.Context, ws, revision uuid.UUID, subject, decision, expectedFingerprint string) (*domain.PlanApproval, error) {
	if r == nil || r.Q == nil {
		return nil, ErrClosed
	}
	if decision != domain.ApprovalApproved && decision != domain.ApprovalRejected {
		return nil, ErrCheckViolation
	}
	if expectedFingerprint == "" {
		return nil, ErrConflict
	}
	const query = `INSERT INTO curriculum_studio.plan_approvals(workspace_id,plan_revision_id,reviewer_subject_ref,reviewer_display_name,status,content_fingerprint)
 SELECT c.workspace_id,r.id,m.subject_ref,m.display_name,$4,$5
 FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id
 JOIN curriculum_studio.workspace_memberships m ON m.workspace_id=c.workspace_id AND m.subject_ref=$3 AND m.role='reviewer' AND m.status='active' AND m.subject_kind='human'
 WHERE r.id=$2 AND c.workspace_id=$1 AND r.status='draft' AND curriculum_studio.plan_content_fingerprint(r.id)=$5
 ON CONFLICT (plan_revision_id) DO UPDATE SET reviewer_subject_ref=EXCLUDED.reviewer_subject_ref,reviewer_display_name=EXCLUDED.reviewer_display_name,status=EXCLUDED.status,content_fingerprint=EXCLUDED.content_fingerprint,created_at=now()
 RETURNING id,workspace_id,plan_revision_id,reviewer_subject_ref,reviewer_display_name,status,content_fingerprint,created_at`
	var out *domain.PlanApproval
	err := WithTx(ctx, r.Q, func(q Querier) error {
		if _, err := lockDraftRevision(ctx, q, ws, revision); err != nil {
			return err
		}
		var id uuid.UUID
		// SHARE (not KEY SHARE) conflicts with status/role/name updates and deletion.
		if err := q.QueryRow(ctx, `SELECT id FROM curriculum_studio.workspace_memberships WHERE workspace_id=$1 AND subject_ref=$2 FOR SHARE`, ws, subject).Scan(&id); err != nil {
			return err
		}
		err := q.QueryRow(ctx, `SELECT id FROM curriculum_studio.plan_approvals WHERE plan_revision_id=$1 FOR UPDATE`, revision).Scan(&id)
		if err != nil && !errors.Is(MapError(err), ErrNotFound) {
			return err
		}
		// This statement begins after every required lock wait. While it executes,
		// neither publication, graph edits nor membership revocation can commit.
		out, err = scanApproval(q.QueryRow(ctx, query, ws, revision, subject, decision, expectedFingerprint))
		return err
	})
	err = MapError(err)
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("%w: stale draft or review authority", ErrConflict)
	}
	return out, err
}

// Current hides obsolete or no-longer-authorized decisions without discarding
// the durable decision row. A revoked/demoted reviewer's approval is not effective.
func (r *ApprovalRepo) Current(ctx context.Context, ws, revision uuid.UUID) (*domain.PlanApproval, error) {
	if r == nil || r.Q == nil {
		return nil, ErrClosed
	}
	out, err := scanApproval(r.Q.QueryRow(ctx, `SELECT a.id,a.workspace_id,a.plan_revision_id,a.reviewer_subject_ref,a.reviewer_display_name,a.status,a.content_fingerprint,a.created_at FROM curriculum_studio.plan_approvals a JOIN curriculum_studio.plan_revisions r ON r.id=a.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.workspace_memberships m ON m.workspace_id=a.workspace_id AND m.subject_ref=a.reviewer_subject_ref AND m.status='active' AND m.role='reviewer' AND m.subject_kind='human' WHERE a.workspace_id=$1 AND c.workspace_id=$1 AND r.id=$2 AND a.content_fingerprint=curriculum_studio.plan_content_fingerprint(r.id)`, ws, revision))
	err = MapError(err)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return out, err
}

// DiffRevisions compares persisted outcomes in one statement snapshot. Stable
// codes survive forks; a rename is a removed old name and an added new name.
func DiffRevisions(ctx context.Context, q Querier, ws, from, to uuid.UUID) (domain.RevisionDiff, error) {
	out := domain.RevisionDiff{AddedOutcomes: []domain.OutcomeChange{}, RemovedOutcomes: []domain.OutcomeChange{}}
	a, err := NewPlanRevisionRepo(q).Get(ctx, ws, from)
	if err != nil {
		return out, err
	}
	b, err := NewPlanRevisionRepo(q).Get(ctx, ws, to)
	if err != nil {
		return out, err
	}
	if a.CurriculumID != b.CurriculumID {
		return out, ErrCheckViolation
	}
	rows, err := q.Query(ctx, `WITH old AS (SELECT code,title FROM curriculum_studio.outcomes WHERE plan_revision_id=$1), new AS (SELECT code,title FROM curriculum_studio.outcomes WHERE plan_revision_id=$2)
 SELECT 'added',n.code,n.title FROM new n WHERE NOT EXISTS(SELECT 1 FROM old o WHERE o.code=n.code AND o.title=n.title)
 UNION ALL SELECT 'removed',o.code,o.title FROM old o WHERE NOT EXISTS(SELECT 1 FROM new n WHERE n.code=o.code AND n.title=o.title) ORDER BY 1,3,2`, from, to)
	if err != nil {
		return out, MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var change domain.OutcomeChange
		if err = rows.Scan(&kind, &change.Code, &change.Name); err != nil {
			return out, MapError(err)
		}
		if kind == "added" {
			out.AddedOutcomes = append(out.AddedOutcomes, change)
		} else {
			out.RemovedOutcomes = append(out.RemovedOutcomes, change)
		}
	}
	return out, MapError(rows.Err())
}
