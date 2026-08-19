package mcp

// Services bundles injectable app-service interfaces used by MCP tools.
// Tool handlers call these interfaces only — never repo directly.
// Concrete implementations backed by repo live in services_impl.go.
type Services struct {
	Workspaces WorkspaceService
	Curricula  CurriculumService
	Plan       PlanService
	Audit      AuditService
	// Standards and Resources are omitted until a cross-framework search
	// helper is available; those inventory entries fail closed meanwhile.
}
