package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
)

const (
	brokerFormMaxBytes = 4096
	brokerCSP          = "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	loginPath          = "/broker/stytch/login"
	csrfContext        = "primer.broker.csrf"
)

type requestKey struct{}

func (s *Server) registerBrokerRoutes(api huma.API) {
	if s.broker == nil {
		return
	}

	attach := huma.Middlewares{s.attachRequest}

	type authorizeOut struct {
		Status             int
		Location           string `header:"Location"`
		CacheControl       string `header:"Cache-Control"`
		ReferrerPolicy     string `header:"Referrer-Policy"`
		ContentTypeOptions string `header:"X-Content-Type-Options"`
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
		Errors:             []int{http.StatusBadRequest},
		Middlewares:        attach,
	}, func(ctx context.Context, _ *struct{}) (*authorizeOut, error) {
		r := requestOf(ctx)
		if r == nil {
			return nil, s.brokerLocalError(true)
		}
		parsed, err := broker.ParseAuthorizeRequest(r.URL.Query())
		if err != nil {
			return nil, s.brokerLocalError(true)
		}
		started, err := s.broker.Authorize(ctx, parsed)
		if err != nil {
			return nil, s.brokerLocalError(true)
		}
		return &authorizeOut{
			Status:             http.StatusSeeOther,
			Location:           loginPath,
			CacheControl:       "no-store",
			ReferrerPolicy:     "no-referrer",
			ContentTypeOptions: "nosniff",
			SetCookie:          s.brokerCookie(started.CookieValue, int(broker.BrokerTransactionTTL.Seconds())).String(),
		}, nil
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
		if cookie == "" || s.broker.BoundCookie(ctx, cookie) != nil {
			return nil, s.brokerLocalError(true)
		}
		csrf := csrfForCookie(cookie)
		accept := ""
		if in != nil {
			accept = in.Accept
		}
		if r != nil && accept == "" {
			accept = r.Header.Get("Accept")
		}
		wantJSON := strings.Contains(strings.ToLower(accept), "application/json")
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			setBrokerHeaders(hctx, true)
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
		ReferrerPolicy     string `header:"Referrer-Policy"`
		ContentTypeOptions string `header:"X-Content-Type-Options"`
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
			return nil, s.brokerLocalError(true)
		}
		cookie := ""
		if c, err := r.Cookie(broker.BrokerCookieName); err == nil {
			cookie = c.Value
		}
		artifact, ok := singleCallbackArtifact(r.URL.Query())
		if !ok || cookie == "" {
			return nil, s.brokerLocalError(true)
		}
		result, err := s.broker.CompleteCallback(ctx, broker.CallbackInput{
			CookieValue: cookie, Artifact: artifact,
		})
		if err != nil {
			if errors.Is(err, broker.ErrProviderUnavailable) {
				return nil, s.brokerUnavailableError()
			}
			return nil, s.brokerLocalError(true)
		}
		loc, err := callbackRedirect(result)
		if err != nil {
			return nil, s.brokerLocalError(true)
		}
		expired := s.brokerCookie(cookie, -1)
		return &callbackOut{
			Status:             http.StatusSeeOther,
			Location:           loc,
			CacheControl:       "no-store",
			ReferrerPolicy:     "no-referrer",
			ContentTypeOptions: "nosniff",
			SetCookie:          expired.String(),
		}, nil
	})
}

func (s *Server) registerStartRoute(api huma.API, attach huma.Middlewares, id, path string, methodOf func(url.Values) (brokerprovider.Method, bool)) {
	type startOut struct {
		Body struct {
			Status string `json:"status" example:"started"`
		}
		CacheControl       string `header:"Cache-Control"`
		ReferrerPolicy     string `header:"Referrer-Policy"`
		ContentTypeOptions string `header:"X-Content-Type-Options"`
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
		Errors:           []int{http.StatusBadRequest},
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
		if _, err := s.broker.StartMethod(ctx, cookie, method); err != nil {
			return nil, s.brokerLocalError(true)
		}
		out := &startOut{
			CacheControl:       "no-store",
			ReferrerPolicy:     "no-referrer",
			ContentTypeOptions: "nosniff",
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
	if !csrfValid(c.Value, csrf) {
		return "", nil, s.brokerLocalError(true)
	}
	if err := s.broker.BoundCookie(r.Context(), c.Value); err != nil {
		return "", nil, s.brokerLocalError(true)
	}
	return c.Value, r.PostForm, nil
}

func exactFormContentType(raw string) bool {
	media, _, err := mime.ParseMediaType(raw)
	return err == nil && media == "application/x-www-form-urlencoded"
}

func csrfForCookie(cookie string) string {
	sum := sha256.Sum256([]byte(csrfContext + "\x00" + cookie))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func csrfValid(cookie, presented string) bool {
	if presented == "" {
		return false
	}
	expected := csrfForCookie(cookie)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
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

func (s *Server) brokerLocalError(withCSP bool) error {
	return brokerHeaderError(huma.Error400BadRequest("invalid request"), withCSP)
}

func (s *Server) brokerUnavailableError() error {
	return brokerHeaderError(huma.Error503ServiceUnavailable("temporarily unavailable"), true)
}

func brokerHeaderError(err error, withCSP bool) error {
	h := make(http.Header)
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	if withCSP {
		h.Set("Content-Security-Policy", brokerCSP)
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

func setBrokerHeaders(ctx huma.Context, withCSP bool) {
	ctx.SetHeader("Cache-Control", "no-store")
	ctx.SetHeader("Referrer-Policy", "no-referrer")
	ctx.SetHeader("X-Content-Type-Options", "nosniff")
	if withCSP {
		ctx.SetHeader("Content-Security-Policy", brokerCSP)
	}
}

func loginHTML(csrf string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Continue</title></head><body>` +
		`<form method="post" action="/broker/continue">` +
		`<input type="hidden" name="csrf" value="` + html.EscapeString(csrf) + `">` +
		`<button type="submit">Continue</button></form></body></html>`
}

func singleCallbackArtifact(query url.Values) (string, bool) {
	tokens := query["token"]
	ssos := query["sso_token"]
	if len(tokens)+len(ssos) != 1 {
		return "", false
	}
	artifact := ""
	if len(tokens) == 1 {
		artifact = tokens[0]
	} else {
		artifact = ssos[0]
	}
	if artifact == "" || len(artifact) > 4096 {
		return "", false
	}
	return artifact, true
}

func callbackRedirect(result broker.CallbackResult) (string, error) {
	u, err := url.Parse(result.RedirectURI)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid redirect")
	}
	q := u.Query()
	q.Set("code", result.Code)
	q.Set("state", string(result.State))
	q.Set("iss", result.Issuer)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
