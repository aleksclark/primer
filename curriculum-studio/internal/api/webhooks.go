package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

const webhookIDPrefix = "wh_"

type webhookView struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspaceId"`
	URL         string   `json:"url"`
	EventTypes  []string `json:"eventTypes,omitempty"`
	Enabled     bool     `json:"enabled"`
	CreatedAt   string   `json:"createdAt"`
}

type webhookWriteBody struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"eventTypes,omitempty"`
	Enabled    *bool    `json:"enabled,omitempty"`
}

type webhookDeliveryView struct {
	ID          string `json:"id"`
	EndpointID  string `json:"endpointId"`
	EventID     string `json:"eventId"`
	HTTPStatus  int    `json:"httpStatus,omitempty"`
	Attempt     int    `json:"attempt"`
	AttemptedAt string `json:"attemptedAt"`
}

type studioWebhookPathInput struct {
	WebhookID string `path:"webhookId"`
}
type studioWebhookListInput struct {
	WorkspaceID string `path:"workspaceId"`
	Limit       int    `query:"limit" minimum:"1" maximum:"200" default:"25"`
	Offset      int    `query:"offset" minimum:"0" default:"0"`
}
type createStudioWebhookInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        webhookWriteBody
}
type updateStudioWebhookInput struct {
	WebhookID string `path:"webhookId"`
	Body      webhookWriteBody
}
type studioWebhookDeliveryListInput struct {
	WebhookID string `path:"webhookId"`
	Limit     int    `query:"limit" minimum:"1" maximum:"200" default:"25"`
	Offset    int    `query:"offset" minimum:"0" default:"0"`
}
type studioWebhookOutput struct{ Body webhookView }
type studioWebhookPageOutput struct {
	Body struct {
		Items      []webhookView `json:"items"`
		TotalCount int           `json:"totalCount"`
		Limit      int           `json:"limit"`
		Offset     int           `json:"offset"`
	}
}
type studioWebhookDeliveryPageOutput struct {
	Body struct {
		Items      []webhookDeliveryView `json:"items"`
		TotalCount int                   `json:"totalCount"`
		Limit      int                   `json:"limit"`
		Offset     int                   `json:"offset"`
	}
}

func encodeWebhookID(id uuid.UUID) string { return webhookIDPrefix + compactUUID(id) }

func decodeWebhookID(raw string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, webhookIDPrefix, "webhook")
}

func webhookViewFor(in *domain.WebhookEndpoint) webhookView {
	types := in.EventTypes
	if types == nil {
		types = []string{}
	}
	return webhookView{
		ID:          encodeWebhookID(in.ID),
		WorkspaceID: encodeWSID(in.WorkspaceID),
		URL:         in.URL,
		EventTypes:  types,
		Enabled:     in.Status == "active",
		CreatedAt:   formatTime(in.CreatedAt),
	}
}

func deliveryViewFor(in *domain.WebhookDelivery) webhookDeliveryView {
	at := in.CreatedAt
	if in.AttemptStartedAt != nil {
		at = *in.AttemptStartedAt
	} else if in.DeliveredAt != nil {
		at = *in.DeliveredAt
	}
	status := 0
	if in.Status == "delivered" {
		status = http.StatusOK
	}
	return webhookDeliveryView{
		ID:          in.ID.String(),
		EndpointID:  encodeWebhookID(in.EndpointID),
		EventID:     in.EventID.String(),
		HTTPStatus:  status,
		Attempt:     in.AttemptCount,
		AttemptedAt: formatTime(at),
	}
}

func knownEventType(v string) bool {
	switch v {
	case domain.EventCurriculumCreated, domain.EventPlanRevisionPublished,
		domain.EventMaterializationRequested, domain.EventMaterializationReady,
		domain.EventMaterializationFailed, domain.EventMaterializedItemSuperseded,
		domain.EventPlanChangeProposed:
		return true
	default:
		return false
	}
}

func parseWebhookURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("url is required")
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("url must be an http(s) URI")
	}
	return value, nil
}

func parseEventTypes(in []string) ([]string, error) {
	if in == nil {
		return []string{}, nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if !knownEventType(v) {
			return nil, fmt.Errorf("unknown event type")
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

func (s *Server) registerWebhookRoutes(api huma.API) {
	s.registerListWebhooks(api)
	s.registerCreateWebhook(api)
	s.registerGetWebhook(api)
	s.registerUpdateWebhook(api)
	s.registerDeleteWebhook(api)
	s.registerListWebhookDeliveries(api)
}

func (s *Server) webhookWorkspace(ctx context.Context, raw string) (uuid.UUID, MembershipView, error) {
	return workspaceIDFromPathForServer(s, ctx, raw)
}

func (s *Server) webhookForCaller(ctx context.Context, webhookID uuid.UUID) (*domain.WebhookEndpoint, MembershipView, error) {
	endpoint, err := repo.NewWebhookEndpointRepo(s.querier).Get(ctx, webhookID)
	if err != nil {
		return nil, MembershipView{}, repo.ErrNotFound
	}
	membership, ok := s.membershipFor(ctx, endpoint.WorkspaceID)
	if !ok {
		return nil, MembershipView{}, repo.ErrNotFound
	}
	return endpoint, membership, nil
}

func (s *Server) registerListWebhooks(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listWebhookEndpoints", Method: http.MethodGet, Path: "/studio/v1/workspaces/{workspaceId}/webhooks", Summary: "List webhook endpoints", Tags: []string{"Webhooks"}}, func(ctx context.Context, in *studioWebhookListInput) (*studioWebhookPageOutput, error) {
		workspaceID, _, err := s.webhookWorkspace(ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, err := repo.NewWebhookEndpointRepo(s.querier).List(ctx, workspaceID)
		if err != nil {
			return nil, webhookError(err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 25
		}
		offset := in.Offset
		if offset < 0 {
			offset = 0
		}
		total := len(items)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := items[offset:end]
		out := &studioWebhookPageOutput{}
		out.Body.Items = make([]webhookView, 0, len(page))
		out.Body.TotalCount = total
		out.Body.Limit = limit
		out.Body.Offset = offset
		for i := range page {
			out.Body.Items = append(out.Body.Items, webhookViewFor(&page[i]))
		}
		return out, nil
	})
}

func (s *Server) registerCreateWebhook(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "createWebhookEndpoint", Method: http.MethodPost, Path: "/studio/v1/workspaces/{workspaceId}/webhooks", Summary: "Create a webhook endpoint", Tags: []string{"Webhooks"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createStudioWebhookInput) (*studioWebhookOutput, error) {
		workspaceID, membership, err := s.webhookWorkspace(ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		hookURL, err := parseWebhookURL(in.Body.URL)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		types, err := parseEventTypes(in.Body.EventTypes)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		status := "active"
		if in.Body.Enabled != nil && !*in.Body.Enabled {
			status = "paused"
		}
		created, err := repo.NewWebhookEndpointRepo(s.querier).Create(ctx, &domain.WebhookEndpoint{
			WorkspaceID: workspaceID,
			URL:         hookURL,
			SecretRef:   "secret-ref:" + uuid.NewString(),
			EventTypes:  types,
			Status:      status,
		})
		if err != nil {
			return nil, webhookError(err)
		}
		return &studioWebhookOutput{Body: webhookViewFor(created)}, nil
	})
}

func (s *Server) registerGetWebhook(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "getWebhookEndpoint", Method: http.MethodGet, Path: "/studio/v1/webhooks/{webhookId}", Summary: "Get a webhook endpoint", Tags: []string{"Webhooks"}}, func(ctx context.Context, in *studioWebhookPathInput) (*studioWebhookOutput, error) {
		id, err := decodeWebhookID(in.WebhookID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		endpoint, _, err := s.webhookForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		return &studioWebhookOutput{Body: webhookViewFor(endpoint)}, nil
	})
}

func (s *Server) registerUpdateWebhook(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "updateWebhookEndpoint", Method: http.MethodPatch, Path: "/studio/v1/webhooks/{webhookId}", Summary: "Update a webhook endpoint", Tags: []string{"Webhooks"}}, func(ctx context.Context, in *updateStudioWebhookInput) (*studioWebhookOutput, error) {
		id, err := decodeWebhookID(in.WebhookID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		endpoint, membership, err := s.webhookForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		hookURL, err := parseWebhookURL(in.Body.URL)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		types, err := parseEventTypes(in.Body.EventTypes)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		endpoint.URL = hookURL
		endpoint.EventTypes = types
		if in.Body.Enabled != nil {
			if *in.Body.Enabled {
				endpoint.Status = "active"
			} else {
				endpoint.Status = "paused"
			}
		}
		updated, err := repo.NewWebhookEndpointRepo(s.querier).Update(ctx, endpoint)
		if err != nil {
			return nil, webhookError(err)
		}
		return &studioWebhookOutput{Body: webhookViewFor(updated)}, nil
	})
}

func (s *Server) registerDeleteWebhook(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "deleteWebhookEndpoint", Method: http.MethodDelete, Path: "/studio/v1/webhooks/{webhookId}", Summary: "Delete a webhook endpoint", Tags: []string{"Webhooks"}, DefaultStatus: http.StatusNoContent}, func(ctx context.Context, in *studioWebhookPathInput) (*struct{}, error) {
		id, err := decodeWebhookID(in.WebhookID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		_, membership, err := s.webhookForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if err := repo.NewWebhookEndpointRepo(s.querier).Delete(ctx, id); err != nil {
			return nil, webhookError(err)
		}
		return nil, nil
	})
}

func (s *Server) registerListWebhookDeliveries(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listWebhookDeliveries", Method: http.MethodGet, Path: "/studio/v1/webhooks/{webhookId}/deliveries", Summary: "List webhook deliveries", Tags: []string{"Webhooks"}}, func(ctx context.Context, in *studioWebhookDeliveryListInput) (*studioWebhookDeliveryPageOutput, error) {
		id, err := decodeWebhookID(in.WebhookID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		_, _, err = s.webhookForCaller(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewWebhookDeliveryRepo(s.querier).ListByEndpoint(ctx, id, in.Limit, in.Offset)
		if err != nil {
			return nil, webhookError(err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 25
		}
		out := &studioWebhookDeliveryPageOutput{}
		out.Body.Items = make([]webhookDeliveryView, 0, len(items))
		out.Body.TotalCount = total
		out.Body.Limit = limit
		out.Body.Offset = in.Offset
		for i := range items {
			out.Body.Items = append(out.Body.Items, deliveryViewFor(&items[i]))
		}
		return out, nil
	})
}

func webhookError(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, repo.ErrConflict), errors.Is(err, repo.ErrForeignKey):
		return huma.Error409Conflict("webhook conflict")
	case errors.Is(err, repo.ErrCheckViolation):
		return huma.Error400BadRequest("invalid webhook")
	default:
		return huma.Error503ServiceUnavailable("webhook operation failed")
	}
}
