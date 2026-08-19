package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestStudentDialogueWSBearerReplayAndRevocationBoundary(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, device, token := uuid.NewString(), uuid.NewString(), uuid.NewString(), "phase4-device-token"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'Student WS')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Terra WS')`, student, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, device, tenant, student, hash(token)); err != nil {
		t.Fatal(err)
	}
	handle := "phase4-student-session"
	if _, err := pool.Exec(ctx, `INSERT INTO student_sessions(id,tenant_id,student_id,handle_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, uuid.NewString(), tenant, student, hash(handle)); err != nil {
		t.Fatal(err)
	}
	serverForIdentity := &Server{DB: pool}
	sessionIdentity, err := serverForIdentity.studentIdentityFromCookie(ctx, handle)
	if err != nil || sessionIdentity.StudentID.String() != student {
		t.Fatalf("cookie identity=%+v err=%v", sessionIdentity, err)
	}
	cookieReq := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
	cookieReq.AddCookie(&http.Cookie{Name: "tasks_student", Value: handle})
	if got, err := serverForIdentity.studentIdentityFromRequest(cookieReq); err != nil || got.StudentID.String() != student {
		t.Fatalf("cookie request identity=%+v err=%v", got, err)
	}
	if err := serverForIdentity.verifyStudentIdentity(ctx, sessionIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := serverForIdentity.studentIdentityFromCookie(ctx, "missing"); err == nil {
		t.Fatal("missing cookie session accepted")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM student_sessions WHERE tenant_id=$1; DELETE FROM student_devices WHERE tenant_id=$1; DELETE FROM students WHERE tenant_id=$1; DELETE FROM tenants WHERE id=$1`, tenant)
	})
	s := New(pool, "test")
	httpServer := httptest.NewServer(s.Routes())
	t.Cleanup(httpServer.Close)
	header := http.Header{"Authorization": {"Bearer " + token}}
	conn, _, err := websocket.Dial(ctx, "ws"+httpServer.URL[len("http"):]+"/student/ws?token=forbidden", &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{"primer-tasks.student.v1"}})
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "unexpected")
		t.Fatal("query-string credential was accepted")
	}
	conn, _, err = websocket.Dial(ctx, "ws"+httpServer.URL[len("http"):]+"/student/ws", &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{"primer-tasks.student.v1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "done") })
	var hello wireStudentEvent
	if err := wsjson.Read(ctx, conn, &hello); err != nil || hello.Type != "hello" || hello.ProtocolVersion != studentProtocolVersion {
		t.Fatalf("hello=%+v err=%v", hello, err)
	}
	if err := wsjson.Write(ctx, conn, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	var notFound wireStudentEvent
	if err := wsjson.Read(ctx, conn, &notFound); err != nil || notFound.Type != "error" || notFound.Code != "not_found" {
		t.Fatalf("not-found=%+v err=%v", notFound, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE student_devices SET revoked_at=now() WHERE id=$1`, device); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, conn, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	var revoked wireStudentEvent
	if err := wsjson.Read(ctx, conn, &revoked); err != nil || revoked.Type != "error" || revoked.Code != "revoked" {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}

	// Browser sessions take the cookie/CSRF branch rather than the bearer
	// branch. Exercise protocol rejection, an unknown command, replay-safe
	// subscription failure, and revocation while the socket is still open.
	cookieHeader := http.Header{
		"Cookie": {"tasks_student=" + handle + "; tasks_csrf=cookie-csrf"},
		"Origin": {httpServer.URL},
	}
	cookieConn, _, err := websocket.Dial(ctx, "ws"+httpServer.URL[len("http"):]+"/student/ws", &websocket.DialOptions{
		HTTPHeader:   cookieHeader,
		Subprotocols: []string{"primer-tasks.student.v1", "primer-tasks.v1.csrf.cookie-csrf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cookieConn.Close(websocket.StatusNormalClosure, "done") })
	var cookieHello wireStudentEvent
	if err := wsjson.Read(ctx, cookieConn, &cookieHello); err != nil || cookieHello.Type != "hello" {
		t.Fatalf("cookie hello=%+v err=%v", cookieHello, err)
	}
	if err := wsjson.Write(ctx, cookieConn, studentCommand{Type: "hello", ProtocolVersion: studentProtocolVersion + 1}); err != nil {
		t.Fatal(err)
	}
	var protocolError wireStudentEvent
	if err := wsjson.Read(ctx, cookieConn, &protocolError); err != nil || protocolError.Code != "protocol_version" {
		t.Fatalf("protocol error=%+v err=%v", protocolError, err)
	}
	if err := wsjson.Write(ctx, cookieConn, studentCommand{Type: "unknown", ProtocolVersion: studentProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	var unknownError wireStudentEvent
	if err := wsjson.Read(ctx, cookieConn, &unknownError); err != nil || unknownError.Code != "unknown_command" {
		t.Fatalf("unknown error=%+v err=%v", unknownError, err)
	}
	if err := wsjson.Write(ctx, cookieConn, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	var cookieNotFound wireStudentEvent
	if err := wsjson.Read(ctx, cookieConn, &cookieNotFound); err != nil || cookieNotFound.Code != "not_found" {
		t.Fatalf("cookie not-found=%+v err=%v", cookieNotFound, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE student_sessions SET revoked_at=now() WHERE handle_hash=$1`, hash(handle)); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, cookieConn, studentCommand{Type: "ack", ProtocolVersion: studentProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	var cookieRevoked wireStudentEvent
	if err := wsjson.Read(ctx, cookieConn, &cookieRevoked); err != nil || cookieRevoked.Code != "revoked" {
		t.Fatalf("cookie revoked=%+v err=%v", cookieRevoked, err)
	}
}
