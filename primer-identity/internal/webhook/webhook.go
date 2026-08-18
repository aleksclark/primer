// Package webhook receives and durably receipts the signed Stytch webhook
// boundary. It deliberately has no provider or product side effects: workers
// consume committed receipts after the HTTP request has returned.
package webhook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	svix "github.com/svix/svix-webhooks/go"
)

const (
	MaxBodyBytes = 256 * 1024
	Provider     = "stytch_b2b"
)

var ErrInvalidEnvelope = errors.New("invalid webhook envelope")

// Collision reason codes are constrained by the migration and fingerprint contract.
var validReasons = map[string]bool{
	"event_id_reused_new_svix_id": true,
	"svix_id_reused_new_event_id": true,
	"cross_id_collision":          true,
	"body_hash_mismatch":          true,
}

type Config struct {
	Pool      *pgxpool.Pool
	Secret    string
	ProjectID string
	Now       func() time.Time
}

type Handler struct {
	pool      *pgxpool.Pool
	verifier  *svix.Webhook
	projectID string
	now       func() time.Time
}

func NewHandler(cfg Config) (*Handler, error) {
	if cfg.Pool == nil || strings.TrimSpace(cfg.Secret) == "" || strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, errors.New("webhook: pool, secret, and project id are required")
	}
	verifier, err := svix.NewWebhook(cfg.Secret)
	if err != nil {
		return nil, fmt.Errorf("webhook verifier: %w", err)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Handler{pool: cfg.Pool, verifier: verifier, projectID: cfg.ProjectID, now: now}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !validContentType(r.Header.Get("Content-Type")) {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil || len(body) > MaxBodyBytes {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	// Verify the exact bytes read from the request. Never decode and re-encode
	// before this call, and never use VerifyIgnoringTimestamp.
	if err := h.verifier.Verify(body, r.Header); err != nil {
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	env, err := parseEnvelope(body)
	svixID := r.Header.Get("svix-id")
	if err != nil || env.ProjectID != h.projectID || !validID(svixID) {
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	hash := sha256.Sum256(body)
	if err := h.receipt(r.Context(), env, svixID, hash[:]); err != nil {
		http.Error(w, "webhook unavailable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validID(value string) bool {
	return value != "" && len(value) <= 255 && utf8.ValidString(value) && strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) < 0
}

func validContentType(value string) bool {
	if value == "" {
		return false
	}
	parts := strings.Split(value, ";")
	if !strings.EqualFold(strings.TrimSpace(parts[0]), "application/json") {
		return false
	}
	for _, p := range parts[1:] {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 || !strings.EqualFold(kv[0], "charset") || !strings.EqualFold(strings.Trim(strings.TrimSpace(kv[1]), `"`), "utf-8") {
			return false
		}
	}
	return true
}

type envelope struct {
	ProjectID  string `json:"project_id"`
	EventID    string `json:"event_id"`
	Action     string `json:"action"`
	ObjectType string `json:"object_type"`
	Source     string `json:"source"`
	EntityID   string `json:"id"`
	Timestamp  string `json:"timestamp"`
	EventType  string `json:"event_type"`
}

func parseEnvelope(body []byte) (envelope, error) {
	var e envelope
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&e); err != nil {
		return e, ErrInvalidEnvelope
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return e, ErrInvalidEnvelope
	}
	fields := []string{e.ProjectID, e.EventID, e.Action, e.ObjectType, e.Source, e.EntityID, e.Timestamp}
	for _, field := range fields {
		if field == "" || !utf8.ValidString(field) || len(field) > 255 || strings.IndexFunc(field, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			return e, ErrInvalidEnvelope
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil || parsed.IsZero() {
		return e, ErrInvalidEnvelope
	}
	wantType := e.Source + "." + e.ObjectType + "." + e.Action
	if e.EventType == "" {
		e.EventType = wantType
	}
	if e.EventType != wantType {
		return e, ErrInvalidEnvelope
	}
	return e, nil
}

type match struct {
	ID   uuid.UUID
	Hash []byte
	ok   bool
}

func (h *Handler) receipt(ctx context.Context, e envelope, svixID string, bodyHash []byte) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Stable ordered transaction locks serialize both unique identities and
	// avoid a cross-ID deadlock when two collisions arrive concurrently.
	locks := []string{Provider + "|" + e.ProjectID + "|event|" + e.EventID, Provider + "|" + svixID}
	sort.Strings(locks)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0)), pg_advisory_xact_lock(hashtextextended($2, 0))`, locks[0], locks[1])
	if err != nil {
		return err
	}
	em, err := lookup(ctx, tx, `SELECT id,body_sha256 FROM webhook_events WHERE provider=$1 AND provider_project_id=$2 AND provider_event_id=$3 FOR UPDATE`, e.ProjectID, e.EventID)
	if err != nil {
		return err
	}
	sm, err := lookup(ctx, tx, `SELECT id,body_sha256 FROM webhook_events WHERE provider=$1 AND svix_message_id=$2 FOR UPDATE`, svixID)
	if err != nil {
		return err
	}
	if em.ok && sm.ok && em.ID == sm.ID && bytes.Equal(em.Hash, bodyHash) {
		return tx.Commit(ctx)
	}
	if em.ok || sm.ok {
		reason := classify(em, sm, bodyHash)
		semantic := SemanticFingerprint(Provider, e.ProjectID, e.EventID, originalHash(em, sm), bodyHash, reason)
		observation := ObservationFingerprint(semantic, reason, svixID, em, sm)
		var securityID uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO webhook_security_events(event_id_match_webhook_event_id,svix_id_match_webhook_event_id,provider,provider_project_id,presented_event_id,presented_svix_message_id,original_body_sha256,presented_body_sha256,collision_fingerprint,reason_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(collision_fingerprint) DO NOTHING RETURNING id`, nullableID(em), nullableID(sm), Provider, e.ProjectID, e.EventID, svixID, originalHash(em, sm), bodyHash, semantic, reason).Scan(&securityID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT id FROM webhook_security_events WHERE collision_fingerprint=$1`, semantic).Scan(&securityID)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO webhook_security_collision_observations(webhook_security_event_id,observation_fingerprint,reason_code,presented_svix_message_id,event_id_match_webhook_event_id,svix_id_match_webhook_event_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(observation_fingerprint) DO NOTHING`, securityID, observation, reason, svixID, nullableID(em), nullableID(sm))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO webhook_security_alerts(webhook_security_event_id) VALUES($1) ON CONFLICT(webhook_security_event_id) DO NOTHING`, securityID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `INSERT INTO webhook_events(provider,provider_project_id,provider_event_id,svix_message_id,event_type,source,object_type,action,entity_id,event_timestamp,body_sha256,signature_verified_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, Provider, e.ProjectID, e.EventID, svixID, e.EventType, e.Source, e.ObjectType, e.Action, e.EntityID, mustTime(e.Timestamp), bodyHash, h.now().UTC())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func mustTime(s string) time.Time { t, _ := time.Parse(time.RFC3339Nano, s); return t }
func lookup(ctx context.Context, tx pgx.Tx, q string, args ...any) (match, error) {
	var m match
	err := tx.QueryRow(ctx, q, append([]any{Provider}, args...)...).Scan(&m.ID, &m.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	m.ok = true
	return m, nil
}
func nullableID(m match) any {
	if !m.ok {
		return nil
	}
	return m.ID
}
func classify(a, b match, hash []byte) string {
	if a.ok && b.ok && a.ID != b.ID {
		return "cross_id_collision"
	}
	if a.ok && !b.ok {
		if bytes.Equal(a.Hash, hash) {
			return "event_id_reused_new_svix_id"
		}
		return "body_hash_mismatch"
	}
	if !a.ok && b.ok {
		if bytes.Equal(b.Hash, hash) {
			return "svix_id_reused_new_event_id"
		}
		return "body_hash_mismatch"
	}
	return "body_hash_mismatch"
}
func originalHash(a, b match) []byte {
	if a.ok {
		return a.Hash
	}
	return b.Hash
}

func appendPart(dst []byte, value []byte) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(value)))
	dst = append(dst, n[:]...)
	return append(dst, value...)
}
func digest(prefix string, parts ...[]byte) []byte {
	b := []byte(prefix)
	b = append(b, 1)
	for _, p := range parts {
		b = appendPart(b, p)
	}
	s := sha256.Sum256(b)
	return s[:]
}
func SemanticFingerprint(provider, project, eventID string, original, newHash []byte, reason string) []byte {
	return digest("primer.webhook.collision", []byte(provider), []byte(project), []byte(eventID), original, newHash, []byte(reason))
}
func ObservationFingerprint(semantic []byte, reason, svixID string, event, svix match) []byte {
	var a, b []byte
	if event.ok {
		a = event.ID[:]
	}
	if svix.ok {
		b = svix.ID[:]
	}
	return digest("primer.webhook.collision.observation", semantic, []byte(reason), []byte(svixID), a, b)
}
func HexFingerprint(value []byte) string { return hex.EncodeToString(value) }
