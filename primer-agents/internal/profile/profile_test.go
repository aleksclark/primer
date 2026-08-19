package profile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/authn/jwttest"
	"github.com/aleksclark/primer/agents/internal/profile"
)

func TestBuildAllProfiles(t *testing.T) {
	t.Parallel()
	for _, n := range []profile.Name{profile.Tutor, profile.Admin, profile.Student, profile.Job} {
		spec, err := profile.Build(n)
		require.NoError(t, err, "profile %q", n)
		assert.NotEmpty(t, spec.AgentSpec.Type)
		assert.NotEmpty(t, spec.MAFConfig.Name)
	}
}

func TestBuildUnknownProfileErrors(t *testing.T) {
	t.Parallel()
	_, err := profile.Build("hacker_admin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown profile")
}

func TestStudentInvariantValidation(t *testing.T) {
	t.Parallel()
	spec, err := profile.Build(profile.Student)
	require.NoError(t, err)
	assert.Equal(t, 0, spec.AgentSpec.MaxChildren)
	assert.Equal(t, 0, spec.AgentSpec.MaxDepth)
	assert.Equal(t, 0, spec.AgentSpec.MaxTotalChildren)
	assert.Nil(t, spec.AgentSpec.Tools)
	assert.NoError(t, profile.ValidateStudentSpec(spec))
}

func TestValidateStudentSpecRejectsPositiveChildren(t *testing.T) {
	t.Parallel()
	spec, _ := profile.Build(profile.Student)
	spec.AgentSpec.MaxChildren = 1 // tamper
	require.Error(t, profile.ValidateStudentSpec(spec))
}

func TestAdmitParentAdminAllowed(t *testing.T) {
	t.Parallel()
	p := authn.Principal{
		SubjectRef: "identity:" + "abc",
		Kind:       authn.KindHuman,
		ClientID:   jwttest.DefaultClientID,
		Scopes:     []string{authn.ScopeRunsWrite, authn.ScopeSessionsWrite},
		Audience:   authn.AudiencePrimerAgents,
		Issuer:     jwttest.DefaultIssuer,
	}
	for _, n := range []profile.Name{profile.Tutor, profile.Admin} {
		admitted, err := profile.AdmitParentAdmin(p, n)
		require.NoError(t, err, "profile %q", n)
		assert.Equal(t, n, admitted)
	}
}

func TestAdmitParentAdminRejectsStudentAndJob(t *testing.T) {
	t.Parallel()
	p := authn.Principal{Scopes: []string{authn.ScopeRunsWrite}}
	for _, n := range []profile.Name{profile.Student, profile.Job} {
		_, err := profile.AdmitParentAdmin(p, n)
		require.ErrorIs(t, err, profile.ErrAdmissionDenied, "profile %q", n)
	}
}

func TestAdmitParentAdminRejectsUnknownProfile(t *testing.T) {
	t.Parallel()
	p := authn.Principal{Scopes: []string{authn.ScopeRunsWrite}}
	_, err := profile.AdmitParentAdmin(p, "hacker")
	require.ErrorIs(t, err, profile.ErrAdmissionDenied)
}

func TestAdmitParentAdminRejectsNoScope(t *testing.T) {
	t.Parallel()
	p := authn.Principal{Scopes: []string{authn.ScopeRunsRead}} // no write scope
	_, err := profile.AdmitParentAdmin(p, profile.Tutor)
	require.ErrorIs(t, err, profile.ErrAdmissionDenied)
}

func TestAdmitJobRequiresJobScope(t *testing.T) {
	t.Parallel()
	withScope := authn.Principal{Scopes: []string{authn.ScopeJobsWrite}}
	admitted, err := profile.AdmitJob(withScope)
	require.NoError(t, err)
	assert.Equal(t, profile.Job, admitted)

	noScope := authn.Principal{Scopes: []string{authn.ScopeRunsWrite}}
	_, err = profile.AdmitJob(noScope)
	require.ErrorIs(t, err, profile.ErrAdmissionDenied)
}

func TestAdmitStudentRequiresStudentScope(t *testing.T) {
	t.Parallel()
	withScope := authn.Principal{Scopes: []string{authn.ScopeStudentSession}}
	admitted, err := profile.AdmitStudent(withScope)
	require.NoError(t, err)
	assert.Equal(t, profile.Student, admitted)
}

func TestAdmitStudentFailsClosedWithoutScope(t *testing.T) {
	t.Parallel()
	noScope := authn.Principal{Scopes: []string{authn.ScopeRunsWrite}}
	_, err := profile.AdmitStudent(noScope)
	require.ErrorIs(t, err, profile.ErrStudentNotEnabled)
	assert.Contains(t, err.Error(), "reviewed Identity student credential required")
}

func TestAllowedSetDoesNotContainStudentOrJob(t *testing.T) {
	t.Parallel()
	assert.False(t, profile.Allowed[profile.Student],
		"student must not be in the caller-requestable Allowed set")
	assert.False(t, profile.Allowed[profile.Job],
		"job must not be in the caller-requestable Allowed set")
	assert.True(t, profile.Allowed[profile.Tutor])
	assert.True(t, profile.Allowed[profile.Admin])
}
