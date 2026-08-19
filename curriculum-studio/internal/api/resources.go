package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

const resourceIDPrefix = "res_"

type resourceView struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspaceId"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Creators    []string `json:"creators,omitempty"`
	Locator     string   `json:"locator,omitempty"`
	Notes       string   `json:"notes,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

type resourceWriteBody struct {
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Creators []string `json:"creators,omitempty"`
	Locator  string   `json:"locator,omitempty"`
	Notes    string   `json:"notes,omitempty"`
}

type studioResourcePathInput struct {
	ResourceID string `path:"resourceId"`
}
type studioResourceWorkspacePathInput struct {
	WorkspaceID string `path:"workspaceId"`
}
type studioResourceListInput struct {
	WorkspaceID string `path:"workspaceId"`
	Limit       int    `query:"limit" minimum:"1" maximum:"100"`
	Offset      int    `query:"offset" minimum:"0"`
	Q           string `query:"q"`
	Kind        string `query:"kind"`
}
type createStudioResourceInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        resourceWriteBody
}
type updateStudioResourceInput struct {
	ResourceID string `path:"resourceId"`
	Body       resourceWriteBody
}
type studioResourcePageOutput struct{ Body standardsPage[resourceView] }
type studioResourceOutput struct{ Body resourceView }

func encodeResourceID(id uuid.UUID) string { return resourceIDPrefix + compactUUID(id) }

func decodeResourceID(raw string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, resourceIDPrefix, "resource")
}

func resourceViewFor(resource *domain.Resource) resourceView {
	view := resourceView{
		ID:          encodeResourceID(resource.ID),
		WorkspaceID: "",
		Kind:        resource.Kind,
		Title:       resource.Title,
		Creators:    splitCreators(resource.Authors),
		Locator:     resource.URL,
		Notes:       resourceNote(resource.Metadata),
		CreatedAt:   formatTime(resource.CreatedAt),
		UpdatedAt:   formatTime(resource.UpdatedAt),
	}
	if resource.WorkspaceID != nil {
		view.WorkspaceID = encodeWSID(*resource.WorkspaceID)
	}
	if view.Locator == "" {
		view.Locator = resource.ArtifactRef
	}
	return view
}

func splitCreators(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func resourceNote(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata struct {
		Note string `json:"note"`
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return ""
	}
	return metadata.Note
}

func validResourceKind(kind string) bool {
	switch kind {
	case domain.ResourceKindBook, domain.ResourceKindDocument, domain.ResourceKindVideo,
		domain.ResourceKindTool, domain.ResourceKindProjectSupply, domain.ResourceKindURL:
		return true
	default:
		return false
	}
}

func resourceFromBody(body resourceWriteBody, tenantID, workspaceID uuid.UUID) (*domain.Resource, error) {
	if !validResourceKind(body.Kind) {
		return nil, fmt.Errorf("invalid resource kind")
	}
	if strings.TrimSpace(body.Title) == "" {
		return nil, fmt.Errorf("resource title is required")
	}
	metadata, err := json.Marshal(map[string]string{"note": body.Notes})
	if err != nil {
		return nil, err
	}
	resource := &domain.Resource{
		TenantID: tenantID, WorkspaceID: &workspaceID, Kind: body.Kind,
		Title: strings.TrimSpace(body.Title), Authors: strings.Join(body.Creators, ", "), Metadata: metadata,
	}
	locator := strings.TrimSpace(body.Locator)
	if strings.HasPrefix(locator, "obj:") || strings.HasPrefix(locator, "urn:") {
		resource.ArtifactRef = locator
	} else {
		resource.URL = locator
	}
	return resource, nil
}

func (s *Server) registerResourcesRoutes(api huma.API) {
	s.registerListResources(api)
	s.registerCreateResource(api)
	s.registerGetResource(api)
	s.registerUpdateResource(api)
	s.registerDeleteResource(api)
}

func (s *Server) resourceWorkspace(ctx context.Context, raw string) (uuid.UUID, *domain.Workspace, MembershipView, error) {
	workspaceID, membership, err := workspaceIDFromPathForServer(s, ctx, raw)
	if err != nil {
		return uuid.Nil, nil, MembershipView{}, err
	}
	workspace, err := repo.NewWorkspaceRepo(s.querier).GetByID(ctx, workspaceID)
	if err != nil {
		return uuid.Nil, nil, MembershipView{}, err
	}
	return workspaceID, workspace, membership, nil
}

func (s *Server) resourceForCaller(ctx context.Context, resourceID uuid.UUID) (*domain.Resource, *domain.Workspace, MembershipView, error) {
	resources := repo.NewResourceRepo(s.querier)
	workspaces := repo.NewWorkspaceRepo(s.querier)
	for _, membership := range MembershipsFromContext(ctx) {
		workspace, err := workspaces.GetByID(ctx, membership.WorkspaceID)
		if err != nil {
			continue
		}
		resource, err := resources.Get(ctx, workspace.TenantID, workspace.ID, resourceID)
		if err == nil {
			return resource, workspace, membership, nil
		}
	}
	return nil, nil, MembershipView{}, repo.ErrNotFound
}

func (s *Server) registerListResources(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listResources", Method: http.MethodGet, Path: "/studio/v1/workspaces/{workspaceId}/resources", Summary: "List curated resources", Tags: []string{"Resources"}}, func(ctx context.Context, in *studioResourceListInput) (*studioResourcePageOutput, error) {
		workspaceID, workspace, _, err := s.resourceWorkspace(ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewResourceRepo(s.querier).ListPageByWorkspace(ctx, workspace.TenantID, workspaceID, repo.ResourceListOptions{Q: in.Q, Kind: in.Kind, Limit: in.Limit, Offset: in.Offset})
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("list failed")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		out := &studioResourcePageOutput{}
		out.Body = standardsPage[resourceView]{Items: make([]resourceView, 0, len(items)), TotalCount: total, Limit: limit, Offset: in.Offset}
		for i := range items {
			out.Body.Items = append(out.Body.Items, resourceViewFor(&items[i]))
		}
		return out, nil
	})
}

func (s *Server) registerCreateResource(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "createResource", Method: http.MethodPost, Path: "/studio/v1/workspaces/{workspaceId}/resources", Summary: "Create a resource reference", Tags: []string{"Resources"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createStudioResourceInput) (*studioResourceOutput, error) {
		workspaceID, workspace, membership, err := s.resourceWorkspace(ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		resource, err := resourceFromBody(in.Body, workspace.TenantID, workspaceID)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		created, err := repo.NewResourceRepo(s.querier).Create(ctx, resource)
		if err != nil {
			return nil, resourceError(err)
		}
		out := &studioResourceOutput{}
		out.Body = resourceViewFor(created)
		return out, nil
	})
}

func (s *Server) registerGetResource(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "getResource", Method: http.MethodGet, Path: "/studio/v1/resources/{resourceId}", Summary: "Get a resource", Tags: []string{"Resources"}}, func(ctx context.Context, in *studioResourcePathInput) (*studioResourceOutput, error) {
		id, err := decodeResourceID(in.ResourceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		resource, _, _, err := s.resourceForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		out := &studioResourceOutput{}
		out.Body = resourceViewFor(resource)
		return out, nil
	})
}

func (s *Server) registerUpdateResource(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "updateResource", Method: http.MethodPatch, Path: "/studio/v1/resources/{resourceId}", Summary: "Update a resource", Tags: []string{"Resources"}}, func(ctx context.Context, in *updateStudioResourceInput) (*studioResourceOutput, error) {
		id, err := decodeResourceID(in.ResourceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		resource, workspace, membership, err := s.resourceForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if resource.WorkspaceID == nil || *resource.WorkspaceID != workspace.ID {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		replacement, err := resourceFromBody(in.Body, workspace.TenantID, workspace.ID)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		updated, err := repo.NewResourceRepo(s.querier).Update(ctx, workspace.TenantID, workspace.ID, id, replacement)
		if err != nil {
			return nil, resourceError(err)
		}
		out := &studioResourceOutput{}
		out.Body = resourceViewFor(updated)
		return out, nil
	})
}

func (s *Server) registerDeleteResource(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "deleteResource", Method: http.MethodDelete, Path: "/studio/v1/resources/{resourceId}", Summary: "Delete a resource reference", Tags: []string{"Resources"}, DefaultStatus: http.StatusNoContent}, func(ctx context.Context, in *studioResourcePathInput) (*struct{}, error) {
		id, err := decodeResourceID(in.ResourceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		resource, workspace, membership, err := s.resourceForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if resource.WorkspaceID == nil || *resource.WorkspaceID != workspace.ID {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if err := repo.NewResourceRepo(s.querier).Delete(ctx, workspace.TenantID, workspace.ID, id); err != nil {
			return nil, resourceError(err)
		}
		return nil, nil
	})
}

func resourceError(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, repo.ErrConflict), errors.Is(err, repo.ErrForeignKey):
		return huma.Error409Conflict("resource conflict")
	case errors.Is(err, repo.ErrCheckViolation), errors.Is(err, repo.ErrPayloadTooLarge):
		return huma.Error400BadRequest("invalid resource metadata")
	default:
		return huma.Error503ServiceUnavailable("resource operation failed")
	}
}
