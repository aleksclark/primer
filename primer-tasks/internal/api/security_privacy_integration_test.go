package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func studentCookieRequest(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "tasks_student", Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestPostgresSecuritySessionRestartAndTenantIsolation exercises the real
// PostgreSQL session predicates through two Server instances. The second
// instance is the restart boundary: no in-memory session or authorization
// state is allowed to make a revoked/expired credential valid again.
func TestPostgresSecuritySessionRestartAndTenantIsolation(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	ctx := t.Context()

	parentToken := "security-parent-restart-a"
	if _, err := pool.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,'parent-a','parent',now()+interval '10 minutes')`, hash(parentToken), tenantA); err != nil {
		t.Fatal(err)
	}
	first := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("security-session"), IssuerSecret: []byte("security-issuer")})
	second := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("security-session"), IssuerSecret: []byte("security-issuer")})
	if got := requestJSON(t, second.Routes(), http.MethodGet, "/students", parentToken, ""); got.Code != http.StatusOK || !contains(got.Body.String(), alice) || contains(got.Body.String(), bob) {
		t.Fatalf("restarted parent scope = (%d, %s)", got.Code, got.Body.String())
	}
	if got := requestJSON(t, second.Routes(), http.MethodGet, "/students/"+bob, parentToken, ""); got.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant object oracle = %d, want 404", got.Code)
	}

	// Revocation is checked on every request, including after a fresh Server.
	if _, err := pool.Exec(ctx, `UPDATE bff_sessions SET revoked_at=now() WHERE handle_hash=$1`, hash(parentToken)); err != nil {
		t.Fatal(err)
	}
	if got := requestJSON(t, first.Routes(), http.MethodGet, "/students", parentToken, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked parent on original server = %d, want 401", got.Code)
	}
	if got := requestJSON(t, second.Routes(), http.MethodGet, "/students", parentToken, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked parent after restart = %d, want 401", got.Code)
	}

	expiredParent := "security-parent-expired"
	if _, err := pool.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,'parent-a','parent',$3)`, hash(expiredParent), tenantA, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := requestJSON(t, second.Routes(), http.MethodGet, "/students", expiredParent, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("expired parent = %d, want 401", got.Code)
	}

	studentToken := "security-student-session"
	if _, err := pool.Exec(ctx, `INSERT INTO student_sessions(id,tenant_id,student_id,handle_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, uuid.New(), tenantA, alice, hash(studentToken)); err != nil {
		t.Fatal(err)
	}
	if got := studentCookieRequest(t, second.Routes(), "/student/profile", studentToken); got.Code != http.StatusOK || !contains(got.Body.String(), alice) || contains(got.Body.String(), bob) {
		t.Fatalf("student profile = (%d, %s)", got.Code, got.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE student_sessions SET revoked_at=now() WHERE handle_hash=$1`, hash(studentToken)); err != nil {
		t.Fatal(err)
	}
	if got := studentCookieRequest(t, second.Routes(), "/student/profile", studentToken); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked student session = %d, want 401", got.Code)
	}

	deviceToken := "security-device-session"
	if _, err := pool.Exec(ctx, `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, uuid.New(), tenantA, alice, hash(deviceToken)); err != nil {
		t.Fatal(err)
	}
	if got := requestBearer(t, second.Routes(), http.MethodGet, "/device/profile", deviceToken); got.Code != http.StatusOK || !contains(got.Body.String(), alice) {
		t.Fatalf("device profile = (%d, %s)", got.Code, got.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE student_devices SET revoked_at=now() WHERE token_hash=$1`, hash(deviceToken)); err != nil {
		t.Fatal(err)
	}
	if got := requestBearer(t, second.Routes(), http.MethodGet, "/device/profile", deviceToken); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked device = %d, want 401", got.Code)
	}
}

func contains(s, want string) bool { return len(want) > 0 && strings.Contains(s, want) }
