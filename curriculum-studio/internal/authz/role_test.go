package authz_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func TestRoleMatrixWrite(t *testing.T) {
	t.Parallel()
	assert.True(t, authz.CanMutate(domain.MembershipRoleOwner))
	assert.True(t, authz.CanMutate(domain.MembershipRoleAdmin))
	assert.True(t, authz.CanMutate(domain.MembershipRoleAuthor))
	assert.False(t, authz.CanMutate(domain.MembershipRoleReviewer))
	assert.False(t, authz.CanMutate(domain.MembershipRoleViewer))
	assert.False(t, authz.CanMutate(""))
	assert.False(t, authz.CanMutate("stytch-admin"))
}

func TestServiceScopeRequired(t *testing.T) {
	t.Parallel()
	assert.True(t, authz.HasScope([]string{"openid", "materialize:write"}, "materialize:write"))
	assert.False(t, authz.HasScope([]string{"openid"}, "materialize:write"))
	assert.False(t, authz.HasScope(nil, "materialize:write"))
}
