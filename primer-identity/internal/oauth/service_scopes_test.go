package oauth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestedServiceScopesFailsClosedAndCanonicalizes(t *testing.T) {
	got, err := requestedServiceScopes("", []string{"studio.draft", "studio.read"})
	require.NoError(t, err)
	require.Equal(t, []string{"studio.draft", "studio.read"}, got)

	for _, tc := range []struct {
		name    string
		request string
		allowed []string
	}{
		{"human scope registration", "", []string{"openid"}},
		{"wildcard registration", "", []string{"studio.*"}},
		{"malformed request", "studio.read\x01studio.draft", []string{"studio.read"}},
		{"unregistered request", "studio.draft", []string{"studio.read"}},
		{"human request", "openid", []string{"studio.read"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := requestedServiceScopes(tc.request, tc.allowed)
			require.Error(t, err)
			require.Equal(t, "invalid_scope", ErrorCodeOf(err))
		})
	}
}
