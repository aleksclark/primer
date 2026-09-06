package api

import (
	"context"
	"net/http"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type itemCommentInput struct {
	ItemID string `path:"itemId"`
	Body   struct {
		Body string `json:"body" minLength:"1" maxLength:"10000"`
	}
}
type itemCommentListInput struct {
	ItemID string `path:"itemId"`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"25"`
	Offset int    `query:"offset" minimum:"0" default:"0"`
}
type policyPath struct {
	WorkspaceID string `path:"workspaceId"`
}
type policyInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        domain.CollaborationPolicy
}
type policyOutput struct{ Body domain.CollaborationPolicy }
type shareListInput struct {
	CurriculumID string `path:"curriculumId"`
	Limit        int    `query:"limit" minimum:"1" maximum:"100" default:"25"`
	Offset       int    `query:"offset" minimum:"0" default:"0"`
}
type ShareView struct {
	TargetWorkspaceID   string `json:"targetWorkspaceId"`
	TargetWorkspaceName string `json:"targetWorkspaceName"`
	Permission          string `json:"permission"`
}
type sharePageOutput struct{ Body standardsPage[ShareView] }

func (s *Server) registerCollabSurfaceRoutes(a huma.API) {
	op := authoringOperation("createItemComment", http.MethodPost, "/studio/v1/materialized-items/{itemId}/comments", "Collaboration", "Comment on a materialized item")
	op.DefaultStatus = 201
	huma.Register(a, op, func(ctx context.Context, in *itemCommentInput) (*commentOutput, error) {
		item, ws, m, err := s.itemForCaller(ctx, in.ItemID)
		if err != nil {
			return nil, planError(err)
		}
		if !authz.CanMutate(m.Role) && !authz.CanReview(m.Role) {
			return nil, huma.Error403Forbidden("commenting role required")
		}
		principal, _ := AuthFromContext(ctx)
		c, err := repo.NewCommentRepo(s.querier).CreateItem(ctx, ws, item.ID, principal.SubjectRef, m.DisplayName, in.Body.Body)
		if err != nil {
			return nil, planError(err)
		}
		return &commentOutput{Body: commentView(c)}, nil
	})
	huma.Register(a, authoringOperation("listItemComments", http.MethodGet, "/studio/v1/materialized-items/{itemId}/comments", "Collaboration", "Read an item's comments"), func(ctx context.Context, in *itemCommentListInput) (*commentPageOutput, error) {
		item, ws, _, err := s.itemForCaller(ctx, in.ItemID)
		if err != nil {
			return nil, planError(err)
		}
		rows, total, err := repo.NewCommentRepo(s.querier).ListPage(ctx, ws, item.PlanRevisionID, repo.ItemCommentKey(item.ID), in.Limit, in.Offset)
		if err != nil {
			return nil, planError(err)
		}
		out := &commentPageOutput{}
		out.Body.Items = []PlanComment{}
		out.Body.TotalCount = total
		out.Body.Limit = in.Limit
		out.Body.Offset = in.Offset
		for i := range rows {
			out.Body.Items = append(out.Body.Items, commentView(&rows[i]))
		}
		return out, nil
	})
	huma.Register(a, authoringOperation("getCollaborationPolicy", http.MethodGet, "/studio/v1/workspaces/{workspaceId}/collaboration-policy", "Collaboration", "Read compatible collaboration policy defaults"), func(ctx context.Context, in *policyPath) (*policyOutput, error) {
		ws, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		p, err := repo.NewPolicyRepo(s.querier).Get(ctx, ws)
		if err != nil {
			return nil, planError(err)
		}
		return &policyOutput{Body: p}, nil
	})
	huma.Register(a, authoringOperation("setCollaborationPolicy", http.MethodPut, "/studio/v1/workspaces/{workspaceId}/collaboration-policy", "Collaboration", "Configure collaboration policy; disabling sharing revokes all outgoing grants"), func(ctx context.Context, in *policyInput) (*policyOutput, error) {
		ws, m, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanManageMembers(m.Role) {
			return nil, huma.Error403Forbidden("workspace administrator required")
		}
		principal, _ := AuthFromContext(ctx)
		if err = repo.NewPolicyRepo(s.querier).Set(ctx, ws, principal.SubjectRef, in.Body); err != nil {
			return nil, planError(err)
		}
		return &policyOutput{Body: in.Body}, nil
	})
	huma.Register(a, authoringOperation("listCurriculumShares", http.MethodGet, "/studio/v1/curricula/{curriculumId}/shares", "Collaboration", "List outgoing read-only grants"), func(ctx context.Context, in *shareListInput) (*sharePageOutput, error) {
		c, ws, _, err := s.curriculumForCaller(ctx, in.CurriculumID)
		if err != nil {
			return nil, planError(err)
		}
		out := &sharePageOutput{Body: standardsPage[ShareView]{Items: []ShareView{}, Limit: in.Limit, Offset: in.Offset}}
		if err = s.querier.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.curriculum_shares WHERE curriculum_id=$1 AND source_workspace_id=$2`, c.ID, ws).Scan(&out.Body.TotalCount); err != nil {
			return nil, planError(err)
		}
		rows, err := s.querier.Query(ctx, `SELECT w.id,w.name,sh.permission FROM curriculum_studio.curriculum_shares sh JOIN curriculum_studio.workspaces w ON w.id=sh.target_workspace_id WHERE sh.curriculum_id=$1 AND sh.source_workspace_id=$2 ORDER BY w.name,w.id LIMIT $3 OFFSET $4`, c.ID, ws, in.Limit, in.Offset)
		if err != nil {
			return nil, planError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var v ShareView
			if err = rows.Scan(&id, &v.TargetWorkspaceName, &v.Permission); err != nil {
				return nil, planError(err)
			}
			v.TargetWorkspaceID = encodeWSID(id)
			out.Body.Items = append(out.Body.Items, v)
		}
		return out, planErrorOrNil(rows.Err())
	})
}
func planErrorOrNil(err error) error {
	if err == nil {
		return nil
	}
	return planError(err)
}
