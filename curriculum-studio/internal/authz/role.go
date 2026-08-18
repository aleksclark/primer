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
