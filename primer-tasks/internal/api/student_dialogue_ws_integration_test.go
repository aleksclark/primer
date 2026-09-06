package api

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// This replay is produced by actual public answers/Fantasy evaluations, never
// by inserting synthetic frames or accepted DB decisions to fill a queue.
func TestPublicStudentDialoguePagedReplayHealthyReaderAndOneClose(t *testing.T) {
	h := newPublicDialogueHarnessWithPolicy(t, 5, 18)
	h.begin()
	producer := h.socket(0)
	current := h.question(producer, 0)
	u, _ := url.Parse(h.base)
	raw, err := net.DialTimeout("tcp", u.Host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err = raw.SetDeadline(time.Now().Add(25 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", h.base+"/student/ws", nil)
	request.Header = studentCookieHeader(h, h.base)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	// RFC6455 nonce is exactly 16 bytes; the test never substitutes a token in
	// the URL, and the server must negotiate only its fixed application token.
	request.Header.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")))
	request.Header.Set("Sec-WebSocket-Protocol", "primer-tasks.student.v1, primer-tasks.v1.csrf."+h.csrf())
	if err = request.Write(raw); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(raw)
	response, err := http.ReadResponse(reader, request)
	if err != nil || response.StatusCode != 101 {
		t.Fatal("real raw student upgrade failed")
	}
	if response.Header.Get("Sec-WebSocket-Protocol") != "primer-tasks.student.v1" {
		t.Fatal("credential subprotocol negotiated")
	}
	encoded, _ := json.Marshal(studentCommand{Protocol: 1, Kind: "subscribe", OccurrenceID: h.occurrence, AttemptID: h.attempt})
	if err = writeProbeClientFrame(raw, 1, encoded); err != nil {
		t.Fatal(err)
	}
	correct := []string{"The family repaired the garden wall after the storm.", "The mortar must dry before the next course of stones.", "Rushing the work would weaken the wall."}
	for question := 0; question < 3; question++ {
		for retry := 0; retry < 5; retry++ {
			h.answer(producer, current, fmt.Sprintf("insufficient-%d-%d", question, retry), "I do not know.")
			current = h.wait(producer, func(e wireStudentEvent) bool { return e.Kind == "answer_evaluation" && e.Status == "rejected" })
			if current.AcceptedCount != question {
				t.Fatal("rejected replay-generating answer counted")
			}
		}
		h.answer(producer, current, fmt.Sprintf("accepted-%d", question), correct[question])
		if question < 2 {
			current = h.question(producer, question+1)
		} else {
			h.wait(producer, func(e wireStudentEvent) bool { return e.Kind == "complete" })
		}
	}
	if m, v, a, d, c := h.counts(); m != 18 || v != 18 || a != 3 || d != 1 || c != 1 {
		t.Fatal("slow subscriber disturbed the real worker/healthy reader")
	}
	frames := 0
	for {
		opcode, payload, err := readProbeServerFrame(reader)
		if err != nil {
			t.Fatal(err)
		}
		if opcode == 8 {
			if len(payload) < 2 || binary.BigEndian.Uint16(payload[:2]) != 1013 || !strings.Contains(string(payload[2:]), "acknowledgement window") {
				t.Fatal("slow subscriber did not receive bounded 1013")
			}
			if err = writeProbeClientFrame(raw, 8, payload); err != nil {
				t.Fatal(err)
			}
			opcode, _, err = readProbeServerFrame(reader)
			if err != io.EOF {
				t.Fatalf("student socket sent a frame after Close reply: opcode=%d err=%v", opcode, err)
			}
			break
		}
		if opcode != 1 {
			t.Fatal("unexpected student socket opcode")
		}
		var event wireStudentEvent
		if json.Unmarshal(payload, &event) != nil {
			t.Fatal("malformed durable student frame")
		}
		if event.Sequence > 0 {
			frames++
		}
	}
	if frames != 64 {
		t.Fatalf("unacknowledged durable window = %d, want 64", frames)
	}
	// An acknowledging reader crosses multiple bounded pages and reaches the
	// same single persisted completion with strictly ordered replay.
	replay := h.socket(0)
	last := int64(0)
	count := 0
	h.wait(replay, func(e wireStudentEvent) bool {
		if e.Sequence > 0 {
			if e.Sequence <= last {
				t.Fatal("replay order is not strictly increasing")
			}
			last = e.Sequence
			count++
		}
		return e.Kind == "complete"
	})
	if count <= 64 {
		t.Fatal("healthy replay did not cross the slow-reader window/page bounds")
	}
}
