package broker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	studentsync "github.com/aleksclark/primer/server/internal/studentclient/sync"
)

// Client is the TUI-side IPC client. It never sees the device token.
type Client struct {
	SocketPath string
	Timeout    time.Duration

	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

// Dial connects to the broker socket.
func Dial(socketPath string) (*Client, error) {
	c := &Client{
		SocketPath: socketPath,
		Timeout:    60 * time.Second,
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.r = nil
		return err
	}
	return nil
}

func (c *Client) connect() error {
	if c.Timeout <= 0 {
		c.Timeout = 60 * time.Second
	}
	conn, err := net.DialTimeout("unix", c.SocketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("broker dial %s: %w", c.SocketPath, err)
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	return nil
}

func (c *Client) ensure() error {
	if c.conn != nil {
		return nil
	}
	return c.connect()
}

// call sends a request and waits for the matching response.
func (c *Client) call(ctx context.Context, typ string, payload any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensure(); err != nil {
		return err
	}

	reqID := uuid.NewString()
	raw, err := MarshalPayload(payload)
	if err != nil {
		return err
	}
	env := Envelope{
		SchemaVersion: SchemaVersion,
		Type:          typ,
		RequestID:     reqID,
		Payload:       raw,
	}

	deadline := time.Now().Add(c.Timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = c.conn.SetDeadline(deadline)

	if err := Encode(c.conn, env); err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return err
	}
	resp, err := Decode(c.r)
	if err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return err
	}
	if resp.RequestID != "" && resp.RequestID != reqID {
		return fmt.Errorf("broker: response requestId mismatch")
	}
	if resp.Type == TypeError || (resp.OK != nil && !*resp.OK) {
		msg := resp.Error
		if msg == "" {
			msg = "broker error"
		}
		return fmt.Errorf("%s", msg)
	}
	if out != nil {
		if err := UnmarshalPayload(resp, out); err != nil {
			return err
		}
	}
	return nil
}

// Health probes the broker.
func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	err := c.call(ctx, TypeHealth, HealthRequest{}, &out)
	return out, err
}

// Pair submits a pairing code; token stays on the broker.
func (c *Client) Pair(ctx context.Context, code, deviceName string) (PairResponse, error) {
	var out PairResponse
	err := c.call(ctx, TypePair, PairRequest{Code: code, DeviceName: deviceName}, &out)
	return out, err
}

// Profile returns non-secret pairing metadata.
func (c *Client) Profile(ctx context.Context) (ProfileResponse, error) {
	var out ProfileResponse
	err := c.call(ctx, TypeProfile, nil, &out)
	return out, err
}

// SyncWork pulls work and flushes the outbox.
func (c *Client) SyncWork(ctx context.Context) (SyncWorkResponse, error) {
	var out SyncWorkResponse
	err := c.call(ctx, TypeSyncWork, nil, &out)
	return out, err
}

// ListWork returns the cached work queue.
func (c *Client) ListWork(ctx context.Context) ([]studentapi.WorkItem, error) {
	var out ListWorkResponse
	if err := c.call(ctx, TypeListWork, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// OpenSession starts an interactive activity on the broker.
func (c *Client) OpenSession(ctx context.Context, assignmentID string) (OpenSessionResponse, error) {
	var out OpenSessionResponse
	err := c.call(ctx, TypeOpenSession, OpenSessionRequest{AssignmentID: assignmentID}, &out)
	return out, err
}

// RunCommand runs a shell line in the open session.
func (c *Client) RunCommand(ctx context.Context, clientSessionID, line string) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeRunCommand, RunCommandRequest{
		ClientSessionID: clientSessionID,
		Line:            line,
	}, &out)
	return out.Snapshot, err
}

// TypeRune types one character in a typing session.
func (c *Client) TypeRune(ctx context.Context, clientSessionID string, r rune) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeTypeRune, TypeRuneRequest{
		ClientSessionID: clientSessionID,
		Rune:            string(r),
	}, &out)
	return out.Snapshot, err
}

// TypeBackspace deletes one character in a typing session.
func (c *Client) TypeBackspace(ctx context.Context, clientSessionID string) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeTypeBackspace, TypeBackspaceRequest{ClientSessionID: clientSessionID}, &out)
	return out.Snapshot, err
}

// Verify re-runs checks.
func (c *Client) Verify(ctx context.Context, clientSessionID string) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeVerify, SessionRef{ClientSessionID: clientSessionID}, &out)
	return out.Snapshot, err
}

// Complete queues completion + evidence.
func (c *Client) Complete(ctx context.Context, clientSessionID string) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeComplete, SessionRef{ClientSessionID: clientSessionID}, &out)
	return out.Snapshot, err
}

// Tutor requests a coaching hint.
func (c *Client) Tutor(ctx context.Context, clientSessionID, message string) (TutorResponse, error) {
	var out TutorResponse
	err := c.call(ctx, TypeTutor, TutorRequest{ClientSessionID: clientSessionID, Message: message}, &out)
	return out, err
}

// Status returns broker health and optional session snapshot.
func (c *Client) Status(ctx context.Context, clientSessionID string) (StatusResponse, error) {
	var out StatusResponse
	err := c.call(ctx, TypeStatus, StatusRequest{ClientSessionID: clientSessionID}, &out)
	return out, err
}

// Pause ends the interactive session on the broker (state remains durable).
func (c *Client) Pause(ctx context.Context, clientSessionID string) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypePause, SessionRef{ClientSessionID: clientSessionID}, &out)
	return out.Snapshot, err
}

// TerminalWrite sends raw keystrokes to the session PTY.
func (c *Client) TerminalWrite(ctx context.Context, clientSessionID, data string) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeTerminalWrite, TerminalWriteRequest{
		ClientSessionID: clientSessionID,
		Data:            data,
	}, &out)
	return out.Snapshot, err
}

// TerminalRead polls PTY screen content and the latest snapshot.
func (c *Client) TerminalRead(ctx context.Context, clientSessionID string) (TerminalReadResponse, error) {
	var out TerminalReadResponse
	err := c.call(ctx, TypeTerminalRead, TerminalReadRequest{ClientSessionID: clientSessionID}, &out)
	return out, err
}

// TerminalResize updates the PTY window size.
func (c *Client) TerminalResize(ctx context.Context, clientSessionID string, rows, cols uint16) (engine.SessionSnapshot, error) {
	var out SnapshotResponse
	err := c.call(ctx, TypeTerminalResize, TerminalResizeRequest{
		ClientSessionID: clientSessionID,
		Rows:            rows,
		Cols:            cols,
	}, &out)
	return out.Snapshot, err
}

// SyncStatus maps a SyncWorkResponse status string to studentsync.Status.
func SyncStatus(s string) studentsync.Status {
	switch studentsync.Status(s) {
	case studentsync.StatusOnline, studentsync.StatusOffline, studentsync.StatusSyncing,
		studentsync.StatusAwaiting, studentsync.StatusRevoked, studentsync.StatusIdle:
		return studentsync.Status(s)
	default:
		if s == "" {
			return studentsync.StatusIdle
		}
		return studentsync.Status(s)
	}
}
