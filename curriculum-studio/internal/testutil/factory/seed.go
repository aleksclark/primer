package factory

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

// SeedMembership inserts an active membership for subject on workspace.
func SeedMembership(t *testing.T, q repo.Querier, workspaceID uuid.UUID, subjectRef, role string) *domain.WorkspaceMembership {
	t.Helper()
	require.NotEqual(t, uuid.Nil, workspaceID)
	return Membership(t, q, func(m *domain.WorkspaceMembership) {
		m.WorkspaceID = workspaceID
		m.SubjectRef = subjectRef
		m.Role = role
		m.Status = domain.MembershipStatusActive
	})
}
