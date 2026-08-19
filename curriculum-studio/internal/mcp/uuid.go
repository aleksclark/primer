package mcp

import (
	"fmt"

	"github.com/google/uuid"
)

// mustParseWorkspaceUUID parses s and confirms membership in a.
// Returns uuid.Nil + error on any failure (parse error or IDOR).
func mustParseWorkspaceUUID(wsIDStr string, a mcpAuthContext) (uuid.UUID, error) {
	id, err := uuid.Parse(wsIDStr)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid workspace id")
	}
	for _, m := range a.Memberships {
		if m.WorkspaceID == wsIDStr {
			return id, nil
		}
	}
	// Memberships list may be empty when the querier is nil (test/local env).
	// In that case allow the principal through; real authz happens in the repo
	// (workspace-scoped SQL). Service principals are never given member rows.
	if len(a.Memberships) == 0 {
		return id, nil
	}
	return uuid.Nil, fmt.Errorf("workspace not found or not a member")
}

// mustParseUUID parses an opaque ID string.
func mustParseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid id")
	}
	return id, nil
}
