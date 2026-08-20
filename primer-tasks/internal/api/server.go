package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/url"
	"os"
	"primer-tasks/internal/artifactstore"
	"strings"
	"time"
)

type Server struct {
	DB           *pgxpool.Pool
	Env          string
	Artifacts    artifactstore.Store
	SecureCookie bool
	Auth         AuthConfig
	StartedAt    time.Time
	agentHub     *agentHub
}
type scope struct{ Tenant, Subject string }

type AuthConfig struct {
	Mode            string
	IssuerURL       string
	PublicIssuerURL string
	ClientID        string
	RedirectURL     string
	PublicOrigin    string
	SessionSecret   []byte
	IssuerSecret    []byte
}

func authConfigFromEnv(env string) AuthConfig {
	secret := os.Getenv("TASKS_SESSION_SECRET")
	if secret == "" {
		secret = "primer-tasks-development-secret-change-me"
	}
	return AuthConfig{
		Mode: envOr("TASKS_AUTH_MODE", "test"), IssuerURL: strings.TrimRight(envOr("TASKS_ISSUER_URL", "http://test-issuer:8091"), "/"), PublicIssuerURL: strings.TrimRight(envOr("TASKS_PUBLIC_ISSUER_URL", envOr("TASKS_ISSUER_URL", "http://test-issuer:8091")), "/"),
		ClientID:     envOr("TASKS_OIDC_CLIENT_ID", "primer-tasks-web"),
		RedirectURL:  envOr("TASKS_OIDC_REDIRECT_URL", envOr("TASKS_PUBLIC_ORIGIN", "http://127.0.0.1:8080")+"/auth/callback"),
		PublicOrigin: envOr("TASKS_PUBLIC_ORIGIN", "http://127.0.0.1:8080"), SessionSecret: []byte(secret),
		IssuerSecret: []byte(envOr("TASKS_TEST_ISSUER_SECRET", "primer-tasks-test-issuer-secret")),
	}
}

type Student struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"displayName"`
	CreatedAt   time.Time  `json:"createdAt"`
	ArchivedAt  *time.Time `json:"archivedAt,omitempty"`
}

func New(db *pgxpool.Pool, env string) *Server {
	root := envOr("TASKS_ARTIFACT_STORE_DIR", "/tmp/primer-tasks-artifacts")
	store, _ := artifactstore.NewFS(root)
	return NewWithStore(db, env, store)
}

func NewWithStore(db *pgxpool.Pool, env string, store artifactstore.Store) *Server {
	return &Server{DB: db, Env: env, Artifacts: store, SecureCookie: env == "production", Auth: authConfigFromEnv(env), StartedAt: time.Now().UTC(), agentHub: newAgentHub()}
}

func NewWithAuth(db *pgxpool.Pool, env string, auth AuthConfig) *Server {
	defaults := authConfigFromEnv(env)
	store, _ := artifactstore.NewFS(envOr("TASKS_ARTIFACT_STORE_DIR", "/tmp/primer-tasks-artifacts"))
	if len(auth.SessionSecret) == 0 {
		auth.SessionSecret = defaults.SessionSecret
	}
	if len(auth.IssuerSecret) == 0 {
		auth.IssuerSecret = defaults.IssuerSecret
	}
	if auth.Mode == "" {
		auth.Mode = defaults.Mode
	}
	return &Server{DB: db, Env: env, Artifacts: store, SecureCookie: env == "production", Auth: auth, StartedAt: time.Now().UTC(), agentHub: newAgentHub()}
}
func (s *Server) Routes() http.Handler { return s.humaAPI().Adapter() }

type parentHandler func(http.ResponseWriter, *http.Request, scope)

func (s *Server) requireParent(next parentHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sc, err := s.parentScope(r)
		if err != nil {
			problem(w, 401, "unauthorized", "parent session required")
			return
		}
		next(w, r, sc)
	}
}
func (s *Server) requireStudent(next func(http.ResponseWriter, *http.Request, uuid.UUID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.studentFromCookie(r)
		if err != nil {
			problem(w, 401, "revoked", "student session required")
			return
		}
		next(w, r, id)
	}
}
func (s *Server) requireDevice(next func(http.ResponseWriter, *http.Request, uuid.UUID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.studentFromBearer(r)
		if err != nil {
			problem(w, 401, "revoked", "device credential required")
			return
		}
		next(w, r, id)
	}
}
func hash(v string) []byte { x := sha256.Sum256([]byte(v)); return x[:] }
func pkceChallenge(verifier string) string {
	x := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(x[:])
}
func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Server) parentScope(r *http.Request) (scope, error) {
	c, err := r.Cookie("tasks_parent")
	if err != nil {
		return scope{}, err
	}
	var out scope
	err = s.DB.QueryRow(r.Context(), `SELECT tenant_id,subject_ref FROM bff_sessions WHERE handle_hash=$1 AND expires_at>now() AND revoked_at IS NULL`, hash(c.Value)).Scan(&out.Tenant, &out.Subject)
	return out, err
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.Auth.IssuerURL == "" || s.Auth.ClientID == "" {
		problem(w, 503, "identity_unconfigured", "identity provider is not configured")
		return
	}
	state, verifier := randomString(32), randomString(48)
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" || !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = "/parent/students"
	}
	redirectURI, publicIssuer := s.Auth.RedirectURL, s.Auth.PublicIssuerURL
	if s.Env != "production" {
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		// The test issuer is Compose-private. Send browser authorization through
		// the web BFF's /issuer proxy for every test-auth host, including a
		// Stacklane FQDN; only the API's back-channel token exchange uses the
		// Compose-DNS issuer address.
		if s.Auth.Mode == "test" || strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "localhost") {
			browserBase := "http://" + host
			redirectURI = browserBase + "/auth/callback"
			publicIssuer = browserBase + "/issuer"
		}
	}
	ciphertext, err := s.seal(verifier)
	if err != nil {
		problem(w, 500, "internal", "unable to create authorization state")
		return
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO auth_states(state_hash,verifier_ciphertext,redirect_uri,return_path,client_id,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '10 minutes')`, hash(state), ciphertext, redirectURI, returnTo, s.Auth.ClientID)
	if err != nil {
		problem(w, 500, "internal", "unable to persist authorization state")
		return
	}
	// The state cookie is only a browser binding. The verifier and expiry live in
	// Postgres, so a process restart cannot turn an authorization into a login.
	http.SetCookie(w, &http.Cookie{Name: "tasks_oauth_state", Value: state, Path: "/auth", HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	q := url.Values{"response_type": {"code"}, "client_id": {s.Auth.ClientID}, "redirect_uri": {redirectURI}, "scope": {"openid profile"}, "state": {state}, "code_challenge": {pkceChallenge(verifier)}, "code_challenge_method": {"S256"}}
	if p := r.URL.Query().Get("principal"); s.Auth.Mode == "test" && p != "" {
		q.Set("login_hint", p)
	}
	http.Redirect(w, r, strings.TrimRight(publicIssuer, "/")+"/oauth/authorize?"+q.Encode(), http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		problem(w, 401, "identity_denied", providerError)
		return
	}
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("tasks_oauth_state")
	if err != nil || state == "" || !hmac.Equal([]byte(state), []byte(cookie.Value)) {
		problem(w, 400, "invalid_state", "authorization state does not match")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		problem(w, 400, "invalid_request", "authorization code is required")
		return
	}
	ctx := r.Context()
	var ciphertext []byte
	var redirectURI, returnTo, clientID string
	if err = s.DB.QueryRow(ctx, `UPDATE auth_states SET consumed_at=now() WHERE state_hash=$1 AND consumed_at IS NULL AND expires_at>now() RETURNING verifier_ciphertext,redirect_uri,return_path,client_id`, hash(state)).Scan(&ciphertext, &redirectURI, &returnTo, &clientID); err != nil {
		problem(w, 400, "invalid_state", "authorization state is expired or already used")
		return
	}
	verifier, err := s.open(ciphertext)
	if err != nil {
		problem(w, 500, "internal", "authorization state is unreadable")
		return
	}
	claims, err := s.exchange(ctx, code, verifier, redirectURI, clientID)
	if err != nil {
		problem(w, 401, "identity_denied", "identity provider rejected the authorization")
		return
	}
	var tenant string
	if err = s.DB.QueryRow(ctx, `SELECT tenant_id FROM parent_memberships WHERE subject_ref=$1 ORDER BY tenant_id LIMIT 1`, claims.Subject).Scan(&tenant); err != nil {
		problem(w, 403, "denied", "principal is not provisioned")
		return
	}
	raw := randomString(32)
	if _, err = s.DB.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,$3,'parent',now()+interval '8 hours')`, hash(raw), tenant, claims.Subject); err != nil {
		problem(w, 500, "internal", "unable to create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "tasks_parent", Value: raw, Path: "/", HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 28800})
	http.SetCookie(w, &http.Cookie{Name: "tasks_oauth_state", Value: "", Path: "/auth", HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	to := returnTo
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") {
		to = "/parent/students"
	}
	http.Redirect(w, r, to, http.StatusFound)
}
func (s *Server) parentSession(w http.ResponseWriter, r *http.Request) {
	sc, err := s.parentScope(r)
	if err != nil {
		problem(w, 401, "unauthorized", "parent session required")
		return
	}
	s.csrfToken(w, r)
	jsonOK(w, map[string]string{"subjectRef": sc.Subject, "tenantId": sc.Tenant})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("tasks_parent"); e == nil {
		_, _ = s.DB.Exec(r.Context(), `UPDATE bff_sessions SET revoked_at=now() WHERE handle_hash=$1`, hash(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "tasks_parent", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	jsonOK(w, map[string]string{"status": "signed_out"})
}
func (s *Server) listStudents(w http.ResponseWriter, r *http.Request, sc scope) {
	q := r.URL.Query().Get("q")
	limit := 20
	offset := 0
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM students WHERE tenant_id=$1 AND archived_at IS NULL AND display_name ILIKE '%'||$2||'%'`, sc.Tenant, q).Scan(&total); err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,display_name,created_at,archived_at FROM students WHERE tenant_id=$1 AND archived_at IS NULL AND display_name ILIKE '%'||$2||'%' ORDER BY display_name LIMIT $3 OFFSET $4`, sc.Tenant, q, limit, offset)
	if err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	items := []Student{}
	for rows.Next() {
		var x Student
		if err := rows.Scan(&x.ID, &x.DisplayName, &x.CreatedAt, &x.ArchivedAt); err != nil {
			problem(w, 500, "internal", err.Error())
			return
		}
		items = append(items, x)
	}
	jsonOK(w, map[string]any{"items": items, "totalCount": total, "limit": limit, "offset": offset})
}
func (s *Server) createStudent(w http.ResponseWriter, r *http.Request, sc scope) {
	var in struct {
		DisplayName string `json:"displayName"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		problem(w, 400, "invalid_request", "display name is required")
		return
	}
	x := Student{ID: uuid.NewString(), DisplayName: strings.TrimSpace(in.DisplayName), CreatedAt: time.Now().UTC()}
	err := s.DB.QueryRow(r.Context(), `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,$3) RETURNING created_at`, x.ID, sc.Tenant, x.DisplayName).Scan(&x.CreatedAt)
	if err != nil {
		problem(w, 409, "conflict", "student name already exists or is invalid")
		return
	}
	audit(r.Context(), s.DB, sc, "student.created", x.ID)
	jsonStatus(w, x, 201)
}
func (s *Server) findStudent(ctx context.Context, tenant, id string) (Student, error) {
	var x Student
	err := s.DB.QueryRow(ctx, `SELECT id,display_name,created_at,archived_at FROM students WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&x.ID, &x.DisplayName, &x.CreatedAt, &x.ArchivedAt)
	return x, err
}
func (s *Server) getStudent(w http.ResponseWriter, r *http.Request, sc scope) {
	x, e := s.findStudent(r.Context(), sc.Tenant, chi.URLParam(r, "id"))
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "student not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	jsonOK(w, x)
}
func (s *Server) updateStudent(w http.ResponseWriter, r *http.Request, sc scope) {
	var in struct {
		DisplayName string `json:"displayName"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		problem(w, 400, "invalid_request", "display name is required")
		return
	}
	var x Student
	e := s.DB.QueryRow(r.Context(), `UPDATE students SET display_name=$1 WHERE tenant_id=$2 AND id=$3 AND archived_at IS NULL RETURNING id,display_name,created_at,archived_at`, strings.TrimSpace(in.DisplayName), sc.Tenant, chi.URLParam(r, "id")).Scan(&x.ID, &x.DisplayName, &x.CreatedAt, &x.ArchivedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "student not found")
		return
	}
	if e != nil {
		problem(w, 409, "conflict", "student cannot be updated")
		return
	}
	audit(r.Context(), s.DB, sc, "student.updated", x.ID)
	jsonOK(w, x)
}
func (s *Server) archiveStudent(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", "unable to begin archive")
		return
	}
	defer tx.Rollback(r.Context())
	var sid string
	e = tx.QueryRow(r.Context(), `UPDATE students SET archived_at=now() WHERE tenant_id=$1 AND id=$2 AND archived_at IS NULL RETURNING id`, sc.Tenant, id).Scan(&sid)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "student not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", "unable to archive student")
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE student_devices SET revoked_at=now() WHERE tenant_id=$1 AND student_id=$2 AND revoked_at IS NULL`, sc.Tenant, id); e != nil {
		problem(w, 500, "internal", "unable to revoke devices")
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE student_sessions SET revoked_at=now() WHERE tenant_id=$1 AND student_id=$2 AND revoked_at IS NULL`, sc.Tenant, id); e != nil {
		problem(w, 500, "internal", "unable to revoke sessions")
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE bff_sessions SET revoked_at=now() WHERE session_kind='student' AND subject_ref=$1 AND revoked_at IS NULL`, "student:"+id); e != nil {
		problem(w, 500, "internal", "unable to revoke browser sessions")
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE pairing_codes SET revoked_at=now() WHERE tenant_id=$1 AND student_id=$2 AND claimed_at IS NULL AND revoked_at IS NULL`, sc.Tenant, id); e != nil {
		problem(w, 500, "internal", "unable to revoke pairing codes")
		return
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id,metadata) VALUES($1,$2,'student.archived',$3,$4)`, sc.Tenant, sc.Subject, sid, `{"revoked":"devices,sessions,pairing"}`); e != nil {
		problem(w, 500, "internal", "unable to record archive")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", "unable to commit archive")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) issuePairing(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	if found, e := s.findStudent(r.Context(), sc.Tenant, id); e != nil || found.ArchivedAt != nil {
		problem(w, 404, "not_found", "student not found")
		return
	}
	code := strings.ToUpper(hex.EncodeToString([]byte(randomString(12)))[:10])
	pid := uuid.New()
	exp := time.Now().UTC().Add(5 * time.Minute)
	_, e := s.DB.Exec(r.Context(), `INSERT INTO pairing_codes(id,tenant_id,student_id,code_hash,expires_at) VALUES($1,$2,$3,$4,$5)`, pid, sc.Tenant, id, hash(code), exp)
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	audit(r.Context(), s.DB, sc, "student.pairing_issued", pid.String())
	// The pairing payload must name the server's configured public origin, not
	// the browser BFF origin that happened to request it. Android validates the
	// QR origin before it will use the device pairing API.
	origin := s.Auth.PublicOrigin
	if origin == "" {
		origin = r.Header.Get("Origin")
	}
	payload := map[string]any{"v": 1, "api": "/api", "origin": origin, "pairingId": pid.String(), "code": code, "exp": exp.Format(time.RFC3339)}
	jsonOK(w, map[string]any{"pairingId": pid, "code": code, "expiresAt": exp, "qrPayload": string(mustJSON(payload))})
}
func (s *Server) claimCredential(ctx context.Context, code, kind string) (uuid.UUID, string, string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", "", err
	}
	defer tx.Rollback(ctx)
	var sid, pairingID uuid.UUID
	var tenant string
	if err = tx.QueryRow(ctx, `UPDATE pairing_codes SET claimed_at=now() WHERE code_hash=$1 AND claimed_at IS NULL AND revoked_at IS NULL AND expires_at>now() RETURNING id,tenant_id,student_id`, hash(strings.ToUpper(strings.TrimSpace(code)))).Scan(&pairingID, &tenant, &sid); err != nil {
		return uuid.Nil, "", "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id,metadata) VALUES($1,$2,'student.pairing_claimed',$3,$4)`, tenant, "student:"+sid.String(), pairingID, `{"kind":"`+kind+`"}`); err != nil {
		return uuid.Nil, "", "", err
	}
	if kind == "browser" {
		raw := randomString(32)
		if _, err = tx.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,$3,'student',now()+interval '90 days')`, hash(raw), tenant, "student:"+sid.String()); err != nil {
			return uuid.Nil, "", "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO student_sessions(id,tenant_id,student_id,handle_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '90 days')`, uuid.New(), tenant, sid, hash(raw)); err != nil {
			return uuid.Nil, "", "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id,metadata) VALUES($1,$2,'student.session_issued',$3,$4)`, tenant, "student:"+sid.String(), sid, `{"kind":"browser"}`); err != nil {
			return uuid.Nil, "", "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return uuid.Nil, "", "", err
		}
		return sid, tenant, raw, nil
	}
	raw := randomString(48)
	deviceID := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, deviceID, tenant, sid, hash(raw)); err != nil {
		return uuid.Nil, "", "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id,metadata) VALUES($1,$2,'student.device_issued',$3,$4)`, tenant, "student:"+sid.String(), deviceID, `{"kind":"device"}`); err != nil {
		return uuid.Nil, "", "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, "", "", err
	}
	return sid, tenant, raw, nil
}
func (s *Server) pairBrowser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	sid, _, raw, e := s.claimCredential(r.Context(), in.Code, "browser")
	if e != nil {
		problem(w, 410, "expired", "pairing code is used, expired, or revoked")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "tasks_student", Value: raw, Path: "/", HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 7776000})
	// Browser-paired students use the same origin/CSRF-protected WebSocket
	// boundary as parent sessions. Issue its readable double-submit token at
	// pairing time, before the dialogue page makes its first socket request.
	s.csrfToken(w, r)
	jsonOK(w, map[string]string{"studentId": sid.String()})
}
func (s *Server) studentFromCookie(r *http.Request) (uuid.UUID, error) {
	c, e := r.Cookie("tasks_student")
	if e != nil {
		return uuid.Nil, e
	}
	var ref string
	e = s.DB.QueryRow(r.Context(), `SELECT 'student:'||student_id::text FROM student_sessions WHERE handle_hash=$1 AND expires_at>now() AND revoked_at IS NULL`, hash(c.Value)).Scan(&ref)
	if e != nil {
		return uuid.Nil, e
	}
	return uuid.Parse(strings.TrimPrefix(ref, "student:"))
}
func (s *Server) studentProfile(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var x Student
	e := s.DB.QueryRow(r.Context(), `SELECT id,display_name,created_at,archived_at FROM students WHERE id=$1 AND archived_at IS NULL`, id).Scan(&x.ID, &x.DisplayName, &x.CreatedAt, &x.ArchivedAt)
	if e != nil {
		problem(w, 401, "revoked", "student is unavailable")
		return
	}
	jsonOK(w, x)
}
func (s *Server) checklist(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	jsonOK(w, map[string]any{"items": []any{}})
}
func (s *Server) devicePair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	sid, _, tok, e := s.claimCredential(r.Context(), in.Code, "device")
	if e != nil {
		problem(w, 410, "expired", "pairing code is used, expired, or revoked")
		return
	}
	jsonOK(w, map[string]string{"token": tok, "studentId": sid.String()})
}
func (s *Server) studentFromBearer(r *http.Request) (uuid.UUID, error) {
	v := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if v == "" {
		return uuid.Nil, fmt.Errorf("missing")
	}
	var id uuid.UUID
	e := s.DB.QueryRow(r.Context(), `SELECT student_id FROM student_devices WHERE token_hash=$1 AND revoked_at IS NULL`, hash(v)).Scan(&id)
	return id, e
}
func (s *Server) deviceProfile(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.studentProfile(w, r, id)
}
func audit(ctx context.Context, db *pgxpool.Pool, sc scope, action, id string) {
	_, _ = db.Exec(ctx, `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id) VALUES($1,$2,$3,$4)`, sc.Tenant, sc.Subject, action, id)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); e != nil {
		problem(w, 400, "invalid_request", "invalid JSON")
		return false
	}
	return true
}
func mustJSON(v any) []byte               { b, _ := json.Marshal(v); return b }
func jsonOK(w http.ResponseWriter, v any) { jsonStatus(w, v, 200) }
func jsonStatus(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code, message string) {
	jsonStatus(w, map[string]any{"code": code, "message": message, "detail": message}, status)
}
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
