package api

import (
	"context"
	"net/http"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/danielgtaylor/huma/v2"
)

type collabWorkspacePageInput struct {
	WorkspaceID string `path:"workspaceId"`
	Limit       int    `query:"limit" minimum:"1" maximum:"100" default:"25"`
	Offset      int    `query:"offset" minimum:"0" default:"0"`
}
type saveLibraryInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        struct {
		RevisionID string `json:"revisionId" minLength:"1"`
		UnitID     string `json:"unitId" minLength:"1"`
		Name       string `json:"name,omitempty" maxLength:"500"`
	}
}
type libraryPath struct {
	WorkspaceID string `path:"workspaceId"`
	EntryID     string `path:"entryId"`
}
type copyLibraryInput struct {
	RevisionID string `path:"revisionId"`
	EntryID    string `path:"entryId"`
}
type LibraryEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}
type libraryOutput struct{ Body LibraryEntry }
type libraryPageOutput struct{ Body standardsPage[LibraryEntry] }

func libraryView(v *domain.UnitLibraryEntry) LibraryEntry {
	return LibraryEntry{ID: encodePlanID("lib_", v.ID), Name: v.Name, CreatedAt: v.CreatedAt}
}

type createTemplateInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        struct {
		Code      string              `json:"code" minLength:"1" maxLength:"100"`
		Name      string              `json:"name" minLength:"1" maxLength:"500"`
		BriefType CurriculumTemplate  `json:"briefType"`
		Seed      domain.TemplateSeed `json:"seed"`
	}
}
type Template struct {
	Code      string              `json:"code"`
	Name      string              `json:"name"`
	BriefType CurriculumTemplate  `json:"briefType"`
	Seed      domain.TemplateSeed `json:"seed"`
}
type templateOutput struct{ Body Template }
type templatePageOutput struct{ Body standardsPage[Template] }

func templateView(v *domain.PlanTemplate) (Template, error) {
	seed, err := repo.DecodeTemplateSeed(v.Seed)
	return Template{Code: v.Code, Name: v.Name, BriefType: CurriculumTemplate(v.BriefType), Seed: seed}, err
}

func (s *Server) registerLibraryRoutes(api huma.API) {
	save := authoringOperation("saveUnitToLibrary", http.MethodPost, "/studio/v1/workspaces/{workspaceId}/unit-library", "Collaboration", "Save a reusable unit snapshot")
	save.DefaultStatus = http.StatusCreated
	huma.Register(api, save, func(ctx context.Context, in *saveLibraryInput) (*libraryOutput, error) {
		ws, m, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if err = requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		rev, owner, _, err := s.revisionForCaller(ctx, in.Body.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		if ws != owner {
			return nil, huma.Error404NotFound("not found")
		}
		unit, err := decodePlanID(in.Body.UnitID, unitIDPrefix)
		if err != nil {
			return nil, huma.Error404NotFound("unit not found")
		}
		entry, err := repo.NewUnitLibraryRepo(s.querier).SaveUnit(ctx, ws, rev.ID, unit, in.Body.Name)
		if err != nil {
			return nil, planError(err)
		}
		return &libraryOutput{Body: libraryView(entry)}, nil
	})
	huma.Register(api, authoringOperation("listUnitLibrary", http.MethodGet, "/studio/v1/workspaces/{workspaceId}/unit-library", "Collaboration", "List saved units"), func(ctx context.Context, in *collabWorkspacePageInput) (*libraryPageOutput, error) {
		ws, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewUnitLibraryRepo(s.querier).ListPage(ctx, ws, in.Limit, in.Offset)
		if err != nil {
			return nil, planError(err)
		}
		out := &libraryPageOutput{Body: standardsPage[LibraryEntry]{Items: []LibraryEntry{}, TotalCount: total, Limit: in.Limit, Offset: in.Offset}}
		for i := range items {
			out.Body.Items = append(out.Body.Items, libraryView(&items[i]))
		}
		return out, nil
	})
	huma.Register(api, authoringOperation("getUnitLibraryEntry", http.MethodGet, "/studio/v1/workspaces/{workspaceId}/unit-library/{entryId}", "Collaboration", "Get a saved unit"), func(ctx context.Context, in *libraryPath) (*libraryOutput, error) {
		ws, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		id, err := decodePlanID(in.EntryID, "lib_")
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		entry, err := repo.NewUnitLibraryRepo(s.querier).Get(ctx, ws, id)
		if err != nil {
			return nil, planError(err)
		}
		return &libraryOutput{Body: libraryView(entry)}, nil
	})
	copyOp := authoringOperation("copyUnitIntoRevision", http.MethodPost, "/studio/v1/revisions/{revisionId}/unit-library/{entryId}", "Collaboration", "Copy a library unit into a draft")
	copyOp.DefaultStatus = http.StatusCreated
	huma.Register(api, copyOp, func(ctx context.Context, in *copyLibraryInput) (*nodeResponse, error) {
		rev, ws, m, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		if err = requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		id, err := decodePlanID(in.EntryID, "lib_")
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		unit, err := repo.NewUnitLibraryRepo(s.querier).CopyIntoRevision(ctx, ws, rev.ID, id)
		if err != nil {
			return nil, planError(err)
		}
		return &nodeResponse{Body: PlanNode{ID: encodePlanID(unitIDPrefix, unit.ID), RevisionID: in.RevisionID, Kind: "unit", Title: unit.Title, Position: unit.Position, Attributes: map[string]string{"code": unit.Code}}}, nil
	})
	templateOp := authoringOperation("createPlanTemplate", http.MethodPost, "/studio/v1/workspaces/{workspaceId}/templates", "Collaboration", "Save a workspace plan template")
	templateOp.DefaultStatus = http.StatusCreated
	huma.Register(api, templateOp, func(ctx context.Context, in *createTemplateInput) (*templateOutput, error) {
		ws, m, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if err = requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		t, err := repo.NewTemplateRepo(s.querier).Create(ctx, ws, in.Body.Code, in.Body.Name, string(in.Body.BriefType), in.Body.Seed)
		if err != nil {
			return nil, planError(err)
		}
		view, err := templateView(t)
		if err != nil {
			return nil, planError(err)
		}
		return &templateOutput{Body: view}, nil
	})
	huma.Register(api, authoringOperation("listPlanTemplates", http.MethodGet, "/studio/v1/workspaces/{workspaceId}/templates", "Collaboration", "List global and workspace templates"), func(ctx context.Context, in *collabWorkspacePageInput) (*templatePageOutput, error) {
		ws, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		items, total, err := repo.NewTemplateRepo(s.querier).ListPage(ctx, ws, in.Limit, in.Offset)
		if err != nil {
			return nil, planError(err)
		}
		out := &templatePageOutput{Body: standardsPage[Template]{Items: []Template{}, TotalCount: total, Limit: in.Limit, Offset: in.Offset}}
		for i := range items {
			v, err := templateView(&items[i])
			if err != nil {
				return nil, planError(err)
			}
			out.Body.Items = append(out.Body.Items, v)
		}
		return out, nil
	})
}
