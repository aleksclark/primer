package outbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// SignatureHeader is the Studio production HMAC header.
	SignatureHeader = "X-Studio-Signature"
	// TimestampHeader is the unix-seconds timestamp covered by the signature.
	TimestampHeader = "X-Studio-Timestamp"
	// EventIDHeader is the C9 event identity header.
	EventIDHeader = "X-Curriculum-Studio-Event-Id"
	// EventTypeHeader is the C9 event type header.
	EventTypeHeader = "X-Curriculum-Studio-Event-Type"
	// CompatSignatureHeader is the C9-compatible signature header.
	CompatSignatureHeader = "X-Curriculum-Studio-Signature"
)

// SignV1 returns X-Studio-Signature: v1=<hmac-sha256 hex> over
// "{unixSeconds}.{body}". The timestamp is sent as X-Studio-Timestamp.
func SignV1(secret []byte, ts time.Time, body []byte) (signature, timestamp string) {
	unix := strconv.FormatInt(ts.UTC().Unix(), 10)
	return "v1=" + hex.EncodeToString(macV1(secret, unix, body)), unix
}

// VerifyV1 checks a v1 signature against the exact body and timestamp.
func VerifyV1(secret []byte, signature, timestamp string, body []byte) bool {
	got, ok := parseV1(signature)
	if !ok {
		return false
	}
	want := macV1(secret, timestamp, body)
	return hmac.Equal(got, want)
}

func parseV1(signature string) ([]byte, bool) {
	raw := strings.TrimSpace(signature)
	if !strings.HasPrefix(raw, "v1=") {
		return nil, false
	}
	sum, err := hex.DecodeString(strings.TrimPrefix(raw, "v1="))
	if err != nil || len(sum) != sha256.Size {
		return nil, false
	}
	return sum, true
}

func macV1(secret []byte, timestamp string, body []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%s.", timestamp)
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}
