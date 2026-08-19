package mcp

import (
	"context"

	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
)

// MCP scope constants (Identity-issued; enforced in Studio).
// These freeze the planning vocabulary from the design doc §4.3.
// Exact strings are ratified in I7/IB3; update this const block then.
const (
	ScopeMCPBase = "studio:mcp"     // base connect / tools/list
	ScopeRead    = "studio:read"    // discovery, search, read graph, findings
	ScopeDraft   = "studio:draft"   // create/patch draft, validate
	ScopePublish = "studio:publish" // generate proposal; confirm is human-only step-up
)

// authContextKey is the request-context key for a validated MCP AuthContext.
type authContextKey struct{}

// mcpAuthContext carries the validated principal for one MCP HTTP request.
type mcpAuthContext struct {
	Auth        authn.AuthContext
	Memberships []MembershipView
}

// MembershipView is the exported workspace membership projection used by MCP tools
// and their test fakes.
type MembershipView struct {
	WorkspaceID   string
	WorkspaceName string
	Role          string
}

// membershipView is the package-internal alias so existing code compiles.
type membershipView = MembershipView

// authFromContext extracts the MCP auth context set by the auth middleware.
// Returns (zero, false) when the middleware has not run or auth failed.
func authFromContext(ctx context.Context) (mcpAuthContext, bool) {
	v, ok := ctx.Value(authContextKey{}).(mcpAuthContext)
	return v, ok
}

// withAuthContext attaches auth to the context.
func withAuthContext(ctx context.Context, a mcpAuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, a)
}

// toolClass classifies what a tool requires.
type toolClass int

const (
	classRead    toolClass = iota // list/read tools
	classDraft                    // create/patch/validate
	classPublish                  // propose/confirm (human-only)
)

// toolDef describes a single MCP tool's authorization requirements.
type toolDef struct {
	name  string
	class toolClass
}

// allToolDefs is the full inventory in sorted order (stable sort).
// tools/list is filtered by principal scopes + memberships before returning.
var allToolDefs = []toolDef{
	{"studio.curricula.get", classRead},
	{"studio.curricula.list", classRead},
	{"studio.drafts.create", classDraft},
	{"studio.drafts.get_findings", classRead},
	{"studio.drafts.get_graph", classRead},
	{"studio.drafts.patch_graph", classDraft},
	{"studio.drafts.validate", classDraft},
	{"studio.publish.confirm", classPublish},
	{"studio.publish.propose", classPublish},
	{"studio.resources.search", classRead},
	{"studio.standards.search", classRead},
	{"studio.workspaces.list", classRead},
}

// isToolAllowed reports whether the principal with the given auth context
// may see/call a tool of the given class.
// Service actors may never call publish tools.
func isToolAllowed(a authn.AuthContext, mems []membershipView, class toolClass) bool {
	if len(mems) == 0 {
		return false
	}
	switch class {
	case classRead:
		// Human with active membership, or service with studio:read scope.
		if a.Kind == authn.KindHuman {
			return true
		}
		return authz.HasScope(a.Scopes, ScopeRead)
	case classDraft:
		// Human author with active membership, or service with studio:draft scope.
		if a.Kind == authn.KindHuman {
			return hasMutatingRole(mems)
		}
		return authz.HasScope(a.Scopes, ScopeDraft)
	case classPublish:
		// Human-only; service always denied.
		return a.Kind == authn.KindHuman
	default:
		return false
	}
}

// hasMutatingRole reports whether any active membership has an author/admin/owner role.
func hasMutatingRole(mems []membershipView) bool {
	for _, m := range mems {
		if authz.CanMutate(m.Role) {
			return true
		}
	}
	return false
}

// AllowedToolNames returns the sorted set of tool names the principal may see.
// Exported for tests.
func AllowedToolNames(a authn.AuthContext, mems []MembershipView) []string {
	return allowedToolNames(a, mems)
}

func allowedToolNames(a authn.AuthContext, mems []membershipView) []string {
	out := make([]string, 0, len(allToolDefs))
	for _, d := range allToolDefs {
		if isToolAllowed(a, mems, d.class) {
			out = append(out, d.name)
		}
	}
	return out // allToolDefs is pre-sorted; output is deterministic
}
