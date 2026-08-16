package domain

import (
	"time"

	"github.com/google/uuid"
)

// Stytch principal identifiers are provider-issued opaque identifiers. They
// are kept as separate columns so delimiters or concatenation can never make
// two different principals compare equal.
const MaxStytchPrincipalFieldLen = 255

const (
	MaxStytchProjectIDLen      = MaxStytchPrincipalFieldLen
	MaxStytchOrganizationIDLen = MaxStytchPrincipalFieldLen
	MaxStytchMemberIDLen       = MaxStytchPrincipalFieldLen
)

// StytchPrincipal is the immutable identity tuple used for account mapping.
// Email and display metadata are deliberately not part of this key.
type StytchPrincipal struct {
	ProjectID      string
	OrganizationID string
	MemberID       string
}

// StytchTuple is a descriptive alias for callers that name the key a tuple.
type StytchTuple = StytchPrincipal

// StytchMappingInput contains the immutable provider tuple and optional account
// profile metadata. Profile changes never alter the tuple's account ownership.
type StytchMappingInput struct {
	StytchPrincipal
	DisplayName  string
	PrimaryEmail *string
}

// StytchMapping is the normalized persisted link from a provider tuple to an
// Identity account UUID.
type StytchMapping struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	ProjectID      string
	OrganizationID string
	MemberID       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ValidateStytchPrincipal validates every component independently. Empty,
// invalid UTF-8, control-containing, and oversized values are rejected.
func ValidateStytchPrincipal(p StytchPrincipal) error {
	for field, value := range map[string]string{
		"project_id":      p.ProjectID,
		"organization_id": p.OrganizationID,
		"member_id":       p.MemberID,
	} {
		if value == "" {
			return invalidf(field, "must not be empty")
		}
		if err := validateTextField(field, value, MaxStytchPrincipalFieldLen); err != nil {
			return err
		}
	}
	return nil
}

// ValidateStytchMappingInput validates the tuple and account metadata at the
// same boundary used by repository writes.
func ValidateStytchMappingInput(in StytchMappingInput) error {
	if err := ValidateStytchPrincipal(in.StytchPrincipal); err != nil {
		return err
	}
	if err := ValidateDisplayName(in.DisplayName); err != nil {
		return err
	}
	if in.PrimaryEmail != nil {
		if err := ValidateEmail(*in.PrimaryEmail); err != nil {
			return err
		}
	}
	return nil
}
