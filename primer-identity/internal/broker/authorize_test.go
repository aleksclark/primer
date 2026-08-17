package broker_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/broker"
)

func validQuery() url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {"studio-bff"},
		"redirect_uri":          {"https://studio.example/callback"},
		"resource":              {"https://studio.example/mcp"},
		"audience":              {"curriculum-studio"},
		"scope":                 {"openid studio.read"},
		"state":                 {"opaque-product-state"},
		"code_challenge":        {strings.Repeat("A", 43)},
		"code_challenge_method": {"S256"},
	}
}

func TestParseAuthorizeRequestAcceptsExactValidRequest(t *testing.T) {
	t.Parallel()
	req, err := broker.ParseAuthorizeRequest(validQuery())
	require.NoError(t, err)
	assert.Equal(t, "studio-bff", req.ClientID)
	assert.Equal(t, "https://studio.example/callback", req.RedirectURI)
	assert.Equal(t, "https://studio.example/mcp", req.ResourceURI)
	assert.Equal(t, "curriculum-studio", req.Audience)
	assert.Equal(t, []string{"openid", "studio.read"}, req.Scopes)
	assert.Equal(t, []byte("opaque-product-state"), req.State)
	assert.Equal(t, strings.Repeat("A", 43), req.CodeChallenge)
}

// Every required parameter must be present exactly once.
func TestParseAuthorizeRequestRejectsMissingRequiredParameters(t *testing.T) {
	t.Parallel()
	required := []string{
		"response_type", "client_id", "redirect_uri", "resource",
		"audience", "scope", "state", "code_challenge", "code_challenge_method",
	}
	for _, name := range required {
		t.Run("missing_"+name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Del(name)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		})
	}
}

// Duplicate parameters are rejected, including duplicate resource and state.
func TestParseAuthorizeRequestRejectsDuplicateParameters(t *testing.T) {
	t.Parallel()
	duplicated := []string{
		"response_type", "client_id", "redirect_uri", "resource",
		"audience", "scope", "state", "code_challenge", "code_challenge_method",
	}
	for _, name := range duplicated {
		t.Run("duplicate_"+name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Add(name, q.Get(name))
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		})
	}
}

// Only response_type=code is supported, and it is rejected before redirect
// validation succeeds only when the redirect itself is unusable.
func TestParseAuthorizeRequestRejectsUnsupportedResponseType(t *testing.T) {
	t.Parallel()
	q := validQuery()
	q.Set("response_type", "token")
	_, err := broker.ParseAuthorizeRequest(q)
	require.Error(t, err)
	assert.Equal(t, broker.ErrorUnsupportedResponseType, broker.ErrorCodeOf(err))
}

// PKCE must be exactly S256 with a 43-character unpadded base64url challenge.
func TestParseAuthorizeRequestEnforcesExactPKCE(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, method, challenge string }{
		{"plain method", "plain", strings.Repeat("A", 43)},
		{"lowercase method", "s256", strings.Repeat("A", 43)},
		{"short challenge", "S256", strings.Repeat("A", 42)},
		{"long challenge", "S256", strings.Repeat("A", 44)},
		{"padded challenge", "S256", strings.Repeat("A", 42) + "="},
		{"invalid character", "S256", strings.Repeat("A", 42) + "+"},
		{"empty challenge", "S256", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Set("code_challenge_method", tc.method)
			q.Set("code_challenge", tc.challenge)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		})
	}
}

// State is 1..1024 raw bytes.
func TestParseAuthorizeRequestEnforcesStateByteBounds(t *testing.T) {
	t.Parallel()
	q := validQuery()
	q.Set("state", "")
	_, err := broker.ParseAuthorizeRequest(q)
	assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))

	q = validQuery()
	q.Set("state", strings.Repeat("s", 1024))
	req, err := require1024(t, q)
	require.NoError(t, err)
	assert.Len(t, req.State, 1024)

	q = validQuery()
	q.Set("state", strings.Repeat("s", 1025))
	_, err = broker.ParseAuthorizeRequest(q)
	assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
}

func require1024(t *testing.T, q url.Values) (broker.AuthorizeRequest, error) {
	t.Helper()
	return broker.ParseAuthorizeRequest(q)
}

// Invalid UTF-8 and control characters must fail before any state change.
func TestParseAuthorizeRequestRejectsInvalidUTF8AndControls(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, field, value string }{
		{"control in client_id", "client_id", "studio\x00bff"},
		{"control in redirect", "redirect_uri", "https://studio.example/cb\n"},
		{"control in resource", "resource", "https://studio.example/mcp\r"},
		{"control in audience", "audience", "curriculum\tstudio"},
		{"control in scope", "scope", "openid\x01studio.read"},
		{"invalid utf8 client_id", "client_id", "studio-\xff\xfe"},
		{"invalid utf8 audience", "audience", "aud-\xff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Set(tc.field, tc.value)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		})
	}
}

// Registered text bounds are byte bounds, not rune counts.
func TestParseAuthorizeRequestEnforcesByteBounds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, field string
		value       string
	}{
		{"redirect over 2048", "redirect_uri", "https://studio.example/" + strings.Repeat("a", 2048)},
		{"resource over 2048", "resource", "https://studio.example/" + strings.Repeat("a", 2048)},
		{"audience over 128", "audience", strings.Repeat("a", 129)},
		{"client_id over 128", "client_id", strings.Repeat("a", 129)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Set(tc.field, tc.value)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		})
	}
}

// Resource must be an absolute URI; no relative/opaque value is accepted.
func TestParseAuthorizeRequestRequiresAbsoluteResourceAndRedirect(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, field, value string }{
		{"relative resource", "resource", "/mcp"},
		{"resource with fragment", "resource", "https://studio.example/mcp#frag"},
		{"relative redirect", "redirect_uri", "/callback"},
		{"redirect with fragment", "redirect_uri", "https://studio.example/cb#frag"},
		{"non http scheme resource", "resource", "javascript:alert(1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Set(tc.field, tc.value)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		})
	}
}

// Scope must be a nonempty, space-delimited set; it is canonicalized to sorted
// unique values for persistence.
func TestParseAuthorizeRequestCanonicalizesScopes(t *testing.T) {
	t.Parallel()
	q := validQuery()
	q.Set("scope", "studio.read openid")
	req, err := broker.ParseAuthorizeRequest(q)
	require.NoError(t, err)
	assert.Equal(t, []string{"openid", "studio.read"}, req.Scopes)
}

func TestParseAuthorizeRequestRejectsEmptyAndDuplicateScopes(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "   ", "openid openid"} {
		q := validQuery()
		q.Set("scope", scope)
		_, err := broker.ParseAuthorizeRequest(q)
		require.Error(t, err, "scope %q must be rejected", scope)
		assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
	}
}

// Unknown query parameters must be rejected rather than silently ignored.
func TestParseAuthorizeRequestRejectsUnknownParameters(t *testing.T) {
	t.Parallel()
	q := validQuery()
	q.Set("prompt", "consent")
	_, err := broker.ParseAuthorizeRequest(q)
	require.Error(t, err)
	assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
}

// Parse errors must not echo attacker-supplied values back to the caller.
func TestAuthorizeErrorsAreSanitized(t *testing.T) {
	t.Parallel()
	const hostile = "hostile-value-should-not-appear"
	q := validQuery()
	q.Set("audience", hostile)
	q.Add("audience", hostile)
	_, err := broker.ParseAuthorizeRequest(q)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), hostile)
}

// A 1-byte state is accepted; empty is already covered by the bound test.
func TestParseAuthorizeRequestAcceptsOneByteState(t *testing.T) {
	t.Parallel()
	q := validQuery()
	q.Set("state", "s")
	req, err := broker.ParseAuthorizeRequest(q)
	require.NoError(t, err)
	assert.Equal(t, []byte("s"), req.State)
}

// Invalid UTF-8 and Unicode/ASCII controls in state fail before any cookie,
// provider start, or redirect is issued.
func TestParseAuthorizeRequestRejectsInvalidUTF8AndControlState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		state string
	}{
		{"nul", "ok\x00state"},
		{"crlf", "ok\r\nstate"},
		{"lf", "ok\nstate"},
		{"tab", "ok	state"},
		{"del", "ok\x7fstate"},
		{"c1 nel", "ok\u0085state"},
		{"line separator", "ok\u2028state"},
		{"paragraph separator", "ok\u2029state"},
		{"invalid utf8", "ok\xffstate"},
		{"truncated utf8", "ok\xc3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Set("state", tc.state)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
			assert.False(t, broker.IsRedirectable(err))
			assert.NotContains(t, err.Error(), tc.state)
			assert.NotContains(t, err.Error(), "\r")
			assert.NotContains(t, err.Error(), "\n")
		})
	}
}

func TestParseAuthorizeRequestAcceptsOpaqueUTF8State(t *testing.T) {
	t.Parallel()
	cases := []string{
		"s",
		"opaque-product-state",
		"café-state",
		"状态",
		"🙂opaque",
		strings.Repeat("s", 1024),
		strings.Repeat("é", 512),
	}
	for _, state := range cases {
		q := validQuery()
		q.Set("state", state)
		req, err := broker.ParseAuthorizeRequest(q)
		require.NoError(t, err, "state %q", state)
		assert.Equal(t, []byte(state), req.State)
	}
}

// Userinfo, forced query, and non-http schemes never become a redirect target.
func TestParseAuthorizeRequestRejectsUnsafeAbsoluteURIs(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, field, value string }{
		{"userinfo redirect", "redirect_uri", "https://user:pass@studio.example/cb"},
		{"userinfo resource", "resource", "https://user:pass@studio.example/mcp"},
		{"forced query redirect", "redirect_uri", "https://studio.example/cb?"},
		{"forced query resource", "resource", "https://studio.example/mcp?"},
		{"http redirect is allowed only as absolute", "redirect_uri", "ftp://studio.example/cb"},
		{"opaque resource", "resource", "urn:example:mcp"},
		{"file scheme redirect", "redirect_uri", "file:///tmp/cb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := validQuery()
			q.Set(tc.field, tc.value)
			_, err := broker.ParseAuthorizeRequest(q)
			require.Error(t, err)
			assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
			assert.False(t, broker.IsRedirectable(err))
			assert.NotContains(t, err.Error(), tc.value)
		})
	}
}

// Scope count and empty tokens fail as invalid_request, never as a redirect.
func TestParseAuthorizeRequestRejectsScopeCountAndEmptyTokens(t *testing.T) {
	t.Parallel()
	tooMany := make([]string, 33)
	for i := range tooMany {
		tooMany[i] = "s" + strings.Repeat("x", i%8) + string(rune('a'+i%26))
	}
	cases := []string{
		"openid  studio.read",
		"openid ",
		" openid",
		strings.Join(tooMany, " "),
		strings.Repeat("a", 129),
	}
	for _, scope := range cases {
		q := validQuery()
		q.Set("scope", scope)
		_, err := broker.ParseAuthorizeRequest(q)
		require.Error(t, err, "scope %q must be rejected", scope)
		assert.Equal(t, broker.ErrorInvalidRequest, broker.ErrorCodeOf(err))
		assert.False(t, broker.IsRedirectable(err))
	}
}

// Pre-redirect parse failures stay local even for unsupported_response_type.
func TestParseAuthorizeRequestErrorsAreNotRedirectable(t *testing.T) {
	t.Parallel()
	q := validQuery()
	q.Set("response_type", "token")
	_, err := broker.ParseAuthorizeRequest(q)
	require.Error(t, err)
	assert.False(t, broker.IsRedirectable(err))
}
