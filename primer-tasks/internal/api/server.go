package api

import (
	"context"
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
	"os"
	"strings"
	"time"
)

type Server struct {
	DB           *pgxpool.Pool
	Env          string
	SecureCookie bool
}
type scope struct{ Tenant, Subject string }
type student struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"displayName"`
	CreatedAt   time.Time  `json:"createdAt"`
	ArchivedAt  *time.Time `json:"archivedAt,omitempty"`
}

func New(db *pgxpool.Pool, env string) *Server {
	return &Server{DB: db, Env: env, SecureCookie: env == "production"}
}
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { jsonOK(w, map[string]string{"status": "ok"}) })
	r.Get("/openapi.yaml", s.openapi)
	r.Get("/auth/login", s.login)
	r.Get("/auth/session", s.parentSession)
	r.Post("/auth/logout", s.logout)
	r.Get("/students", s.requireParent(s.listStudents))
	r.Post("/students", s.requireParent(s.createStudent))
	r.Get("/students/{id}", s.requireParent(s.getStudent))
	r.Patch("/students/{id}", s.requireParent(s.updateStudent))
	r.Delete("/students/{id}", s.requireParent(s.archiveStudent))
	r.Post("/students/{id}/pairing", s.requireParent(s.issuePairing))
	r.Post("/student/pair", s.pairBrowser)
	r.Get("/student/profile", s.requireStudent(s.studentProfile))
	r.Get("/student/checklist", s.requireStudent(s.checklist))
	r.Post("/device/pair", s.devicePair)
	r.Get("/device/profile", s.requireDevice(s.deviceProfile))
	return r
}

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
	if s.Env == "production" {
		problem(w, 503, "identity_unconfigured", "production requires Primer Identity")
		return
	}
	sub := r.URL.Query().Get("principal")
	if sub == "" {
		sub = "parent-a"
	}
	var tenant string
	err := s.DB.QueryRow(r.Context(), `SELECT tenant_id FROM parent_memberships WHERE subject_ref=$1 ORDER BY tenant_id LIMIT 1`, sub).Scan(&tenant)
	if err != nil {
		problem(w, 403, "denied", "principal is not provisioned")
		return
	}
	raw := randomString(32)
	_, err = s.DB.Exec(r.Context(), `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,expires_at) VALUES($1,$2,$3,now()+interval '8 hours')`, hash(raw), tenant, sub)
	if err != nil {
		problem(w, 500, "internal", err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "tasks_parent", Value: raw, Path: "/", HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 28800})
	http.Redirect(w, r, "/parent/students", http.StatusFound)
}
func (s *Server) parentSession(w http.ResponseWriter, r *http.Request) {
	sc, err := s.parentScope(r)
	if err != nil {
		problem(w, 401, "unauthorized", "parent session required")
		return
	}
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
	items := []student{}
	for rows.Next() {
		var x student
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
	if !decode(w, r, &in) || strings.TrimSpace(in.DisplayName) == "" {
		return
	}
	x := student{ID: uuid.NewString(), DisplayName: strings.TrimSpace(in.DisplayName), CreatedAt: time.Now().UTC()}
	err := s.DB.QueryRow(r.Context(), `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,$3) RETURNING created_at`, x.ID, sc.Tenant, x.DisplayName).Scan(&x.CreatedAt)
	if err != nil {
		problem(w, 409, "conflict", "student name already exists or is invalid")
		return
	}
	audit(r.Context(), s.DB, sc, "student.created", x.ID)
	jsonStatus(w, x, 201)
}
func (s *Server) findStudent(ctx context.Context, tenant, id string) (student, error) {
	var x student
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
	if !decode(w, r, &in) || strings.TrimSpace(in.DisplayName) == "" {
		return
	}
	var x student
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
	var sid string
	e := s.DB.QueryRow(r.Context(), `UPDATE students SET archived_at=now() WHERE tenant_id=$1 AND id=$2 AND archived_at IS NULL RETURNING id`, sc.Tenant, id).Scan(&sid)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "student not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	_, _ = s.DB.Exec(r.Context(), `UPDATE student_devices SET revoked_at=now() WHERE tenant_id=$1 AND student_id=$2 AND revoked_at IS NULL`, sc.Tenant, id)
	audit(r.Context(), s.DB, sc, "student.archived", sid)
	w.WriteHeader(204)
}
func (s *Server) issuePairing(w http.ResponseWriter, r *http.Request, sc scope) {
	id := chi.URLParam(r, "id")
	if _, e := s.findStudent(r.Context(), sc.Tenant, id); e != nil {
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
	payload := map[string]any{"v": 1, "api": "/api", "origin": "/student/pair", "pairingId": pid.String(), "code": code, "exp": exp.Format(time.RFC3339)}
	jsonOK(w, map[string]any{"pairingId": pid, "code": code, "expiresAt": exp, "qrPayload": string(mustJSON(payload))})
}
func (s *Server) claim(ctx context.Context, code string) (uuid.UUID, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return uuid.Nil, e
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	e = tx.QueryRow(ctx, `UPDATE pairing_codes SET claimed_at=now() WHERE code_hash=$1 AND claimed_at IS NULL AND revoked_at IS NULL AND expires_at>now() RETURNING student_id`, hash(strings.ToUpper(strings.TrimSpace(code)))).Scan(&id)
	if e != nil {
		return uuid.Nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return uuid.Nil, e
	}
	return id, nil
}
func (s *Server) pairBrowser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	sid, e := s.claim(r.Context(), in.Code)
	if e != nil {
		problem(w, 410, "expired", "pairing code is used, expired, or revoked")
		return
	}
	raw := randomString(32)
	var tenant string
	if e = s.DB.QueryRow(r.Context(), `SELECT tenant_id FROM students WHERE id=$1`, sid).Scan(&tenant); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	_, e = s.DB.Exec(r.Context(), `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,expires_at) VALUES($1,$2,$3,now()+interval '90 days')`, hash(raw), tenant, "student:"+sid.String())
	if e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "tasks_student", Value: raw, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 7776000})
	jsonOK(w, map[string]string{"studentId": sid.String()})
}
func (s *Server) studentFromCookie(r *http.Request) (uuid.UUID, error) {
	c, e := r.Cookie("tasks_student")
	if e != nil {
		return uuid.Nil, e
	}
	var ref string
	e = s.DB.QueryRow(r.Context(), `SELECT subject_ref FROM bff_sessions WHERE handle_hash=$1 AND expires_at>now() AND revoked_at IS NULL AND subject_ref LIKE 'student:%'`, hash(c.Value)).Scan(&ref)
	if e != nil {
		return uuid.Nil, e
	}
	return uuid.Parse(strings.TrimPrefix(ref, "student:"))
}
func (s *Server) studentProfile(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var x student
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
	sid, e := s.claim(r.Context(), in.Code)
	if e != nil {
		problem(w, 410, "expired", "pairing code is used, expired, or revoked")
		return
	}
	var tenant string
	if e = s.DB.QueryRow(r.Context(), `SELECT tenant_id FROM students WHERE id=$1`, sid).Scan(&tenant); e != nil {
		problem(w, 500, "internal", e.Error())
		return
	}
	tok := randomString(48)
	_, e = s.DB.Exec(r.Context(), `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, uuid.New(), tenant, sid, hash(tok))
	if e != nil {
		problem(w, 500, "internal", e.Error())
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
func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write([]byte(openapiYAML))
}
func OpenAPI() string { return openapiYAML }

const openapiYAML = `openapi: 3.0.3
info:
  title: Primer Tasks
  version: 1.0.0
paths:
  /auth/session: {get: {responses: {'200': {description: session, content: {application/json: {schema: {type: object, properties: {subjectRef: {type: string}, tenantId: {type: string}}}}}}}}}
  /auth/logout: {post: {responses: {'200': {description: signed out, content: {application/json: {schema: {type: object, properties: {status: {type: string}}}}}}}}}
  /students:
    get: {parameters: [{name: q, in: query, schema: {type: string}}, {name: limit, in: query, schema: {type: integer}}, {name: offset, in: query, schema: {type: integer}}], responses: {'200': {description: student page, content: {application/json: {schema: {$ref: '#/components/schemas/StudentPage'}}}}}
    post: {requestBody: {required: true, content: {application/json: {schema: {$ref: '#/components/schemas/CreateStudent'}}}}, responses: {'201': {description: student, content: {application/json: {schema: {$ref: '#/components/schemas/Student'}}}}}}
  /students/{id}: {parameters: [{name: id, in: path, required: true, schema: {type: string, format: uuid}}], get: {responses: {'200': {description: student, content: {application/json: {schema: {$ref: '#/components/schemas/Student'}}}}}, patch: {requestBody: {content: {application/json: {schema: {$ref: '#/components/schemas/UpdateStudent'}}}}, responses: {'200': {description: student, content: {application/json: {schema: {$ref: '#/components/schemas/Student'}}}}}, delete: {responses: {'204': {description: archived}}}}
  /students/{id}/pairing: {parameters: [{name: id, in: path, required: true, schema: {type: string, format: uuid}}], post: {responses: {'200': {description: pairing, content: {application/json: {schema: {$ref: '#/components/schemas/Pairing'}}}}}}
  /student/pair: {post: {requestBody: {content: {application/json: {schema: {$ref: '#/components/schemas/Pair'}}}}, responses: {'200': {description: paired, content: {application/json: {schema: {type: object, properties: {studentId: {type: string}}}}}}}}
  /student/profile: {get: {responses: {'200': {description: profile, content: {application/json: {schema: {$ref: '#/components/schemas/Student'}}}}}}}
  /student/checklist: {get: {responses: {'200': {description: checklist, content: {application/json: {schema: {$ref: '#/components/schemas/Checklist'}}}}}}}
components:
  schemas:
    Student: {type: object, required: [id, displayName, createdAt], properties: {id: {type: string}, displayName: {type: string}, createdAt: {type: string, format: date-time}, archivedAt: {type: string, format: date-time, nullable: true}}}
    StudentPage: {type: object, required: [items, totalCount, limit, offset], properties: {items: {type: array, items: {$ref: '#/components/schemas/Student'}}, totalCount: {type: integer}, limit: {type: integer}, offset: {type: integer}}}
    Pairing: {type: object, required: [pairingId, code, expiresAt, qrPayload], properties: {pairingId: {type: string}, code: {type: string}, expiresAt: {type: string, format: date-time}, qrPayload: {type: string}}}
    Checklist: {type: object, required: [items], properties: {items: {type: array, items: {type: object, properties: {id: {type: string}, title: {type: string}, description: {type: string}, status: {type: string}}}}}}
    UpdateStudent: {type: object, required: [displayName], properties: {displayName: {type: string}}}
    CreateStudent: {type: object, required: [displayName], properties: {displayName: {type: string}}}
    Pair: {type: object, required: [code], properties: {code: {type: string}}}
`

func init() { _ = os.Getenv("TASKS_ENV") }
