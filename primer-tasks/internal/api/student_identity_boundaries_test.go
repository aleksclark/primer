package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestStudentIdentityRevocationAndBrowserHandshakeBoundaries(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student := uuid.NewString(), uuid.NewString()
	device, token := uuid.NewString(), "identity-boundary-device"
	handle := "identity-boundary-session"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'Identity boundaries')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Boundary student')`, student, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, device, tenant, student, hash(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO student_sessions(id,tenant_id,student_id,handle_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, uuid.NewString(), tenant, student, hash(handle)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM student_sessions WHERE tenant_id=$1; DELETE FROM student_devices WHERE tenant_id=$1; DELETE FROM students WHERE tenant_id=$1; DELETE FROM tenants WHERE id=$1`, tenant)
	})

	s := &Server{DB: pool}
	if _, err := s.studentIdentityFromRequest(httptest.NewRequest(http.MethodGet, "/student/ws", nil)); err == nil {
		t.Fatal("request without credentials accepted")
	}
	basic := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
	basic.Header.Set("Authorization", "Basic not-a-bearer")
	if _, err := s.studentIdentityFromRequest(basic); err == nil {
		t.Fatal("non-bearer authorization accepted")
	}
	validCookie := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
	validCookie.AddCookie(&http.Cookie{Name: "tasks_student", Value: handle})
	identity, err := s.studentIdentityFromRequest(validCookie)
	if err != nil || identity.Source != "cookie" || identity.StudentID.String() != student {
		t.Fatalf("cookie identity=%+v err=%v", identity, err)
	}
	if err := s.verifyStudentIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if err := s.verifyStudentIdentity(ctx, studentIdentity{Source: "other"}); err == nil {
		t.Fatal("unknown identity source accepted")
	}
	if err := s.verifyStudentIdentity(ctx, studentIdentity{Source: "cookie", Token: handle, StudentID: uuid.New(), TenantID: tenant}); err == nil {
		t.Fatal("identity mismatch accepted")
	}

	// Cookie upgrades require both a same-origin request and the negotiated
	// CSRF subprotocol. These are HTTP handshake failures, before websocket
	// state or any dialogue row is touched.
	for name, tc := range map[string]struct {
		origin string
		csrf   string
	}{
		"wrong origin": {origin: "https://evil.example", csrf: "cookie-csrf"},
		"missing csrf": {origin: "https://student.example", csrf: ""},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
			req.Host = "student.example"
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(&http.Cookie{Name: "tasks_student", Value: handle})
			if tc.csrf != "" {
				req.Header.Set("Sec-WebSocket-Protocol", "primer-tasks.student.v1, primer-tasks.v1.csrf-"+tc.csrf)
			}
			rec := httptest.NewRecorder()
			s.studentWS(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("handshake status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}

	if _, err := pool.Exec(ctx, `UPDATE student_sessions SET expires_at=now()-interval '1 second' WHERE handle_hash=$1`, hash(handle)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.studentIdentityFromCookie(ctx, handle); err == nil {
		t.Fatal("expired session accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE student_sessions SET expires_at=now()+interval '1 hour' WHERE handle_hash=$1`, hash(handle)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE students SET archived_at=now() WHERE id=$1`, student); err != nil {
		t.Fatal(err)
	}
	if _, err := s.studentIdentityFromCookie(ctx, handle); err == nil {
		t.Fatal("archived student session accepted")
	}
	if err := s.verifyStudentIdentity(ctx, studentIdentity{Source: "bearer", Token: token, StudentID: uuid.MustParse(student), TenantID: tenant}); err == nil {
		t.Fatal("archived bearer accepted")
	}
}
