package domain_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
)

func TestValidateProviderSubjectBounds(t *testing.T) {
	t.Parallel()

	maxOK := strings.Repeat("a", domain.MaxProviderSubjectLen)
	require.NoError(t, domain.ValidateProviderSubject(maxOK))

	maxPlus := maxOK + "x"
	err := domain.ValidateProviderSubject(maxPlus)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))

	// Multibyte: 200 runes of "世" is fine if ≤255 runes.
	multi := strings.Repeat("世", 200)
	require.True(t, utf8.RuneCountInString(multi) == 200)
	require.NoError(t, domain.ValidateProviderSubject(multi))

	multiOver := strings.Repeat("世", domain.MaxProviderSubjectLen+1)
	require.Error(t, domain.ValidateProviderSubject(multiOver))

	require.Error(t, domain.ValidateProviderSubject(""))
	require.Error(t, domain.ValidateProviderSubject("has\nnewline"))
	require.Error(t, domain.ValidateProviderSubject("tab\there"))
	require.Error(t, domain.ValidateProviderSubject(string([]byte{0xff, 0xfe, 0xfd})))
}

func TestValidateEmailBounds(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 64)
	domainPart := strings.Repeat("b", domain.MaxEmailLen-len(local)-1)
	maxOK := local + "@" + domainPart
	// Ensure we hit exactly MaxEmailLen with a plausible shape when possible.
	if len(maxOK) > domain.MaxEmailLen {
		maxOK = strings.Repeat("c", domain.MaxEmailLen)
	}
	require.Equal(t, domain.MaxEmailLen, utf8.RuneCountInString(maxOK))
	require.NoError(t, domain.ValidateEmail(maxOK))

	require.Error(t, domain.ValidateEmail(maxOK+"x"))
	require.Error(t, domain.ValidateEmail(""))
	require.Error(t, domain.ValidateEmail("a\x00b@example.com"))
	require.NoError(t, domain.ValidateEmail("user@example.com"))
}

func TestValidateDisplayNameBounds(t *testing.T) {
	t.Parallel()

	require.NoError(t, domain.ValidateDisplayName(""))
	require.NoError(t, domain.ValidateDisplayName(strings.Repeat("n", domain.MaxDisplayNameLen)))
	require.Error(t, domain.ValidateDisplayName(strings.Repeat("n", domain.MaxDisplayNameLen+1)))
	require.Error(t, domain.ValidateDisplayName("bad\x01name"))
	require.NoError(t, domain.ValidateDisplayName(strings.Repeat("名", 100)))
}

func TestValidateProvider(t *testing.T) {
	t.Parallel()
	require.NoError(t, domain.ValidateProvider(domain.ProviderGoogle))
	require.NoError(t, domain.ValidateProvider(domain.ProviderPassword))
	require.NoError(t, domain.ValidateProvider(domain.ProviderBreakglass))
	require.Error(t, domain.ValidateProvider("student"))
	require.Error(t, domain.ValidateProvider(""))
	require.Error(t, domain.ValidateProvider("oauth2"))
}

func TestValidateCreateAccount(t *testing.T) {
	t.Parallel()
	email := "a@b.co"
	require.NoError(t, domain.ValidateCreateAccount(domain.CreateAccountInput{
		DisplayName:  "Ada",
		PrimaryEmail: &email,
	}))
	require.Error(t, domain.ValidateCreateAccount(domain.CreateAccountInput{
		DisplayName: strings.Repeat("x", domain.MaxDisplayNameLen+1),
	}))
	require.Error(t, domain.ValidateCreateAccount(domain.CreateAccountInput{
		Status: "deleted",
	}))
}

func TestNoStudentProviderConstant(t *testing.T) {
	t.Parallel()
	// Inventory: students are not Identity principals in v1.
	assert.NotEqual(t, "student", domain.ProviderGoogle)
	assert.NotEqual(t, "student", domain.ProviderPassword)
	assert.NotEqual(t, "student", domain.ProviderBreakglass)
	err := domain.ValidateProvider("student")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))
}
