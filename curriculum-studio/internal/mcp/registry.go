package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerAllowedTools adds to srv only the tools the principal may call.
// allToolDefs is pre-sorted so the filtered subset is deterministic.
func registerAllowedTools(srv *sdkmcp.Server, h *Handler, a mcpAuthContext) {
	for _, d := range allToolDefs {
		if !isToolAllowed(a.Auth, a.Memberships, d.class) {
			continue
		}
		switch d.name {
		case "studio.workspaces.list":
			registerWorkspacesList(srv, h)
		case "studio.curricula.list":
			registerCurriculaList(srv, h)
		case "studio.curricula.get":
			registerCurriculaGet(srv, h)
		case "studio.drafts.get_graph":
			registerDraftsGetGraph(srv, h)
		default:
			registerUnavailable(srv, d.name)
		}
	}
}

// ─── studio.workspaces.list ───────────────────────────────────────────────────

type listWorkspacesIn struct {
	Q string `json:"q,omitempty"`
}

type workspaceItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type listWorkspacesOut struct {
	Workspaces []workspaceItem `json:"workspaces"`
}

func registerWorkspacesList(srv *sdkmcp.Server, h *Handler) {
	sdkmcp.AddTool(srv,
		&sdkmcp.Tool{
			Name:        "studio.workspaces.list",
			Description: "List workspaces the authenticated principal is a member of.",
		},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, in listWorkspacesIn) (*sdkmcp.CallToolResult, listWorkspacesOut, error) {
			h.Metrics.ToolCalls.Add(1)
			authCtx, ok := authFromContext(ctx)
			if !ok {
				return toolDenied(), listWorkspacesOut{}, nil
			}
			wss, err := h.services.Workspaces.ListForSubject(ctx, authCtx.Auth.SubjectRef, in.Q)
			if err != nil {
				return toolError(), listWorkspacesOut{}, nil
			}
			out := listWorkspacesOut{Workspaces: make([]workspaceItem, 0, len(wss))}
			for _, w := range wss {
				out.Workspaces = append(out.Workspaces, workspaceItem{
					ID:   w.ID,
					Name: w.Name,
					Kind: w.Kind,
				})
			}
			return toolOK(out), out, nil
		},
	)
}

// ─── studio.curricula.list ────────────────────────────────────────────────────

type listCurriculaIn struct {
	WorkspaceID string `json:"workspace_id"`
}

type curriculumItem struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Title       string `json:"title"`
	GradeBand   string `json:"grade_band,omitempty"`
	Status      string `json:"status"`
}

type listCurriculaOut struct {
	Curricula []curriculumItem `json:"curricula"`
}

func registerCurriculaList(srv *sdkmcp.Server, h *Handler) {
	sdkmcp.AddTool(srv,
		&sdkmcp.Tool{
			Name:        "studio.curricula.list",
			Description: "List curricula in a workspace.",
		},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, in listCurriculaIn) (*sdkmcp.CallToolResult, listCurriculaOut, error) {
			h.Metrics.ToolCalls.Add(1)
			authCtx, ok := authFromContext(ctx)
			if !ok {
				return toolDenied(), listCurriculaOut{}, nil
			}
			wsID, err := mustParseWorkspaceUUID(in.WorkspaceID, authCtx)
			if err != nil {
				return toolNotFound(), listCurriculaOut{}, nil
			}
			curs, err := h.services.Curricula.ListForWorkspace(ctx, wsID)
			if err != nil {
				return toolError(), listCurriculaOut{}, nil
			}
			out := listCurriculaOut{Curricula: make([]curriculumItem, 0, len(curs))}
			for _, c := range curs {
				out.Curricula = append(out.Curricula, curriculumItem{
					ID:          c.ID.String(),
					WorkspaceID: c.WorkspaceID.String(),
					Title:       c.Title,
					GradeBand:   c.GradeBand,
					Status:      c.Status,
				})
			}
			return toolOK(out), out, nil
		},
	)
}

// ─── studio.curricula.get ─────────────────────────────────────────────────────

type getCurriculumIn struct {
	WorkspaceID  string `json:"workspace_id"`
	CurriculumID string `json:"curriculum_id"`
}

func registerCurriculaGet(srv *sdkmcp.Server, h *Handler) {
	sdkmcp.AddTool(srv,
		&sdkmcp.Tool{
			Name:        "studio.curricula.get",
			Description: "Get a single curriculum by ID.",
		},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, in getCurriculumIn) (*sdkmcp.CallToolResult, curriculumItem, error) {
			h.Metrics.ToolCalls.Add(1)
			authCtx, ok := authFromContext(ctx)
			if !ok {
				return toolDenied(), curriculumItem{}, nil
			}
			wsID, err := mustParseWorkspaceUUID(in.WorkspaceID, authCtx)
			if err != nil {
				return toolNotFound(), curriculumItem{}, nil
			}
			curID, err := mustParseUUID(in.CurriculumID)
			if err != nil {
				return toolNotFound(), curriculumItem{}, nil
			}
			c, err := h.services.Curricula.Get(ctx, wsID, curID)
			if err != nil {
				return toolNotFound(), curriculumItem{}, nil
			}
			out := curriculumItem{
				ID:          c.ID.String(),
				WorkspaceID: c.WorkspaceID.String(),
				Title:       c.Title,
				GradeBand:   c.GradeBand,
				Status:      c.Status,
			}
			return toolOK(out), out, nil
		},
	)
}

// ─── studio.drafts.get_graph ──────────────────────────────────────────────────

type getGraphIn struct {
	WorkspaceID string `json:"workspace_id"`
	RevisionID  string `json:"revision_id"`
}

type graphItem struct {
	RevisionID   string `json:"revision_id"`
	CurriculumID string `json:"curriculum_id"`
	Status       string `json:"status"`
	Objectives   int    `json:"objectives"`
	Outcomes     int    `json:"outcomes"`
}

func registerDraftsGetGraph(srv *sdkmcp.Server, h *Handler) {
	sdkmcp.AddTool(srv,
		&sdkmcp.Tool{
			Name:        "studio.drafts.get_graph",
			Description: "Load the plan graph for a draft revision (summary).",
		},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, in getGraphIn) (*sdkmcp.CallToolResult, graphItem, error) {
			h.Metrics.ToolCalls.Add(1)
			authCtx, ok := authFromContext(ctx)
			if !ok {
				return toolDenied(), graphItem{}, nil
			}
			wsID, err := mustParseWorkspaceUUID(in.WorkspaceID, authCtx)
			if err != nil {
				return toolNotFound(), graphItem{}, nil
			}
			revID, err := mustParseUUID(in.RevisionID)
			if err != nil {
				return toolNotFound(), graphItem{}, nil
			}
			g, err := h.services.Plan.GetGraph(ctx, wsID, revID)
			if err != nil {
				return toolNotFound(), graphItem{}, nil
			}
			out := graphItem{
				RevisionID:   g.Revision.ID.String(),
				CurriculumID: g.Revision.CurriculumID.String(),
				Status:       g.Revision.Status,
				Objectives:   len(g.Objectives),
				Outcomes:     len(g.Outcomes),
			}
			return toolOK(out), out, nil
		},
	)
}

// ─── unavailable domain surfaces ─────────────────────────────────────────────

type unavailableIn struct{}
type unavailableOut struct {
	Message string `json:"message"`
}

func registerUnavailable(srv *sdkmcp.Server, name string) {
	// Keep the frozen inventory discoverable, but fail closed until the owning
	// domain service is wired. Returning a successful fixed payload would make
	// MCP appear to have mutated or read Studio state when it had not.
	sdkmcp.AddTool(srv,
		&sdkmcp.Tool{Name: name, Description: "Unavailable until the corresponding Studio domain service is wired."},
		func(_ context.Context, _ *sdkmcp.CallToolRequest, _ unavailableIn) (*sdkmcp.CallToolResult, unavailableOut, error) {
			out := unavailableOut{Message: fmt.Sprintf("%s is unavailable", name)}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: out.Message}},
				IsError: true,
			}, out, nil
		},
	)
}

// ─── result helpers ───────────────────────────────────────────────────────────

func toolOK(v any) *sdkmcp.CallToolResult {
	txt, _ := json.Marshal(v)
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(txt)}},
		StructuredContent: v,
	}
}

func toolError() *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "internal error"}},
		IsError: true,
	}
}

func toolDenied() *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "forbidden"}},
		IsError: true,
	}
}

func toolNotFound() *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "not found"}},
		IsError: true,
	}
}
