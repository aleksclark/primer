package domain_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
)

func TestValidateStytchPrincipalTuple(t *testing.T) {
	t.Parallel()

	valid := domain.StytchPrincipal{
		ProjectID:      "project-test",
		OrganizationID: "organization-test",
		MemberID:       "member-test",
	}
	require.NoError(t, domain.ValidateStytchPrincipal(valid))

	for _, tc := range []struct {
		name string
		edit func(*domain.StytchPrincipal)
	}{
		{"missing project", func(v *domain.StytchPrincipal) { v.ProjectID = "" }},
		{"missing organization", func(v *domain.StytchPrincipal) { v.OrganizationID = "" }},
		{"missing member", func(v *domain.StytchPrincipal) { v.MemberID = "" }},
		{"project control", func(v *domain.StytchPrincipal) { v.ProjectID = "project\n" }},
		{"organization control", func(v *domain.StytchPrincipal) { v.OrganizationID = "organization\x00" }},
		{"member control", func(v *domain.StytchPrincipal) { v.MemberID = "member\t" }},
		{"invalid utf8", func(v *domain.StytchPrincipal) { v.MemberID = string([]byte{0xff, 0xfe}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := valid
			tc.edit(&got)
			err := domain.ValidateStytchPrincipal(got)
			require.Error(t, err)
			require.True(t, errors.Is(err, domain.ErrInvalid))
		})
	}
}

func TestValidateStytchPrincipalTupleBoundsByRunes(t *testing.T) {
	t.Parallel()

	max := strings.Repeat("x", domain.MaxStytchPrincipalFieldLen)
	require.NoError(t, domain.ValidateStytchPrincipal(domain.StytchPrincipal{
		ProjectID: max, OrganizationID: "org", MemberID: "member",
	}))
	require.Error(t, domain.ValidateStytchPrincipal(domain.StytchPrincipal{
		ProjectID: max + "x", OrganizationID: "org", MemberID: "member",
	}))

	unicodeID := strings.Repeat("界", domain.MaxStytchPrincipalFieldLen)
	require.Equal(t, domain.MaxStytchPrincipalFieldLen, utf8.RuneCountInString(unicodeID))
	require.NoError(t, domain.ValidateStytchPrincipal(domain.StytchPrincipal{
		ProjectID: "project", OrganizationID: unicodeID, MemberID: "member",
	}))
}

func TestValidateStytchMappingInputValidatesAccountMetadata(t *testing.T) {
	t.Parallel()

	email := "same@example.com"
	in := domain.StytchMappingInput{
		StytchPrincipal: domain.StytchPrincipal{
			ProjectID: "project-test", OrganizationID: "organization-test", MemberID: "member-test",
		},
		DisplayName:  "First display",
		PrimaryEmail: &email,
	}
	require.NoError(t, domain.ValidateStytchMappingInput(in))

	in.DisplayName = strings.Repeat("x", domain.MaxDisplayNameLen+1)
	require.Error(t, domain.ValidateStytchMappingInput(in))
}
