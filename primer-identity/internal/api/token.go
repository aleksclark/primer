package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/identity/internal/oauth"
)

const (
	tokenFormMaxBytes      = 32 * 1024
	tokenPath              = "/oauth/token"
	tokenJSONContentType   = "application/json; charset=utf-8"
	tokenFormContentType   = "application/x-www-form-urlencoded"
	tokenBasicChallenge    = `Basic realm="token", charset="UTF-8"`
	tokenAssertionTypeURN  = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	tokenMaxSecretBytes    = 256
	descInvalidRequest     = "the request is invalid"
	descInvalidClient      = "client authentication failed"
	descInvalidGrant       = "the authorization grant is invalid"
	descInvalidScope       = "the requested scope is invalid"
	descInvalidTarget      = "the requested target is invalid"
	descUnsupportedGrant   = "the grant type is not supported"
	descTemporarilyUnavail = "the service is temporarily unavailable"
	descUnauthorizedClient = "the client is not authorized for this grant"
)

type oauthTokenForm struct {
	GrantType           string `form:"grant_type" json:"grant_type"`
	Code                string `form:"code" json:"code" writeOnly:"true"`
	RedirectURI         string `form:"redirect_uri" json:"redirect_uri"`
	Resource            string `form:"resource" json:"resource"`
	CodeVerifier        string `form:"code_verifier" json:"code_verifier" writeOnly:"true"`
	ClientID            string `form:"client_id" json:"client_id"`
	ClientAssertionType string `form:"client_assertion_type" json:"client_assertion_type"`
	ClientAssertion     string `form:"client_assertion" json:"client_assertion" writeOnly:"true"`
	ClientSecret        string `form:"client_secret" json:"client_secret" writeOnly:"true"`
	RefreshToken        string `form:"refresh_token" json:"refresh_token" writeOnly:"true"`
	Scope               string `form:"scope" json:"scope"`
}

type oauthTokenSuccess struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

type oauthTokenError struct {
	Code             string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func (s *Server) registerTokenRoutes(api huma.API) {
	if s.oauth == nil && !s.registerTokenInventory {
		return
	}

	attach := huma.Middlewares{s.attachRequest}
	huma.Register(api, huma.Operation{
		OperationID:      "oauthToken",
		Method:           http.MethodPost,
		Path:             tokenPath,
		Summary:          "Exchange an authorization code for Primer tokens",
		Tags:             []string{"OAuth"},
		DefaultStatus:    http.StatusOK,
		SkipValidateBody: true,
		MaxBodyBytes:     tokenFormMaxBytes,
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusRequestEntityTooLarge,
			http.StatusUnsupportedMediaType,
			http.StatusServiceUnavailable,
		},
		Middlewares: attach,
		Metadata: map[string]any{
			"oauthTokenForm": oauthTokenForm{},
		},
	}, func(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
		r := requestOf(ctx)
		if r == nil {
			return s.tokenFail(oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false), nil
		}
		req, auth, challenge, err := s.parseTokenRequest(r)
		if err != nil {
			var we *tokenWireError
			if errors.As(err, &we) {
				return s.tokenFail(we.code, we.desc, we.challenge), nil
			}
			return nil, err
		}
		if s.oauth == nil {
			return s.tokenFail(oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false), nil
		}
		resp, err := s.oauth.Exchange(ctx, req, auth)
		if err != nil {
			we := mapTokenServiceError(err, challenge)
			return s.tokenFail(we.code, we.desc, we.challenge), nil
		}
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			s.writeTokenSuccess(hctx, resp)
		}}, nil
	})
}

func (s *Server) parseTokenRequest(r *http.Request) (oauth.ExchangeRequest, oauth.ClientAuth, bool, error) {
	if r.URL != nil && r.URL.RawQuery != "" {
		return oauth.ExchangeRequest{}, oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	if !exactTokenContentType(r.Header.Get("Content-Type")) {
		return oauth.ExchangeRequest{}, oauth.ClientAuth{}, false, s.tokenMediaError()
	}
	if r.ContentLength > tokenFormMaxBytes {
		return oauth.ExchangeRequest{}, oauth.ClientAuth{}, false, s.tokenTooLargeError()
	}
	r.Body = http.MaxBytesReader(nil, r.Body, tokenFormMaxBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return oauth.ExchangeRequest{}, oauth.ClientAuth{}, false, s.tokenTooLargeError()
		}
		return oauth.ExchangeRequest{}, oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	form, err := parseStrictTokenForm(raw)
	if err != nil {
		return oauth.ExchangeRequest{}, oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	auth, challenge, err := parseTokenClientAuth(r.Header.Values("Authorization"), form)
	if err != nil {
		return oauth.ExchangeRequest{}, oauth.ClientAuth{}, challenge, err
	}
	req := oauth.ExchangeRequest{
		GrantType:           form.Get("grant_type"),
		Code:                form.Get("code"),
		RedirectURI:         form.Get("redirect_uri"),
		Resource:            form.Get("resource"),
		CodeVerifier:        form.Get("code_verifier"),
		ClientID:            form.Get("client_id"),
		ClientAssertionType: form.Get("client_assertion_type"),
		ClientAssertion:     form.Get("client_assertion"),
		RefreshToken:        form.Get("refresh_token"),
	}
	return req, auth, challenge, nil
}

func parseStrictTokenForm(raw []byte) (url.Values, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("invalid utf8")
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{
		"grant_type": {}, "code": {}, "redirect_uri": {}, "resource": {},
		"code_verifier": {}, "client_id": {}, "client_assertion_type": {},
		"client_assertion": {}, "client_secret": {}, "refresh_token": {}, "scope": {},
	}
	for name, vals := range values {
		if _, ok := allowed[name]; !ok {
			return nil, errors.New("unknown field")
		}
		if len(vals) != 1 {
			return nil, errors.New("duplicate field")
		}
		if !validTokenField(vals[0]) {
			return nil, errors.New("invalid field")
		}
	}
	return values, nil
}

func validTokenField(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func parseTokenClientAuth(authorizations []string, form url.Values) (oauth.ClientAuth, bool, error) {
	hasAuth := false
	for _, value := range authorizations {
		if strings.TrimSpace(value) != "" {
			hasAuth = true
			break
		}
	}
	hasSecret := form.Has("client_secret")
	hasAssertion := form.Has("client_assertion") || form.Has("client_assertion_type")
	modes := 0
	if hasAuth {
		modes++
	}
	if hasSecret {
		modes++
	}
	if hasAssertion {
		modes++
	}
	if modes > 1 {
		return oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	if hasSecret {
		return oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	if hasAuth {
		if len(authorizations) != 1 {
			return oauth.ClientAuth{}, true, tokenWire(oauth.ErrorInvalidClient, descInvalidClient, true)
		}
		user, pass, ok := parseSingleBasic(authorizations[0])
		if !ok {
			return oauth.ClientAuth{}, true, tokenWire(oauth.ErrorInvalidClient, descInvalidClient, true)
		}
		if form.Get("client_id") != "" && form.Get("client_id") != user {
			return oauth.ClientAuth{}, true, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
		}
		return oauth.ClientAuth{Method: oauth.AuthBasic, ClientID: user, Secret: pass}, true, nil
	}
	if hasAssertion {
		if form.Get("client_assertion_type") != tokenAssertionTypeURN || form.Get("client_assertion") == "" || form.Get("client_id") == "" {
			return oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
		}
		return oauth.ClientAuth{
			Method:    oauth.AuthPrivateKeyJWT,
			ClientID:  form.Get("client_id"),
			Assertion: form.Get("client_assertion"),
		}, false, nil
	}
	if form.Get("client_id") == "" {
		return oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	return oauth.ClientAuth{Method: oauth.AuthNone, ClientID: form.Get("client_id")}, false, nil
}

func parseSingleBasic(raw string) (string, string, bool) {
	scheme, creds, ok := strings.Cut(raw, " ")
	if !ok || !strings.EqualFold(scheme, "Basic") || creds == "" || strings.ContainsAny(creds, " 	") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(creds)
	if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) {
		return "", "", false
	}
	user, pass, found := strings.Cut(string(decoded), ":")
	if !found || !validBasicClientID(user) || !validBasicSecret(pass) {
		return "", "", false
	}
	return user, pass, true
}

func validBasicClientID(value string) bool {
	return validTokenField(value) && len(value) <= 128
}

func validBasicSecret(value string) bool {
	return validTokenField(value) && len(value) <= tokenMaxSecretBytes
}

func exactTokenContentType(raw string) bool {
	return raw == tokenFormContentType
}

func mapTokenServiceError(err error, basicPresented bool) *tokenWireError {
	code := oauth.ErrorCodeOf(err)
	if code == "" {
		return tokenWire(oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
	}
	return tokenWire(code, tokenDescription(code), basicPresented && code == oauth.ErrorInvalidClient)
}

func tokenDescription(code string) string {
	switch code {
	case oauth.ErrorInvalidClient:
		return descInvalidClient
	case oauth.ErrorInvalidGrant:
		return descInvalidGrant
	case "invalid_scope":
		return descInvalidScope
	case "invalid_target":
		return descInvalidTarget
	case oauth.ErrorUnsupportedGrantType:
		return descUnsupportedGrant
	case oauth.ErrorTemporarilyUnavail:
		return descTemporarilyUnavail
	case oauth.ErrorUnauthorizedClient:
		return descUnauthorizedClient
	default:
		return descInvalidRequest
	}
}

func (s *Server) writeTokenSuccess(hctx huma.Context, resp oauth.TokenResponse) {
	setTokenHeaders(hctx, s.productionHSTS(), false)
	hctx.SetHeader("Content-Type", tokenJSONContentType)
	hctx.SetStatus(http.StatusOK)
	body, err := json.Marshal(oauthTokenSuccess{
		AccessToken:  resp.AccessToken,
		TokenType:    resp.TokenType,
		ExpiresIn:    resp.ExpiresIn,
		Scope:        resp.Scope,
		RefreshToken: resp.RefreshToken,
	})
	if err != nil {
		return
	}
	_, _ = hctx.BodyWriter().Write(body)
}

func (s *Server) tokenFail(code, description string, basicChallenge bool) *huma.StreamResponse {
	return &huma.StreamResponse{Body: func(hctx huma.Context) {
		writeTokenOAuthError(hctx, s.productionHSTS(), code, description, basicChallenge)
	}}
}

type tokenWireError struct {
	code      string
	desc      string
	challenge bool
}

func (e *tokenWireError) Error() string {
	if e == nil {
		return oauth.ErrorTemporarilyUnavail
	}
	return e.code
}

func tokenWire(code, description string, challenge bool) *tokenWireError {
	return &tokenWireError{code: code, desc: description, challenge: challenge}
}

func writeTokenOAuthError(hctx huma.Context, hsts, code, description string, basicChallenge bool) {
	status := http.StatusBadRequest
	switch code {
	case oauth.ErrorInvalidClient:
		status = http.StatusUnauthorized
	case oauth.ErrorTemporarilyUnavail:
		status = http.StatusServiceUnavailable
	}
	setTokenHeaders(hctx, hsts, basicChallenge)
	hctx.SetHeader("Content-Type", tokenJSONContentType)
	hctx.SetStatus(status)
	body, err := json.Marshal(oauthTokenError{Code: code, ErrorDescription: description})
	if err != nil {
		return
	}
	_, _ = hctx.BodyWriter().Write(body)
}

func (s *Server) tokenMediaError() error {
	h := tokenSecurityHeaders(s.productionHSTS() != "")
	if s.productionHSTS() != "" {
		h.Set("Strict-Transport-Security", s.productionHSTS())
	}
	return &brokerStatusError{
		statusErr: huma.Error415UnsupportedMediaType("unsupported media type"),
		headers:   h,
	}
}

func (s *Server) tokenTooLargeError() error {
	h := tokenSecurityHeaders(s.productionHSTS() != "")
	if s.productionHSTS() != "" {
		h.Set("Strict-Transport-Security", s.productionHSTS())
	}
	return &brokerStatusError{
		statusErr: huma.Error413RequestEntityTooLarge("request too large"),
		headers:   h,
	}
}

func tokenSecurityHeaders(withHSTS bool) http.Header {
	h := make(http.Header)
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Type", tokenJSONContentType)
	if withHSTS {
		h.Set("Strict-Transport-Security", productionHSTS)
	}
	return h
}

func setTokenHeaders(ctx huma.Context, hsts string, basicChallenge bool) {
	ctx.SetHeader("Cache-Control", "no-store")
	ctx.SetHeader("Pragma", "no-cache")
	ctx.SetHeader("X-Content-Type-Options", "nosniff")
	if hsts != "" {
		ctx.SetHeader("Strict-Transport-Security", hsts)
	}
	if basicChallenge {
		ctx.SetHeader("WWW-Authenticate", tokenBasicChallenge)
	}
}
