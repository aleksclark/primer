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
	// Empty membership state is fail-closed as well. A missing loader, database
	// failure, or service principal without an active membership must never turn
	// an opaque workspace handle into an authorization bypass.
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
