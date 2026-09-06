package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

type PlanComment struct {
	ID               string    `json:"id"`
	RevisionID       string    `json:"revisionId"`
	NodeID           string    `json:"nodeId"`
	ItemID           string    `json:"itemId,omitempty"`
	AuthorSubjectRef string    `json:"authorSubjectRef"`
	AuthorName       string    `json:"authorName"`
	Body             string    `json:"body"`
	CreatedAt        time.Time `json:"createdAt"`
}
type commentInput struct {
	RevisionID string `path:"revisionId"`
	Body       struct {
		NodeID string `json:"nodeId" minLength:"1"`
		Body   string `json:"body" minLength:"1" maxLength:"10000"`
	}
}
type commentListInput struct {
	RevisionID string `path:"revisionId"`
	NodeID     string `query:"nodeId"`
	Limit      int    `query:"limit" minimum:"1" maximum:"100" default:"25"`
	Offset     int    `query:"offset" minimum:"0" default:"0"`
}
type commentOutput struct{ Body PlanComment }
type commentPageOutput struct {
	Body struct {
		Items      []PlanComment `json:"items" nullable:"false"`
		TotalCount int           `json:"totalCount"`
		Limit      int           `json:"limit"`
		Offset     int           `json:"offset"`
	}
}

func commentView(c *domain.PlanComment) PlanComment {
	itemID := ""
	if strings.HasPrefix(c.NodeID, itemIDPrefix) {
		itemID = c.NodeID
	}
	return PlanComment{ItemID: itemID, ID: encodePlanID("comment_", c.ID), RevisionID: revisionID(c.PlanRevisionID), NodeID: c.NodeID, AuthorSubjectRef: c.AuthorSubjectRef, AuthorName: c.AuthorDisplayName, Body: c.Body, CreatedAt: c.CreatedAt}
}

type PlanApproval struct {
	ID                 string     `json:"id,omitempty"`
	State              string     `json:"state" enum:"pending,approved,rejected"`
	ContentFingerprint string     `json:"contentFingerprint"`
	ReviewerSubjectRef string     `json:"reviewerSubjectRef,omitempty"`
	ReviewerName       string     `json:"reviewerName,omitempty"`
	CreatedAt          *time.Time `json:"createdAt,omitempty"`
}
type approvalInput struct {
	RevisionID string `path:"revisionId"`
	Body       struct {
		Decision           string `json:"decision" enum:"approved,rejected"`
		ContentFingerprint string `json:"contentFingerprint" minLength:"32" maxLength:"32"`
	}
}
type approvalOutput struct{ Body PlanApproval }

func approvalView(a *domain.PlanApproval) PlanApproval {
	return PlanApproval{ID: encodePlanID("approval_", a.ID), State: a.Status, ContentFingerprint: a.ContentFingerprint, ReviewerSubjectRef: a.ReviewerSubjectRef, ReviewerName: a.ReviewerDisplayName, CreatedAt: &a.CreatedAt}
}

type diffInput struct {
	RevisionID     string `path:"revisionId"`
	FromRevisionID string `query:"fromRevisionId" required:"true"`
}
type diffOutput struct{ Body domain.RevisionDiff }

type shareInput struct {
	CurriculumID string `path:"curriculumId"`
	Body         struct {
		TargetWorkspaceID string `json:"targetWorkspaceId"`
		Permission        string `json:"permission" enum:"read" default:"read"`
	}
}
type shareOutput struct {
	Body struct {
		ID                string `json:"id"`
		TargetWorkspaceID string `json:"targetWorkspaceId"`
		Permission        string `json:"permission"`
	}
}
type revokeShareInput struct {
	CurriculumID      string `path:"curriculumId"`
	TargetWorkspaceID string `path:"targetWorkspaceId"`
}

// Shared grants are used only by explicit plan read paths. They never become
// workspace memberships, nor authorize materializations, exports or mutations.
func (s *Server) curriculumReadable(ctx context.Context, raw string) (*domain.Curriculum, uuid.UUID, error) {
	if c, ws, _, err := s.curriculumForCaller(ctx, raw); err == nil {
		return c, ws, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, uuid.Nil, err
	}
	id, err := decodePlanID(raw, curriculumIDPrefix)
	if err != nil {
		return nil, uuid.Nil, repo.ErrNotFound
	}
	for _, m := range MembershipsFromContext(ctx) {
		grant, err := repo.NewShareRepo(s.querier).Get(ctx, m.WorkspaceID, id)
		if errors.Is(err, repo.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, uuid.Nil, err
		}
		c, err := repo.NewCurriculumRepo(s.querier).Get(ctx, grant.SourceWorkspaceID, id)
		if err == nil {
			return c, grant.SourceWorkspaceID, nil
		}
		if !errors.Is(err, repo.ErrNotFound) {
			return nil, uuid.Nil, err
		}
	}
	return nil, uuid.Nil, repo.ErrNotFound
}
func (s *Server) revisionReadable(ctx context.Context, raw string) (*domain.PlanRevision, uuid.UUID, error) {
	if r, ws, _, err := s.revisionForCaller(ctx, raw); err == nil {
		return r, ws, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, uuid.Nil, err
	}
	id, err := decodePlanID(raw, revisionIDPrefix)
	if err != nil {
		return nil, uuid.Nil, repo.ErrNotFound
	}
	for _, m := range MembershipsFromContext(ctx) {
		var owner uuid.UUID
		err := s.querier.QueryRow(ctx, `SELECT c.workspace_id FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.curriculum_shares sh ON sh.curriculum_id=c.id AND sh.source_workspace_id=c.workspace_id WHERE r.id=$1 AND sh.target_workspace_id=$2 AND sh.permission='read'`, id, m.WorkspaceID).Scan(&owner)
		err = repo.MapError(err)
		if errors.Is(err, repo.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, uuid.Nil, err
		}
		r, err := repo.NewPlanRevisionRepo(s.querier).Get(ctx, owner, id)
		if err == nil {
			return r, owner, nil
		}
		if !errors.Is(err, repo.ErrNotFound) {
			return nil, uuid.Nil, err
		}
	}
	return nil, uuid.Nil, repo.ErrNotFound
}

func (s *Server) registerCollabRoutes(api huma.API) {
	commentOp := authoringOperation("createComment", http.MethodPost, "/studio/v1/revisions/{revisionId}/comments", "Collaboration", "Comment on a plan node")
	commentOp.DefaultStatus = http.StatusCreated
	huma.Register(api, commentOp, func(ctx context.Context, in *commentInput) (*commentOutput, error) {
		r, ws, m, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		if !authz.CanMutate(m.Role) && !authz.CanReview(m.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		id, table, err := nodeTable(in.Body.NodeID)
		if err != nil {
			return nil, huma.Error404NotFound("node not found")
		}
		var exists bool
		err = s.querier.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM curriculum_studio.`+table+` WHERE id=$1 AND plan_revision_id=$2)`, id, r.ID).Scan(&exists)
		if err != nil {
			return nil, planError(err)
		}
		if !exists {
			return nil, huma.Error404NotFound("node not found")
		}
		if strings.TrimSpace(in.Body.Body) == "" {
			return nil, huma.Error400BadRequest("comment body required")
		}
		principal, _ := AuthFromContext(ctx)
		c, err := repo.NewCommentRepo(s.querier).Create(ctx, &domain.PlanComment{WorkspaceID: ws, PlanRevisionID: r.ID, NodeID: in.Body.NodeID, AuthorSubjectRef: principal.SubjectRef, AuthorDisplayName: m.DisplayName, Body: in.Body.Body})
		if err != nil {
			return nil, planError(err)
		}
		return &commentOutput{Body: commentView(c)}, nil
	})
	huma.Register(api, authoringOperation("listComments", http.MethodGet, "/studio/v1/revisions/{revisionId}/comments", "Collaboration", "List plan comments"), func(ctx context.Context, in *commentListInput) (*commentPageOutput, error) {
		r, ws, _, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		limit, offset := repo.CommentPageBounds(in.Limit, in.Offset)
		comments, total, err := repo.NewCommentRepo(s.querier).ListPage(ctx, ws, r.ID, in.NodeID, limit, offset)
		if err != nil {
			return nil, planError(err)
		}
		out := &commentPageOutput{}
		out.Body.Items = []PlanComment{}
		out.Body.TotalCount = total
		out.Body.Limit = limit
		out.Body.Offset = offset
		for i := range comments {
			out.Body.Items = append(out.Body.Items, commentView(&comments[i]))
		}
		return out, nil
	})
	huma.Register(api, authoringOperation("decideRevision", http.MethodPost, "/studio/v1/revisions/{revisionId}/approval", "Collaboration", "Record a reviewer decision on a draft snapshot"), func(ctx context.Context, in *approvalInput) (*approvalOutput, error) {
		r, ws, m, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		principal, _ := AuthFromContext(ctx)
		if !authz.CanReview(m.Role) || principal.Kind != authn.KindHuman {
			return nil, huma.Error403Forbidden("reviewer membership required")
		}
		if r.Status != "draft" {
			return nil, huma.Error409Conflict("only drafts can be reviewed")
		}
		a, err := repo.NewApprovalRepo(s.querier).Decide(ctx, ws, r.ID, principal.SubjectRef, in.Body.Decision, in.Body.ContentFingerprint)
		if err != nil {
			return nil, planError(err)
		}
		return &approvalOutput{Body: approvalView(a)}, nil
	})
	huma.Register(api, authoringOperation("getRevisionApproval", http.MethodGet, "/studio/v1/revisions/{revisionId}/approval", "Collaboration", "Get the decision for the current draft snapshot"), func(ctx context.Context, in *revisionPath) (*approvalOutput, error) {
		r, ws, _, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		approvals := repo.NewApprovalRepo(s.querier)
		fingerprint, err := approvals.Fingerprint(ctx, ws, r.ID)
		if err != nil {
			return nil, planError(err)
		}
		a, err := approvals.Current(ctx, ws, r.ID)
		if err != nil {
			return nil, planError(err)
		}
		if a == nil || a.ContentFingerprint != fingerprint {
			return &approvalOutput{Body: PlanApproval{State: "pending", ContentFingerprint: fingerprint}}, nil
		}
		return &approvalOutput{Body: approvalView(a)}, nil
	})
	huma.Register(api, authoringOperation("diffRevisions", http.MethodGet, "/studio/v1/revisions/{revisionId}/diff", "Collaboration", "Compare outcomes by stable code and name"), func(ctx context.Context, in *diffInput) (*diffOutput, error) {
		to, ws, err := s.revisionReadable(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		from, fromWS, err := s.revisionReadable(ctx, in.FromRevisionID)
		if err != nil {
			return nil, planError(err)
		}
		if ws != fromWS || from.CurriculumID != to.CurriculumID {
			return nil, huma.Error400BadRequest("revisions must belong to the same curriculum")
		}
		diff, err := repo.DiffRevisions(ctx, s.querier, ws, from.ID, to.ID)
		if err != nil {
			return nil, planError(err)
		}
		return &diffOutput{Body: diff}, nil
	})
	huma.Register(api, authoringOperation("shareCurriculum", http.MethodPost, "/studio/v1/curricula/{curriculumId}/shares", "Collaboration", "Grant read-only curriculum access to a workspace"), func(ctx context.Context, in *shareInput) (*shareOutput, error) {
		c, ws, m, err := s.curriculumForCaller(ctx, in.CurriculumID)
		if err != nil {
			return nil, planError(err)
		}
		if err = requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		target, err := decodeWSID(in.Body.TargetWorkspaceID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid target workspace")
		}
		principal, _ := AuthFromContext(ctx)
		grant, err := repo.NewShareRepo(s.querier).GrantAuthorized(ctx, &domain.CurriculumShare{CurriculumID: c.ID, SourceWorkspaceID: ws, TargetWorkspaceID: target, Permission: in.Body.Permission, CreatedBySubjectRef: principal.SubjectRef})
		if err != nil {
			return nil, planError(err)
		}
		out := &shareOutput{}
		out.Body.ID = encodePlanID("share_", grant.ID)
		out.Body.TargetWorkspaceID = encodeWSID(target)
		out.Body.Permission = grant.Permission
		return out, nil
	})
	op := authoringOperation("revokeCurriculumShare", http.MethodDelete, "/studio/v1/curricula/{curriculumId}/shares/{targetWorkspaceId}", "Collaboration", "Revoke a read-only grant")
	op.DefaultStatus = http.StatusNoContent
	huma.Register(api, op, func(ctx context.Context, in *revokeShareInput) (*struct{}, error) {
		c, ws, m, err := s.curriculumForCaller(ctx, in.CurriculumID)
		if err != nil {
			return nil, planError(err)
		}
		if err = requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		target, err := decodeWSID(in.TargetWorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		principal, _ := AuthFromContext(ctx)
		err = repo.NewShareRepo(s.querier).RevokeAuthorized(ctx, ws, c.ID, target, principal.SubjectRef)
		if err != nil {
			return nil, planError(err)
		}
		return nil, nil
	})
	s.registerLibraryRoutes(api)
	s.registerCollabSurfaceRoutes(api)
}

// seedTemplate is invoked inside curriculum creation's transaction: a failed
// seed never leaves an empty curriculum or an orphaned draft behind.
func seedTemplate(ctx context.Context, q repo.Querier, ws uuid.UUID, c *domain.Curriculum, code string, brief json.RawMessage) error {
	t, err := repo.NewTemplateRepo(q).GetByCode(ctx, ws, code)
	if err != nil {
		return err
	}
	seed, err := repo.DecodeTemplateSeed(t.Seed)
	if err != nil {
		return err
	}
	if err = repo.ValidateTemplateSeed(seed); err != nil {
		return err
	}
	r, err := repo.NewPlanRevisionRepo(q).Create(ctx, ws, &domain.PlanRevision{CurriculumID: c.ID, Revision: 1, Title: c.Title + " — draft 1", Brief: brief})
	if err != nil {
		return err
	}
	g := repo.NewPlanGraphRepo(q)
	outcomes := map[string]uuid.UUID{}
	for i, o := range seed.Outcomes {
		code := o.Code
		if code == "" {
			code = "template-outcome-" + uuid.NewString()
		}
		created, e := g.CreateOutcome(ctx, ws, &domain.Outcome{PlanRevisionID: r.ID, Code: code, Title: o.DisplayName(), Position: i})
		if e != nil {
			return e
		}
		outcomes[o.Code] = created.ID
	}
	for i, u := range seed.Units {
		code := u.Code
		if code == "" {
			code = "template-unit-" + uuid.NewString()
		}
		unit, e := g.CreateUnit(ctx, ws, &domain.Unit{PlanRevisionID: r.ID, Code: code, Title: u.DisplayName(), Position: i})
		if e != nil {
			return e
		}
		for _, outcome := range u.OutcomeCodes {
			if _, e = g.CreateUnitOutcome(ctx, ws, &domain.UnitOutcome{UnitID: unit.ID, OutcomeID: outcomes[outcome], Role: "target"}); e != nil {
				return e
			}
		}
	}
	return nil
}
