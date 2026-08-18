package bff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Config is the static product BFF registration. Redirect, resource, and
// audience are exact; there is no DCR or Host-derived substitution.
type Config struct {
	Product     Product
	Issuer      string
	PublicURL   string
	ClientID    string
	RedirectURI string
	Resource    string
	Audience    string
	Scopes      []string
	Origins     []string
	Cookie      CookiePolicy
	Store       Store
	HTTPClient  *http.Client
	Now         func() time.Time
}

// Handler is the product BFF HTTP surface.
type Handler struct {
	cfg Config
}

const (
	csrfFormField = "csrf"
	csrfHeader    = "X-CSRF-Token"
)

// CSRFHeader is the header accepted for state-changing BFF requests. It is
// exported so API adapters can use the same spelling without importing a
// session implementation or gaining access to token custody.
const CSRFHeader = csrfHeader

// NewHandler validates static registration at request time (so a misconfigured
// process fails closed rather than silently selecting a request-derived URL).
func NewHandler(cfg Config) http.Handler {
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
	}
	if cfg.Product == "" {
		cfg.Product = ProductStudio
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid"}
	}
	h := &Handler{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", h.handleLogin)
	mux.HandleFunc("/auth/callback", h.handleCallback)
	mux.HandleFunc("/auth/logout", h.handleLogout)
	mux.HandleFunc("/auth/me", h.handleMe)
	return mux
}

func (h *Handler) now() time.Time { return h.cfg.Now().UTC() }

func (h *Handler) noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000")
}

func (h *Handler) localError(w http.ResponseWriter, status int, message string) {
	h.noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><title>Authentication error</title><p>%s</p>\n", message)
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = io.WriteString(w, "<!doctype html><title>Method not allowed</title>\n")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/login" {
		h.localError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.hostAllowed(r) {
		h.localError(w, http.StatusBadRequest, "invalid host")
		return
	}
	if err := h.cfg.validateStatic(); err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	if r.URL.RawQuery != "" {
		h.localError(w, http.StatusBadRequest, "invalid login request")
		return
	}
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	state, err := randomURLToken(stateBytes)
	if err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	now := h.now()
	rec := PreAuth{
		Product:       h.cfg.Product,
		State:         state,
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
		ClientID:      h.cfg.ClientID,
		RedirectURI:   h.cfg.RedirectURI,
		Resource:      h.cfg.Resource,
		Audience:      h.cfg.Audience,
		Issuer:        h.cfg.Issuer,
		CreatedAt:     now,
		ExpiresAt:     now.Add(PreAuthTTL),
	}
	if err := h.cfg.Store.PutPreAuth(rec); err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.cfg.ClientID},
		"redirect_uri":          {h.cfg.RedirectURI},
		"resource":              {h.cfg.Resource},
		"audience":              {h.cfg.Audience},
		"scope":                 {strings.Join(h.cfg.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	loc, err := url.Parse(strings.TrimRight(h.cfg.Issuer, "/") + "/oauth/authorize")
	if err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	loc.RawQuery = q.Encode()
	h.noStore(w)
	http.Redirect(w, r, loc.String(), http.StatusSeeOther)
}

func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/callback" {
		h.localError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.hostAllowed(r) {
		h.localError(w, http.StatusBadRequest, "invalid host")
		return
	}
	if err := h.cfg.validateStatic(); err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	if len(r.URL.RawQuery) > 8*1024 {
		h.localError(w, http.StatusRequestURITooLong, "invalid callback")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	if len(q) != 3 || len(q["state"]) != 1 || len(q["iss"]) != 1 {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	code, hasCode := singleNonempty(q, "code")
	providerError, hasProviderError := singleNonempty(q, "error")
	if hasCode == hasProviderError {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	state := q["state"][0]
	iss := q["iss"][0]
	if !validCallbackText(state, 1024) || !validCallbackText(iss, 2048) ||
		(hasCode && !validCallbackText(code, 8192)) ||
		(hasProviderError && !allowedProviderError(providerError)) {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	if !constantTimeEqual(iss, h.cfg.Issuer) {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	rec, ok := h.cfg.Store.TakePreAuth(state)
	if !ok || rec.State == "" || h.now().After(rec.ExpiresAt) {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	if rec.Product != h.cfg.Product || rec.ClientID != h.cfg.ClientID ||
		rec.RedirectURI != h.cfg.RedirectURI || rec.Resource != h.cfg.Resource ||
		rec.Audience != h.cfg.Audience || rec.Issuer != h.cfg.Issuer {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	if hasProviderError {
		h.noStore(w)
		http.Redirect(w, r, "/?error="+url.QueryEscape(providerError), http.StatusSeeOther)
		return
	}
	tokens, err := h.exchangeCode(r, rec, code)
	if err != nil {
		h.localError(w, http.StatusBadRequest, "invalid callback")
		return
	}
	sid, err := randomURLToken(sessionIDBytes)
	if err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	csrf, err := randomURLToken(32)
	if err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	now := h.now()
	sess := Session{
		ID:           sid,
		Product:      h.cfg.Product,
		ClientID:     rec.ClientID,
		Audience:     rec.Audience,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		Scope:        tokens.Scope,
		ExpiresAt:    now.Add(time.Duration(tokens.ExpiresIn) * time.Second),
		CSRF:         csrf,
		CreatedAt:    now,
	}
	if err := h.cfg.Store.PutSession(sess); err != nil {
		h.localError(w, http.StatusInternalServerError, "login unavailable")
		return
	}
	http.SetCookie(w, SessionCookie(h.cfg.Product, sid, h.cfg.Cookie))
	h.noStore(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func singleNonempty(q url.Values, name string) (string, bool) {
	values, ok := q[name]
	return func() string {
		if ok && len(values) == 1 {
			return values[0]
		}
		return ""
	}(), ok && len(values) == 1 && values[0] != ""
}

func validCallbackText(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func allowedProviderError(value string) bool {
	switch value {
	case "access_denied", "invalid_request", "unsupported_response_type", "temporarily_unavailable":
		return true
	default:
		return false
	}
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/logout" {
		h.localError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	origins := r.Header.Values("Origin")
	if !h.hostAllowed(r) || len(origins) != 1 || !h.originAllowed(origins[0]) {
		h.localError(w, http.StatusForbidden, "forbidden")
		return
	}
	cookie, err := requestSessionCookie(r, SessionCookieName(h.cfg.Product))
	if err != nil || cookie.Value == "" {
		h.localError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sess, ok := h.cfg.Store.GetSession(cookie.Value)
	if !ok || sess.Product != h.cfg.Product || h.now().After(sess.ExpiresAt) {
		h.localError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	csrf, err := parseCSRF(r)
	if err != nil || !constantTimeEqual(csrf, sess.CSRF) {
		h.localError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := h.cfg.Store.DeleteSession(cookie.Value); err != nil {
		h.localError(w, http.StatusInternalServerError, "logout unavailable")
		return
	}
	http.SetCookie(w, ClearSessionCookie(h.cfg.Product, h.cfg.Cookie))
	h.noStore(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/me" {
		h.localError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.hostAllowed(r) {
		h.localError(w, http.StatusBadRequest, "invalid host")
		return
	}
	cookie, err := requestSessionCookie(r, SessionCookieName(h.cfg.Product))
	if err != nil || cookie.Value == "" {
		h.localError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sess, ok := h.cfg.Store.GetSession(cookie.Value)
	if !ok || sess.Product != h.cfg.Product || h.now().After(sess.ExpiresAt) {
		h.localError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.noStore(w)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "csrf": sess.CSRF})
}

func requestSessionCookie(r *http.Request, name string) (*http.Cookie, error) {
	var found *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("bff cookie: duplicate session cookie")
		}
		copy := *cookie
		found = &copy
	}
	if found == nil {
		return nil, http.ErrNoCookie
	}
	return found, nil
}

func parseCSRF(r *http.Request) (string, error) {
	contentTypes := r.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return "", fmt.Errorf("bff csrf: invalid content type")
	}
	mediaType, params, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/x-www-form-urlencoded" || len(params) != 0 {
		return "", fmt.Errorf("bff csrf: invalid content type")
	}
	if r.URL.RawQuery != "" {
		return "", fmt.Errorf("bff csrf: query is not accepted")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024+1))
	if err != nil {
		return "", err
	}
	if len(body) > 8*1024 {
		return "", fmt.Errorf("bff csrf: body too large")
	}
	form, err := url.ParseQuery(string(body))
	if err != nil || len(form) != 1 || len(form[csrfFormField]) != 1 || !validCallbackText(form[csrfFormField][0], 256) {
		return "", fmt.Errorf("bff csrf: invalid form")
	}
	headerValues := r.Header.Values(csrfHeader)
	if len(headerValues) != 1 || headerValues[0] == "" || !validCallbackText(headerValues[0], 256) {
		return "", fmt.Errorf("bff csrf: invalid header")
	}
	if !constantTimeEqual(form[csrfFormField][0], headerValues[0]) {
		return "", fmt.Errorf("bff csrf: mismatch")
	}
	return form[csrfFormField][0], nil
}

func (c Config) validateStatic() error {
	if c.Product != ProductStudio && c.Product != ProductLMS && c.Product != ProductTV {
		return fmt.Errorf("bff: unknown product")
	}
	if c.Issuer == "" || c.PublicURL == "" || c.ClientID == "" || c.RedirectURI == "" || c.Resource == "" || c.Audience == "" {
		return fmt.Errorf("bff: static registration is incomplete")
	}
	issuer, err := url.Parse(c.Issuer)
	if err != nil || !validAbsoluteURL(issuer) || issuer.RawQuery != "" || issuer.Fragment != "" {
		return fmt.Errorf("bff: issuer")
	}
	public, err := url.Parse(c.PublicURL)
	if err != nil || !validAbsoluteURL(public) || public.Path != "" && public.Path != "/" || public.RawQuery != "" || public.Fragment != "" {
		return fmt.Errorf("bff: public url")
	}
	redirect, err := url.Parse(c.RedirectURI)
	if err != nil || !validAbsoluteURL(redirect) || redirect.RawQuery != "" || redirect.Fragment != "" || strings.Contains(c.RedirectURI, "*") {
		return fmt.Errorf("bff: redirect uri")
	}
	resource, err := url.Parse(c.Resource)
	if err != nil || !validAbsoluteURL(resource) || strings.Contains(c.Resource, "*") {
		return fmt.Errorf("bff: resource")
	}
	if redirect.Scheme != public.Scheme || !strings.EqualFold(redirect.Host, public.Host) || redirect.Path != "/auth/callback" {
		return fmt.Errorf("bff: redirect is not the registered public callback")
	}
	if resource.Scheme != "https" {
		return fmt.Errorf("bff: resource must use https")
	}
	if !validCallbackText(c.ClientID, 128) || strings.ContainsAny(c.ClientID, " \t\r\n") ||
		!validCallbackText(c.Audience, 256) || strings.ContainsAny(c.Audience, " \t\r\n") || !validCallbackText(c.Product.String(), 32) {
		return fmt.Errorf("bff: invalid registration text")
	}
	for _, scope := range c.Scopes {
		if !validCallbackText(scope, 128) || strings.ContainsAny(scope, " \t\r\n") {
			return fmt.Errorf("bff: invalid scope")
		}
	}
	if public.Scheme == "https" && !c.Cookie.Secure {
		return fmt.Errorf("bff: secure public url requires secure cookie")
	}
	if len(c.Origins) == 0 {
		return fmt.Errorf("bff: origin allowlist is empty")
	}
	for _, origin := range c.Origins {
		u, err := url.Parse(origin)
		if err != nil || !validAbsoluteURL(u) || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !strings.EqualFold(u.Scheme, public.Scheme) || !strings.EqualFold(u.Host, public.Host) {
			return fmt.Errorf("bff: invalid origin")
		}
	}
	return nil
}

func validAbsoluteURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}

func (p Product) String() string { return string(p) }

func (h *Handler) hostAllowed(r *http.Request) bool {
	public, err := url.Parse(h.cfg.PublicURL)
	if err != nil || public.Host == "" || r.Host == "" {
		return false
	}
	return hmac.Equal([]byte(strings.ToLower(r.Host)), []byte(strings.ToLower(public.Host)))
}

func (h *Handler) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range h.cfg.Origins {
		if constantTimeEqual(origin, allowed) {
			return true
		}
	}
	return false
}

func constantTimeEqual(a, b string) bool {
	sumA := sha256.Sum256([]byte(a))
	sumB := sha256.Sum256([]byte(b))
	return hmac.Equal(sumA[:], sumB[:])
}

func encodeB64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
