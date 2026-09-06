package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Real public HTTP/PG read-lock qualification. The database barrier holds a
// progress writer; no fake event/evaluation/decision is inserted. This guards
// against assembling a state version and evidence from different commits.
func TestPublicDialogueStateReadWaitsForCoherentProgress(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	conn := h.socket(0)
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "question" })
	conn.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT attempt_id FROM dialogue_attempts WHERE attempt_id=$1 FOR UPDATE`, h.attempt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	response := make(chan error, 1)
	go func() {
		r, err := http.NewRequestWithContext(ctx, "GET", h.base+"/student/occurrences/"+h.occurrence+"/dialogue", nil)
		if err != nil {
			response <- err
			return
		}
		res, err := h.student.Do(r)
		if err != nil {
			response <- err
			return
		}
		defer res.Body.Close()
		var state wireStudentEvent
		if res.StatusCode != 200 {
			response <- fmt.Errorf("state HTTP status %d", res.StatusCode)
			return
		}
		if err = json.NewDecoder(res.Body).Decode(&state); err != nil {
			response <- err
			return
		}
		if state.QuestionID == "" || state.Version != 2 {
			response <- fmt.Errorf("state was not a coherent initial question")
			return
		}
		response <- nil
	}()
	for {
		var blocked bool
		if err = h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0 AND query LIKE '%SELECT attempt_id FROM dialogue_attempts%' AND query LIKE '%FOR SHARE%')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-response:
			t.Fatalf("public state read completed without waiting for the progress writer: %v", err)
		case <-ctx.Done():
			t.Fatal("coherent state lock was not observed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-response:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("coherent state read did not resume")
	}
}
