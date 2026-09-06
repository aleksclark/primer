package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/validation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrPolicyDenied = errors.New("workspace collaboration policy denies operation")
var ErrApprovalRequired = errors.New("current reviewer approval required")
var ErrRoleDenied = errors.New("local role does not authorize operation")

type PolicyRepo struct{ Q Querier }

func NewPolicyRepo(q Querier) *PolicyRepo { return &PolicyRepo{q} }
func (r *PolicyRepo) Get(ctx context.Context, ws uuid.UUID) (domain.CollaborationPolicy, error) {
	p := domain.DefaultCollaborationPolicy()
	var raw []byte
	err := r.Q.QueryRow(ctx, `SELECT policies FROM curriculum_studio.workspace_policies WHERE workspace_id=$1`, ws).Scan(&raw)
	if errors.Is(MapError(err), ErrNotFound) {
		return p, nil
	}
	if err != nil {
		return p, MapError(err)
	}
	err = json.Unmarshal(raw, &p)
	return p, err
}

// Policy is first in the collaboration lock order, before curriculum/revision
// and membership. A missing row is materialized with explicit compatible defaults.
func lockPolicy(ctx context.Context, q Querier, ws uuid.UUID, write bool) (domain.CollaborationPolicy, error) {
	// Policy/authority locks have the same transaction requirements as approval.
	if _, ok := q.(pgx.Tx); !ok {
		return domain.CollaborationPolicy{}, ErrConflict
	}
	var isolation string
	if err := q.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
		return domain.CollaborationPolicy{}, MapError(err)
	}
	if isolation != "read committed" {
		return domain.CollaborationPolicy{}, ErrConflict
	}
	_, err := q.Exec(ctx, `INSERT INTO curriculum_studio.workspace_policies(workspace_id,policies) VALUES($1,'{"requireApprovalForPublish":false,"sharingEnabled":true}') ON CONFLICT DO NOTHING`, ws)
	if err != nil {
		return domain.CollaborationPolicy{}, MapError(err)
	}
	mode := "SHARE"
	if write {
		mode = "UPDATE"
	}
	var id uuid.UUID
	if err = q.QueryRow(ctx, `SELECT workspace_id FROM curriculum_studio.workspace_policies WHERE workspace_id=$1 FOR `+mode, ws).Scan(&id); err != nil {
		return domain.CollaborationPolicy{}, MapError(err)
	}
	return NewPolicyRepo(q).Get(ctx, ws)
}
func lockLocalRole(ctx context.Context, q Querier, ws uuid.UUID, subject string, roles ...string) error {
	var role, status string
	if err := q.QueryRow(ctx, `SELECT role,status FROM curriculum_studio.workspace_memberships WHERE workspace_id=$1 AND subject_ref=$2 FOR SHARE`, ws, subject).Scan(&role, &status); err != nil {
		if errors.Is(MapError(err), ErrNotFound) {
			return ErrRoleDenied
		}
		return MapError(err)
	}
	if status != "active" {
		return ErrRoleDenied
	}
	for _, allowed := range roles {
		if role == allowed {
			return nil
		}
	}
	return ErrRoleDenied
}
func (r *PolicyRepo) Set(ctx context.Context, ws uuid.UUID, subject string, p domain.CollaborationPolicy) error {
	return MapError(WithTx(ctx, r.Q, func(q Querier) error {
		if _, err := lockPolicy(ctx, q, ws, true); err != nil {
			return err
		}
		if err := lockLocalRole(ctx, q, ws, subject, "owner", "admin"); err != nil {
			return err
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE curriculum_studio.workspace_policies SET policies=$2,updated_at=now() WHERE workspace_id=$1`, ws, raw)
		if err != nil {
			return err
		}
		if !p.SharingEnabled {
			_, err = q.Exec(ctx, `DELETE FROM curriculum_studio.curriculum_shares WHERE source_workspace_id=$1`, ws)
		}
		return err
	}))
}

// Caller already holds the revision and policy locks. The reviewer's membership
// must remain active through the publication commit, not merely through a read.
func requireCurrentApproval(ctx context.Context, q Querier, ws, revision uuid.UUID) error {
	var subject string
	if err := q.QueryRow(ctx, `SELECT reviewer_subject_ref FROM curriculum_studio.plan_approvals WHERE workspace_id=$1 AND plan_revision_id=$2`, ws, revision).Scan(&subject); err != nil {
		return ErrApprovalRequired
	}
	if err := lockLocalRole(ctx, q, ws, subject, "reviewer"); err != nil {
		return ErrApprovalRequired
	}
	a, err := NewApprovalRepo(q).Current(ctx, ws, revision)
	if err != nil {
		return err
	}
	if a == nil || a.Status != domain.ApprovalApproved {
		return ErrApprovalRequired
	}
	return nil
}

// PublishAuthorized is the HTTP authoring boundary. Validation and authorization
// run after the same locks as publication, so edits cannot race the validation.
func (r *PlanRevisionRepo) PublishAuthorized(ctx context.Context, ws, revision uuid.UUID, subject string) error {
	return MapError(WithTx(ctx, r.Q, func(q Querier) error {
		if _, err := lockPolicy(ctx, q, ws, false); err != nil {
			return err
		}
		var id uuid.UUID
		if err := q.QueryRow(ctx, `SELECT c.id FROM curriculum_studio.curricula c JOIN curriculum_studio.plan_revisions r ON r.curriculum_id=c.id WHERE r.id=$1 AND c.workspace_id=$2 FOR UPDATE OF c`, revision, ws).Scan(&id); err != nil {
			return err
		}
		if _, err := lockDraftRevision(ctx, q, ws, revision); err != nil {
			return err
		}
		if err := lockLocalRole(ctx, q, ws, subject, "owner", "admin", "author"); err != nil {
			return err
		}
		graph, err := NewPlanGraphRepo(q).Load(ctx, ws, revision)
		if err != nil {
			return err
		}
		if validation.Run(graph).Status == "failed" {
			return fmt.Errorf("%w: graph validation failed", ErrInvalidTransition)
		}
		return publishRevision(ctx, q, revision, subject)
	}))
}
func (r *ShareRepo) GrantAuthorized(ctx context.Context, in *domain.CurriculumShare) (*domain.CurriculumShare, error) {
	var out *domain.CurriculumShare
	err := WithTx(ctx, r.Q, func(q Querier) error {
		p, err := lockPolicy(ctx, q, in.SourceWorkspaceID, false)
		if err != nil {
			return err
		}
		if !p.SharingEnabled {
			return ErrPolicyDenied
		}
		if err = lockLocalRole(ctx, q, in.SourceWorkspaceID, in.CreatedBySubjectRef, "owner", "admin", "author"); err != nil {
			return err
		}
		out, err = NewShareRepo(q).Create(ctx, in)
		return err
	})
	return out, MapError(err)
}
func (r *ShareRepo) RevokeAuthorized(ctx context.Context, ws, curriculum, target uuid.UUID, subject string) error {
	return MapError(WithTx(ctx, r.Q, func(q Querier) error {
		if _, err := lockPolicy(ctx, q, ws, false); err != nil {
			return err
		}
		if err := lockLocalRole(ctx, q, ws, subject, "owner", "admin", "author"); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `DELETE FROM curriculum_studio.curriculum_shares WHERE source_workspace_id=$1 AND curriculum_id=$2 AND target_workspace_id=$3`, ws, curriculum, target)
		return err
	}))
}
