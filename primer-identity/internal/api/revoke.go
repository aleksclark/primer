package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/aleksclark/primer/identity/internal/oauth"
)

const (
	revokeFormMaxBytes   = 16 * 1024
	revokePath           = "/oauth/revoke"
	revokeBasicChallenge = `Basic realm="oauth/revoke", charset="UTF-8"`
)

func (s *Server) registerRevokeRoutes(api huma.API, router chi.Router) {
	if s.oauth == nil && !s.registerTokenInventory {
		return
	}
	if api != nil {
		api.OpenAPI().AddOperation(revokeOpenAPIOperation())
	}
	if router != nil {
		router.Post(revokePath, s.handleRevoke)
	}
}

func revokeOpenAPIOperation() *huma.Operation {
	return &huma.Operation{
		OperationID:   "oauthRevoke",
		Method:        http.MethodPost,
		Path:          revokePath,
		Summary:       "Revoke an IB2 refresh family",
		Tags:          []string{"OAuth"},
		DefaultStatus: http.StatusOK,
		RequestBody:   revokeRequestBody(),
		Responses:     revokeOpenAPIResponses(),
		Security: []map[string][]string{
			{"oauthTokenBasic": {}},
			{},
		},
	}
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r == nil {
		s.writeRevokeFail(w, oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
		return
	}
	req, auth, challenge, err := s.parseRevokeRequest(r)
	if err != nil {
		var we *tokenWireError
		if errors.As(err, &we) {
			s.writeRevokeFail(w, we.code, we.desc, we.challenge)
			return
		}
		var se *brokerStatusError
		if errors.As(err, &se) {
			writeBrokerStatusError(w, se)
			return
		}
		s.writeRevokeFail(w, oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
		return
	}
	if s.oauth == nil {
		s.writeRevokeFail(w, oauth.ErrorTemporarilyUnavail, descTemporarilyUnavail, false)
		return
	}
	if err := s.oauth.Revoke(r.Context(), req, auth); err != nil {
		we := mapTokenServiceError(err, challenge)
		s.writeRevokeFail(w, we.code, we.desc, we.challenge)
		return
	}
	s.writeRevokeSuccess(w)
}

func (s *Server) parseRevokeRequest(r *http.Request) (oauth.RevokeRequest, oauth.ClientAuth, bool, error) {
	if r.URL != nil && r.URL.RawQuery != "" {
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	if !exactTokenContentType(r.Header.Get("Content-Type")) {
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, false, s.tokenMediaError()
	}
	if r.ContentLength > revokeFormMaxBytes {
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, false, s.tokenTooLargeError()
	}
	r.Body = http.MaxBytesReader(nil, r.Body, revokeFormMaxBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return oauth.RevokeRequest{}, oauth.ClientAuth{}, false, s.tokenTooLargeError()
		}
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	form, err := parseStrictRevokeForm(raw)
	if err != nil {
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, false, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	auth, challenge, err := parseTokenClientAuth(r.Header.Values("Authorization"), form)
	if err != nil {
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, challenge, err
	}
	if form.Get("token") == "" {
		return oauth.RevokeRequest{}, oauth.ClientAuth{}, challenge, tokenWire(oauth.ErrorInvalidRequest, descInvalidRequest, false)
	}
	return oauth.RevokeRequest{
		Token:               form.Get("token"),
		TokenTypeHint:       form.Get("token_type_hint"),
		ClientID:            form.Get("client_id"),
		ClientAssertionType: form.Get("client_assertion_type"),
		ClientAssertion:     form.Get("client_assertion"),
	}, auth, challenge, nil
}

func parseStrictRevokeForm(raw []byte) (url.Values, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("invalid utf8")
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{
		"token": {}, "token_type_hint": {}, "client_id": {},
		"client_assertion_type": {}, "client_assertion": {}, "client_secret": {},
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

func (s *Server) writeRevokeSuccess(w http.ResponseWriter) {
	setRevokeHeaders(w, s.productionHSTS(), false)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) writeRevokeFail(w http.ResponseWriter, code, description string, basicChallenge bool) {
	status := http.StatusBadRequest
	switch code {
	case oauth.ErrorInvalidClient:
		status = http.StatusUnauthorized
	case oauth.ErrorTemporarilyUnavail:
		status = http.StatusServiceUnavailable
	}
	setRevokeHeaders(w, s.productionHSTS(), basicChallenge)
	w.Header().Set("Content-Type", tokenJSONContentType)
	w.WriteHeader(status)
	body, err := jsonMarshalRevokeError(code, description)
	if err != nil {
		return
	}
	_, _ = w.Write(body)
}

func jsonMarshalRevokeError(code, description string) ([]byte, error) {
	return json.Marshal(oauthTokenError{Code: code, ErrorDescription: tokenWireDescription(code, description)})
}

func setRevokeHeaders(w http.ResponseWriter, hsts string, basicChallenge bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if hsts != "" {
		w.Header().Set("Strict-Transport-Security", hsts)
	}
	if basicChallenge {
		w.Header().Set("WWW-Authenticate", revokeBasicChallenge)
	}
}

func revokeRequestBody() *huma.RequestBody {
	writeOnly := func() *huma.Schema {
		return &huma.Schema{Type: huma.TypeString, WriteOnly: true}
	}
	plain := func() *huma.Schema {
		return &huma.Schema{Type: huma.TypeString}
	}
	return &huma.RequestBody{
		Required:    true,
		Description: "OAuth revocation form",
		Content: map[string]*huma.MediaType{
			tokenFormContentType: {
				Schema: &huma.Schema{
					Type: huma.TypeObject,
					Properties: map[string]*huma.Schema{
						"token":                 writeOnly(),
						"token_type_hint":       plain(),
						"client_id":             plain(),
						"client_assertion_type": plain(),
						"client_assertion":      writeOnly(),
					},
					Required: []string{"token"},
				},
			},
		},
	}
}

func revokeOpenAPIResponses() map[string]*huma.Response {
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
	errSchema := &huma.Schema{
		Type: huma.TypeObject,
		Properties: map[string]*huma.Schema{
			"error":             {Type: huma.TypeString},
			"error_description": {Type: huma.TypeString},
		},
		Required: []string{"error"},
	}
	return map[string]*huma.Response{
		"200": {Description: "RFC7009 empty success", Headers: security},
		"400": {Description: "OAuth invalid_request", Headers: security, Content: jsonMedia(errSchema)},
		"401": {Description: "OAuth invalid_client", Headers: security, Content: jsonMedia(&huma.Schema{
			Type:                 huma.TypeObject,
			AdditionalProperties: false,
			Properties: map[string]*huma.Schema{
				"error": {Type: huma.TypeString, Const: oauth.ErrorInvalidClient},
			},
			Required: []string{"error"},
		})},
		"413": {Description: "Request too large", Headers: security},
		"415": {Description: "Unsupported media type", Headers: security},
		"503": {Description: "temporarily_unavailable", Headers: security, Content: jsonMedia(errSchema)},
	}
}
