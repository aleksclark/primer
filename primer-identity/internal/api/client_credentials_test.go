package api

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/oauth"
)

func TestClientCredentialsFormPolicyAndResponseShape(t *testing.T) {
	basic := url.Values{"grant_type": {oauth.GrantClientCredentials}, "resource": {"https://resource.example"}}
	require.NoError(t, validateClientCredentialsTokenForm(basic, oauth.AuthBasic))
	require.Error(t, validateClientCredentialsTokenForm(url.Values{"grant_type": {oauth.GrantClientCredentials}, "resource": {"r"}, "code": {"unexpected"}}, oauth.AuthBasic))
	private := url.Values{"grant_type": {oauth.GrantClientCredentials}, "resource": {"r"}, "client_id": {"service"}, "client_assertion_type": {tokenAssertionTypeURN}, "client_assertion": {"assertion"}}
	require.NoError(t, validateClientCredentialsTokenForm(private, oauth.AuthPrivateKeyJWT))
	require.NoError(t, validateClientCredentialsTokenForm(basic, oauth.AuthNone))
}
