package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const studentProtocolVersion = 1

var (
	errStudentRevoked      = errors.New("student credential revoked")
	errStudentUnauthorized = errors.New("student credential required")
)

type studentIdentity struct {
	StudentID uuid.UUID
	TenantID  string
	Source    string
	Token     string
}

type studentCommand struct {
	Type             string `json:"kind"`
	ProtocolVersion  int    `json:"protocol"`
	AttemptID        string `json:"attemptId"`
	OccurrenceID     string `json:"occurrenceId"`
	Cursor           int64  `json:"cursor"`
	ClientMessageID  string `json:"clientMessageId"`
	SubmissionID     string `json:"submissionId"`
	Text             string `json:"text"`
	ExpectedSequence int64  `json:"expectedSequence"`
}

func studentQueryCredential(r *http.Request) bool {
	q := r.URL.Query()
	for _, key := range []string{"token", "access_token", "authorization", "auth"} {
		if strings.TrimSpace(q.Get(key)) != "" {
			return true
		}
	}
	return false
}

func (s *Server) StudentDialogueHandler() http.Handler {
	return http.HandlerFunc(s.studentWS)
}

func (s *Server) studentWS(w http.ResponseWriter, r *http.Request) {
	if studentQueryCredential(r) {
		http.Error(w, "query credentials are not allowed", http.StatusUnauthorized)
		return
	}
	identity, err := s.studentIdentityFromRequest(r)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errStudentRevoked) {
			status = http.StatusUnauthorized
		}
		http.Error(w, "student credential required", status)
		return
	}
	if identity.Source == "cookie" {
		if !s.agentOriginAllowed(r) || !s.validCSRF(r) {
			http.Error(w, "origin or csrf rejected", http.StatusForbidden)
			return
		}
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       []string{"primer-tasks.student.v1"},
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "closed")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := &studentSubscriber{student: identity.StudentID.String(), tenant: identity.TenantID, queue: make(chan wireStudentEvent, 64), done: make(chan struct{}), cancel: cancel}
	s.studentDialogueHub().add(sub)
	defer s.studentDialogueHub().remove(sub)
	hello := wireStudentEvent{
		Type:             "hello",
		ProtocolVersion:  studentProtocolVersion,
		ConnectionID:     uuid.NewString(),
		HeartbeatSeconds: 30,
		Time:             time.Now().UTC(),
	}
	if err := wsjson.Write(ctx, conn, hello); err != nil {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sub.done:
				return
			case event, ok := <-sub.queue:
				if !ok {
					return
				}
				_ = wsjson.Write(ctx, conn, event)
			}
		}
	}()
	for {
		var cmd studentCommand
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
			return
		}
		if err := s.verifyStudentIdentity(ctx, identity); err != nil {
			// The reader is about to terminate the connection; do not enqueue the
			// revocation notice behind the writer goroutine and then close it.
			_ = wsjson.Write(ctx, conn, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, Code: "revoked", Message: "This device pairing is no longer active.", Retryable: false})
			return
		}
		if cmd.ProtocolVersion != studentProtocolVersion {
			s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, Code: "protocol_version", Message: "unsupported protocol version", Retryable: false})
			continue
		}
		switch cmd.Type {
		case "hello":
		case "artifact_subscribe":
			s.studentArtifactSubscribe(ctx, identity, sub, cmd)
		case "subscribe":
			s.studentSubscribe(ctx, identity, sub, cmd)
		case "user_message":
			s.studentMessage(ctx, identity, sub, cmd)
		case "retry":
			s.studentRetry(ctx, identity, sub, cmd)
		case "unsubscribe":
			sub.attempt = ""
			sub.occurrence = ""
		case "ack":
		default:
			s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: cmd.AttemptID, OccurrenceID: cmd.OccurrenceID, Code: "unknown_command", Message: "unknown command", Retryable: false})
		}
	}
}

func (s *Server) studentIdentityFromRequest(r *http.Request) (studentIdentity, error) {
	if bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); bearer != "" {
		return s.studentIdentityFromBearer(r.Context(), bearer)
	}
	cookie, err := r.Cookie("tasks_student")
	if err != nil || cookie.Value == "" {
		return studentIdentity{}, errStudentUnauthorized
	}
	return s.studentIdentityFromCookie(r.Context(), cookie.Value)
}

func (s *Server) studentIdentityFromBearer(ctx context.Context, token string) (studentIdentity, error) {
	var id uuid.UUID
	var tenant string
	err := s.DB.QueryRow(ctx, `SELECT d.student_id, d.tenant_id FROM student_devices d JOIN students st ON st.id=d.student_id AND st.tenant_id=d.tenant_id WHERE d.token_hash=$1 AND d.revoked_at IS NULL AND st.archived_at IS NULL`, hash(token)).Scan(&id, &tenant)
	if err != nil {
		return studentIdentity{}, errStudentRevoked
	}
	return studentIdentity{StudentID: id, TenantID: tenant, Source: "bearer", Token: token}, nil
}

func (s *Server) studentIdentityFromCookie(ctx context.Context, handle string) (studentIdentity, error) {
	var id uuid.UUID
	var tenant string
	err := s.DB.QueryRow(ctx, `SELECT student_id, tenant_id FROM student_sessions WHERE handle_hash=$1 AND expires_at>now() AND revoked_at IS NULL`, hash(handle)).Scan(&id, &tenant)
	if err != nil {
		return studentIdentity{}, errStudentRevoked
	}
	var archived bool
	if err = s.DB.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM students WHERE id=$1 AND tenant_id=$2`, id, tenant).Scan(&archived); err != nil || archived {
		return studentIdentity{}, errStudentRevoked
	}
	return studentIdentity{StudentID: id, TenantID: tenant, Source: "cookie", Token: handle}, nil
}

func (s *Server) verifyStudentIdentity(ctx context.Context, identity studentIdentity) error {
	switch identity.Source {
	case "bearer":
		got, err := s.studentIdentityFromBearer(ctx, identity.Token)
		if err != nil || got.StudentID != identity.StudentID || got.TenantID != identity.TenantID {
			return errStudentRevoked
		}
	case "cookie":
		got, err := s.studentIdentityFromCookie(ctx, identity.Token)
		if err != nil || got.StudentID != identity.StudentID || got.TenantID != identity.TenantID {
			return errStudentRevoked
		}
	default:
		return errStudentUnauthorized
	}
	return nil
}

func expectedSequenceConflict(expected, next int64) bool {
	return expected != 0 && expected != next
}
