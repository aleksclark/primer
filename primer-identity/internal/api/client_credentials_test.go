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
	require.Error(t, validateClientCredentialsTokenForm(basic, oauth.AuthNone))
	require.Error(t, validateClientCredentialsTokenForm(basic, ""))
}

func TestRefreshTokenFormPolicy(t *testing.T) {
	base := url.Values{"grant_type": {oauth.GrantRefreshToken}, "refresh_token": {"opaque"}, "resource": {"https://resource.example"}, "client_id": {"public"}}
	require.NoError(t, validateRefreshTokenForm(base, oauth.AuthNone))
	private := url.Values{"grant_type": {oauth.GrantRefreshToken}, "refresh_token": {"opaque"}, "resource": {"r"}, "client_id": {"service"}, "client_assertion_type": {tokenAssertionTypeURN}, "client_assertion": {"assertion"}}
	require.NoError(t, validateRefreshTokenForm(private, oauth.AuthPrivateKeyJWT))
	require.Error(t, validateRefreshTokenForm(url.Values{"grant_type": {oauth.GrantRefreshToken}, "unexpected": {"x"}}, oauth.AuthBasic))
	require.Error(t, validateRefreshTokenForm(url.Values{"grant_type": {oauth.GrantRefreshToken}, "refresh_token": {"opaque"}, "resource": {"r"}, "unexpected": {"x"}}, oauth.AuthNone))
}
