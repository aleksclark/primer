package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/google/uuid"
)

func ItemCommentKey(id uuid.UUID) string { return "mit_" + strings.ReplaceAll(id.String(), "-", "") }

// Item provenance is resolved in the INSERT, not accepted from the client.
func (r *CommentRepo) CreateItem(ctx context.Context, ws, item uuid.UUID, subject, name, body string) (*domain.PlanComment, error) {
	if err := domain.ValidateCommentBody(body); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCheckViolation, err)
	}
	out, err := scanComment(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.plan_comments(workspace_id,plan_revision_id,node_id,item_id,author_subject_ref,author_display_name,body)
 SELECT c.workspace_id,r.id,$3,i.id,$4,$5,$6 FROM curriculum_studio.materialized_items i JOIN curriculum_studio.materialization_runs m ON m.id=i.run_id JOIN curriculum_studio.plan_revisions r ON r.id=m.plan_revision_id AND i.plan_revision_id=r.id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id AND m.workspace_id=c.workspace_id
 WHERE i.id=$2 AND c.workspace_id=$1 RETURNING id,workspace_id,plan_revision_id,node_id,author_subject_ref,author_display_name,body,created_at`, ws, item, ItemCommentKey(item), subject, name, strings.TrimSpace(body)))
	return out, MapError(err)
}
