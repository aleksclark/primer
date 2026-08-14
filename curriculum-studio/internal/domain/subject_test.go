package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func TestHumanSubjectRefRoundTrip(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ref := domain.HumanSubjectRef(id)
	require.Equal(t, "identity:11111111-1111-1111-1111-111111111111", ref)
	got, kind, err := domain.ParseSubjectRef(ref)
	require.NoError(t, err)
	require.Equal(t, domain.SubjectKindHuman, kind)
	require.Equal(t, id.String(), got)
	require.NoError(t, domain.ValidateSubjectRef(ref, domain.SubjectKindHuman))
}

func TestServiceSubjectRefRoundTrip(t *testing.T) {
	t.Parallel()
	ref, err := domain.ServiceSubjectRef("primer-lms")
	require.NoError(t, err)
	require.Equal(t, "identity:svc:primer-lms", ref)
	got, kind, err := domain.ParseSubjectRef(ref)
	require.NoError(t, err)
	require.Equal(t, domain.SubjectKindService, kind)
	require.Equal(t, "primer-lms", got)
	require.NoError(t, domain.ValidateSubjectRef(ref, domain.SubjectKindService))
}

func TestParseSubjectRefRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ref  string
	}{
		{"empty", ""},
		{"whitespace", " identity:11111111-1111-1111-1111-111111111111"},
		{"trailing space", "identity:11111111-1111-1111-1111-111111111111 "},
		{"no prefix", "11111111-1111-1111-1111-111111111111"},
		{"wrong prefix", "user:11111111-1111-1111-1111-111111111111"},
		{"bad uuid", "identity:not-a-uuid"},
		{"nil uuid", "identity:00000000-0000-0000-0000-000000000000"},
		{"nested colon human", "identity:foo:bar"},
		{"empty service", "identity:svc:"},
		{"service whitespace id", "identity:svc: primer"},
		{"service control", "identity:svc:abc\x00"},
		{"human control", "identity:11111111-1111-1111-1111-111111111111\n"},
		{"oversize", "identity:" + strings.Repeat("a", domain.MaxSubjectRefLen)},
		{"service bad char", "identity:svc:foo/bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := domain.ParseSubjectRef(tc.ref)
			require.Error(t, err)
			require.ErrorIs(t, err, domain.ErrInvalidSubject)
		})
	}
}

func TestValidateSubjectRefKindMismatch(t *testing.T) {
	t.Parallel()
	human := domain.HumanSubjectRef(uuid.New())
	require.ErrorIs(t, domain.ValidateSubjectRef(human, domain.SubjectKindService), domain.ErrInvalidSubject)
	svc, err := domain.ServiceSubjectRef("primer-lms")
	require.NoError(t, err)
	require.ErrorIs(t, domain.ValidateSubjectRef(svc, domain.SubjectKindHuman), domain.ErrInvalidSubject)
}

func TestServiceSubjectRefRejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := domain.ServiceSubjectRef("")
	require.ErrorIs(t, err, domain.ErrInvalidSubject)
	_, err = domain.ServiceSubjectRef("  ")
	require.ErrorIs(t, err, domain.ErrInvalidSubject)
	_, err = domain.ServiceSubjectRef(strings.Repeat("x", domain.MaxServiceSubjectIDLen+1))
	require.ErrorIs(t, err, domain.ErrInvalidSubject)
}

func TestCanonicalSubjectRefHumanLowercasesUUID(t *testing.T) {
	t.Parallel()
	raw := "IDENTITY:AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE"
	canon, err := domain.CanonicalizeSubjectRef(raw)
	require.NoError(t, err)
	require.Equal(t, "identity:aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", canon.String())
	require.Equal(t, domain.SubjectKindHuman, canon.Kind)
	require.Equal(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", canon.ID)
	// Mixed-case UUID body under lowercase prefix also canonicalizes.
	mixed := "identity:AaAaAaAa-BbBb-4CcC-8DdD-EeEeEeEeEeEe"
	c2, err := domain.CanonicalizeSubjectRef(mixed)
	require.NoError(t, err)
	require.Equal(t, "identity:aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", c2.String())
}

func TestCanonicalSubjectRefServiceDeterministic(t *testing.T) {
	t.Parallel()
	// Service IDs are case-sensitive tokens; canonical form is prefix + validated id.
	// Leading/trailing whitespace is stripped once at the outer boundary when building
	// via ServiceSubjectRef; CanonicalSubjectRef of a full ref rejects outer whitespace
	// but lowercases only the "identity:svc:" prefix letters if presented upper-cased.
	ref, err := domain.ServiceSubjectRef("Primer-LMS")
	require.NoError(t, err)
	require.Equal(t, "identity:svc:Primer-LMS", ref)
	canon, err := domain.CanonicalizeSubjectRef(ref)
	require.NoError(t, err)
	require.Equal(t, "identity:svc:Primer-LMS", canon.String())
	require.Equal(t, domain.SubjectKindService, canon.Kind)
	require.Equal(t, "Primer-LMS", canon.ID)

	// Prefix case variants still resolve to identity:svc:<id>.
	upperPrefix := "IDENTITY:SVC:primer-lms"
	c2, err := domain.CanonicalizeSubjectRef(upperPrefix)
	require.NoError(t, err)
	require.Equal(t, "identity:svc:primer-lms", c2.String())
}

func TestCanonicalSubjectRefRejectsInvalid(t *testing.T) {
	t.Parallel()
	_, err := domain.CanonicalizeSubjectRef("")
	require.ErrorIs(t, err, domain.ErrInvalidSubject)
	_, err = domain.CanonicalizeSubjectRef("not-a-ref")
	require.ErrorIs(t, err, domain.ErrInvalidSubject)
}
