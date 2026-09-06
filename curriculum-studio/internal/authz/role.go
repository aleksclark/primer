// Package authz evaluates local Studio membership roles and service scopes.
// Stytch / Identity roles are never consulted here.
package authz

import "github.com/aleksclark/primer/curriculum-studio/internal/domain"

// CanMutate reports whether a local membership role may POST/PATCH.
func CanMutate(role string) bool {
	switch role {
	case domain.MembershipRoleOwner, domain.MembershipRoleAdmin, domain.MembershipRoleAuthor:
		return true
	default:
		return false
	}
}

// CanManageMembers reports whether role may add, update, or revoke workspace
// memberships and change workspace metadata such as name and status. This is
// stricter than CanMutate: authors can create/edit curriculum content but may
// not change who belongs to the workspace.
func CanManageMembers(role string) bool {
	switch role {
	case domain.MembershipRoleOwner, domain.MembershipRoleAdmin:
		return true
	default:
		return false
	}
}

// CanReview is deliberately separate from authoring permission. Authors cannot
// approve their own work by virtue of their author role.
func CanReview(role string) bool {
	return role == domain.MembershipRoleReviewer
}

// HasScope reports whether the validated JWT scopes include required.
func HasScope(scopes []string, required string) bool {
	if required == "" {
		return false
	}
	for _, s := range scopes {
		if s == required {
			return true
		}
	}
	return false
}
