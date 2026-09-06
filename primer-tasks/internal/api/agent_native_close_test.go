package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func seedNonAckReplay(t *testing.T) (*agentWSTest, string) {
	t.Helper()
	h := newAgentWSTest(t)
	conv := h.conversation("parent-a")
	producer := h.socket("parent-a")
	run := h.run(producer, conv, "native-close", "List students.")
	_ = producer.CloseNow()
	for i := 0; i < 70; i++ {
		if err := h.s.publishAgent(h.ctx, tenantA, conv, wireAgentEvent{Type: "text_delta", RunID: run.RunID, Text: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	return h, conv
}

// Node's built-in WHATWG WebSocket is an independent native client, not the Go
// server library used as its own permissive close oracle. Only local BFF fixture
// credentials are passed; emitted evidence excludes headers/protocol inputs.
func TestPublicAgentBackpressureNativeCloseHandshake(t *testing.T) {
	h, conv := seedNonAckReplay(t)
	ctx, cancel := context.WithTimeout(h.ctx, 12*time.Second)
	defer cancel()
	script := `let input='';for await(const chunk of process.stdin)input+=chunk;const c=JSON.parse(input);
const ws=new WebSocket(c.url,{protocols:['primer-tasks.v1','primer-tasks.v1.csrf.fixture'],headers:{Origin:c.origin,Cookie:'tasks_parent=parent-a; tasks_csrf=fixture'}});
let frames=0,errors=0;const timeout=setTimeout(()=>{console.error('native close deadline exceeded');process.exit(2)},8000);
ws.onopen=()=>ws.send(JSON.stringify({protocol:1,kind:'subscribe',conversationId:c.conversation}));
ws.onmessage=e=>{const event=JSON.parse(String(e.data));if(event.cursor>0)frames++};
ws.onerror=()=>errors++;
ws.onclose=e=>{clearTimeout(timeout);console.log(JSON.stringify({code:e.code,reason:e.reason,frames,errors,protocol:ws.protocol}))};`
	input, _ := json.Marshal(map[string]string{"url": "ws" + strings.TrimPrefix(h.http.URL, "http") + "/ws", "origin": h.s.Auth.PublicOrigin, "conversation": conv})
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", script)
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native close probe failed: %v %s", err, out)
	}
	var result struct {
		Code, Frames, Errors int
		Reason, Protocol     string
	}
	if json.Unmarshal(out, &result) != nil {
		t.Fatalf("invalid native close result: %s", out)
	}
	if result.Code != 1013 || !strings.Contains(result.Reason, "acknowledgement window") || result.Frames != 64 || result.Errors != 0 || result.Protocol != "primer-tasks.v1" {
		t.Fatalf("native close result: %+v", result)
	}
}

// Assert the actual RFC6455 close exchange, including EOF after the client's
// one close reply. A native browser reports1006 if a server sends Close again
// after that reply. Reading only the first CloseError cannot detect the bug.
func TestPublicAgentBackpressureSendsExactlyOneCloseFrame(t *testing.T) {
	h, conv := seedNonAckReplay(t)
	conn, err := net.DialTimeout("tcp", h.http.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", h.http.URL+"/ws", nil)
	req.Header = http.Header{"Connection": []string{"Upgrade"}, "Upgrade": []string{"websocket"}, "Sec-WebSocket-Version": []string{"13"}, "Sec-WebSocket-Key": []string{base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))}, "Sec-WebSocket-Protocol": []string{"primer-tasks.v1, primer-tasks.v1.csrf.fixture"}, "Origin": []string{h.s.Auth.PublicOrigin}, "Cookie": []string{"tasks_parent=parent-a; tasks_csrf=fixture"}}
	if err = req.Write(conn); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil || resp.StatusCode != 101 {
		t.Fatal("real upgrade failed")
	}
	payload, _ := json.Marshal(agentCommand{Type: "subscribe", ProtocolVersion: 1, ConversationID: conv})
	if err = writeProbeClientFrame(conn, 1, payload); err != nil {
		t.Fatal(err)
	}
	frames := 0
	for {
		opcode, p, err := readProbeServerFrame(reader)
		if err != nil {
			t.Fatal(err)
		}
		if opcode == 8 {
			if len(p) < 2 || binary.BigEndian.Uint16(p[:2]) != 1013 || !strings.Contains(string(p[2:]), "acknowledgement window") {
				t.Fatalf("unexpected close payload %q", p)
			}
			if err = writeProbeClientFrame(conn, 8, p); err != nil {
				t.Fatal(err)
			}
			opcode, _, err = readProbeServerFrame(reader)
			if err != io.EOF {
				t.Fatalf("server sent data/control after close reply: opcode=%d err=%v", opcode, err)
			}
			break
		}
		if opcode != 1 {
			t.Fatalf("unexpected frame opcode%d", opcode)
		}
		var e wireAgentEvent
		if json.Unmarshal(p, &e) != nil {
			t.Fatal("bad wire event")
		}
		if e.Cursor > 0 {
			frames++
		}
	}
	if frames != 64 {
		t.Fatalf("frames=%d", frames)
	}
}
func writeProbeClientFrame(w io.Writer, opcode byte, p []byte) error {
	header := []byte{0x80 | opcode}
	if len(p) < 126 {
		header = append(header, 0x80|byte(len(p)))
	} else {
		header = append(header, 0x80|126, byte(len(p)>>8), byte(len(p)))
	}
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	header = append(header, mask[:]...)
	payload := append([]byte(nil), p...)
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	_, err := w.Write(append(header, payload...))
	return err
}
func readProbeServerFrame(r io.Reader) (byte, []byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	if h[1]&0x80 != 0 {
		return 0, nil, fmt.Errorf("server masked frame")
	}
	n := uint64(h[1] & 0x7f)
	if n == 126 {
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	} else if n == 127 {
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > 65536 {
		return 0, nil, fmt.Errorf("unbounded frame")
	}
	payload := make([]byte, int(n))
	_, err := io.ReadFull(r, payload)
	return h[0] & 0x0f, payload, err
}
