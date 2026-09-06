package api

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"primer-tasks/internal/verification"
)

func (s *Server) studentWS(w http.ResponseWriter, r *http.Request) {
	// This endpoint is browser-cookie ONLY. Native continuation is deferred;
	// neither Authorization nor query credentials can become a fallback lane.
	if r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
		http.Error(w, "student cookie required", 401)
		return
	}
	if !s.studentOriginAllowed(r) || !s.validCSRF(r) {
		http.Error(w, "origin or csrf rejected", 403)
		return
	}
	check, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	a, err := s.studentIdentityFromRequest(r.WithContext(check))
	if err != nil {
		cancel()
		http.Error(w, "student cookie required", 401)
		return
	}
	upgrade, err := s.DB.Begin(check)
	if err == nil {
		err = verification.LockStudentAuthority(check, upgrade, a)
	}
	if err != nil {
		if upgrade != nil {
			_ = upgrade.Rollback(check)
		}
		cancel()
		http.Error(w, "student cookie required", 401)
		return
	}
	// Serialize upgrade itself with revocation. The HTTP handshake write is
	// bounded as well; no slow handshake may retain authority locks forever.
	controller := http.NewResponseController(w)
	if err = controller.SetWriteDeadline(time.Now().Add(studentWriteTimeout)); err != nil {
		_ = upgrade.Rollback(check)
		cancel()
		http.Error(w, "bounded upgrade unavailable", 503)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"primer-tasks.student.v1"}, InsecureSkipVerify: true})
	resetErr := controller.SetWriteDeadline(time.Time{})
	_ = upgrade.Rollback(check)
	cancel()
	if err != nil {
		return
	}
	if resetErr != nil {
		conn.CloseNow()
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 * 1024)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if err = s.writeStudentFrame(ctx, conn, a, wireStudentEvent{Protocol: 1, Kind: "hello", Time: time.Now().UTC()}); err != nil {
		closeStudentDelivery(conn, err)
		return
	}
	sub := &studentSubscriber{queue: make(chan wireStudentEvent, 16), lastAck: time.Now()}
	done := make(chan struct{})
	go func() { defer close(done); defer cancel(); s.tailStudentSocket(ctx, conn, a, sub) }()
	defer func() { cancel(); <-done }()
	send := func(event wireStudentEvent) bool {
		select {
		case sub.queue <- event:
			return true
		case <-ctx.Done():
			return false
		default:
			_ = conn.Close(websocket.StatusTryAgainLater, "student control queue exhausted")
			return false
		}
	}
	engine := verification.DialogueEngine{DB: s.DB}
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			_ = conn.Close(websocket.StatusUnsupportedData, "text commands required")
			return
		}
		command, err := decodeStudentCommand(data)
		if err != nil {
			if !send(wireStudentEvent{Protocol: 1, Kind: "error", Time: time.Now().UTC(), Code: "invalid_request"}) {
				return
			}
			continue
		}
		commandCtx, finish := context.WithTimeout(ctx, 3*time.Second)
		// Even ack/unsubscribe revalidate the current session. Mutation methods
		// additionally hold scoped authority locks through their actual commit.
		tx, err := s.DB.Begin(commandCtx)
		if err == nil {
			err = verification.LockStudentAuthority(commandCtx, tx, a)
			_ = tx.Rollback(commandCtx)
		}
		if err != nil {
			finish()
			closeStudentDelivery(conn, err)
			return
		}
		sub.mu.Lock()
		switch command.Kind {
		case "subscribe":
			var state wireStudentEvent
			state, err = s.readStudentDialogueState(commandCtx, a, command.OccurrenceID, command.AttemptID)
			var maxCursor int64
			if err == nil {
				err = s.DB.QueryRow(commandCtx, `SELECT COALESCE(max(sequence),0) FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2`, a.TenantID, command.AttemptID).Scan(&maxCursor)
			}
			if err == nil && command.Cursor > maxCursor {
				err = verification.ErrDialogueConflict
			}
			if err == nil {
				sub.occurrence, sub.attempt, sub.cursor, sub.acked, sub.lastAck, sub.lastState = command.OccurrenceID, command.AttemptID, command.Cursor, command.Cursor, time.Now(), ""
				_ = state
			}
		case "unsubscribe":
			sub.occurrence, sub.attempt, sub.lastState = "", "", ""
		case "ack":
			if command.Cursor > sub.cursor {
				err = verification.ErrDialogueConflict
			} else if command.Cursor > sub.acked {
				sub.acked, sub.lastAck = command.Cursor, time.Now()
			} // Duplicate/old acknowledgments never reopen the bounded window.
		case "user_message", "retry":
			if sub.attempt == "" || command.AttemptID != sub.attempt || command.OccurrenceID != sub.occurrence {
				err = verification.ErrDialogueContext
				break
			}
			if command.Kind == "retry" {
				err = engine.Retry(commandCtx, a, sub.occurrence, sub.attempt, command.ExpectedVersion)
			} else {
				var ack wireStudentEvent
				ack, err = engine.Admit(commandCtx, a, sub.occurrence, sub.attempt, verification.DialogueMessage{QuestionID: command.QuestionID, PolicyVersion: command.PolicyVersion, SnapshotDigest: command.SnapshotDigest, ClientMessageID: command.ClientMessageID, Content: command.Text, ExpectedVersion: command.ExpectedVersion})
				// Identical replay gets its ORIGINAL durable acknowledgment. Fresh
				// admission is delivered by the ordinary ordered database tail.
				if err == nil && ack.Sequence > 0 {
					if !send(ack) {
						sub.mu.Unlock()
						finish()
						return
					}
				}
			}
		}
		sub.mu.Unlock()
		finish()
		if err != nil {
			if !send(studentError(err)) {
				return
			}
		}
	}
}
