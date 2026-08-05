// Package broker implements the privileged primer-student broker: Unix-socket
// IPC, credential isolation, and ownership of cache/API/sandbox/engine state.
//
// The unprivileged TUI never receives the device token. All durable ops go
// through the broker; IPC success means the broker has stored the operation.
package broker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	studentsync "github.com/aleksclark/primer/server/internal/studentclient/sync"
)

// SchemaVersion is the IPC envelope version.
const SchemaVersion = 1

// Request / response type names.
const (
	TypeHealth         = "health"
	TypePair           = "pair"
	TypeProfile        = "profile"
	TypeSyncWork       = "syncWork"
	TypeListWork       = "listWork"
	TypeOpenSession    = "openSession"
	TypeRunCommand     = "runCommand"
	TypeTypeRune       = "typeRune"
	TypeTypeBackspace  = "typeBackspace"
	TypeVerify         = "verify"
	TypeComplete       = "complete"
	TypeTutor          = "tutor"
	TypeStatus         = "status"
	TypePause          = "pause"
	TypeTerminalWrite  = "terminalWrite"
	TypeTerminalRead   = "terminalRead"
	TypeTerminalResize = "terminalResize"
	TypeResponse       = "response"
	TypeError          = "error"
)

// Envelope is one JSON-lines IPC message.
type Envelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Type          string          `json:"type"`
	RequestID     string          `json:"requestId"`
	OK            *bool           `json:"ok,omitempty"`
	Error         string          `json:"error,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// HealthRequest is empty; reserved for future filters.
type HealthRequest struct{}

// HealthResponse is returned by Health.
type HealthResponse struct {
	OK               bool     `json:"ok"`
	Version          string   `json:"version"`
	Paired           bool     `json:"paired"`
	BaseURL          string   `json:"baseUrl,omitempty"`
	SandboxOK        bool     `json:"sandboxOk"`
	AllowUnsandboxed bool     `json:"allowUnsandboxed"`
	PendingEvents    int      `json:"pendingEvents"`
	PendingCompletes int      `json:"pendingCompletes"`
	Message          string   `json:"message,omitempty"`
	SupportedKinds   []string `json:"supportedKinds,omitempty"`
	RunnerVersion    string   `json:"runnerVersion,omitempty"`
}

// PairRequest carries the one-use pairing code from the TUI.
// Never include a token or student identity claim here.
type PairRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"deviceName,omitempty"`
}

// PairResponse returns non-secret device metadata only.
type PairResponse struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	StudentID  string `json:"studentId"`
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
}

// ProfileResponse is non-secret profile info.
type ProfileResponse struct {
	Paired     bool   `json:"paired"`
	DeviceID   string `json:"deviceId,omitempty"`
	DeviceName string `json:"deviceName,omitempty"`
	StudentID  string `json:"studentId,omitempty"`
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	// SupportedKinds lists activity kinds this broker/client can run.
	SupportedKinds []string `json:"supportedKinds,omitempty"`
	// RunnerVersion is the durable runner state / capability version.
	RunnerVersion string `json:"runnerVersion,omitempty"`
	// AppVersion is the primer-student build version.
	AppVersion string `json:"appVersion,omitempty"`
}

// SyncWorkResponse is the outcome of a pull/flush.
type SyncWorkResponse struct {
	Status           string `json:"status"`
	WorkItems        int    `json:"workItems"`
	EventsFlushed    int    `json:"eventsFlushed"`
	CompletionsSent  int    `json:"completionsSent"`
	ArtifactsSent    int    `json:"artifactsSent"`
	PendingEvents    int    `json:"pendingEvents"`
	PendingCompletes int    `json:"pendingCompletes"`
	Error            string `json:"error,omitempty"`
}

// ListWorkResponse is the cached work queue (no secrets).
type ListWorkResponse struct {
	Items []studentapi.WorkItem `json:"items"`
}

// OpenSessionRequest opens an interactive activity session.
type OpenSessionRequest struct {
	AssignmentID string `json:"assignmentId"`
	RequestID    string `json:"requestId,omitempty"` // optional idempotency key
}

// OpenSessionResponse identifies the session and returns the first snapshot.
type OpenSessionResponse struct {
	ClientSessionID string                 `json:"clientSessionId"`
	Snapshot        engine.SessionSnapshot `json:"snapshot"`
}

// SessionRef identifies an open session on the broker.
type SessionRef struct {
	ClientSessionID string `json:"clientSessionId"`
}

// RunCommandRequest runs one shell line in the session workspace.
type RunCommandRequest struct {
	ClientSessionID string `json:"clientSessionId"`
	Line            string `json:"line"`
	RequestID       string `json:"requestId,omitempty"`
}

// TypeRuneRequest types one rune in a typing session.
type TypeRuneRequest struct {
	ClientSessionID string `json:"clientSessionId"`
	Rune            string `json:"rune"` // single character (UTF-8)
}

// TypeBackspaceRequest deletes one character in a typing session.
type TypeBackspaceRequest struct {
	ClientSessionID string `json:"clientSessionId"`
}

// TutorRequest asks for a coaching hint.
type TutorRequest struct {
	ClientSessionID string `json:"clientSessionId"`
	Message         string `json:"message,omitempty"`
}

// TutorResponse is a non-secret tutor reply.
type TutorResponse struct {
	Hint     string                 `json:"hint"`
	Snapshot engine.SessionSnapshot `json:"snapshot"`
}

// SnapshotResponse is a generic session snapshot reply.
type SnapshotResponse struct {
	Snapshot engine.SessionSnapshot `json:"snapshot"`
}

// TerminalWriteRequest sends keystrokes to the session PTY.
type TerminalWriteRequest struct {
	ClientSessionID string `json:"clientSessionId"`
	// Data is raw PTY input (UTF-8 text / control bytes as a string).
	Data string `json:"data"`
}

// TerminalReadRequest polls the current PTY screen content.
type TerminalReadRequest struct {
	ClientSessionID string `json:"clientSessionId"`
}

// TerminalReadResponse is a polled screen snapshot.
type TerminalReadResponse struct {
	Screen   string                 `json:"screen"`
	Snapshot engine.SessionSnapshot `json:"snapshot"`
}

// TerminalResizeRequest updates the PTY window size.
type TerminalResizeRequest struct {
	ClientSessionID string `json:"clientSessionId"`
	Rows            uint16 `json:"rows"`
	Cols            uint16 `json:"cols"`
}

// StatusRequest may target a session or the broker overall.
type StatusRequest struct {
	ClientSessionID string `json:"clientSessionId,omitempty"`
}

// StatusResponse is broker + optional session status.
type StatusResponse struct {
	Health   HealthResponse          `json:"health"`
	Snapshot *engine.SessionSnapshot `json:"snapshot,omitempty"`
	Sync     string                  `json:"sync,omitempty"`
}

// Encode writes one envelope as a single JSON line.
func Encode(w io.Writer, env Envelope) error {
	if env.SchemaVersion == 0 {
		env.SchemaVersion = SchemaVersion
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// Decode reads one JSON line into an envelope.
func Decode(r *bufio.Reader) (Envelope, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Envelope{}, err
	}
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return Envelope{}, fmt.Errorf("empty ipc line")
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("decode ipc: %w", err)
	}
	return env, nil
}

// MarshalPayload encodes v as JSON raw message.
func MarshalPayload(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// UnmarshalPayload decodes env.Payload into dest.
func UnmarshalPayload(env Envelope, dest any) error {
	if len(env.Payload) == 0 || string(env.Payload) == "null" {
		return nil
	}
	return json.Unmarshal(env.Payload, dest)
}

// SyncResultFrom converts a studentsync.Result to an IPC response (no secrets).
func SyncResultFrom(res studentsync.Result) SyncWorkResponse {
	out := SyncWorkResponse{
		Status:           string(res.Status),
		WorkItems:        res.WorkItems,
		EventsFlushed:    res.EventsFlushed,
		CompletionsSent:  res.CompletionsSent,
		ArtifactsSent:    res.ArtifactsSent,
		PendingEvents:    res.PendingEvents,
		PendingCompletes: res.PendingCompletes,
	}
	if res.Err != nil {
		out.Error = res.Err.Error()
	}
	return out
}
