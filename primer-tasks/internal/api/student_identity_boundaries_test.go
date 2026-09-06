package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5/pgxpool"
)

func studentCookieHeader(h *publicDialogueHarness, origin string) http.Header {
	u, _ := neturl.Parse(origin)
	r := &http.Request{Header: http.Header{"Origin": {origin}}}
	for _, cookie := range h.student.Jar.Cookies(u) {
		r.AddCookie(cookie)
	}
	return r.Header
}
func expectStudentClose(t *testing.T, conn *websocket.Conn, forbiddenKind string, want websocket.StatusCode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	for {
		var event wireStudentEvent
		err := wsjson.Read(ctx, conn, &event)
		if err != nil {
			if websocket.CloseStatus(err) != want {
				t.Fatalf("student close status %d, want %d", websocket.CloseStatus(err), want)
			}
			return
		}
		if forbiddenKind != "" && event.Kind == forbiddenKind {
			t.Fatal("revoked student received protected frame")
		}
	}
}
func publicArchiveAsync(h *publicDialogueHarness) <-chan error {
	result := make(chan error, 1)
	go func() {
		request, err := http.NewRequest(http.MethodDelete, h.base+"/students/"+h.studentID, nil)
		if err != nil {
			result <- err
			return
		}
		response, err := h.parent.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != 204 {
				err = fmt.Errorf("public archive status %d", response.StatusCode)
			}
		}
		result <- err
	}()
	return result
}
func awaitStudentLock(t *testing.T, pool *pgxpool.Pool, ctx context.Context, queryFragment string) {
	t.Helper()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0 AND query LIKE '%'||$1||'%')`, queryFragment).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("expected actual PostgreSQL authority lock waiter was not observed")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestPublicStudentDialogueCookieOnlyAndCrossStudentScope(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	normal := h.socket(0)
	q := h.question(normal, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(h.base, "http") + "/student/ws"
	for _, tc := range []struct {
		name, path string
		header     http.Header
		protocols  []string
		status     int
	}{
		{"no cookie", url, http.Header{"Origin": {h.base}}, []string{"primer-tasks.student.v1"}, 403},
		{"foreign origin", url, studentCookieHeader(h, "http://evil.invalid"), []string{"primer-tasks.student.v1", "primer-tasks.v1.csrf." + h.csrf()}, 403},
		{"missing csrf", url, studentCookieHeader(h, h.base), []string{"primer-tasks.student.v1"}, 403},
		{"query credentials", url + "?token=not-a-credential", studentCookieHeader(h, h.base), []string{"primer-tasks.student.v1", "primer-tasks.v1.csrf." + h.csrf()}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, response, err := websocket.Dial(ctx, tc.path, &websocket.DialOptions{HTTPHeader: tc.header, Subprotocols: tc.protocols})
			if err == nil {
				conn.CloseNow()
				t.Fatal("invalid student upgrade accepted")
			}
			if response == nil || response.StatusCode != tc.status {
				t.Fatal("invalid upgrade did not fail at expected public boundary")
			}
			response.Body.Close()
		})
	}
	header := studentCookieHeader(h, h.base)
	header.Set("Authorization", "Bearer opaque-device-credential")
	conn, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{"primer-tasks.student.v1", "primer-tasks.v1.csrf." + h.csrf()}})
	if err == nil {
		conn.CloseNow()
		t.Fatal("bearer fallback admitted to browser socket")
	}
	if response == nil || response.StatusCode != 401 {
		t.Fatal("bearer was not refused")
	}
	response.Body.Close()
	// A second student in this SAME household pairs through the public API.
	var second Student
	h.request(h.parent, "POST", "/students", map[string]string{"displayName": "Second public student"}, 201, &second)
	jar, _ := cookiejar.New(nil)
	other := *h
	other.student = &http.Client{Jar: jar, Timeout: 5 * time.Second}
	var pairing struct {
		Code string `json:"code"`
	}
	h.request(h.parent, "POST", "/students/"+second.ID+"/pairing", nil, 200, &pairing)
	other.request(other.student, "POST", "/student/pair", map[string]string{"code": pairing.Code}, 200, nil)
	foreign := other.socket(0)
	other.wait(foreign, func(e wireStudentEvent) bool {
		if e.Kind == "question" || e.Kind == "message_ack" || e.Kind == "state" {
			t.Fatal("cross-student transcript leaked")
		}
		return e.Kind == "error" && e.Code == "not_found"
	})
	h.answer(normal, q, "own-first", "The family repaired the garden wall after the storm.")
	h.question(normal, 1)
	// An unauthorized/empty subscription receives no tenant-wide broadcasts.
	if err = wsjson.Write(ctx, foreign, studentCommand{Protocol: 1, Kind: "user_message", OccurrenceID: h.occurrence, AttemptID: h.attempt, QuestionID: q.QuestionID, ExpectedVersion: q.Version, PolicyVersion: q.PolicyVersion, SnapshotDigest: q.SnapshotDigest, ClientMessageID: "foreign", Text: "The family repaired the garden wall."}); err != nil {
		t.Fatal(err)
	}
	other.wait(foreign, func(e wireStudentEvent) bool {
		if e.Kind != "error" {
			t.Fatal("empty subscription received private broadcast")
		}
		return e.Code == "not_found"
	})
	if messages, _, _, _, _ := h.counts(); messages != 1 {
		t.Fatal("foreign student submitted an answer")
	}
	parentJar, _ := cookiejar.New(nil)
	origin, _ := neturl.Parse(h.base)
	parentJar.SetCookies(origin, []*http.Cookie{{Name: "tasks_parent", Value: "parent-b", Path: "/"}})
	parentB := &http.Client{Jar: parentJar, Timeout: 5 * time.Second}
	var students struct {
		Items []Student `json:"items"`
	}
	h.request(parentB, "GET", "/students", nil, 200, &students)
	if len(students.Items) != 1 {
		t.Fatal("foreign household public prerequisite missing")
	}
	var foreignPairing struct {
		Code string `json:"code"`
	}
	h.request(parentB, "POST", "/students/"+students.Items[0].ID+"/pairing", nil, 200, &foreignPairing)
	tenantJar, _ := cookiejar.New(nil)
	tenantPeer := *h
	tenantPeer.student = &http.Client{Jar: tenantJar, Timeout: 5 * time.Second}
	tenantPeer.request(tenantPeer.student, "POST", "/student/pair", map[string]string{"code": foreignPairing.Code}, 200, nil)
	tenantSocket := tenantPeer.socket(0)
	tenantPeer.wait(tenantSocket, func(e wireStudentEvent) bool {
		if e.Kind != "error" {
			t.Fatal("cross-tenant transcript leaked")
		}
		return e.Code == "not_found"
	})
}

func TestPublicStudentDialogueIdleRevocationClosesWithoutCommand(t *testing.T) {
	h := newPublicDialogueHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.base, "http")+"/student/ws", &websocket.DialOptions{HTTPHeader: studentCookieHeader(h, h.base), Subprotocols: []string{"primer-tasks.student.v1", "primer-tasks.v1.csrf." + h.csrf()}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var hello wireStudentEvent
	if err = wsjson.Read(ctx, conn, &hello); err != nil || hello.Kind != "hello" {
		t.Fatal("idle student hello missing")
	}
	h.request(h.parent, "DELETE", "/students/"+h.studentID, nil, 204, nil)
	expectStudentClose(t, conn, "question", websocket.StatusPolicyViolation)
}

// The P3 transport gate wraps REAL TCP below the production router. It pauses
// an actual frame; it does not inject an event, run, decision or fake handler.
func gatedStudentConsumer(t *testing.T, h *publicDialogueHarness, kind string, pause bool) (*publicDialogueHarness, *privateFrameGate) {
	t.Helper()
	server := New(h.pool, "test")
	gate := newPrivateFrameGate(`"kind":"`+kind+`"`, pause)
	httpServer := httptest.NewUnstartedServer(server.Routes())
	httpServer.Listener = &privateGateListener{Listener: httpServer.Listener, gate: gate}
	httpServer.Start()
	server.Auth.PublicOrigin = httpServer.URL
	t.Cleanup(httpServer.Close)
	consumer := *h
	consumer.base = httpServer.URL
	consumer.process = nil
	return &consumer, gate
}

func TestPublicStudentDialogueFrameAndRevocationWinnerOrdering(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(fmt.Sprintf("delivery-first-live-%t", live), func(t *testing.T) {
			h := newPublicDialogueHarness(t)
			h.begin()
			producer := h.socket(0)
			q := h.wait(producer, func(e wireStudentEvent) bool { return e.Kind == "question" && e.AcceptedCount == 0 })
			kind := "question"
			cursor := int64(0)
			if live {
				kind = "answer_evaluation"
				cursor = q.Cursor
			}
			consumer, gate := gatedStudentConsumer(t, h, kind, true)
			gate.armed.Store(true)
			conn := consumer.socket(cursor)
			if live {
				h.answer(producer, q, "live-before-revoke", "The family repaired the garden wall after the storm.")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			deliveryBarrier(t, ctx, gate.entered, "actual student private TCP write")
			revoked := publicArchiveAsync(h)
			// Reuse the reviewed DB-only observer, never the parent handler/runtime.
			awaitDeliveryLock(t, &agentWSTest{db: h.pool}, ctx, 0, revoked)
			close(gate.release)
			deliveryBarrier(t, ctx, gate.finished, "student private write return")
			if !gate.wrote.Load() {
				t.Fatal("delivery-first frame did not actually write")
			}
			select {
			case err := <-revoked:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("public archive did not unblock")
			}
			consumer.wait(conn, func(e wireStudentEvent) bool { return e.Kind == kind })
			expectStudentClose(t, conn, kind, websocket.StatusPolicyViolation)
		})
	}
	t.Run("revocation-first-replay", func(t *testing.T) {
		h := newPublicDialogueHarness(t)
		h.begin()
		producer := h.socket(0)
		q := h.wait(producer, func(e wireStudentEvent) bool { return e.Kind == "question" && e.AcceptedCount == 0 })
		consumer, gate := gatedStudentConsumer(t, h, "question", false)
		conn := consumer.socket(q.Cursor)
		consumer.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "state" && e.QuestionID == q.QuestionID })
		gate.armed.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		tx, err := h.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM student_sessions WHERE tenant_id=$1 AND student_id=$2 FOR UPDATE`, tenantA, h.studentID); err != nil {
			t.Fatal(err)
		}
		revoked := publicArchiveAsync(h)
		awaitStudentLock(t, h.pool, ctx, "UPDATE student_sessions") // archiver owns student EXCLUSIVE
		if err = wsjson.Write(ctx, conn, studentCommand{Protocol: 1, Kind: "subscribe", OccurrenceID: h.occurrence, AttemptID: h.attempt, Cursor: 0}); err != nil {
			t.Fatal(err)
		}
		awaitStudentLock(t, h.pool, ctx, "SELECT id FROM students")
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-revoked:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("public archive did not commit")
		}
		expectStudentClose(t, conn, "question", websocket.StatusPolicyViolation)
		select {
		case <-gate.entered:
			t.Fatal("revocation-first replay reached private network write")
		default:
		}
	})
}

func TestPublicStudentDialogueStalledFrameBoundsAuthorityLocks(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	producer := h.socket(0)
	h.question(producer, 0)
	consumer, gate := gatedStudentConsumer(t, h, "question", true)
	gate.armed.Store(true)
	conn := consumer.socket(0)
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	deliveryBarrier(t, ctx, gate.entered, "stalled private student frame")
	revoked := publicArchiveAsync(h)
	awaitDeliveryLock(t, &agentWSTest{db: h.pool}, ctx, 0, revoked)
	// Never release the gate. Production timeout must close real TCP, release
	// the student/session locks and let the actual archive commit.
	deliveryBarrier(t, ctx, gate.finished, "production bounded write termination")
	if gate.wrote.Load() {
		t.Fatal("stalled private frame unexpectedly wrote")
	}
	select {
	case err := <-revoked:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("stalled socket retained revocation authority")
	}
}
