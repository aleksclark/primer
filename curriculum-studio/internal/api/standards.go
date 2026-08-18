package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/standards"
)

const (
	catalogIDPrefix  = "cat_"
	standardIDPrefix = "std_"
)

type standardsCatalogView struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspaceId,omitempty"`
	Source       string `json:"source"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
	Title        string `json:"title"`
	CreatedAt    string `json:"createdAt"`
}

type standardView struct {
	ID                string   `json:"id"`
	CatalogID         string   `json:"catalogId"`
	Code              string   `json:"code"`
	Source            string   `json:"source"`
	Subject           string   `json:"subject,omitempty"`
	Grade             string   `json:"grade,omitempty"`
	Domain            string   `json:"domain,omitempty"`
	Cluster           string   `json:"cluster,omitempty"`
	Description       string   `json:"description"`
	TCAPWeight        string   `json:"tcapWeight,omitempty"`
	MasteryCriteria   []string `json:"masteryCriteria,omitempty"`
	PrerequisiteCodes []string `json:"prerequisiteCodes,omitempty"`
}

type standardCrosswalkView struct {
	ID       string `json:"id"`
	FromCode string `json:"fromCode"`
	ToCode   string `json:"toCode"`
	Note     string `json:"note,omitempty"`
}

type standardsPage[T any] struct {
	Items      []T `json:"items"`
	TotalCount int `json:"totalCount"`
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
}

func encodeCatalogID(id uuid.UUID) string   { return catalogIDPrefix + compactUUID(id) }
func encodeStandardID(id uuid.UUID) string  { return standardIDPrefix + compactUUID(id) }
func encodeCrosswalkID(id uuid.UUID) string { return "cw_" + compactUUID(id) }

func compactUUID(id uuid.UUID) string { return strings.ReplaceAll(id.String(), "-", "") }

func decodeCatalogID(raw string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, catalogIDPrefix, "catalog")
}
func decodeStandardID(raw string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, standardIDPrefix, "standard")
}

func decodeOpaqueUUID(raw, prefix, label string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, prefix) {
		raw = raw[len(prefix):]
		if len(raw) == 32 {
			raw = raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
		}
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid %s id", label)
	}
	return id, nil
}

func catalogSource(code string) string { return standards.SourceFromCode(code) }

func validStandardSource(source string) bool { return standards.ValidSource(source) }

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func frameworkView(f *domain.StandardFramework) standardsCatalogView {
	v := standardsCatalogView{
		ID:           encodeCatalogID(f.ID),
		Source:       catalogSource(f.Code),
		Jurisdiction: f.Jurisdiction,
		Title:        f.Name,
		CreatedAt:    formatTime(f.CreatedAt),
	}
	if f.WorkspaceID != nil {
		v.WorkspaceID = encodeWSID(*f.WorkspaceID)
	}
	return v
}

func standardViewFor(s *domain.CatalogStandard, f *domain.StandardFramework) standardView {
	v := standardView{
		ID:          encodeStandardID(s.ID),
		CatalogID:   encodeCatalogID(s.FrameworkID),
		Code:        s.Code,
		Source:      catalogSource(f.Code),
		Subject:     s.SubjectCode,
		Grade:       s.GradeBand,
		Domain:      s.Domain,
		Cluster:     s.Cluster,
		Description: s.Description,
	}
	if len(s.Metadata) != 0 {
		var metadata struct {
			TCAPWeight        string   `json:"tcapWeight"`
			MasteryCriteria   []string `json:"masteryCriteria"`
			PrerequisiteCodes []string `json:"prerequisiteCodes"`
		}
		if json.Unmarshal(s.Metadata, &metadata) == nil {
			v.TCAPWeight = metadata.TCAPWeight
			v.MasteryCriteria = metadata.MasteryCriteria
			v.PrerequisiteCodes = metadata.PrerequisiteCodes
		}
	}
	return v
}

// workspaceForCatalog resolves a catalog through the caller's active local
// memberships. Global frameworks are visible through any membership; owned
// frameworks are visible only through their owning workspace.
func (s *Server) workspaceForCatalog(ctx context.Context, id uuid.UUID) (*domain.StandardFramework, uuid.UUID, error) {
	if s.querier == nil {
		return nil, uuid.Nil, repo.ErrClosed
	}
	frameworks := repo.NewFrameworkRepo(s.querier)
	for _, membership := range MembershipsFromContext(ctx) {
		framework, err := frameworks.Get(ctx, membership.WorkspaceID, id)
		if err == nil {
			return framework, membership.WorkspaceID, nil
		}
	}
	return nil, uuid.Nil, repo.ErrNotFound
}

func (s *Server) registerStandardsRoutes(api huma.API) {
	s.registerListCatalogs(api)
	s.registerImportCatalog(api)
	s.registerGetCatalog(api)
	s.registerListStandards(api)
	s.registerCreateStandard(api)
	s.registerGetStandard(api)
	s.registerListCrosswalks(api)
}

type listCatalogsInput struct {
	WorkspaceID string `path:"workspaceID"`
	Limit       int    `query:"limit" minimum:"1" maximum:"100"`
	Offset      int    `query:"offset" minimum:"0"`
}
type listCatalogsOutput struct {
	Body standardsPage[standardsCatalogView]
}

func (s *Server) registerListCatalogs(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listStandardsCatalogs", Method: http.MethodGet, Path: "/studio/v1/workspaces/{workspaceID}/standards-catalogs", Summary: "List standards catalogs", Tags: []string{"Standards"}}, func(ctx context.Context, in *listCatalogsInput) (*listCatalogsOutput, error) {
		workspaceID, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewFrameworkRepo(s.querier).ListVisiblePage(ctx, workspaceID, in.Limit, in.Offset)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("list failed")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		out := &listCatalogsOutput{}
		out.Body = standardsPage[standardsCatalogView]{Items: make([]standardsCatalogView, 0, len(items)), TotalCount: total, Limit: limit, Offset: in.Offset}
		for i := range items {
			out.Body.Items = append(out.Body.Items, frameworkView(&items[i]))
		}
		return out, nil
	})
}

func workspaceIDFromPathForServer(s *Server, ctx context.Context, raw string) (uuid.UUID, MembershipView, error) {
	id, err := decodeWSID(raw)
	if err != nil {
		return uuid.Nil, MembershipView{}, err
	}
	membership, ok := s.membershipFor(ctx, id)
	if !ok {
		return uuid.Nil, MembershipView{}, repo.ErrNotFound
	}
	return id, membership, nil
}

type importCatalogBody struct {
	Source       string              `json:"source" enum:"tennessee,common_core,custom"`
	Jurisdiction string              `json:"jurisdiction,omitempty"`
	Title        string              `json:"title" minLength:"1"`
	Standards    []standardWriteBody `json:"standards,omitempty"`
}
type standardWriteBody struct {
	Code              string   `json:"code" minLength:"1"`
	Source            string   `json:"source" enum:"tennessee,common_core,custom"`
	Subject           string   `json:"subject,omitempty"`
	Grade             string   `json:"grade,omitempty"`
	Domain            string   `json:"domain,omitempty"`
	Cluster           string   `json:"cluster,omitempty"`
	Description       string   `json:"description"`
	TCAPWeight        string   `json:"tcapWeight,omitempty"`
	MasteryCriteria   []string `json:"masteryCriteria,omitempty"`
	PrerequisiteCodes []string `json:"prerequisiteCodes,omitempty"`
}
type importCatalogInput struct {
	WorkspaceID string `path:"workspaceID"`
	Body        importCatalogBody
}
type importCatalogOutput struct{ Body standardsCatalogView }

func (s *Server) registerImportCatalog(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "importStandardsCatalog", Method: http.MethodPost, Path: "/studio/v1/workspaces/{workspaceID}/standards-catalogs", Summary: "Import or create a standards catalog", Tags: []string{"Standards"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *importCatalogInput) (*importCatalogOutput, error) {
		workspaceID, membership, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if !validStandardSource(in.Body.Source) || strings.TrimSpace(in.Body.Title) == "" {
			return nil, huma.Error400BadRequest("source and title are required")
		}
		for _, standard := range in.Body.Standards {
			if strings.TrimSpace(standard.Code) == "" || strings.TrimSpace(standard.Description) == "" || !validStandardSource(standard.Source) {
				return nil, huma.Error400BadRequest("each standard requires code, source, and description")
			}
		}
		code := standards.ImportCode(in.Body.Source, in.Body.Title)
		var framework *domain.StandardFramework
		err = repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			frameworks := repo.NewFrameworkRepo(q)
			framework, err = frameworks.GetByCode(ctx, &workspaceID, code)
			if err == nil {
				return nil
			} // stable import key: replay is idempotent
			if !errors.Is(err, repo.ErrNotFound) {
				return err
			}
			framework, err = frameworks.Create(ctx, &domain.StandardFramework{WorkspaceID: &workspaceID, Code: code, Name: strings.TrimSpace(in.Body.Title), Jurisdiction: strings.TrimSpace(in.Body.Jurisdiction)})
			if err != nil {
				return err
			}
			items := make([]domain.CatalogStandard, 0, len(in.Body.Standards))
			for _, input := range in.Body.Standards {
				metadata, _ := json.Marshal(map[string]any{"tcapWeight": input.TCAPWeight, "masteryCriteria": input.MasteryCriteria, "prerequisiteCodes": input.PrerequisiteCodes})
				items = append(items, domain.CatalogStandard{FrameworkID: framework.ID, Code: strings.TrimSpace(input.Code), SubjectCode: strings.TrimSpace(input.Subject), GradeBand: strings.TrimSpace(input.Grade), Domain: strings.TrimSpace(input.Domain), Cluster: strings.TrimSpace(input.Cluster), Description: strings.TrimSpace(input.Description), Metadata: metadata})
			}
			created, createErr := repo.NewCatalogStandardRepo(q).Import(ctx, workspaceID, framework.ID, items)
			if createErr != nil {
				return createErr
			}
			byCode := make(map[string]uuid.UUID, len(created))
			for _, item := range created {
				byCode[item.Code] = item.ID
			}
			prereqs := repo.NewCatalogPrereqRepo(q)
			for _, input := range in.Body.Standards {
				standardID := byCode[strings.TrimSpace(input.Code)]
				for _, prerequisiteCode := range input.PrerequisiteCodes {
					prerequisiteID := byCode[strings.TrimSpace(prerequisiteCode)]
					if prerequisiteID == uuid.Nil {
						return fmt.Errorf("prerequisite code %q is not in catalog", prerequisiteCode)
					}
					if _, err := prereqs.Create(ctx, workspaceID, domain.CatalogPrerequisite{StandardID: standardID, PrerequisiteID: prerequisiteID}); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict("catalog already exists")
			}
			if errors.Is(err, repo.ErrPrerequisiteCycle) {
				return nil, huma.Error409Conflict("catalog prerequisite cycle")
			}
			return nil, huma.Error400BadRequest("catalog import failed")
		}
		out := &importCatalogOutput{}
		out.Body = frameworkView(framework)
		return out, nil
	})
}

type catalogIDInput struct {
	CatalogID string `path:"catalogId"`
}
type getCatalogOutput struct{ Body standardsCatalogView }

func (s *Server) registerGetCatalog(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "getStandardsCatalog", Method: http.MethodGet, Path: "/studio/v1/standards-catalogs/{catalogId}", Summary: "Get a standards catalog", Tags: []string{"Standards"}}, func(ctx context.Context, in *catalogIDInput) (*getCatalogOutput, error) {
		id, err := decodeCatalogID(in.CatalogID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		framework, _, err := s.workspaceForCatalog(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		out := &getCatalogOutput{}
		out.Body = frameworkView(framework)
		return out, nil
	})
}

type listStandardsInput struct {
	CatalogID string `path:"catalogId"`
	Limit     int    `query:"limit" minimum:"1" maximum:"100"`
	Offset    int    `query:"offset" minimum:"0"`
	Q         string `query:"q"`
	Subject   string `query:"subject"`
	Grade     string `query:"grade"`
}
type listStandardsOutput struct{ Body standardsPage[standardView] }

func (s *Server) registerListStandards(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listStandards", Method: http.MethodGet, Path: "/studio/v1/standards-catalogs/{catalogId}/standards", Summary: "List standards in a catalog", Tags: []string{"Standards"}}, func(ctx context.Context, in *listStandardsInput) (*listStandardsOutput, error) {
		id, err := decodeCatalogID(in.CatalogID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		framework, workspaceID, err := s.workspaceForCatalog(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewCatalogStandardRepo(s.querier).ListPage(ctx, workspaceID, framework.ID, repo.StandardListOptions{Q: in.Q, Subject: in.Subject, Grade: in.Grade, Limit: in.Limit, Offset: in.Offset})
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("list failed")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		out := &listStandardsOutput{}
		out.Body = standardsPage[standardView]{Items: make([]standardView, 0, len(items)), TotalCount: total, Limit: limit, Offset: in.Offset}
		for i := range items {
			out.Body.Items = append(out.Body.Items, standardViewFor(&items[i], framework))
		}
		return out, nil
	})
}

type createStandardInput struct {
	CatalogID string `path:"catalogId"`
	Body      standardWriteBody
}
type createStandardOutput struct{ Body standardView }

func (s *Server) registerCreateStandard(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "createStandard", Method: http.MethodPost, Path: "/studio/v1/standards-catalogs/{catalogId}/standards", Summary: "Add a custom standard", Tags: []string{"Standards"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createStandardInput) (*createStandardOutput, error) {
		id, err := decodeCatalogID(in.CatalogID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		framework, workspaceID, err := s.workspaceForCatalog(ctx, id)
		if err != nil || framework.WorkspaceID == nil || *framework.WorkspaceID != workspaceID {
			return nil, huma.Error404NotFound("not found")
		}
		membership, ok := s.membershipFor(ctx, workspaceID)
		if !ok || !authz.CanMutate(membership.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if strings.TrimSpace(in.Body.Code) == "" || strings.TrimSpace(in.Body.Description) == "" || !validStandardSource(in.Body.Source) {
			return nil, huma.Error400BadRequest("code, source, and description are required")
		}
		metadata, _ := json.Marshal(map[string]any{"tcapWeight": in.Body.TCAPWeight, "masteryCriteria": in.Body.MasteryCriteria, "prerequisiteCodes": in.Body.PrerequisiteCodes})
		standard, err := repo.NewCatalogStandardRepo(s.querier).Create(ctx, workspaceID, &domain.CatalogStandard{FrameworkID: framework.ID, Code: in.Body.Code, SubjectCode: in.Body.Subject, GradeBand: in.Body.Grade, Domain: in.Body.Domain, Cluster: in.Body.Cluster, Description: in.Body.Description, Metadata: metadata})
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict("standard already exists")
			}
			return nil, huma.Error400BadRequest("create failed")
		}
		out := &createStandardOutput{}
		out.Body = standardViewFor(standard, framework)
		return out, nil
	})
}

type getStandardInput struct {
	StandardID string `path:"standardId"`
}
type getStandardOutput struct{ Body standardView }

func (s *Server) registerGetStandard(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "getStandard", Method: http.MethodGet, Path: "/studio/v1/standards/{standardId}", Summary: "Get a standard", Tags: []string{"Standards"}}, func(ctx context.Context, in *getStandardInput) (*getStandardOutput, error) {
		id, err := decodeStandardID(in.StandardID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		for _, membership := range MembershipsFromContext(ctx) {
			standard, err := repo.NewCatalogStandardRepo(s.querier).Get(ctx, membership.WorkspaceID, id)
			if err == nil {
				framework, ferr := repo.NewFrameworkRepo(s.querier).Get(ctx, membership.WorkspaceID, standard.FrameworkID)
				if ferr == nil {
					out := &getStandardOutput{}
					out.Body = standardViewFor(standard, framework)
					return out, nil
				}
			}
		}
		return nil, huma.Error404NotFound("not found")
	})
}

type listCrosswalksInput struct {
	CatalogID string `path:"catalogId"`
	Limit     int    `query:"limit" minimum:"1" maximum:"100"`
	Offset    int    `query:"offset" minimum:"0"`
}
type listCrosswalksOutput struct {
	Body standardsPage[standardCrosswalkView]
}

func (s *Server) registerListCrosswalks(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listStandardCrosswalks", Method: http.MethodGet, Path: "/studio/v1/standards-catalogs/{catalogId}/crosswalks", Summary: "List standard crosswalks", Tags: []string{"Standards"}}, func(ctx context.Context, in *listCrosswalksInput) (*listCrosswalksOutput, error) {
		id, err := decodeCatalogID(in.CatalogID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		framework, workspaceID, err := s.workspaceForCatalog(ctx, id)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewCrosswalkRepo(s.querier).ListForFramework(ctx, workspaceID, framework.ID, in.Limit, in.Offset)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("list failed")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		out := &listCrosswalksOutput{}
		out.Body = standardsPage[standardCrosswalkView]{Items: make([]standardCrosswalkView, 0, len(items)), TotalCount: total, Limit: limit, Offset: in.Offset}
		for _, item := range items {
			out.Body.Items = append(out.Body.Items, standardCrosswalkView{ID: encodeCrosswalkID(item.ID), FromCode: item.FromCode, ToCode: item.ToCode, Note: item.Notes})
		}
		return out, nil
	})
}
