// Command external-verifier-fixture is a real, process-bound verifier used by
// the Tasks boundary tests. It has no dependency on the Tasks module or its
// database. Its JSON ledger is deliberately boring: a restart must be able to
// prove receipt and callback idempotency without a process-local fake.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	signatureHeader = "X-External-Signature"
	timestampHeader = "X-External-Timestamp"
	requestIDHeader = "X-External-Request-ID"
)

type requestEnvelope struct {
	Version        int             `json:"version"`
	RequestID      string          `json:"requestId"`
	AttemptRef     string          `json:"attemptRef"`
	RequirementRef string          `json:"requirementRef"`
	SchemaVersion  string          `json:"schemaVersion"`
	ExpiresAt      time.Time       `json:"expiresAt"`
	IdempotencyKey string          `json:"idempotencyKey"`
	CallbackPath   string          `json:"callbackPath"`
	PayloadDigest  string          `json:"payloadDigest"`
	Payload        json.RawMessage `json:"payload"`
	AllowedHandles []string        `json:"allowedHandles,omitempty"`
}

type progressResult struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Percent   int    `json:"percent,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}
type acceptedResult struct {
	Rationale string `json:"rationale,omitempty"`
}
type rejectedResult struct {
	Rationale string `json:"rationale,omitempty"`
}
type errorResult struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}
type callbackEnvelope struct {
	Version       int             `json:"version"`
	CallbackID    string          `json:"callbackId"`
	RequestID     string          `json:"requestId"`
	AttemptRef    string          `json:"attemptRef"`
	VerifierID    string          `json:"verifierId"`
	SchemaVersion string          `json:"schemaVersion"`
	Sequence      int64           `json:"sequence"`
	RequestDigest string          `json:"requestDigest"`
	Type          string          `json:"type"`
	Progress      *progressResult `json:"progress,omitempty"`
	Accepted      *acceptedResult `json:"accepted,omitempty"`
	Rejected      *rejectedResult `json:"rejected,omitempty"`
	Error         *errorResult    `json:"error,omitempty"`
}

type requestRecord struct {
	Request      requestEnvelope `json:"request"`
	BodyDigest   string          `json:"body_digest"`
	ReceivedAt   time.Time       `json:"received_at"`
	LastDelivery time.Time       `json:"last_delivery_at,omitempty"`
	Deliveries   int             `json:"deliveries"`
	Final        bool            `json:"final"`
	Outcome      string          `json:"outcome,omitempty"`
}
type callbackRecord struct {
	CallbackID string    `json:"callback_id"`
	RequestID  string    `json:"request_id"`
	Sequence   int64     `json:"sequence"`
	Type       string    `json:"type"`
	SentAt     time.Time `json:"sent_at"`
	Attempts   int       `json:"attempts"`
	LastStatus int       `json:"last_status,omitempty"`
}

type fixture struct {
	ledger       *ledger
	secret       []byte
	window       time.Duration
	callbackBase string
	client       *http.Client
	faultMu      sync.RWMutex
	faults       map[string]bool
	barrierMu    sync.Mutex
	barriers     map[string]chan struct{}
}

func newFixture(l *ledger, secret string) *fixture {
	f := &fixture{ledger: l, secret: []byte(secret), window: 5 * time.Minute, callbackBase: strings.TrimRight(os.Getenv("FIXTURE_CALLBACK_BASE_URL"), "/"), faults: map[string]bool{}, barriers: map[string]chan struct{}{}}
	for _, mode := range strings.Split(os.Getenv("FIXTURE_FAULT_MODE"), ",") {
		if mode = strings.TrimSpace(strings.ToLower(mode)); mode != "" {
			f.faults[mode] = true
		}
	}
	f.client = &http.Client{Timeout: durationEnv("FIXTURE_CALLBACK_TIMEOUT", 5*time.Second), CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("redirects are forbidden for callbacks")
	}}
	return f
}
func (f *fixture) fault(mode string) bool {
	f.faultMu.RLock()
	defer f.faultMu.RUnlock()
	return f.faults[mode]
}
func (f *fixture) setFaults(modes []string) {
	f.faultMu.Lock()
	defer f.faultMu.Unlock()
	f.faults = map[string]bool{}
	for _, mode := range modes {
		if mode = strings.TrimSpace(strings.ToLower(mode)); mode != "" {
			f.faults[mode] = true
		}
	}
}

func (f *fixture) verify(method, path string, body []byte, h http.Header) error {
	at, err := time.Parse(time.RFC3339Nano, h.Get(timestampHeader))
	if err != nil || time.Since(at) > f.window || time.Until(at) > f.window {
		return errors.New("timestamp outside signature window")
	}
	requestID := h.Get(requestIDHeader)
	if requestID == "" {
		return errors.New("missing request id")
	}
	want := sign(f.secret, method, path, requestID, body, at)
	got := h.Get(signatureHeader)
	if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return errors.New("invalid signature")
	}
	return nil
}
func sign(secret []byte, method, path, requestID string, body []byte, at time.Time) string {
	sum := sha256.Sum256(body)
	canonical := strings.ToUpper(strings.TrimSpace(method)) + "\n" + path + "\n" + at.UTC().Format(time.RFC3339Nano) + "\n" + hex.EncodeToString(sum[:]) + "\n" + requestID
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (f *fixture) verifyRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err = f.verify(r.Method, r.URL.Path, body, r.Header); err != nil {
		http.Error(w, "unauthorized request", http.StatusUnauthorized)
		return
	}
	var req requestEnvelope
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&req); err != nil || req.Version != 1 || req.RequestID == "" || req.AttemptRef == "" || req.RequirementRef == "" || req.SchemaVersion == "" || req.IdempotencyKey == "" || req.CallbackPath == "" || len(req.Payload) == 0 {
		http.Error(w, "invalid verification envelope", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.CallbackPath, "/") || strings.HasPrefix(req.CallbackPath, "//") || strings.ContainsAny(req.CallbackPath, "?#") {
		http.Error(w, "invalid callback path", http.StatusBadRequest)
		return
	}
	if req.ExpiresAt.Before(time.Now().UTC()) {
		http.Error(w, "expired verification envelope", http.StatusBadRequest)
		return
	}
	if digest(req.Payload) != strings.ToLower(req.PayloadDigest) {
		http.Error(w, "payload digest mismatch", http.StatusBadRequest)
		return
	}
	bodyDigest := digest(body)
	if old, ok := f.ledger.findRequest(req.IdempotencyKey); ok {
		if old.BodyDigest != bodyDigest || old.Request.RequestID != req.RequestID {
			http.Error(w, "idempotency key conflict", http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"requestId": old.Request.RequestID, "idempotent": true, "status": "received"})
		return
	}
	record := requestRecord{Request: req, BodyDigest: bodyDigest, ReceivedAt: time.Now().UTC()}
	if err = f.ledger.putRequest(record); err != nil {
		http.Error(w, "fixture storage unavailable", http.StatusInternalServerError)
		return
	}
	if f.fault("http_429") {
		http.Error(w, "fixture busy", http.StatusTooManyRequests)
		return
	}
	if f.fault("http_500") {
		http.Error(w, "fixture failure", http.StatusBadGateway)
		return
	}
	if f.fault("timeout") {
		time.Sleep(durationEnv("FIXTURE_TIMEOUT", 30*time.Second))
	}
	if f.fault("barrier") {
		f.waitBarrier(req.RequestID)
	}
	if err = f.deliver(record); err != nil {
		slog.Warn("fixture callback delivery failed", "request_id", req.RequestID, "class", "callback_unavailable", "error", err)
	}
	if f.fault("lost_ack") {
		f.closeWithoutAck(w)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": req.RequestID, "idempotent": false, "status": "received"})
}

func (f *fixture) deliver(record requestRecord) error {
	order := []int64{1, 2, 3}
	if f.fault("out_of_order") {
		order = []int64{3, 1, 2}
	}
	for _, sequence := range order {
		cb := callbackEnvelope{Version: 1, CallbackID: callbackID(record.Request.RequestID, sequence), RequestID: record.Request.RequestID, AttemptRef: record.Request.AttemptRef, VerifierID: envOr("FIXTURE_VERIFIER_ID", "fixture"), SchemaVersion: record.Request.SchemaVersion, Sequence: sequence, RequestDigest: record.Request.PayloadDigest}
		switch sequence {
		case 1:
			cb.Type = "progress"
			cb.Progress = &progressResult{Code: "queued", Message: "Verification queued", Percent: 10}
		case 2:
			cb.Type = "progress"
			cb.Progress = &progressResult{Code: "checking", Message: "Checking submitted response", Percent: 60}
		case 3:
			if strings.EqualFold(os.Getenv("FIXTURE_OUTCOME"), "rejected") {
				cb.Type = "rejected"
				cb.Rejected = &rejectedResult{Rationale: "The fixture rejected the submitted response"}
			} else {
				cb.Type = "accepted"
				cb.Accepted = &acceptedResult{Rationale: "The fixture accepted the submitted response"}
			}
		}
		if f.fault("retryable_error") {
			cb.Type = "retryable_error"
			cb.Progress = nil
			cb.Error = &errorResult{Code: "fixture_retryable", Message: "The fixture is temporarily unavailable", Retryable: true}
		}
		if f.fault("terminal_error") {
			cb.Type = "terminal_error"
			cb.Progress = nil
			cb.Error = &errorResult{Code: "fixture_terminal", Message: "The fixture could not verify this response", Retryable: false}
		}
		if err := f.sendCallback(record, cb); err != nil {
			return err
		}
		if f.fault("duplicate_callbacks") {
			if err := f.sendCallback(record, cb); err != nil {
				return err
			}
		}
	}
	return f.ledger.updateRequest(record.Request.IdempotencyKey, func(r *requestRecord) {
		r.Final = true
		r.Outcome = cbOutcome(record)
		r.Deliveries++
		r.LastDelivery = time.Now().UTC()
	})
}
func cbOutcome(r requestRecord) string {
	if f := strings.ToLower(os.Getenv("FIXTURE_OUTCOME")); f == "rejected" {
		return f
	}
	if os.Getenv("FIXTURE_FAULT_MODE") == "terminal_error" {
		return "terminal_error"
	}
	return "accepted"
}
func (f *fixture) sendCallback(record requestRecord, cb callbackEnvelope) error {
	body, err := json.Marshal(cb)
	if err != nil {
		return err
	}
	if err = f.ledger.putCallback(callbackRecord{CallbackID: cb.CallbackID, RequestID: record.Request.RequestID, Sequence: cb.Sequence, Type: cb.Type, SentAt: time.Now().UTC()}); err != nil {
		return err
	}
	callbackURL, callbackPath, err := f.callbackURL(record.Request.CallbackPath)
	if err != nil {
		return err
	}
	at := time.Now().UTC()
	signature := sign(f.secret, http.MethodPost, callbackPath, cb.CallbackID, body, at)
	if f.fault("corrupt_callback") {
		body = append(body[:len(body)-1], 'x', '}')
		signature = "sha256=00"
	}
	if f.fault("stale_callback") {
		at = at.Add(-f.window - time.Second)
	}
	req, err := http.NewRequest(http.MethodPost, callbackURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(signatureHeader, signature)
	req.Header.Set(timestampHeader, at.UTC().Format(time.RFC3339Nano))
	req.Header.Set(requestIDHeader, cb.CallbackID)
	req.Header.Set("Idempotency-Key", cb.CallbackID)
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = f.ledger.updateCallback(cb.CallbackID, func(c *callbackRecord) { c.Attempts++; c.LastStatus = resp.StatusCode })
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("callback returned %d", resp.StatusCode)
	}
	return nil
}
func (f *fixture) callbackURL(path string) (string, string, error) {
	if f.callbackBase == "" {
		return "", "", errors.New("FIXTURE_CALLBACK_BASE_URL is required")
	}
	base, err := url.Parse(f.callbackBase)
	if err != nil || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", "", errors.New("invalid fixture callback base")
	}
	if os.Getenv("FIXTURE_EGRESS_MODE") != "test" && base.Scheme != "https" {
		return "", "", errors.New("production fixture egress requires HTTPS")
	}
	if os.Getenv("FIXTURE_EGRESS_MODE") == "test" && base.Scheme != "http" && base.Scheme != "https" {
		return "", "", errors.New("test fixture egress requires HTTP(S)")
	}
	joined := strings.TrimRight(base.String(), "/") + path
	target, err := url.Parse(joined)
	if err != nil || target.Host != base.Host {
		return "", "", errors.New("invalid callback path")
	}
	return target.String(), path, nil
}

func (f *fixture) reconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct {
		RequestID string `json:"requestId"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&input)
	snapshot := f.ledger.snapshot()
	count := 0
	for _, record := range snapshot.Requests {
		if input.RequestID != "" && record.Request.RequestID != input.RequestID {
			continue
		}
		if record.Final && input.RequestID == "" {
			continue
		}
		if f.deliver(record) == nil {
			count++
		}
	}
	writeJSON(w, 202, map[string]any{"reconciled": count})
}
func (f *fixture) control(w http.ResponseWriter, r *http.Request) {
	if token := os.Getenv("FIXTURE_ADMIN_TOKEN"); token != "" && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(token)) != 1 {
		http.Error(w, "forbidden", 403)
		return
	}
	var input struct {
		Action    string   `json:"action"`
		RequestID string   `json:"requestId"`
		Faults    []string `json:"faults"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&input); err != nil {
		http.Error(w, "invalid control request", 400)
		return
	}
	if input.Action == "faults" {
		f.setFaults(input.Faults)
		writeJSON(w, 200, map[string]any{"faults": input.Faults})
		return
	}
	if input.Action != "release" {
		http.Error(w, "unknown control action", 400)
		return
	}
	f.barrierMu.Lock()
	ch, ok := f.barriers[input.RequestID]
	if ok {
		close(ch)
		delete(f.barriers, input.RequestID)
	}
	f.barrierMu.Unlock()
	writeJSON(w, 200, map[string]any{"released": ok, "requestId": input.RequestID})
}
func (f *fixture) waitBarrier(id string) {
	f.barrierMu.Lock()
	ch := f.barriers[id]
	if ch == nil {
		ch = make(chan struct{})
		f.barriers[id] = ch
	}
	f.barrierMu.Unlock()
	<-ch
}
func (f *fixture) closeWithoutAck(w http.ResponseWriter) {
	if h, ok := w.(http.Hijacker); ok {
		conn, _, err := h.Hijack()
		if err == nil {
			_ = conn.Close()
			return
		}
	}
	w.WriteHeader(202)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func callbackID(requestID string, sequence int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", requestID, sequence)))
	hexed := hex.EncodeToString(sum[:])
	return hexed[:8] + "-" + hexed[8:12] + "-4" + hexed[13:16] + "-8" + hexed[17:20] + "-" + hexed[20:32]
}
func durationEnv(key string, fallback time.Duration) time.Duration {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return time.Duration(n) * time.Millisecond
	}
	return fallback
}
func main() {
	path := os.Getenv("FIXTURE_LEDGER_PATH")
	if path == "" {
		path = "/var/lib/external-verifier/ledger.json"
	}
	l, err := openLedger(path)
	if err != nil {
		slog.Error("open fixture ledger", "error", err)
		os.Exit(2)
	}
	f := newFixture(l, envOr("FIXTURE_SHARED_SECRET", "fixture-development-secret"))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ready"}) })
	mux.HandleFunc("/v1/verify", f.verifyRequest)
	mux.HandleFunc("/verify", f.verifyRequest)
	mux.HandleFunc("/v1/reconcile", f.reconcile)
	mux.HandleFunc("/v1/control", f.control)
	mux.HandleFunc("/control", f.control)
	mux.HandleFunc("/v1/ledger", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, f.ledger.snapshot()) })
	addr := envOr("FIXTURE_HOST", "127.0.0.1") + ":" + envOr("FIXTURE_PORT", "8092")
	slog.Info("external verifier fixture listening", "addr", addr, "ledger", path)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("fixture stopped", "error", err)
		os.Exit(1)
	}
}
func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
