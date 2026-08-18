// Package api — workspace and membership HTTP handlers (S3).
//
// ID encoding: workspace IDs in response bodies are represented as opaque
// "ws_<32-hex>" strings; membership IDs as "mem_<32-hex>". Path parameters
// accept both the ws_/mem_ prefixed form and the bare RFC-4122 UUID form so
// that the existing S2 probe tests remain valid while new clients use the
// canonical prefixed form.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

// ─── opaque ID codec ─────────────────────────────────────────────────────────

const (
	wsIDPrefix  = "ws_"
	memIDPrefix = "mem_"
)

// encodeWSID returns the canonical ws_<32-hex> representation for a UUID.
func encodeWSID(id uuid.UUID) string {
	return wsIDPrefix + strings.ReplaceAll(id.String(), "-", "")
}

// decodeWSID accepts ws_<32-hex>, ws_<uuid>, or a bare RFC-4122 UUID.
func decodeWSID(s string) (uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, wsIDPrefix) {
		s = rehydrateUUID(s[len(wsIDPrefix):])
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid workspace id %q", s)
	}
	return id, nil
}

// encodeMemID returns the canonical mem_<32-hex> representation.
func encodeMemID(id uuid.UUID) string {
	return memIDPrefix + strings.ReplaceAll(id.String(), "-", "")
}

// decodeMemID accepts mem_<32-hex>, mem_<uuid>, or a bare RFC-4122 UUID.
func decodeMemID(s string) (uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, memIDPrefix) {
		s = rehydrateUUID(s[len(memIDPrefix):])
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid membership id %q", s)
	}
	return id, nil
}

// rehydrateUUID inserts hyphens back into a compact 32-hex UUID string.
// If the string is not 32 chars it is returned unchanged for uuid.Parse to
// reject with a readable error.
func rehydrateUUID(s string) string {
	if len(s) == 32 {
		return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
	}
	return s
}

// ─── response view types ─────────────────────────────────────────────────────

// WorkspaceView is the public representation of a workspace resource.
// The id field is always ws_<32-hex>; name is the primary display field.
type WorkspaceView struct {
	// ID is the opaque prefixed workspace identifier (ws_<32-hex>).
	ID string `json:"id" doc:"Opaque workspace ID (ws_<hex>)."`
	// Name is the human-readable display name — primary field per P3-S5.
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	TenantID  string `json:"tenantId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// WorkspacePage is the paginated list response for /workspaces.
type WorkspacePage struct {
	Items  []WorkspaceView `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// WorkspaceMembershipView is the public representation of a membership row.
type WorkspaceMembershipView struct {
	// ID is the opaque prefixed membership identifier (mem_<hex>).
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	SubjectRef  string `json:"subjectRef"`
	SubjectKind string `json:"subjectKind"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ─── view converters ─────────────────────────────────────────────────────────

func workspaceToView(w *domain.Workspace) WorkspaceView {
	return WorkspaceView{
		ID:        encodeWSID(w.ID),
		Name:      w.Name,
		Slug:      w.Slug,
		Kind:      w.Kind,
		Status:    w.Status,
		TenantID:  w.TenantID.String(),
		CreatedAt: w.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: w.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func membershipToView(m *domain.WorkspaceMembership) WorkspaceMembershipView {
	return WorkspaceMembershipView{
		ID:          encodeMemID(m.ID),
		WorkspaceID: encodeWSID(m.WorkspaceID),
		SubjectRef:  m.SubjectRef,
		SubjectKind: m.SubjectKind,
		DisplayName: m.DisplayName,
		Role:        m.Role,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   m.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ─── slug helpers ─────────────────────────────────────────────────────────────

// generateSlug derives a URL-safe slug from name, appending a short UUID suffix
// to ensure uniqueness within the tenant. This avoids collision-retry loops.
func generateSlug(name string) string {
	base := slugify(name)
	if base == "" {
		base = "workspace"
	}
	return base + "-" + uuid.NewString()[:8]
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prev := rune('-')
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prev = r
		} else if prev != '-' {
			b.WriteRune('-')
			prev = '-'
		}
	}
	return strings.Trim(b.String(), "-")
}

// ─── tenant resolution ───────────────────────────────────────────────────────

// resolveOrCreateTenant returns the subject's primary tenant inside a running
// transaction. If the subject has active memberships in existing workspaces the
// oldest workspace's tenant is reused; otherwise a new personal tenant is
// created. This ensures the very-first workspace creation (P3-S1 zero-
// membership path) always succeeds without a pre-existing tenant.
func resolveOrCreateTenant(ctx context.Context, q repo.Querier, subjectRef string) (uuid.UUID, error) {
	canon, err := domain.CanonicalizeSubjectRef(subjectRef)
	if err != nil {
		return uuid.Nil, err
	}

	// Look for an existing tenant through the membership → workspace chain.
	var existingTenantID uuid.UUID
	err = q.QueryRow(ctx, `
SELECT w.tenant_id
FROM curriculum_studio.workspace_memberships m
JOIN curriculum_studio.workspaces w ON w.id = m.workspace_id
WHERE m.subject_ref = $1 AND m.status = 'active'
ORDER BY w.created_at ASC
LIMIT 1`, canon.String()).Scan(&existingTenantID)
	if err == nil {
		return existingTenantID, nil
	}
	// pgx.ErrNoRows means the subject has no workspaces yet — create a tenant.

	// Derive a collision-resistant slug from the canonical subject ID.
	idHex := strings.ReplaceAll(canon.ID, "-", "")
	slug := "personal-" + idHex
	if len(slug) > 63 {
		slug = slug[:63]
	}
	t, err := repo.NewTenantRepo(q).Create(ctx, &domain.Tenant{
		Slug:   slug,
		Name:   "Personal",
		Status: domain.TenantStatusActive,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create personal tenant: %w", err)
	}
	return t.ID, nil
}

// ─── RegisterWorkspaceRoutes ─────────────────────────────────────────────────

// RegisterWorkspaceRoutes wires all S3 workspace and membership endpoints.
func (s *Server) RegisterWorkspaceRoutes(api huma.API) {
	s.registerListWorkspaces(api)
	s.registerCreateWorkspace(api)
	s.registerGetWorkspace(api)
	s.registerUpdateWorkspace(api)
	s.registerListMemberships(api)
	s.registerAddMembership(api)
	s.registerUpdateMembership(api)
	s.registerRevokeMembership(api)
}

// ─── listWorkspaces ───────────────────────────────────────────────────────────

type listWorkspacesInput struct {
	Q      string `query:"q"      doc:"Free-text search on name (case-insensitive)."`
	Sort   string `query:"sort"   doc:"Sort column: name (default), status, created_at."`
	Dir    string `query:"dir"    doc:"Sort direction: asc (default) or desc."`
	Limit  int    `query:"limit"  doc:"Page size 1–100; default 50."  minimum:"1" maximum:"100"`
	Offset int    `query:"offset" doc:"Zero-based row offset; default 0." minimum:"0"`
}

type listWorkspacesOutput struct {
	Body WorkspacePage
}

func (s *Server) registerListWorkspaces(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listWorkspaces",
		Method:      http.MethodGet,
		Path:        "/studio/v1/workspaces",
		Summary:     "List workspaces visible to the caller",
		Tags:        []string{"Workspaces"},
	}, func(ctx context.Context, in *listWorkspacesInput) (*listWorkspacesOutput, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		result, err := repo.NewWorkspaceRepo(s.querier).ListForSubject(ctx, principal.SubjectRef, repo.WorkspaceListOptions{
			Q:      in.Q,
			Sort:   in.Sort,
			Dir:    in.Dir,
			Limit:  in.Limit,
			Offset: in.Offset,
		})
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("list failed")
		}

		items := make([]WorkspaceView, 0, len(result.Items))
		for i := range result.Items {
			items = append(items, workspaceToView(&result.Items[i]))
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		out := &listWorkspacesOutput{}
		out.Body = WorkspacePage{
			Items:  items,
			Total:  result.Total,
			Limit:  limit,
			Offset: in.Offset,
		}
		return out, nil
	})
}

// ─── createWorkspace ─────────────────────────────────────────────────────────

type createWorkspaceBody struct {
	// Name is the required human-readable workspace name.
	Name string `json:"name" minLength:"1" maxLength:"200" doc:"Workspace display name (required)."`
	// Kind is the workspace type. Defaults to teacher.
	Kind string `json:"kind,omitempty" doc:"Workspace kind: school, family, coop, teacher (default), organization."`
}

type createWorkspaceInput struct {
	Body createWorkspaceBody
}

type createWorkspaceOutput struct {
	Body WorkspaceView
}

func (s *Server) registerCreateWorkspace(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "createWorkspace",
		Method:        http.MethodPost,
		Path:          "/studio/v1/workspaces",
		Summary:       "Create a new workspace",
		Tags:          []string{"Workspaces"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *createWorkspaceInput) (*createWorkspaceOutput, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		name := strings.TrimSpace(in.Body.Name)
		if name == "" {
			return nil, huma.Error422UnprocessableEntity("name is required")
		}
		kind := strings.TrimSpace(in.Body.Kind)
		if kind == "" {
			kind = domain.WorkspaceKindTeacher
		}
		if !validWorkspaceKind(kind) {
			return nil, huma.Error422UnprocessableEntity("invalid kind; must be one of: school, family, coop, teacher, organization")
		}

		var wsView WorkspaceView
		err := repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			tenantID, err := resolveOrCreateTenant(ctx, q, principal.SubjectRef)
			if err != nil {
				return fmt.Errorf("resolve tenant: %w", err)
			}

			slug := generateSlug(name)
			ws, err := repo.NewWorkspaceRepo(q).Create(ctx, &domain.Workspace{
				TenantID: tenantID,
				Slug:     slug,
				Name:     name,
				Kind:     kind,
				Status:   domain.WorkspaceStatusActive,
			})
			if err != nil {
				return fmt.Errorf("create workspace: %w", err)
			}

			canon, err := domain.CanonicalizeSubjectRef(principal.SubjectRef)
			if err != nil {
				return err
			}
			_, err = repo.NewMembershipRepo(q).Create(ctx, &domain.WorkspaceMembership{
				WorkspaceID: ws.ID,
				SubjectRef:  canon.String(),
				SubjectKind: canon.Kind,
				Role:        domain.MembershipRoleOwner,
				Status:      domain.MembershipStatusActive,
				DisplayName: "",
			})
			if err != nil {
				return fmt.Errorf("create owner membership: %w", err)
			}

			wsID := ws.ID
			_, err = repo.NewAuditRepo(q).Insert(ctx, &repo.AuditEvent{
				WorkspaceID:     &wsID,
				ActorSubjectRef: principal.SubjectRef,
				Action:          "workspace.create",
				EntityKind:      "workspace",
				EntityID:        &wsID,
				After:           json.RawMessage(`{"source":"api"}`),
			})
			if err != nil {
				return fmt.Errorf("audit workspace.create: %w", err)
			}

			wsView = workspaceToView(ws)
			return nil
		})
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict("workspace already exists")
			}
			return nil, huma.Error503ServiceUnavailable("create failed")
		}

		out := &createWorkspaceOutput{}
		out.Body = wsView
		return out, nil
	})
}

func validWorkspaceKind(k string) bool {
	switch k {
	case domain.WorkspaceKindSchool, domain.WorkspaceKindFamily,
		domain.WorkspaceKindCoop, domain.WorkspaceKindTeacher,
		domain.WorkspaceKindOrganization:
		return true
	}
	return false
}

// ─── getWorkspace ─────────────────────────────────────────────────────────────

type getWorkspaceInput struct {
	// WorkspaceID accepts ws_<32-hex> or a bare RFC-4122 UUID.
	WorkspaceID string `path:"workspaceID" doc:"Workspace ID (ws_<hex> or UUID)."`
}

type getWorkspaceOutput struct {
	Body WorkspaceView
}

func (s *Server) registerGetWorkspace(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getWorkspace",
		Method:      http.MethodGet,
		Path:        "/studio/v1/workspaces/{workspaceID}",
		Summary:     "Get a workspace by ID",
		Tags:        []string{"Workspaces"},
	}, func(ctx context.Context, in *getWorkspaceInput) (*getWorkspaceOutput, error) {
		wsID, err := decodeWSID(in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		// Membership check enforces isolation — returns 404 (not 403) to avoid
		// leaking workspace existence to callers who are not members.
		if _, ok := s.membershipFor(ctx, wsID); !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if s.querier == nil {
			return nil, huma.Error404NotFound("not found")
		}
		ws, err := repo.NewWorkspaceRepo(s.querier).GetByID(ctx, wsID)
		if err != nil || ws == nil {
			return nil, huma.Error404NotFound("not found")
		}
		out := &getWorkspaceOutput{}
		out.Body = workspaceToView(ws)
		return out, nil
	})
}

// ─── updateWorkspace ─────────────────────────────────────────────────────────

type updateWorkspaceBody struct {
	Name   string `json:"name"   minLength:"1" maxLength:"200" doc:"New display name."`
	Status string `json:"status,omitempty" doc:"New status: active or archived."`
}

type updateWorkspaceInput struct {
	WorkspaceID string `path:"workspaceID"`
	Body        updateWorkspaceBody
}

type updateWorkspaceOutput struct {
	Body WorkspaceView
}

func (s *Server) registerUpdateWorkspace(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "updateWorkspace",
		Method:      http.MethodPut,
		Path:        "/studio/v1/workspaces/{workspaceID}",
		Summary:     "Update workspace name or status (owner/admin only)",
		Tags:        []string{"Workspaces"},
	}, func(ctx context.Context, in *updateWorkspaceInput) (*updateWorkspaceOutput, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		wsID, err := decodeWSID(in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		mem, ok := s.membershipFor(ctx, wsID)
		if !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanManageMembers(mem.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		name := strings.TrimSpace(in.Body.Name)
		status := strings.TrimSpace(in.Body.Status)
		if name == "" {
			return nil, huma.Error422UnprocessableEntity("name is required")
		}
		if status != "" && status != domain.WorkspaceStatusActive && status != domain.WorkspaceStatusArchived {
			return nil, huma.Error422UnprocessableEntity("status must be active or archived")
		}

		// Fetch workspace to get tenant_id for the tenant-scoped update.
		existing, err := repo.NewWorkspaceRepo(s.querier).GetByID(ctx, wsID)
		if err != nil || existing == nil {
			return nil, huma.Error404NotFound("not found")
		}

		ws, err := repo.NewWorkspaceRepo(s.querier).Update(ctx, existing.TenantID, wsID, name, status)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, huma.Error404NotFound("not found")
			}
			return nil, huma.Error503ServiceUnavailable("update failed")
		}

		afterJSON, _ := json.Marshal(map[string]string{"name": ws.Name, "status": ws.Status})
		wsIDCopy := ws.ID
		_, _ = repo.NewAuditRepo(s.querier).Insert(ctx, &repo.AuditEvent{
			WorkspaceID:     &wsIDCopy,
			ActorSubjectRef: principal.SubjectRef,
			Action:          "workspace.update",
			EntityKind:      "workspace",
			EntityID:        &wsIDCopy,
			After:           json.RawMessage(afterJSON),
		})

		out := &updateWorkspaceOutput{}
		out.Body = workspaceToView(ws)
		return out, nil
	})
}

// ─── listMemberships ─────────────────────────────────────────────────────────

type listMembershipsInput struct {
	WorkspaceID string `path:"workspaceID"`
}

type listMembershipsOutput struct {
	Body struct {
		Items []WorkspaceMembershipView `json:"items"`
	}
}

func (s *Server) registerListMemberships(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listMemberships",
		Method:      http.MethodGet,
		Path:        "/studio/v1/workspaces/{workspaceID}/memberships",
		Summary:     "List memberships for a workspace",
		Tags:        []string{"Memberships"},
	}, func(ctx context.Context, in *listMembershipsInput) (*listMembershipsOutput, error) {
		wsID, err := decodeWSID(in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if _, ok := s.membershipFor(ctx, wsID); !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		rows, err := repo.NewMembershipRepo(s.querier).List(ctx, wsID)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("list failed")
		}
		items := make([]WorkspaceMembershipView, 0, len(rows))
		for i := range rows {
			items = append(items, membershipToView(&rows[i]))
		}
		out := &listMembershipsOutput{}
		out.Body.Items = items
		return out, nil
	})
}

// ─── addMembership ────────────────────────────────────────────────────────────

type addMembershipBody struct {
	// SubjectRef is the canonical identity:<uuid> or identity:svc:<id> ref.
	SubjectRef  string `json:"subjectRef"  minLength:"1"  doc:"Canonical subject reference."`
	SubjectKind string `json:"subjectKind,omitempty" doc:"human or service; inferred from subjectRef if omitted."`
	Role        string `json:"role"        minLength:"1"  doc:"Membership role: owner, admin, author, reviewer, viewer."`
	DisplayName string `json:"displayName,omitempty" doc:"Optional human-readable name."`
}

type addMembershipInput struct {
	WorkspaceID string `path:"workspaceID"`
	Body        addMembershipBody
}

type addMembershipOutput struct {
	Body WorkspaceMembershipView
}

func (s *Server) registerAddMembership(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "addMembership",
		Method:        http.MethodPost,
		Path:          "/studio/v1/workspaces/{workspaceID}/memberships",
		Summary:       "Add a membership to a workspace (owner/admin only)",
		Tags:          []string{"Memberships"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addMembershipInput) (*addMembershipOutput, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		wsID, err := decodeWSID(in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		mem, ok := s.membershipFor(ctx, wsID)
		if !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanManageMembers(mem.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		if !validMembershipRole(in.Body.Role) {
			return nil, huma.Error422UnprocessableEntity("invalid role; must be one of: owner, admin, author, reviewer, viewer")
		}

		var memView WorkspaceMembershipView
		err = repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			m, err := repo.NewMembershipRepo(q).Create(ctx, &domain.WorkspaceMembership{
				WorkspaceID: wsID,
				SubjectRef:  in.Body.SubjectRef,
				SubjectKind: in.Body.SubjectKind,
				Role:        in.Body.Role,
				Status:      domain.MembershipStatusActive,
				DisplayName: in.Body.DisplayName,
			})
			if err != nil {
				return err
			}

			wsIDCopy := wsID
			mIDCopy := m.ID
			afterJSON, _ := json.Marshal(map[string]string{
				"subject_ref": m.SubjectRef,
				"role":        m.Role,
				"status":      m.Status,
			})
			_, err = repo.NewAuditRepo(q).Insert(ctx, &repo.AuditEvent{
				WorkspaceID:     &wsIDCopy,
				ActorSubjectRef: principal.SubjectRef,
				Action:          "membership.create",
				EntityKind:      "membership",
				EntityID:        &mIDCopy,
				After:           json.RawMessage(afterJSON),
			})
			if err != nil {
				return fmt.Errorf("audit membership.create: %w", err)
			}

			memView = membershipToView(m)
			return nil
		})
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict("membership already exists")
			}
			if errors.Is(err, domain.ErrInvalidSubject) {
				return nil, huma.Error422UnprocessableEntity("invalid subjectRef")
			}
			return nil, huma.Error503ServiceUnavailable("add membership failed")
		}

		out := &addMembershipOutput{}
		out.Body = memView
		return out, nil
	})
}

func validMembershipRole(r string) bool {
	switch r {
	case domain.MembershipRoleOwner, domain.MembershipRoleAdmin,
		domain.MembershipRoleAuthor, domain.MembershipRoleReviewer,
		domain.MembershipRoleViewer:
		return true
	}
	return false
}

// ─── updateMembership ────────────────────────────────────────────────────────

type updateMembershipBody struct {
	Role string `json:"role" minLength:"1" doc:"New role for the membership."`
}

type updateMembershipInput struct {
	WorkspaceID  string `path:"workspaceID"`
	MembershipID string `path:"membershipID"`
	Body         updateMembershipBody
}

type updateMembershipOutput struct {
	Body WorkspaceMembershipView
}

func (s *Server) registerUpdateMembership(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "updateMembership",
		Method:      http.MethodPatch,
		Path:        "/studio/v1/workspaces/{workspaceID}/memberships/{membershipID}",
		Summary:     "Update membership role (owner/admin only)",
		Tags:        []string{"Memberships"},
	}, func(ctx context.Context, in *updateMembershipInput) (*updateMembershipOutput, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		wsID, err := decodeWSID(in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		callerMem, ok := s.membershipFor(ctx, wsID)
		if !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanManageMembers(callerMem.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}

		memID, err := decodeMemID(in.MembershipID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if !validMembershipRole(in.Body.Role) {
			return nil, huma.Error422UnprocessableEntity("invalid role")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		var memView WorkspaceMembershipView
		err = repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			m, err := repo.NewMembershipRepo(q).UpdateRole(ctx, wsID, memID, in.Body.Role)
			if err != nil {
				return err
			}

			wsIDCopy := wsID
			mIDCopy := m.ID
			afterJSON, _ := json.Marshal(map[string]string{"role": m.Role})
			_, err = repo.NewAuditRepo(q).Insert(ctx, &repo.AuditEvent{
				WorkspaceID:     &wsIDCopy,
				ActorSubjectRef: principal.SubjectRef,
				Action:          "membership.update",
				EntityKind:      "membership",
				EntityID:        &mIDCopy,
				After:           json.RawMessage(afterJSON),
			})
			if err != nil {
				return fmt.Errorf("audit membership.update: %w", err)
			}
			memView = membershipToView(m)
			return nil
		})
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, huma.Error404NotFound("not found")
			}
			return nil, huma.Error503ServiceUnavailable("update membership failed")
		}

		out := &updateMembershipOutput{}
		out.Body = memView
		return out, nil
	})
}

// ─── revokeMembership ────────────────────────────────────────────────────────

type revokeMembershipInput struct {
	WorkspaceID  string `path:"workspaceID"`
	MembershipID string `path:"membershipID"`
}

func (s *Server) registerRevokeMembership(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "revokeMembership",
		Method:        http.MethodDelete,
		Path:          "/studio/v1/workspaces/{workspaceID}/memberships/{membershipID}",
		Summary:       "Revoke a membership (owner/admin only)",
		Tags:          []string{"Memberships"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *revokeMembershipInput) (*struct{}, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		wsID, err := decodeWSID(in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		callerMem, ok := s.membershipFor(ctx, wsID)
		if !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanManageMembers(callerMem.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}

		memID, err := decodeMemID(in.MembershipID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if s.querier == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}

		err = repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			m, err := repo.NewMembershipRepo(q).UpdateStatus(ctx, wsID, memID, domain.MembershipStatusRevoked)
			if err != nil {
				return err
			}

			wsIDCopy := wsID
			mIDCopy := m.ID
			afterJSON, _ := json.Marshal(map[string]string{"status": m.Status})
			_, err = repo.NewAuditRepo(q).Insert(ctx, &repo.AuditEvent{
				WorkspaceID:     &wsIDCopy,
				ActorSubjectRef: principal.SubjectRef,
				Action:          "membership.revoke",
				EntityKind:      "membership",
				EntityID:        &mIDCopy,
				After:           json.RawMessage(afterJSON),
			})
			if err != nil {
				return fmt.Errorf("audit membership.revoke: %w", err)
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, huma.Error404NotFound("not found")
			}
			return nil, huma.Error503ServiceUnavailable("revoke failed")
		}

		return nil, nil
	})
}
