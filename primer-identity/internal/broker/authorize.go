// Package broker implements the Primer Identity OAuth broker: exact
// authorization-request validation, registration lookup, recoverable sealed
// state, and the serializable callback completion that issues exactly one
// Primer authorization code.
//
// IB1 scope: this package issues codes only. It deliberately implements no
// token endpoint, no code redemption or consumption, no JWT/JWKS signing, and
// no refresh lifecycle.
package broker

import (
	"errors"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OAuth error codes used by the authorize endpoint.
const (
	ErrorInvalidRequest          = "invalid_request"
	ErrorUnsupportedResponseType = "unsupported_response_type"
	ErrorAccessDenied            = "access_denied"
	ErrorInvalidTarget           = "invalid_target"
	ErrorInvalidScope            = "invalid_scope"
	ErrorServerError             = "server_error"
	ErrorTemporarilyUnavailable  = "temporarily_unavailable"
)

// Exact registered text bounds, in bytes.
const (
	MaxRedirectURIBytes = 2048
	MaxResourceURIBytes = 2048
	MaxAudienceBytes    = 128
	MaxClientIDBytes    = 128
	MinStateBytes       = 1
	MaxStateBytes       = 1024
	PKCEChallengeLen    = 43
	MaxScopeCount       = 32
	MaxScopeValueBytes  = 128
)

// Error is a sanitized OAuth error. Its message is a fixed description and
// never contains attacker-supplied request values.
type Error struct {
	Code        string
	Description string
}

func (e *Error) Error() string { return e.Code + ": " + e.Description }

func oauthErr(code, description string) error {
	return &Error{Code: code, Description: description}
}

// Fixed descriptions for the only two post-registration redirectable codes.
const (
	DescInvalidScope = "requested scope is not registered"
	DescAccessDenied = "the authorization request was denied"
)

// TrustedRedirect is the only post-registration OAuth error that HTTP may
// turn into a Location. Error() is deliberately a fixed code:state/URI
// never appear in logs or Error strings.
type TrustedRedirect struct {
	Code        string
	Description string
	RedirectURI string
	State       []byte
	Issuer      string
}

func (e *TrustedRedirect) Error() string {
	if e == nil {
		return ErrorServerError
	}
	switch e.Code {
	case ErrorInvalidScope:
		return ErrorInvalidScope
	case ErrorAccessDenied:
		return ErrorAccessDenied
	default:
		return ErrorServerError
	}
}

func (e *TrustedRedirect) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Code {
	case ErrorInvalidScope:
		return oauthErr(ErrorInvalidScope, DescInvalidScope)
	case ErrorAccessDenied:
		return ErrProviderDenied
	default:
		return nil
	}
}

func trustedRedirect(code, description, redirectURI, issuer string, state []byte) *TrustedRedirect {
	copied := append([]byte(nil), state...)
	return &TrustedRedirect{
		Code:        code,
		Description: description,
		RedirectURI: redirectURI,
		State:       copied,
		Issuer:      issuer,
	}
}

// AsTrustedRedirect reports the only typed trusted redirect outcome.
func AsTrustedRedirect(err error) (*TrustedRedirect, bool) {
	var tr *TrustedRedirect
	if errors.As(err, &tr) && tr != nil && (tr.Code == ErrorInvalidScope || tr.Code == ErrorAccessDenied) &&
		tr.RedirectURI != "" && tr.Issuer != "" && len(tr.State) > 0 {
		return tr, true
	}
	return nil, false
}

// ErrorCodeOf extracts the OAuth error code from err, or "" when err is not an
// OAuth error.
func ErrorCodeOf(err error) string {
	if tr, ok := AsTrustedRedirect(err); ok {
		return tr.Code
	}
	var oe *Error
	if errors.As(err, &oe) {
		return oe.Code
	}
	return ""
}

// AuthorizeRequest is a fully validated authorization request. Scopes are
// canonical (sorted, unique) and State holds the exact raw bytes.
type AuthorizeRequest struct {
	ClientID      string
	RedirectURI   string
	ResourceURI   string
	Audience      string
	Scopes        []string
	State         []byte
	CodeChallenge string
	CodeMethod    string
}

// authorizeParams is the exact accepted parameter set. Anything else is an
// unknown parameter and fails before any state change.
var authorizeParams = map[string]struct{}{
	"response_type": {}, "client_id": {}, "redirect_uri": {}, "resource": {},
	"audience": {}, "scope": {}, "state": {}, "code_challenge": {},
	"code_challenge_method": {},
}

// ParseAuthorizeRequest applies the exact GET /oauth/authorize rules: each
// required parameter exactly once, no unknown parameters, strict UTF-8 without
// controls, exact byte bounds, absolute redirect/resource URIs, canonical
// nonempty scope, 1..1024-byte state, and S256 PKCE with a 43-character
// unpadded base64url challenge.
//
// It performs no registration lookup and no persistence, so every failure here
// happens before any state change.
func ParseAuthorizeRequest(query url.Values) (AuthorizeRequest, error) {
	for name, values := range query {
		if _, known := authorizeParams[name]; !known {
			return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "unknown parameter")
		}
		if len(values) != 1 {
			return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "parameter must appear exactly once")
		}
	}
	for name := range authorizeParams {
		if _, present := query[name]; !present {
			return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "missing required parameter")
		}
	}

	// response_type is checked before the remaining parameter grammar so an
	// unsupported type reports its own exact code.
	if query.Get("response_type") != "code" {
		return AuthorizeRequest{}, oauthErr(ErrorUnsupportedResponseType, "only response_type=code is supported")
	}

	clientID := query.Get("client_id")
	if !validBoundedText(clientID, MaxClientIDBytes) {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "invalid client_id")
	}

	redirectURI := query.Get("redirect_uri")
	if !validBoundedText(redirectURI, MaxRedirectURIBytes) || !validAbsoluteURI(redirectURI) {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "invalid redirect_uri")
	}

	resourceURI := query.Get("resource")
	if !validBoundedText(resourceURI, MaxResourceURIBytes) || !validAbsoluteURI(resourceURI) {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "invalid resource")
	}

	audience := query.Get("audience")
	if !validBoundedText(audience, MaxAudienceBytes) {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "invalid audience")
	}

	scopes, err := canonicalScopes(query.Get("scope"))
	if err != nil {
		return AuthorizeRequest{}, err
	}

	stateRaw := query.Get("state")
	if !validOpaqueState(stateRaw) {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "invalid state")
	}
	state := []byte(stateRaw)

	if query.Get("code_challenge_method") != "S256" {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "code_challenge_method must be S256")
	}
	challenge := query.Get("code_challenge")
	if !validCodeChallenge(challenge) {
		return AuthorizeRequest{}, oauthErr(ErrorInvalidRequest, "invalid code_challenge")
	}

	return AuthorizeRequest{
		ClientID: clientID, RedirectURI: redirectURI, ResourceURI: resourceURI,
		Audience: audience, Scopes: scopes, State: state,
		CodeChallenge: challenge, CodeMethod: "S256",
	}, nil
}

// canonicalScopes splits an exact space-delimited scope string into sorted
// unique values. Empty, duplicate, oversized, and control-bearing values fail.
func canonicalScopes(raw string) ([]string, error) {
	if raw == "" || !utf8.ValidString(raw) {
		return nil, oauthErr(ErrorInvalidRequest, "invalid scope")
	}
	fields := strings.Split(raw, " ")
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			return nil, oauthErr(ErrorInvalidRequest, "invalid scope")
		}
		if !validBoundedText(field, MaxScopeValueBytes) {
			return nil, oauthErr(ErrorInvalidRequest, "invalid scope")
		}
		if _, duplicate := seen[field]; duplicate {
			return nil, oauthErr(ErrorInvalidRequest, "duplicate scope")
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	if len(out) == 0 || len(out) > MaxScopeCount {
		return nil, oauthErr(ErrorInvalidRequest, "invalid scope count")
	}
	sort.Strings(out)
	return out, nil
}

// validBoundedText requires nonempty strict UTF-8 within a byte bound and
// without control characters.
func validBoundedText(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// validOpaqueState accepts 1..1024 bytes of strict UTF-8 with no Unicode or
// ASCII control characters. Opaque printable text, including non-ASCII, is kept.
func validOpaqueState(value string) bool {
	if len(value) < MinStateBytes || len(value) > MaxStateBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

// validAbsoluteURI requires an absolute http/https URI with no fragment,
// userinfo, or forced query.
func validAbsoluteURI(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	return parsed.Fragment == "" && !parsed.ForceQuery && parsed.User == nil &&
		!strings.Contains(value, "#")
}

// validCodeChallenge requires exactly 43 unpadded base64url characters.
func validCodeChallenge(value string) bool {
	if len(value) != PKCEChallengeLen {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
