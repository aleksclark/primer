package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

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

type oauthTokenSuccess struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type oauthTokenError struct {
	Code             string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func (s *Server) registerTokenRoutes(api huma.API, router chi.Router) {
	if s.oauth == nil && !s.registerTokenInventory {
		return
	}
	if api != nil {
		api.OpenAPI().AddOperation(tokenOpenAPIOperation())
	}
	if router != nil {
		router.Post(tokenPath, s.handleToken)
	}
	s.registerRevokeRoutes(api, router)
}

func tokenOpenAPIOperation() *huma.Operation {
	return &huma.Operation{
		OperationID:   "oauthToken",
		Method:        http.MethodPost,
		Path:          tokenPath,
		Summary:       "Exchange an authorization code for Primer tokens",
		Tags:          []string{"OAuth"},
		DefaultStatus: http.StatusOK,
		RequestBody:   tokenRequestBody(),
		Responses:     tokenOpenAPIResponses(),
		Security: []map[string][]string{
			{"oauthTokenBasic": {}},
			{},
		},
	}
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r == nil {
		s.writeTokenFail(w, oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
		return
	}
	req, auth, challenge, err := s.parseTokenRequest(r)
	if err != nil {
		var we *tokenWireError
		if errors.As(err, &we) {
			s.writeTokenFail(w, we.code, we.desc, we.challenge)
			return
		}
		var se *brokerStatusError
		if errors.As(err, &se) {
			writeBrokerStatusError(w, se)
			return
		}
		s.writeTokenFail(w, oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
		return
	}
	if s.oauth == nil {
		s.writeTokenFail(w, oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
		return
	}
	resp, err := s.oauth.Exchange(r.Context(), req, auth)
	if err != nil {
		we := mapTokenServiceError(err, challenge)
		s.writeTokenFail(w, we.code, we.desc, we.challenge)
		return
	}
	s.writeTokenSuccess(w, resp)
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
	if form.Get("grant_type") == oauth.GrantAuthorizationCode {
		if err := validateAuthorizationCodeTokenForm(form, auth.Method); err != nil {
			return oauth.ExchangeRequest{}, oauth.ClientAuth{}, challenge, err
		}
	}
	if form.Get("grant_type") == oauth.GrantClientCredentials {
		if err := validateClientCredentialsTokenForm(form, auth.Method); err != nil {
			return oauth.ExchangeRequest{}, oauth.ClientAuth{}, challenge, err
		}
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
		Scope:               form.Get("scope"),
	}
	return req, auth, challenge, nil
}

func validateClientCredentialsTokenForm(form url.Values, authMethod string) error {
	allowed := map[string]struct{}{"grant_type": {}, "resource": {}, "scope": {}}
	switch authMethod {
	case oauth.AuthBasic:
	case oauth.AuthPrivateKeyJWT:
		allowed["client_id"] = struct{}{}
		allowed["client_assertion_type"] = struct{}{}
		allowed["client_assertion"] = struct{}{}
	default:
		return nil
	}
	for name := range form {
		if _, ok := allowed[name]; !ok {
			return tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
		}
	}
	return nil
}

func validateAuthorizationCodeTokenForm(form url.Values, authMethod string) error {
	allowed := map[string]struct{}{
		"grant_type": {}, "code": {}, "redirect_uri": {}, "resource": {}, "code_verifier": {},
	}
	switch authMethod {
	case oauth.AuthNone:
		allowed["client_id"] = struct{}{}
	case oauth.AuthBasic:
		// Basic authentication identifies the client in Authorization. A form
		// client_id would make this a different, ambiguous field set.
	case oauth.AuthPrivateKeyJWT:
		allowed["client_id"] = struct{}{}
		allowed["client_assertion_type"] = struct{}{}
		allowed["client_assertion"] = struct{}{}
	default:
		return tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	for name := range form {
		if _, ok := allowed[name]; !ok {
			return tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
		}
	}
	return nil
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
		if form.Get("client_id") != "" {
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

func (s *Server) writeTokenSuccess(w http.ResponseWriter, resp oauth.TokenResponse) {
	setTokenHeaders(w, s.productionHSTS(), false)
	w.Header().Set("Content-Type", tokenJSONContentType)
	w.WriteHeader(http.StatusOK)
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
	_, _ = w.Write(body)
}

func (s *Server) writeTokenFail(w http.ResponseWriter, code, description string, basicChallenge bool) {
	writeTokenOAuthError(w, s.productionHSTS(), code, description, basicChallenge)
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

func writeTokenOAuthError(w http.ResponseWriter, hsts, code, description string, basicChallenge bool) {
	status := http.StatusBadRequest
	switch code {
	case oauth.ErrorInvalidClient:
		status = http.StatusUnauthorized
	case oauth.ErrorTemporarilyUnavail:
		status = http.StatusServiceUnavailable
	}
	setTokenHeaders(w, hsts, basicChallenge)
	w.Header().Set("Content-Type", tokenJSONContentType)
	w.WriteHeader(status)
	body, err := json.Marshal(oauthTokenError{Code: code, ErrorDescription: tokenWireDescription(code, description)})
	if err != nil {
		return
	}
	_, _ = w.Write(body)
}

func tokenWireDescription(code, description string) string {
	if code == oauth.ErrorInvalidClient {
		return ""
	}
	return description
}

func writeBrokerStatusError(w http.ResponseWriter, err *brokerStatusError) {
	if err == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	for name, values := range err.GetHeaders() {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	status := err.GetStatus()
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", tokenJSONContentType)
	}
	w.WriteHeader(status)
	var se huma.StatusError
	if errors.As(err.statusErr, &se) {
		body, marshalErr := json.Marshal(map[string]any{
			"status": status,
			"title":  http.StatusText(status),
			"detail": se.Error(),
		})
		if marshalErr == nil {
			_, _ = w.Write(body)
			return
		}
	}
	if err.statusErr != nil {
		_, _ = io.WriteString(w, err.statusErr.Error())
	}
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

func tokenRequestBody() *huma.RequestBody {
	writeOnly := func() *huma.Schema {
		return &huma.Schema{Type: huma.TypeString, WriteOnly: true}
	}
	plain := func() *huma.Schema {
		return &huma.Schema{Type: huma.TypeString}
	}
	baseProperties := func() map[string]*huma.Schema {
		return map[string]*huma.Schema{
			"grant_type":    {Type: huma.TypeString, Const: oauth.GrantAuthorizationCode},
			"code":          writeOnly(),
			"redirect_uri":  plain(),
			"resource":      plain(),
			"code_verifier": writeOnly(),
		}
	}
	baseRequired := []string{"grant_type", "code", "redirect_uri", "resource", "code_verifier"}
	variant := func(properties map[string]*huma.Schema, required []string) *huma.Schema {
		return &huma.Schema{
			Type:                 huma.TypeObject,
			Properties:           properties,
			Required:             required,
			AdditionalProperties: false,
		}
	}
	publicProperties := baseProperties()
	publicProperties["client_id"] = plain()
	publicRequired := append(append([]string(nil), baseRequired...), "client_id")
	basicProperties := baseProperties()
	privateProperties := baseProperties()
	privateProperties["client_id"] = plain()
	privateProperties["client_assertion_type"] = &huma.Schema{Type: huma.TypeString, Const: tokenAssertionTypeURN}
	privateProperties["client_assertion"] = writeOnly()
	privateRequired := append(append([]string(nil), baseRequired...), "client_id", "client_assertion_type", "client_assertion")
	serviceBasic := map[string]*huma.Schema{"grant_type": {Type: huma.TypeString, Const: oauth.GrantClientCredentials}, "resource": plain(), "scope": plain()}
	servicePrivate := map[string]*huma.Schema{"grant_type": {Type: huma.TypeString, Const: oauth.GrantClientCredentials}, "resource": plain(), "scope": plain(), "client_id": plain(), "client_assertion_type": {Type: huma.TypeString, Const: tokenAssertionTypeURN}, "client_assertion": writeOnly()}

	return &huma.RequestBody{
		Required:    true,
		Description: "OAuth token form",
		Content: map[string]*huma.MediaType{
			tokenFormContentType: {
				Schema: &huma.Schema{
					OneOf: []*huma.Schema{
						variant(publicProperties, publicRequired),
						variant(basicProperties, baseRequired),
						variant(privateProperties, privateRequired),
						variant(serviceBasic, []string{"grant_type", "resource"}),
						variant(servicePrivate, []string{"grant_type", "resource", "client_id", "client_assertion_type", "client_assertion"}),
					},
				},
			},
		},
	}
}

func tokenOpenAPIResponses() map[string]*huma.Response {
	header := func(desc string) *huma.Header {
		return &huma.Header{Description: desc, Schema: &huma.Schema{Type: huma.TypeString}}
	}
	security := map[string]*huma.Header{
		"Cache-Control":          header("Must be no-store"),
		"Pragma":                 header("Must be no-cache"),
		"X-Content-Type-Options": header("Must be nosniff"),
		"WWW-Authenticate":       header("Exact Basic challenge on failed client_secret_basic"),
	}
	jsonMedia := func(schema *huma.Schema) map[string]*huma.MediaType {
		return map[string]*huma.MediaType{
			tokenJSONContentType: {Schema: schema},
		}
	}
	return map[string]*huma.Response{
		"200": {
			Description: "Authorization-code token response",
			Headers:     security,
			Content: jsonMedia(&huma.Schema{
				Type: huma.TypeObject,
				Properties: map[string]*huma.Schema{
					"access_token":  {Type: huma.TypeString},
					"token_type":    {Type: huma.TypeString},
					"expires_in":    {Type: huma.TypeInteger},
					"scope":         {Type: huma.TypeString},
					"refresh_token": {Type: huma.TypeString},
				},
				Required: []string{"access_token", "token_type", "expires_in", "scope", "refresh_token"},
			}),
		},
		"400": {
			Description: "OAuth invalid_request, invalid_grant, or unsupported_grant_type",
			Headers:     security,
			Content: jsonMedia(&huma.Schema{
				Type: huma.TypeObject,
				Properties: map[string]*huma.Schema{
					"error":             {Type: huma.TypeString},
					"error_description": {Type: huma.TypeString},
				},
				Required: []string{"error"},
			}),
		},
		"401": {
			Description: "OAuth invalid_client",
			Headers:     security,
			Content: jsonMedia(&huma.Schema{
				Type:                 huma.TypeObject,
				AdditionalProperties: false,
				Properties: map[string]*huma.Schema{
					"error": {Type: huma.TypeString, Const: oauth.ErrorInvalidClient},
				},
				Required: []string{"error"},
			}),
		},
		"413": {Description: "Request too large", Headers: security},
		"415": {Description: "Unsupported media type", Headers: security},
		"503": {
			Description: "temporarily_unavailable",
			Headers:     security,
			Content: jsonMedia(&huma.Schema{
				Type: huma.TypeObject,
				Properties: map[string]*huma.Schema{
					"error":             {Type: huma.TypeString},
					"error_description": {Type: huma.TypeString},
				},
				Required: []string{"error"},
			}),
		},
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

func setTokenHeaders(w http.ResponseWriter, hsts string, basicChallenge bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if hsts != "" {
		w.Header().Set("Strict-Transport-Security", hsts)
	}
	if basicChallenge {
		w.Header().Set("WWW-Authenticate", tokenBasicChallenge)
	}
}
