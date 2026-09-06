package api

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
)

const (
	brokerFormMaxBytes = 4096
	brokerCSP          = "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	loginPath          = "/broker/stytch/login"
)

// CallbackCapture is test diagnostic plumbing, not an HTTP endpoint. It stores
// only the safe projection; listener/request keys are private in-memory routing
// and are never included in diagnostic output. Captures are explicitly removed.
type CallbackCapture struct {
	mu       sync.Mutex
	failure  broker.CallbackFailure
	captured bool
}

func (c *CallbackCapture) Snapshot() (broker.CallbackFailure, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failure, c.captured
}

type callbackCaptureKey struct{ address, requestID string }

var callbackCaptures sync.Map

// CaptureBrokerCallbackForTest correlates a real network request without
// scraping process-global slog buffers (concurrent app.Run instances replace
// that logger). requestID uses the existing X-Request-ID middleware contract;
// no diagnostic header or public response is added. Only tests register sinks.
func CaptureBrokerCallbackForTest(address, requestID string) (*CallbackCapture, func()) {
	if address == "" || requestID == "" {
		panic("callback capture requires private routing keys")
	}
	key := callbackCaptureKey{address, requestID}
	capture := &CallbackCapture{}
	if _, exists := callbackCaptures.LoadOrStore(key, capture); exists {
		panic("duplicate callback capture")
	}
	return capture, func() { callbackCaptures.CompareAndDelete(key, capture) }
}

func reportCallbackFailure(ctx context.Context, r *http.Request, d broker.CallbackFailure) {
	// No err.Error(), HTTP data, identifiers, SQL details or request IDs here.
	slog.WarnContext(ctx, "broker_callback_failure", "stage", d.Stage(), "class", d.Class(), "attempts", d.Attempts(), "limit", d.Limit(), "exhausted", d.Exhausted(), "mapping_calls", d.MappingCalls())
	if r == nil {
		return
	}
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return
	}
	key := callbackCaptureKey{addr.String(), RequestIDFromContext(ctx)}
	if sink, ok := callbackCaptures.Load(key); ok {
		capture := sink.(*CallbackCapture)
		capture.mu.Lock()
		capture.failure = d
		capture.captured = true
		capture.mu.Unlock()
	}
}

type requestKey struct{}

func (s *Server) registerBrokerRoutes(api huma.API) {
	if s.broker == nil && !s.registerBrokerInventory {
		return
	}

	attach := huma.Middlewares{s.attachRequest}

	type authorizeOut struct {
		Status             int
		Location           string `header:"Location"`
		CacheControl       string `header:"Cache-Control"`
		Pragma             string `header:"Pragma"`
		ReferrerPolicy     string `header:"Referrer-Policy"`
		ContentTypeOptions string `header:"X-Content-Type-Options"`
		StrictTransport    string `header:"Strict-Transport-Security"`
		SetCookie          string `header:"Set-Cookie"`
	}
	huma.Register(api, huma.Operation{
		OperationID:        "oauthAuthorize",
		Method:             http.MethodGet,
		Path:               "/oauth/authorize",
		Summary:            "Start an OAuth authorization request",
		Tags:               []string{"Broker"},
		DefaultStatus:      http.StatusSeeOther,
		SkipValidateParams: true,
		Errors:             []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge},
		Middlewares:        attach,
	}, func(ctx context.Context, _ *struct{}) (*authorizeOut, error) {
		r := requestOf(ctx)
		if r == nil {
			return nil, s.brokerLocalError(true)
		}
		if requestTargetBytes(r) > s.requestTargetMax() {
			return nil, s.brokerRequestTooLargeError()
		}
		parsed, err := broker.ParseAuthorizeRequest(r.URL.Query())
		if err != nil {
			return nil, s.brokerLocalError(true)
		}
		if s.broker == nil {
			return nil, s.brokerLocalError(true)
		}
		started, err := s.broker.Authorize(ctx, parsed)
		if err != nil {
			if loc, ok := s.trustedOAuthErrorLocation(err); ok {
				out := &authorizeOut{
					Status:             http.StatusSeeOther,
					Location:           loc,
					CacheControl:       "no-store",
					Pragma:             "no-cache",
					ReferrerPolicy:     "no-referrer",
					ContentTypeOptions: "nosniff",
				}
				out.StrictTransport = s.productionHSTS()
				return out, nil
			}
			return nil, s.brokerLocalError(true)
		}
		out := &authorizeOut{
			Status:             http.StatusSeeOther,
			Location:           loginPath,
			CacheControl:       "no-store",
			Pragma:             "no-cache",
			ReferrerPolicy:     "no-referrer",
			ContentTypeOptions: "nosniff",
			SetCookie:          s.brokerCookie(started.CookieValue, int(broker.BrokerTransactionTTL.Seconds())).String(),
		}
		out.StrictTransport = s.productionHSTS()
		return out, nil
	})

	type loginIn struct {
		Accept string `header:"Accept"`
		Cookie string `cookie:"__Host-primer-broker"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "brokerLogin",
		Method:        http.MethodGet,
		Path:          loginPath,
		Summary:       "Broker login discovery",
		Tags:          []string{"Broker"},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusBadRequest},
		Middlewares:   attach,
	}, func(ctx context.Context, in *loginIn) (*huma.StreamResponse, error) {
		r := requestOf(ctx)
		cookie := in.Cookie
		if cookie == "" && r != nil {
			if c, err := r.Cookie(broker.BrokerCookieName); err == nil {
				cookie = c.Value
			}
		}
		if cookie == "" || s.broker == nil || s.broker.BoundCookie(ctx, cookie) != nil {
			return nil, s.brokerLocalError(true)
		}
		csrf, err := s.broker.MintCSRF(cookie)
		if err != nil || csrf == "" {
			return nil, s.brokerLocalError(true)
		}
		accept := ""
		if in != nil {
			accept = in.Accept
		}
		if r != nil && accept == "" {
			accept = r.Header.Get("Accept")
		}
		wantJSON := strings.Contains(strings.ToLower(accept), "application/json")
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			setBrokerHeaders(hctx, true, s.productionHSTS())
			if wantJSON {
				hctx.SetHeader("Content-Type", "application/json")
				_ = json.NewEncoder(hctx.BodyWriter()).Encode(struct {
					CSRF string `json:"csrf"`
				}{CSRF: csrf})
				return
			}
			hctx.SetHeader("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(hctx.BodyWriter(), loginHTML(csrf))
		}}, nil
	})

	s.registerStartRoute(api, attach, "brokerEmailStart", "/broker/stytch/email/start", func(url.Values) (brokerprovider.Method, bool) {
		return brokerprovider.MethodEmailMagicLink, true
	})
	s.registerStartRoute(api, attach, "brokerEmailVerify", "/broker/stytch/email/verify", func(url.Values) (brokerprovider.Method, bool) {
		return brokerprovider.MethodEmailOTP, true
	})
	s.registerStartRoute(api, attach, "brokerSSOStart", "/broker/stytch/sso/start", func(form url.Values) (brokerprovider.Method, bool) {
		switch form.Get("type") {
		case "saml":
			return brokerprovider.MethodSSOSAML, true
		case "oidc":
			return brokerprovider.MethodSSOOIDC, true
		default:
			return "", false
		}
	})

	type callbackIn struct {
		Token    string `query:"token" doc:"Single-use callback artifact" writeOnly:"true"`
		SSOToken string `query:"sso_token" doc:"Single-use SSO callback artifact" writeOnly:"true"`
		Type     string `query:"type"`
	}
	type callbackOut struct {
		Status             int
		Location           string `header:"Location"`
		CacheControl       string `header:"Cache-Control"`
		Pragma             string `header:"Pragma"`
		ReferrerPolicy     string `header:"Referrer-Policy"`
		ContentTypeOptions string `header:"X-Content-Type-Options"`
		StrictTransport    string `header:"Strict-Transport-Security"`
		SetCookie          string `header:"Set-Cookie"`
	}
	huma.Register(api, huma.Operation{
		OperationID:                  "brokerCallback",
		Method:                       http.MethodGet,
		Path:                         "/broker/stytch/callback",
		Summary:                      "Complete a broker callback",
		Tags:                         []string{"Broker"},
		DefaultStatus:                http.StatusSeeOther,
		SkipValidateParams:           true,
		RejectUnknownQueryParameters: false,
		Errors:                       []int{http.StatusBadRequest, http.StatusServiceUnavailable},
		Middlewares:                  attach,
	}, func(ctx context.Context, _ *callbackIn) (*callbackOut, error) {
		r := requestOf(ctx)
		if r == nil {
			reportCallbackFailure(ctx, r, broker.CallbackInputFailure())
			return nil, s.brokerLocalError(true)
		}
		cookie := ""
		if c, err := r.Cookie(broker.BrokerCookieName); err == nil {
			cookie = c.Value
		}
		_, in, ok := parseCallbackInput(r.URL.Query())
		if !ok || cookie == "" || s.broker == nil {
			reportCallbackFailure(ctx, r, broker.CallbackInputFailure())
			return nil, s.brokerLocalError(true)
		}
		in.CookieValue = cookie
		result, err := s.broker.CompleteCallback(ctx, in)
		if err != nil {
			reportCallbackFailure(ctx, r, broker.CallbackFailureDetails(err))
			if errors.Is(err, broker.ErrProviderUnavailable) {
				return nil, s.brokerUnavailableError()
			}
			if loc, ok := s.trustedOAuthErrorLocation(err); ok {
				expired := s.brokerCookie(cookie, -1)
				out := &callbackOut{
					Status:             http.StatusSeeOther,
					Location:           loc,
					CacheControl:       "no-store",
					Pragma:             "no-cache",
					ReferrerPolicy:     "no-referrer",
					ContentTypeOptions: "nosniff",
					SetCookie:          expired.String(),
				}
				out.StrictTransport = s.productionHSTS()
				return out, nil
			}
			return nil, s.brokerLocalError(true)
		}
		loc, err := callbackRedirect(result)
		if err != nil {
			reportCallbackFailure(ctx, r, broker.CallbackRedirectFailure())
			return nil, s.brokerLocalError(true)
		}
		expired := s.brokerCookie(cookie, -1)
		out := &callbackOut{
			Status:             http.StatusSeeOther,
			Location:           loc,
			CacheControl:       "no-store",
			Pragma:             "no-cache",
			ReferrerPolicy:     "no-referrer",
			ContentTypeOptions: "nosniff",
			SetCookie:          expired.String(),
		}
		out.StrictTransport = s.productionHSTS()
		return out, nil
	})
}

func (s *Server) registerStartRoute(api huma.API, attach huma.Middlewares, id, path string, methodOf func(url.Values) (brokerprovider.Method, bool)) {
	type startOut struct {
		Status   int
		Location string `header:"Location"`
		Body     struct {
			Status string `json:"status" example:"started"`
		}
		CacheControl       string `header:"Cache-Control"`
		Pragma             string `header:"Pragma"`
		ReferrerPolicy     string `header:"Referrer-Policy"`
		ContentTypeOptions string `header:"X-Content-Type-Options"`
		StrictTransport    string `header:"Strict-Transport-Security"`
	}
	huma.Register(api, huma.Operation{
		OperationID:      id,
		Method:           http.MethodPost,
		Path:             path,
		Summary:          "Start a broker authentication method",
		Tags:             []string{"Broker"},
		DefaultStatus:    http.StatusOK,
		SkipValidateBody: true,
		MaxBodyBytes:     brokerFormMaxBytes,
		Errors:           []int{http.StatusBadRequest, http.StatusSeeOther},
		Middlewares:      attach,
	}, func(ctx context.Context, _ *struct{}) (*startOut, error) {
		r := requestOf(ctx)
		if r == nil {
			return nil, s.brokerLocalError(true)
		}
		cookie, form, err := s.requireBrokerPOST(r)
		if err != nil {
			return nil, err
		}
		method, ok := methodOf(form)
		if !ok {
			return nil, s.brokerLocalError(true)
		}
		in, err := parseStartInput(method, form)
		if err != nil {
			return nil, s.brokerLocalError(true)
		}
		if s.broker == nil {
			return nil, s.brokerLocalError(true)
		}
		started, err := s.broker.Start(ctx, cookie, in)
		if err != nil {
			return nil, s.brokerLocalError(true)
		}
		out := &startOut{
			Status:             http.StatusOK,
			CacheControl:       "no-store",
			Pragma:             "no-cache",
			ReferrerPolicy:     "no-referrer",
			ContentTypeOptions: "nosniff",
		}
		out.StrictTransport = s.productionHSTS()
		if method == brokerprovider.MethodSSOSAML || method == brokerprovider.MethodSSOOIDC {
			loc, err := s.officialSSOContinueURL(started.ContinueURL)
			if err != nil {
				return nil, s.brokerLocalError(true)
			}
			out.Status = http.StatusSeeOther
			out.Location = loc
			return out, nil
		}
		out.Body.Status = "started"
		return out, nil
	})
}

func (s *Server) attachRequest(ctx huma.Context, next func(huma.Context)) {
	r, _ := humachi.Unwrap(ctx)
	next(huma.WithValue(ctx, requestKey{}, r))
}

func requestOf(ctx context.Context) *http.Request {
	r, _ := ctx.Value(requestKey{}).(*http.Request)
	return r
}

func (s *Server) requireBrokerPOST(r *http.Request) (string, url.Values, error) {
	if !exactFormContentType(r.Header.Get("Content-Type")) {
		return "", nil, s.brokerLocalError(true)
	}
	if s.brokerHTTP.AllowedOrigin == "" || r.Header.Get("Origin") != s.brokerHTTP.AllowedOrigin {
		return "", nil, s.brokerLocalError(true)
	}
	c, err := r.Cookie(broker.BrokerCookieName)
	if err != nil || c.Value == "" {
		return "", nil, s.brokerLocalError(true)
	}
	r.Body = http.MaxBytesReader(nil, r.Body, brokerFormMaxBytes)
	if err := r.ParseForm(); err != nil {
		return "", nil, s.brokerLocalError(true)
	}
	csrf := r.Header.Get("X-CSRF-Token")
	if csrf == "" {
		csrf = r.PostForm.Get("csrf")
	}
	if s.broker == nil || !s.broker.ValidateCSRF(c.Value, csrf) {
		return "", nil, s.brokerLocalError(true)
	}
	if s.broker.BoundCookie(r.Context(), c.Value) != nil {
		return "", nil, s.brokerLocalError(true)
	}
	return c.Value, r.PostForm, nil
}

func exactFormContentType(raw string) bool {
	media, _, err := mime.ParseMediaType(raw)
	return err == nil && media == "application/x-www-form-urlencoded"
}

func (s *Server) brokerCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     broker.BrokerCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !s.brokerHTTP.InsecureTestCookie,
	}
}

func (s *Server) productionHSTS() string {
	if s != nil && s.brokerHTTP.Production {
		return productionHSTS
	}
	return ""
}

func (s *Server) requestTargetMax() int {
	if s == nil || s.brokerHTTP.MaxRequestTargetBytes == 0 {
		return DefaultAuthorizeRequestTargetMax
	}
	return s.brokerHTTP.MaxRequestTargetBytes
}

func requestTargetBytes(r *http.Request) int {
	if r == nil {
		return 0
	}
	// RFC 9112 request-target is the raw URI as received (path + query).
	if raw := r.RequestURI; raw != "" {
		return len(raw)
	}
	if r.URL == nil {
		return 0
	}
	if q := r.URL.RawQuery; q != "" {
		return len(r.URL.Path) + 1 + len(q)
	}
	return len(r.URL.Path)
}

func (s *Server) brokerLocalError(withCSP bool) error {
	return brokerHeaderError(huma.Error400BadRequest("invalid request"), withCSP, s.productionHSTS())
}

func (s *Server) brokerUnavailableError() error {
	return brokerHeaderError(huma.Error503ServiceUnavailable("temporarily unavailable"), true, s.productionHSTS())
}

func (s *Server) brokerRequestTooLargeError() error {
	return brokerHeaderError(huma.Error413RequestEntityTooLarge("invalid request"), true, s.productionHSTS())
}

func brokerHeaderError(err error, withCSP bool, hsts string) error {
	h := make(http.Header)
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	if withCSP {
		h.Set("Content-Security-Policy", brokerCSP)
	}
	if hsts != "" {
		h.Set("Strict-Transport-Security", hsts)
	}
	return &brokerStatusError{statusErr: err, headers: h}
}

type brokerStatusError struct {
	statusErr error
	headers   http.Header
}

func (e *brokerStatusError) Error() string { return e.statusErr.Error() }
func (e *brokerStatusError) Unwrap() error { return e.statusErr }
func (e *brokerStatusError) GetHeaders() http.Header {
	return e.headers
}
func (e *brokerStatusError) GetStatus() int {
	var se huma.StatusError
	if errors.As(e.statusErr, &se) {
		return se.GetStatus()
	}
	return http.StatusBadRequest
}

func setBrokerHeaders(ctx huma.Context, withCSP bool, hsts string) {
	ctx.SetHeader("Cache-Control", "no-store")
	ctx.SetHeader("Pragma", "no-cache")
	ctx.SetHeader("Referrer-Policy", "no-referrer")
	ctx.SetHeader("X-Content-Type-Options", "nosniff")
	if withCSP {
		ctx.SetHeader("Content-Security-Policy", brokerCSP)
	}
	if hsts != "" {
		ctx.SetHeader("Strict-Transport-Security", hsts)
	}
}

func loginHTML(csrf string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Continue</title></head><body>` +
		`<form method="post" action="/broker/continue">` +
		`<input type="hidden" name="csrf" value="` + html.EscapeString(csrf) + `">` +
		`<button type="submit">Continue</button></form></body></html>`
}

func parseStartInput(method brokerprovider.Method, form url.Values) (broker.StartInput, error) {
	in := broker.StartInput{Method: method}
	switch method {
	case brokerprovider.MethodEmailMagicLink, brokerprovider.MethodEmailOTP:
		if err := rejectUnknownForm(form, "csrf", "email"); err != nil {
			return broker.StartInput{}, err
		}
		email, err := exactlyOneBounded(form, "email", 254, true)
		if err != nil {
			return broker.StartInput{}, err
		}
		if !validStartEmail(email) {
			return broker.StartInput{}, errors.New("invalid email")
		}
		in.EmailAddress = email
	case brokerprovider.MethodSSOSAML, brokerprovider.MethodSSOOIDC:
		if err := rejectUnknownForm(form, "csrf", "type", "connection_id", "organization_id"); err != nil {
			return broker.StartInput{}, err
		}
		if _, err := exactlyOneBounded(form, "type", 16, false); err != nil {
			return broker.StartInput{}, err
		}
		connPresent := len(form["connection_id"]) > 0
		orgPresent := len(form["organization_id"]) > 0
		if connPresent == orgPresent {
			return broker.StartInput{}, errors.New("sso selector required")
		}
		if connPresent {
			conn, err := exactlyOneBounded(form, "connection_id", 255, false)
			if err != nil {
				return broker.StartInput{}, err
			}
			in.ConnectionID = conn
		} else {
			org, err := exactlyOneBounded(form, "organization_id", 255, false)
			if err != nil {
				return broker.StartInput{}, err
			}
			in.OrganizationID = org
		}
	default:
		return broker.StartInput{}, errors.New("unsupported method")
	}
	return in, nil
}

func rejectUnknownForm(form url.Values, allowed ...string) error {
	permit := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		permit[name] = struct{}{}
	}
	for name := range form {
		if _, ok := permit[name]; !ok {
			return errors.New("unknown field")
		}
	}
	return nil
}

func exactlyOneBounded(form url.Values, name string, maxBytes int, email bool) (string, error) {
	values := form[name]
	if len(values) != 1 {
		return "", errors.New("parameter must appear exactly once")
	}
	value := values[0]
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return "", errors.New("invalid field")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("invalid field")
		}
	}
	_ = email
	return value, nil
}

func validStartEmail(value string) bool {
	at := strings.IndexByte(value, '@')
	if at <= 0 || at != strings.LastIndexByte(value, '@') || at == len(value)-1 {
		return false
	}
	return !strings.ContainsAny(value, " ")
}

func (s *Server) officialSSOContinueURL(raw string) (string, error) {
	if raw == "" || s.brokerHTTP.PublicHost == "" || s.brokerHTTP.PublicToken == "" {
		return "", errors.New("unofficial continue url")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Fragment != "" || u.ForceQuery {
		return "", errors.New("unofficial continue url")
	}
	if u.Hostname() != s.brokerHTTP.PublicHost || u.Port() != "" || u.EscapedPath() != "/v1/public/sso/start" {
		return "", errors.New("unofficial continue url")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("unofficial continue url")
	}
	allowed := map[string]struct{}{
		"public_token": {}, "connection_id": {}, "organization_id": {},
	}
	for name, values := range q {
		if _, ok := allowed[name]; !ok {
			return "", errors.New("unofficial continue url")
		}
		if len(values) != 1 {
			return "", errors.New("unofficial continue url")
		}
		if values[0] == "" || !utf8.ValidString(values[0]) || containsControl(values[0]) {
			return "", errors.New("unofficial continue url")
		}
	}
	tokens := q["public_token"]
	if len(tokens) != 1 || tokens[0] != s.brokerHTTP.PublicToken {
		return "", errors.New("unofficial continue url")
	}
	conns := q["connection_id"]
	orgs := q["organization_id"]
	if (len(conns) == 1) == (len(orgs) == 1) {
		return "", errors.New("unofficial continue url")
	}
	out := url.Values{}
	out.Set("public_token", s.brokerHTTP.PublicToken)
	if len(conns) == 1 {
		out.Set("connection_id", conns[0])
	} else {
		out.Set("organization_id", orgs[0])
	}
	return "https://" + s.brokerHTTP.PublicHost + "/v1/public/sso/start?" + out.Encode(), nil
}

func parseCallbackInput(query url.Values) (string, broker.CallbackInput, bool) {
	allowed := map[string]struct{}{
		"token": {}, "sso_token": {}, "type": {}, "email": {}, "organization_id": {},
	}
	for name, values := range query {
		if _, ok := allowed[name]; !ok {
			return "", broker.CallbackInput{}, false
		}
		if len(values) != 1 {
			return "", broker.CallbackInput{}, false
		}
	}
	types := query["type"]
	if len(types) != 1 {
		return "", broker.CallbackInput{}, false
	}
	typ := brokerprovider.ArtifactType(types[0])
	tokens := query["token"]
	ssos := query["sso_token"]
	emails := query["email"]
	orgs := query["organization_id"]
	in := broker.CallbackInput{Type: typ}
	switch typ {
	case brokerprovider.ArtifactTypeMagicLink, brokerprovider.ArtifactTypeDiscoveryMagicLink:
		if len(tokens) != 1 || len(ssos) != 0 || len(emails) != 0 || len(orgs) != 0 {
			return "", broker.CallbackInput{}, false
		}
		if !validCallbackArtifact(tokens[0]) {
			return "", broker.CallbackInput{}, false
		}
		in.Artifact = tokens[0]
	case brokerprovider.ArtifactTypeDiscoveryEmailOTP:
		if len(tokens) != 1 || len(ssos) != 0 || len(emails) != 1 || len(orgs) != 0 {
			return "", broker.CallbackInput{}, false
		}
		if !validCallbackArtifact(tokens[0]) || !validStartEmail(emails[0]) || len(emails[0]) > 254 {
			return "", broker.CallbackInput{}, false
		}
		in.Artifact = tokens[0]
		in.EmailAddress = emails[0]
	case brokerprovider.ArtifactTypeEmailOTP:
		if len(tokens) != 1 || len(ssos) != 0 || len(emails) != 1 || len(orgs) != 1 {
			return "", broker.CallbackInput{}, false
		}
		if !validCallbackArtifact(tokens[0]) || !validStartEmail(emails[0]) || len(emails[0]) > 254 || !validBoundedSelector(orgs[0]) {
			return "", broker.CallbackInput{}, false
		}
		in.Artifact = tokens[0]
		in.EmailAddress = emails[0]
		in.OrganizationID = orgs[0]
	case brokerprovider.ArtifactTypeSSOToken:
		if len(ssos) != 1 || len(tokens) != 0 || len(emails) != 0 || len(orgs) != 0 {
			return "", broker.CallbackInput{}, false
		}
		if !validCallbackArtifact(ssos[0]) {
			return "", broker.CallbackInput{}, false
		}
		in.Artifact = ssos[0]
	default:
		return "", broker.CallbackInput{}, false
	}
	return in.Artifact, in, true
}

func validCallbackArtifact(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && !containsControl(value)
}

func validBoundedSelector(value string) bool {
	return value != "" && len(value) <= 255 && utf8.ValidString(value) && !containsControl(value)
}

func containsControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func callbackRedirect(result broker.CallbackResult) (string, error) {
	u, err := url.Parse(result.RedirectURI)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid redirect")
	}
	q := url.Values{}
	q.Set("code", result.Code)
	q.Set("state", string(result.State))
	q.Set("iss", result.Issuer)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Server) trustedOAuthErrorLocation(err error) (string, bool) {
	tr, ok := broker.AsTrustedRedirect(err)
	if !ok {
		return "", false
	}
	loc, buildErr := trustedErrorRedirect(tr)
	for i := range tr.State {
		tr.State[i] = 0
	}
	if buildErr != nil {
		return "", false
	}
	return loc, true
}

func trustedErrorRedirect(tr *broker.TrustedRedirect) (string, error) {
	if tr == nil {
		return "", errors.New("invalid redirect")
	}
	u, err := url.Parse(tr.RedirectURI)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid redirect")
	}
	q := url.Values{}
	q.Set("error", tr.Code)
	q.Set("error_description", tr.Description)
	q.Set("state", string(tr.State))
	q.Set("iss", tr.Issuer)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
