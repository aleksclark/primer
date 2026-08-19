package mcp

import (
	"context"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

// ─── Interfaces ──────────────────────────────────────────────────────────────

// WorkspaceEntry is a minimal workspace projection returned by WorkspaceService.
// Tests inject fakes using this type; production code maps domain.Workspace → WorkspaceEntry.
type WorkspaceEntry struct {
	ID   string
	Name string
	Kind string
}

// WorkspaceService is the list surface for workspaces visible to a principal.
type WorkspaceService interface {
	ListForSubject(ctx context.Context, subjectRef, q string) ([]WorkspaceEntry, error)
}

// CurriculumService is the curriculum list/get surface.
type CurriculumService interface {
	ListForWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.Curriculum, error)
	Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Curriculum, error)
}

// PlanService is the draft revision read surface.
type PlanService interface {
	ListRevisions(ctx context.Context, workspaceID, curriculumID uuid.UUID) ([]domain.PlanRevision, error)
	GetGraph(ctx context.Context, workspaceID, revisionID uuid.UUID) (*domain.PlanGraph, error)
}

// AuditService records audit events from mutable MCP tool invocations.
type AuditService interface {
	RecordToolCall(ctx context.Context, ev ToolAuditEvent) error
}

// ToolAuditEvent is the sanitised payload for an MCP tool audit row.
type ToolAuditEvent struct {
	WorkspaceID *uuid.UUID
	SubjectRef  string
	ClientID    string // validated public client_id string from JWT; never azp/UUID
	Tool        string
	Outcome     string // "ok" | "denied" | "error"
}

// ─── Concrete implementations ────────────────────────────────────────────────

type repoWorkspaceService struct{ q repo.Querier }

func (s *repoWorkspaceService) ListForSubject(ctx context.Context, subjectRef, q string) ([]WorkspaceEntry, error) {
	result, err := repo.NewWorkspaceRepo(s.q).ListForSubject(ctx, subjectRef, repo.WorkspaceListOptions{Q: q, Limit: 100})
	if err != nil {
		return nil, err
	}
	out := make([]WorkspaceEntry, 0, len(result.Items))
	for _, w := range result.Items {
		out = append(out, WorkspaceEntry{ID: w.ID.String(), Name: w.Name, Kind: w.Kind})
	}
	return out, nil
}

type repoCurriculumService struct{ q repo.Querier }

func (s *repoCurriculumService) ListForWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.Curriculum, error) {
	return repo.NewCurriculumRepo(s.q).List(ctx, workspaceID)
}

func (s *repoCurriculumService) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Curriculum, error) {
	return repo.NewCurriculumRepo(s.q).Get(ctx, workspaceID, id)
}

type repoPlanService struct{ q repo.Querier }

func (s *repoPlanService) ListRevisions(ctx context.Context, workspaceID, curriculumID uuid.UUID) ([]domain.PlanRevision, error) {
	return repo.NewPlanRevisionRepo(s.q).List(ctx, workspaceID, curriculumID)
}

func (s *repoPlanService) GetGraph(ctx context.Context, workspaceID, revisionID uuid.UUID) (*domain.PlanGraph, error) {
	return repo.NewPlanGraphRepo(s.q).Load(ctx, workspaceID, revisionID)
}

type repoAuditService struct{ q repo.Querier }

func (s *repoAuditService) RecordToolCall(ctx context.Context, ev ToolAuditEvent) error {
	_, err := repo.NewAuditRepo(s.q).Insert(ctx, &repo.AuditEvent{
		WorkspaceID:     ev.WorkspaceID,
		ActorSubjectRef: ev.SubjectRef,
		Action:          "mcp.tool." + ev.Tool,
		EntityKind:      "mcp_tool",
		After:           auditPayload(ev),
	})
	return err
}

func auditPayload(ev ToolAuditEvent) []byte {
	// Minimal structured payload — no token fragments, no PII.
	return []byte(`{"tool":"` + escJSON(ev.Tool) + `","client_id":"` + escJSON(ev.ClientID) + `","outcome":"` + escJSON(ev.Outcome) + `"}`)
}

// escJSON escapes a string for safe embedding in a JSON string literal.
// Only the characters that need escaping in JSON string values are handled.
func escJSON(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		default:
			if s[i] < 0x20 {
				// drop control chars
				continue
			}
			out = append(out, s[i])
		}
	}
	return string(out)
}

// NewServicesFromQuerier builds the concrete Services bundle from any Querier
// (typically a *pgxpool.Pool). The querier must target the Studio DB only.
func NewServicesFromQuerier(q repo.Querier) Services {
	return Services{
		Workspaces: &repoWorkspaceService{q: q},
		Curricula:  &repoCurriculumService{q: q},
		Plan:       &repoPlanService{q: q},
		Audit:      &repoAuditService{q: q},
	}
}
