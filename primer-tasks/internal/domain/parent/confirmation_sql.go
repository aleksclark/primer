package parent

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLConfirmationStore is the durable confirmation boundary.  The table is
// intentionally not created here: schema ownership belongs to the Tasks
// migration track, and a missing table must fail closed rather than silently
// falling back to process memory.
type SQLConfirmationStore struct {
	DB  *pgxpool.Pool
	Now func() time.Time
}

func (s *SQLConfirmationStore) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s *SQLConfirmationStore) Issue(ctx context.Context, p ConfirmationPreview) (ConfirmationPreview, error) {
	if s == nil || s.DB == nil {
		return ConfirmationPreview{}, ErrServiceMissing
	}
	var err error
	p, err = validatePreview(p, s.now())
	if err != nil {
		return ConfirmationPreview{}, err
	}
	action, err := json.Marshal(p.Action)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	_, err = s.DB.Exec(ctx, `
		INSERT INTO parent_confirmation_previews
		(id,tenant_id,actor_id,handle_hash,action_kind,action_digest,action,summary,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)`,
		p.ID, p.TenantID, p.ActorID, handleHash(p.Handle), p.Action.Kind,
		decodeDigest(p.ActionDigest), action, p.Summary, p.ExpiresAt)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	return p, nil
}

func decodeDigest(v string) []byte {
	if len(v) != 64 {
		return make([]byte, 31)
	}
	b, err := hex.DecodeString(v)
	if err != nil {
		return make([]byte, 31)
	}
	return b
}

func (s *SQLConfirmationStore) Consume(ctx context.Context, handle, tenant, actor, digest string) (ConfirmationPreview, error) {
	if s == nil || s.DB == nil {
		return ConfirmationPreview{}, ErrServiceMissing
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	defer tx.Rollback(ctx)

	var p ConfirmationPreview
	var actionJSON []byte
	var storedDigest []byte
	var storedKind string
	var consumedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id,tenant_id,actor_id,action_kind,action_digest,action,summary,expires_at,consumed_at
		FROM parent_confirmation_previews WHERE handle_hash=$1 FOR UPDATE`, handleHash(handle)).Scan(
		&p.ID, &p.TenantID, &p.ActorID, &storedKind, &storedDigest, &actionJSON,
		&p.Summary, &p.ExpiresAt, &consumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmationPreview{}, ErrConfirmationNotFound
	}
	if err != nil {
		return ConfirmationPreview{}, err
	}
	p.Handle = handle
	p.ConsumedAt = consumedAt
	if err := json.Unmarshal(actionJSON, &p.Action); err != nil {
		return ConfirmationPreview{}, ErrConfirmationRejected
	}
	p.ActionDigest = fmt.Sprintf("%x", storedDigest)
	calculated, digestErr := ActionDigest(p.Action)
	if digestErr != nil || storedKind != p.Action.Kind || subtle.ConstantTimeCompare([]byte(calculated), []byte(p.ActionDigest)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationRejected
	}

	now := s.now()
	if p.ConsumedAt != nil {
		return ConfirmationPreview{}, ErrConfirmationReplay
	}
	if !p.ExpiresAt.After(now) {
		return ConfirmationPreview{}, ErrConfirmationExpired
	}
	if subtle.ConstantTimeCompare([]byte(p.TenantID), []byte(tenant)) != 1 || subtle.ConstantTimeCompare([]byte(p.ActorID), []byte(actor)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationForeign
	}
	if subtle.ConstantTimeCompare([]byte(p.ActionDigest), []byte(digest)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationAltered
	}
	// A lock plus this predicate makes consume single-use under concurrent
	// confirmation requests.  The update is the durable state transition.
	updated, updateErr := tx.Exec(ctx, `UPDATE parent_confirmation_previews SET consumed_at=$2 WHERE handle_hash=$1 AND consumed_at IS NULL AND expires_at>$2`, handleHash(handle), now)
	if updateErr != nil {
		return ConfirmationPreview{}, updateErr
	}
	if updated.RowsAffected() != 1 {
		return ConfirmationPreview{}, ErrConfirmationRejected
	}
	if err = tx.Commit(ctx); err != nil {
		return ConfirmationPreview{}, err
	}
	p.ConsumedAt = &now
	return p, nil
}

// ConfirmationTableSQL is documentation for the migration owner and a useful
// contract test fixture.  It is not executed by this package.
const ConfirmationTableSQL = `
CREATE TABLE parent_confirmation_previews (
  id uuid PRIMARY KEY,
  tenant_id text NOT NULL,
  actor_id text NOT NULL,
  handle_hash bytea NOT NULL UNIQUE,
  action_kind text NOT NULL,
  action_digest bytea NOT NULL CHECK (octet_length(action_digest)=32),
  action jsonb NOT NULL,
  summary text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes')
);
CREATE INDEX parent_confirmation_previews_expiry
  ON parent_confirmation_previews (expires_at) WHERE consumed_at IS NULL;
`
