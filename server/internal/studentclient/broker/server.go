package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	stdsync "sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/studentclient/sandbox"
	studentsync "github.com/aleksclark/primer/server/internal/studentclient/sync"
	versionpkg "github.com/aleksclark/primer/server/internal/studentclient/version"
)

// Options configure the privileged broker service.
type Options struct {
	// SocketPath is the Unix domain socket path (required).
	SocketPath string
	// DBPath is the broker-owned SQLite cache path (required).
	DBPath string
	// TokenFile is the 0600 device token path (required in production).
	TokenFile string
	// BaseURL is the LMS API base URL.
	BaseURL string
	// WorkspaceRoot is where exercise dirs are created.
	WorkspaceRoot string
	// DeviceName is the default name used when pairing.
	DeviceName string
	// UseSandbox runs terminal commands under bubblewrap (default true).
	UseSandbox bool
	// AllowUnsandboxed permits plain exec when bwrap is unavailable.
	// Production must leave this false.
	AllowUnsandboxed bool
	// Offline skips network after cache is populated.
	Offline bool
	// LegacyDBPath, when set, is migrated into DBPath on first start.
	LegacyDBPath string
	// AllowedUIDs, when non-empty, restricts peers to these UIDs (plus root=0).
	// When empty, any peer that passes group/uid checks is accepted: root, same
	// UID as the broker process, or members of AllowedGroup.
	AllowedUIDs []uint32
	// AllowedGroup is an optional group name whose members may connect
	// (default "students" when empty string is not explicitly disabled).
	// Set to "-" to disable group membership checks.
	AllowedGroup string
	// SocketGroup, when set, chowns the listening socket to this group
	// (mode 0660) so unprivileged students can connect without reading state.
	SocketGroup string
	// SkipPeerCred disables SO_PEERCRED checks (tests only).
	SkipPeerCred bool
	// Logger defaults to slog.Default().
	Logger *slog.Logger
	// Version string reported by Health (optional).
	Version string
}

// Server is the privileged broker.
type Server struct {
	opts   Options
	log    *slog.Logger
	store  *cache.Store
	client *studentapi.Client
	syncL  *studentsync.Loop
	ln     net.Listener

	mu       stdsync.Mutex
	sessions map[string]*engine.Session // clientSessionID -> session
	// idempotency for complete / open by request id
	doneReqs map[string]json.RawMessage

	ctx    context.Context
	cancel context.CancelFunc
	wg     stdsync.WaitGroup
}

// New constructs a Server but does not listen yet.
func New(opts Options) (*Server, error) {
	if opts.SocketPath == "" {
		return nil, fmt.Errorf("broker: socket path required")
	}
	if opts.DBPath == "" {
		return nil, fmt.Errorf("broker: db path required")
	}
	if opts.TokenFile == "" {
		// Default beside the DB.
		opts.TokenFile = filepath.Join(filepath.Dir(opts.DBPath), DefaultTokenFileName)
	}
	if opts.DeviceName == "" {
		opts.DeviceName = "workstation"
	}
	if opts.WorkspaceRoot == "" {
		opts.WorkspaceRoot = filepath.Join(filepath.Dir(opts.DBPath), "workspaces")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.AllowedGroup == "" {
		opts.AllowedGroup = "students"
	}
	// Production default: sandbox on, unsandboxed off unless explicitly set.
	// Callers that want AllowUnsandboxed must set it after zero-value construction
	// via the field (zero value is false — correct for production).

	if opts.LegacyDBPath != "" {
		if err := MigrateLegacyState(opts.LegacyDBPath, opts.DBPath, opts.TokenFile); err != nil {
			return nil, fmt.Errorf("broker migrate: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(opts.DBPath), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opts.WorkspaceRoot, 0o750); err != nil {
		return nil, err
	}

	store, err := cache.Open(opts.DBPath)
	if err != nil {
		return nil, err
	}

	// Load token: prefer token file, fall back to DB meta (legacy), then write file.
	tok, err := ReadTokenFile(opts.TokenFile)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	if tok == "" {
		if dbTok, err := store.DeviceToken(context.Background()); err == nil && dbTok != "" {
			tok = dbTok
			if err := WriteTokenFile(opts.TokenFile, tok); err != nil {
				opts.Logger.Warn("broker: could not write token file", "err", err)
			}
		}
	} else {
		// Keep DB meta in sync for engine/sync code paths that read Store.DeviceToken.
		_ = store.SetDeviceToken(context.Background(), tok)
	}

	cl := studentapi.New(opts.BaseURL, tok)
	loop := studentsync.New(cl, store)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		opts:     opts,
		log:      opts.Logger,
		store:    store,
		client:   cl,
		syncL:    loop,
		sessions: make(map[string]*engine.Session),
		doneReqs: make(map[string]json.RawMessage),
		ctx:      ctx,
		cancel:   cancel,
	}
	return s, nil
}

// Store exposes the cache for tests. Production TUI must not use this.
func (s *Server) Store() *cache.Store { return s.store }

// Client exposes the API client for tests. Production TUI must not use this.
func (s *Server) Client() *studentapi.Client { return s.client }

// Close stops the listener and releases resources.
func (s *Server) Close() error {
	s.cancel()
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.wg.Wait()
	s.mu.Lock()
	s.sessions = nil
	s.mu.Unlock()
	return s.store.Close()
}

// ListenAndServe binds the Unix socket and serves until ctx cancel or Close.
func (s *Server) ListenAndServe() error {
	if err := os.MkdirAll(filepath.Dir(s.opts.SocketPath), 0o755); err != nil {
		return err
	}
	// Remove stale socket.
	_ = os.Remove(s.opts.SocketPath)

	ln, err := net.Listen("unix", s.opts.SocketPath)
	if err != nil {
		return fmt.Errorf("broker listen: %w", err)
	}
	// World-connectable socket: authorization is SO_PEERCRED, not filesystem
	// ACLs. Stale login sessions often lack the students supplementary group
	// until re-login; 0666 keeps the TUI reachable while peer checks remain.
	if err := os.Chmod(s.opts.SocketPath, 0o666); err != nil {
		_ = ln.Close()
		return err
	}
	sockGroup := s.opts.SocketGroup
	if sockGroup == "" {
		sockGroup = s.opts.AllowedGroup
	}
	if sockGroup != "" && sockGroup != "-" {
		if err := chownSocketGroup(s.opts.SocketPath, sockGroup); err != nil {
			s.log.Warn("broker: could not chown socket group", "group", sockGroup, "err", err)
		}
	}
	s.ln = ln
	s.log.Info("broker listening", "socket", s.opts.SocketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			// Transient accept errors: brief backoff then continue.
			time.Sleep(10 * time.Millisecond)
			continue
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.serveConn(c)
		}(conn)
	}
}

func (s *Server) serveConn(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Minute))

	if !s.opts.SkipPeerCred {
		cred, err := peerCredFromConn(c)
		if err != nil {
			s.log.Warn("broker: peercred failed", "err", err)
			s.writeErr(c, "", "unauthorized: peer credentials unavailable")
			return
		}
		if !s.authorize(cred) {
			s.log.Warn("broker: peer rejected", "uid", cred.UID, "gid", cred.GID, "pid", cred.PID)
			s.writeErr(c, "", fmt.Sprintf("unauthorized: uid %d not allowed", cred.UID))
			return
		}
	}

	r := bufio.NewReader(c)
	for {
		_ = c.SetDeadline(time.Now().Add(5 * time.Minute))
		env, err := Decode(r)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || isEOF(err) {
				return
			}
			s.writeErr(c, "", "bad request: "+err.Error())
			return
		}
		resp := s.handle(env)
		if err := Encode(c, resp); err != nil {
			return
		}
	}
}

func isEOF(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) ||
		strings.Contains(err.Error(), "EOF") ||
		errors.Is(err, os.ErrClosed))
}

func (s *Server) authorize(cred *PeerCred) bool {
	if cred == nil {
		return false
	}
	// Root always allowed (systemd helpers / health).
	if cred.UID == 0 {
		return true
	}
	// Same UID as broker process.
	if int(cred.UID) == os.Getuid() {
		return true
	}
	if slices.Contains(s.opts.AllowedUIDs, cred.UID) {
		return true
	}
	if s.opts.AllowedGroup != "" && s.opts.AllowedGroup != "-" {
		if groupHasUID(s.opts.AllowedGroup, cred.UID) {
			return true
		}
	}
	// If no explicit allow list and group missing on this host, allow same-user
	// only (already checked). Reject others.
	return false
}

func (s *Server) handle(env Envelope) Envelope {
	reqID := env.RequestID
	if reqID == "" {
		reqID = uuid.NewString()
	}
	if env.SchemaVersion != 0 && env.SchemaVersion != SchemaVersion {
		return s.errEnv(reqID, fmt.Sprintf("unsupported schemaVersion %d", env.SchemaVersion))
	}

	// Idempotent replay for complete/open when request id was recorded.
	if env.Type == TypeComplete || env.Type == TypeOpenSession {
		s.mu.Lock()
		if prev, ok := s.doneReqs[reqID]; ok {
			s.mu.Unlock()
			okb := true
			return Envelope{
				SchemaVersion: SchemaVersion,
				Type:          TypeResponse,
				RequestID:     reqID,
				OK:            &okb,
				Payload:       prev,
			}
		}
		s.mu.Unlock()
	}

	var (
		payload any
		err     error
	)
	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	defer cancel()

	switch env.Type {
	case TypeHealth:
		payload, err = s.health(ctx)
	case TypePair:
		var req PairRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.pair(ctx, req)
		}
	case TypeProfile:
		payload, err = s.profile(ctx)
	case TypeSyncWork:
		payload, err = s.syncWork(ctx)
	case TypeListWork:
		payload, err = s.listWork(ctx)
	case TypeOpenSession:
		var req OpenSessionRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			if req.RequestID == "" {
				req.RequestID = reqID
			}
			payload, err = s.openSession(ctx, req)
			if err == nil {
				s.remember(reqID, payload)
			}
		}
	case TypeRunCommand:
		var req RunCommandRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.runCommand(ctx, req)
		}
	case TypeTypeRune:
		var req TypeRuneRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.typeRune(ctx, req)
		}
	case TypeTypeBackspace:
		var req TypeBackspaceRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.typeBackspace(ctx, req)
		}
	case TypeVerify:
		var req SessionRef
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.verify(ctx, req)
		}
	case TypeComplete:
		var req SessionRef
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.complete(ctx, req)
			if err == nil {
				s.remember(reqID, payload)
			}
		}
	case TypeTutor:
		var req TutorRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.tutor(ctx, req)
		}
	case TypeStatus:
		var req StatusRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.status(ctx, req)
		}
	case TypePause:
		var req SessionRef
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.pause(ctx, req)
		}
	case TypeTerminalWrite:
		var req TerminalWriteRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.terminalWrite(ctx, req)
		}
	case TypeTerminalRead:
		var req TerminalReadRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.terminalRead(ctx, req)
		}
	case TypeTerminalResize:
		var req TerminalResizeRequest
		if err = UnmarshalPayload(env, &req); err == nil {
			payload, err = s.terminalResize(ctx, req)
		}
	default:
		err = fmt.Errorf("unknown method %q", env.Type)
	}

	if err != nil {
		return s.errEnv(reqID, err.Error())
	}
	raw, mErr := MarshalPayload(payload)
	if mErr != nil {
		return s.errEnv(reqID, mErr.Error())
	}
	// Defense in depth: never emit a device token field.
	if containsTokenLeak(raw) {
		s.log.Error("broker: refused to emit payload that looks like a token leak")
		return s.errEnv(reqID, "internal error: refused token leak")
	}
	ok := true
	return Envelope{
		SchemaVersion: SchemaVersion,
		Type:          TypeResponse,
		RequestID:     reqID,
		OK:            &ok,
		Payload:       raw,
	}
}

func (s *Server) remember(reqID string, payload any) {
	if reqID == "" {
		return
	}
	raw, err := MarshalPayload(payload)
	if err != nil {
		return
	}
	s.mu.Lock()
	// Bound memory.
	if len(s.doneReqs) > 256 {
		s.doneReqs = make(map[string]json.RawMessage)
	}
	s.doneReqs[reqID] = raw
	s.mu.Unlock()
}

func (s *Server) errEnv(reqID, msg string) Envelope {
	ok := false
	return Envelope{
		SchemaVersion: SchemaVersion,
		Type:          TypeError,
		RequestID:     reqID,
		OK:            &ok,
		Error:         msg,
	}
}

func (s *Server) writeErr(c net.Conn, reqID, msg string) {
	_ = Encode(c, s.errEnv(reqID, msg))
}

func containsTokenLeak(raw json.RawMessage) bool {
	// Heuristic: JSON object keys that must never appear in IPC responses.
	s := string(raw)
	return strings.Contains(s, `"token"`) ||
		strings.Contains(s, `"deviceToken"`) ||
		strings.Contains(s, `"device_token"`)
}

// --- handlers ----------------------------------------------------------------

func (s *Server) loadToken(ctx context.Context) (string, error) {
	tok, err := ReadTokenFile(s.opts.TokenFile)
	if err != nil {
		return "", err
	}
	if tok == "" {
		tok, err = s.store.DeviceToken(ctx)
		if err != nil {
			return "", err
		}
	}
	if tok != "" {
		s.client.SetToken(tok)
	}
	return tok, nil
}

func (s *Server) health(ctx context.Context) (HealthResponse, error) {
	tok, _ := s.loadToken(ctx)
	pending, _ := s.store.GetPendingSync(ctx)
	h := HealthResponse{
		OK:               true,
		Version:          s.version(),
		Paired:           tok != "",
		BaseURL:          s.opts.BaseURL,
		SandboxOK:        sandbox.Available(),
		AllowUnsandboxed: s.opts.AllowUnsandboxed,
		SupportedKinds:   activities.SupportedKinds(),
		RunnerVersion:    activities.RunnerVersion,
	}
	if pending != nil {
		h.PendingEvents = pending.EventCount
		h.PendingCompletes = pending.CompleteCount
	}
	if !h.SandboxOK && !s.opts.AllowUnsandboxed {
		h.Message = "bubblewrap missing; terminal activities blocked"
	}
	return h, nil
}

func (s *Server) version() string {
	if s.opts.Version != "" {
		return s.opts.Version
	}
	return versionpkg.String()
}

func (s *Server) pair(ctx context.Context, req PairRequest) (PairResponse, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return PairResponse{}, fmt.Errorf("pairing code required")
	}
	name := req.DeviceName
	if name == "" {
		name = s.opts.DeviceName
	}
	pair, err := s.client.Pair(ctx, code, name)
	if err != nil {
		return PairResponse{}, err
	}
	// Store token only in token file + broker DB — never return it.
	if err := WriteTokenFile(s.opts.TokenFile, pair.Token); err != nil {
		return PairResponse{}, fmt.Errorf("store token: %w", err)
	}
	if err := s.store.SetDeviceToken(ctx, pair.Token); err != nil {
		return PairResponse{}, err
	}
	if err := s.store.SetDeviceIdentity(ctx, pair.DeviceID, pair.Student.ID, pair.Device.Name); err != nil {
		return PairResponse{}, err
	}
	s.client.SetToken(pair.Token)
	return PairResponse{
		DeviceID:   pair.DeviceID,
		DeviceName: pair.Device.Name,
		StudentID:  pair.Student.ID,
		FirstName:  pair.Student.FirstName,
		LastName:   pair.Student.LastName,
	}, nil
}

func (s *Server) profile(ctx context.Context) (ProfileResponse, error) {
	tok, err := s.loadToken(ctx)
	if err != nil {
		return ProfileResponse{}, err
	}
	if tok == "" {
		return ProfileResponse{
			Paired:         false,
			SupportedKinds: activities.SupportedKinds(),
			RunnerVersion:  activities.RunnerVersion,
			AppVersion:     s.version(),
		}, nil
	}
	// Prefer local meta (works offline).
	deviceID, _ := s.store.GetMeta(ctx, "device_id")
	studentID, _ := s.store.GetMeta(ctx, "student_id")
	deviceName, _ := s.store.GetMeta(ctx, "device_name")
	out := ProfileResponse{
		Paired:         true,
		DeviceID:       deviceID,
		DeviceName:     deviceName,
		StudentID:      studentID,
		SupportedKinds: activities.SupportedKinds(),
		RunnerVersion:  activities.RunnerVersion,
		AppVersion:     s.version(),
	}
	if !s.opts.Offline {
		if p, err := s.client.Profile(ctx); err == nil && p != nil {
			out.DeviceID = p.DeviceID
			out.DeviceName = p.DeviceName
			out.StudentID = p.Student.ID
			out.FirstName = p.Student.FirstName
			out.LastName = p.Student.LastName
		}
	}
	return out, nil
}

func (s *Server) syncWork(ctx context.Context) (SyncWorkResponse, error) {
	if s.opts.Offline {
		return SyncWorkResponse{Status: string(studentsync.StatusOffline)}, nil
	}
	if _, err := s.loadToken(ctx); err != nil {
		return SyncWorkResponse{}, err
	}
	res := s.syncL.SyncOnce(ctx)
	return SyncResultFrom(res), nil
}

func (s *Server) listWork(ctx context.Context) (ListWorkResponse, error) {
	items, err := s.store.ListWork(ctx)
	if err != nil {
		return ListWorkResponse{}, err
	}
	return ListWorkResponse{Items: items}, nil
}

func (s *Server) newEngine() (*engine.Engine, error) {
	return engine.New(engine.Options{
		Client:           s.client,
		Store:            s.store,
		WorkspaceRoot:    s.opts.WorkspaceRoot,
		Offline:          s.opts.Offline,
		UseSandbox:       s.opts.UseSandbox || !s.opts.AllowUnsandboxed,
		AllowUnsandboxed: s.opts.AllowUnsandboxed,
		Sync:             s.syncL,
	})
}

func (s *Server) openSession(ctx context.Context, req OpenSessionRequest) (OpenSessionResponse, error) {
	if strings.TrimSpace(req.AssignmentID) == "" {
		return OpenSessionResponse{}, fmt.Errorf("assignmentId required")
	}
	if _, err := s.loadToken(ctx); err != nil {
		return OpenSessionResponse{}, err
	}
	eng, err := s.newEngine()
	if err != nil {
		return OpenSessionResponse{}, err
	}
	sess, err := eng.OpenSession(ctx, req.AssignmentID)
	if err != nil {
		return OpenSessionResponse{}, err
	}
	snap := sess.Snapshot()
	s.mu.Lock()
	s.sessions[snap.ClientSessionID] = sess
	s.mu.Unlock()
	return OpenSessionResponse{
		ClientSessionID: snap.ClientSessionID,
		Snapshot:        snap,
	}, nil
}

func (s *Server) getSession(id string) (*engine.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	if sess == nil {
		return nil, fmt.Errorf("unknown session %q", id)
	}
	return sess, nil
}

func (s *Server) runCommand(ctx context.Context, req RunCommandRequest) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	if err := sess.RunLine(ctx, req.Line); err != nil {
		return SnapshotResponse{Snapshot: sess.Snapshot()}, err
	}
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

func (s *Server) typeRune(ctx context.Context, req TypeRuneRequest) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	r, size := utf8.DecodeRuneInString(req.Rune)
	if r == utf8.RuneError && size == 0 {
		return SnapshotResponse{}, fmt.Errorf("empty rune")
	}
	if err := sess.TypeRune(ctx, r); err != nil {
		return SnapshotResponse{Snapshot: sess.Snapshot()}, err
	}
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

func (s *Server) typeBackspace(ctx context.Context, req TypeBackspaceRequest) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	if err := sess.TypeBackspace(ctx); err != nil {
		return SnapshotResponse{Snapshot: sess.Snapshot()}, err
	}
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

func (s *Server) verify(ctx context.Context, req SessionRef) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	_ = sess.Verify(ctx)
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

func (s *Server) complete(ctx context.Context, req SessionRef) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	if err := sess.Complete(ctx); err != nil {
		return SnapshotResponse{Snapshot: sess.Snapshot()}, err
	}
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

func (s *Server) tutor(ctx context.Context, req TutorRequest) (TutorResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return TutorResponse{}, err
	}
	snap := sess.Snapshot()
	hint := ""
	if !s.opts.Offline && snap.ServerSessionID != "" {
		if _, err := s.loadToken(ctx); err == nil {
			msg := req.Message
			if msg == "" {
				msg = "Need a hint for the current task."
			}
			if resp, err := s.client.TutorMessage(ctx, snap.ServerSessionID, msg); err == nil && resp != nil && resp.Reply != "" {
				hint = resp.Reply
			}
		}
	}
	if hint == "" {
		hint = sess.LocalHint()
	}
	sess.SetTutorHint(hint)
	return TutorResponse{Hint: hint, Snapshot: sess.Snapshot()}, nil
}

func (s *Server) status(ctx context.Context, req StatusRequest) (StatusResponse, error) {
	h, err := s.health(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	out := StatusResponse{Health: h, Sync: string(s.syncL.Status())}
	if req.ClientSessionID != "" {
		if sess, err := s.getSession(req.ClientSessionID); err == nil {
			snap := sess.Snapshot()
			out.Snapshot = &snap
		}
	}
	return out, nil
}

func (s *Server) pause(ctx context.Context, req SessionRef) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	_ = sess.Pause(ctx)
	snap := sess.Snapshot()
	s.mu.Lock()
	delete(s.sessions, req.ClientSessionID)
	s.mu.Unlock()
	return SnapshotResponse{Snapshot: snap}, nil
}

func (s *Server) terminalWrite(ctx context.Context, req TerminalWriteRequest) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	if err := sess.WriteTerminal(ctx, []byte(req.Data)); err != nil {
		return SnapshotResponse{Snapshot: sess.Snapshot()}, err
	}
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

func (s *Server) terminalRead(ctx context.Context, req TerminalReadRequest) (TerminalReadResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return TerminalReadResponse{}, err
	}
	_ = ctx
	snap := sess.Snapshot()
	return TerminalReadResponse{
		Screen:   sess.TerminalScreen(),
		Snapshot: snap,
	}, nil
}

func (s *Server) terminalResize(ctx context.Context, req TerminalResizeRequest) (SnapshotResponse, error) {
	sess, err := s.getSession(req.ClientSessionID)
	if err != nil {
		return SnapshotResponse{}, err
	}
	_ = ctx
	if err := sess.ResizeTerminal(req.Rows, req.Cols); err != nil {
		return SnapshotResponse{Snapshot: sess.Snapshot()}, err
	}
	return SnapshotResponse{Snapshot: sess.Snapshot()}, nil
}

// ServeBackground starts ListenAndServe on a goroutine and waits until the
// socket accepts connections (or timeout).
func ServeBackground(s *Server, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe()
	}()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(s.opts.SocketPath); err == nil {
			// Probe connect.
			c, err := net.DialTimeout("unix", s.opts.SocketPath, 100*time.Millisecond)
			if err == nil {
				_ = c.Close()
				return nil
			}
		}
		select {
		case err := <-errCh:
			if err != nil {
				return err
			}
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("broker socket not ready: %s", s.opts.SocketPath)
}
