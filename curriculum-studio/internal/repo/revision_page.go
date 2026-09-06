package repo

import (
	"context"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/google/uuid"
)

func (r *PlanRevisionRepo) ListPage(ctx context.Context, ws, cur uuid.UUID, limit, offset int) ([]domain.PlanRevision, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := r.Q.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE c.workspace_id=$1 AND c.id=$2`, ws, cur).Scan(&total); err != nil {
		return nil, 0, MapError(err)
	}
	rows, err := listRows(ctx, r.Q, revisionSelect+` WHERE c.workspace_id=$1 AND c.id=$2 ORDER BY r.revision DESC LIMIT $3 OFFSET $4`, []any{ws, cur, limit, offset}, scanPlanRevision)
	if rows == nil {
		rows = []domain.PlanRevision{}
	}
	return rows, total, err
}
