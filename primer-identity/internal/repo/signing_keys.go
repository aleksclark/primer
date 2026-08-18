package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const signingKeyColumns = `id, kid, alg, key_version, public_jwk, sealed_private_key, status, not_before, not_after, created_at, activated_at, retired_at, destroyed_at`

// SigningKeyRecord is the persistence row, including sealed private bytes.
// Sealed material is never included in JSON or formatted output.
type SigningKeyRecord struct {
	ID               uuid.UUID        `json:"id"`
	Kid              string           `json:"kid"`
	Alg              string           `json:"alg"`
	KeyVersion       int              `json:"key_version"`
	PublicJWK        domain.PublicJWK `json:"public_jwk"`
	SealedPrivateKey []byte           `json:"-"`
	Status           string           `json:"status"`
	NotBefore        time.Time        `json:"not_before"`
	NotAfter         *time.Time       `json:"not_after,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	ActivatedAt      *time.Time       `json:"activated_at,omitempty"`
	RetiredAt        *time.Time       `json:"retired_at,omitempty"`
	DestroyedAt      *time.Time       `json:"destroyed_at,omitempty"`
}

func (r SigningKeyRecord) String() string {
	return fmt.Sprintf("signing-key-record kid=%s status=%s alg=%s sealed_len=%d", r.Kid, r.Status, r.Alg, len(r.SealedPrivateKey))
}

func (r SigningKeyRecord) GoString() string { return r.String() }

// Format intentionally ignores the requested verb and flags so copied values
// and pointers cannot fall back to fmt's raw struct/byte-slice formatting.
func (r SigningKeyRecord) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, r.String())
}

// MarshalJSON emits the public signing-key projection and refuses to expose
// sealed private material through future field additions to this record.
func (r SigningKeyRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Public())
}

// Public returns the safe domain projection without sealed bytes.
func (r SigningKeyRecord) Public() domain.SigningKey {
	return domain.SigningKey{
		ID:          r.ID,
		Kid:         r.Kid,
		Alg:         r.Alg,
		KeyVersion:  r.KeyVersion,
		PublicJWK:   r.PublicJWK,
		Status:      r.Status,
		NotBefore:   r.NotBefore,
		NotAfter:    r.NotAfter,
		CreatedAt:   r.CreatedAt,
		ActivatedAt: r.ActivatedAt,
		RetiredAt:   r.RetiredAt,
		DestroyedAt: r.DestroyedAt,
	}
}

// InsertSigningKey persists a generated, already-sealed signing key.
// IB2 allows only next and active inserts; retired/destroyed writes wait for IB7.
func InsertSigningKey(ctx context.Context, q Querier, rec SigningKeyRecord) (*SigningKeyRecord, error) {
	if err := domain.ValidateSigningKid(rec.Kid); err != nil {
		return nil, wrapf("insert signing key", err)
	}
	if rec.Alg != domain.SigningAlgES256 {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: alg must be ES256", domain.ErrInvalid))
	}
	if rec.KeyVersion <= 0 {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: key_version must be positive", domain.ErrInvalid))
	}
	if rec.Status != domain.SigningKeyStatusActive && rec.Status != domain.SigningKeyStatusNext {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: IB2 insert allows only next or active", domain.ErrInvalid))
	}
	if rec.PublicJWK.Kid != rec.Kid {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: public kid mismatch", domain.ErrInvalid))
	}
	if _, err := rec.PublicJWK.ECDSAPublic(); err != nil {
		return nil, wrapf("insert signing key", err)
	}
	if len(rec.SealedPrivateKey) == 0 || len(rec.SealedPrivateKey) > domain.MaxSealedPrivate {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: sealed private length", domain.ErrInvalid))
	}
	if rec.Status == domain.SigningKeyStatusActive && rec.ActivatedAt == nil {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: active requires activated_at", domain.ErrInvalid))
	}
	if rec.Status == domain.SigningKeyStatusNext && rec.ActivatedAt != nil {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: next must not have activated_at", domain.ErrInvalid))
	}
	publicJSON, err := rec.PublicJWK.MarshalJSON()
	if err != nil {
		return nil, wrapf("insert signing key", err)
	}
	notBefore := rec.NotBefore
	if notBefore.IsZero() {
		notBefore = time.Now().UTC()
	}
	if rec.NotAfter != nil && !rec.NotAfter.After(notBefore) {
		return nil, wrapf("insert signing key", fmt.Errorf("%w: not_after precedes not_before", domain.ErrInvalid))
	}
	const sqlStr = `
INSERT INTO signing_keys (kid, alg, key_version, public_jwk, sealed_private_key, status, not_before, not_after, created_at, activated_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $7, $9)
RETURNING ` + signingKeyColumns
	got, err := scanSigningKey(q.QueryRow(ctx, sqlStr, rec.Kid, rec.Alg, rec.KeyVersion, publicJSON, rec.SealedPrivateKey, rec.Status, notBefore, rec.NotAfter, rec.ActivatedAt))
	if err != nil {
		return nil, wrapf("insert signing key", err)
	}
	return got, nil
}

// GetSigningKeyByKid loads one row by kid, including sealed bytes.
func GetSigningKeyByKid(ctx context.Context, q Querier, kid string) (*SigningKeyRecord, error) {
	if err := domain.ValidateSigningKid(kid); err != nil {
		return nil, wrapf("get signing key", err)
	}
	const sqlStr = `SELECT ` + signingKeyColumns + ` FROM signing_keys WHERE kid = $1`
	got, err := scanSigningKey(q.QueryRow(ctx, sqlStr, kid))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("get signing key", domain.ErrNotFound)
		}
		return nil, wrapf("get signing key", err)
	}
	return got, nil
}

// GetSigningKeyByKidForShare loads and validates one complete row while holding
// PostgreSQL's row-level FOR SHARE lock until the caller commits or rolls back.
func GetSigningKeyByKidForShare(ctx context.Context, tx pgx.Tx, kid string) (*SigningKeyRecord, error) {
	if err := domain.ValidateSigningKid(kid); err != nil {
		return nil, wrapf("get signing key for share", err)
	}
	const sqlStr = `SELECT ` + signingKeyColumns + ` FROM signing_keys WHERE kid = $1 FOR SHARE`
	got, err := scanSigningKey(tx.QueryRow(ctx, sqlStr, kid))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("get signing key for share", domain.ErrNotFound)
		}
		return nil, wrapf("get signing key for share", err)
	}
	return got, nil
}

// ListSigningKeysByStatus returns every row with the given status, locked when
// the caller is inside a transaction that requested FOR UPDATE via lock.
func ListSigningKeysByStatus(ctx context.Context, q Querier, status string, forUpdate bool) ([]SigningKeyRecord, error) {
	if err := domain.ValidateSigningKeyStatus(status); err != nil {
		return nil, wrapf("list signing keys", err)
	}
	sqlStr := `SELECT ` + signingKeyColumns + ` FROM signing_keys WHERE status = $1 ORDER BY created_at ASC, kid ASC`
	if forUpdate {
		sqlStr += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, sqlStr, status)
	if err != nil {
		return nil, wrapf("list signing keys", err)
	}
	defer rows.Close()
	var out []SigningKeyRecord
	for rows.Next() {
		got, err := scanSigningKey(rows)
		if err != nil {
			return nil, wrapf("list signing keys", err)
		}
		out = append(out, *got)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapf("list signing keys", err)
	}
	if out == nil {
		out = []SigningKeyRecord{}
	}
	return out, nil
}

// ListSigningKeysByStatusForShare returns every row with the given status while
// holding PostgreSQL's row-level FOR SHARE lock until the caller commits or
// rolls back. Use this from an existing transaction so issuance does not open
// a nested pool checkout.
func ListSigningKeysByStatusForShare(ctx context.Context, tx pgx.Tx, status string) ([]SigningKeyRecord, error) {
	if err := domain.ValidateSigningKeyStatus(status); err != nil {
		return nil, wrapf("list signing keys for share", err)
	}
	const sqlStr = `SELECT ` + signingKeyColumns + ` FROM signing_keys WHERE status = $1 ORDER BY created_at ASC, kid ASC FOR SHARE`
	rows, err := tx.Query(ctx, sqlStr, status)
	if err != nil {
		return nil, wrapf("list signing keys for share", err)
	}
	defer rows.Close()
	var out []SigningKeyRecord
	for rows.Next() {
		got, err := scanSigningKey(rows)
		if err != nil {
			return nil, wrapf("list signing keys for share", err)
		}
		out = append(out, *got)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapf("list signing keys for share", err)
	}
	if out == nil {
		out = []SigningKeyRecord{}
	}
	return out, nil
}

// CountSigningKeysByStatus counts rows in one lifecycle state.
func CountSigningKeysByStatus(ctx context.Context, q Querier, status string) (int, error) {
	if err := domain.ValidateSigningKeyStatus(status); err != nil {
		return 0, wrapf("count signing keys", err)
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = $1`, status).Scan(&n)
	if err != nil {
		return 0, wrapf("count signing keys", err)
	}
	return n, nil
}

// ActivateSigningKey promotes an authenticated next row to the single active key.
func ActivateSigningKey(ctx context.Context, q Querier, kid string, activatedAt time.Time) (*SigningKeyRecord, error) {
	if err := domain.ValidateSigningKid(kid); err != nil {
		return nil, wrapf("activate signing key", err)
	}
	if activatedAt.IsZero() {
		return nil, wrapf("activate signing key", fmt.Errorf("%w: activated_at is required", domain.ErrInvalid))
	}
	const sqlStr = `
UPDATE signing_keys
SET status = $2, activated_at = $3
WHERE kid = $1 AND status = $4
RETURNING ` + signingKeyColumns
	got, err := scanSigningKey(q.QueryRow(ctx, sqlStr, kid, domain.SigningKeyStatusActive, activatedAt, domain.SigningKeyStatusNext))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("activate signing key", domain.ErrNotFound)
		}
		return nil, wrapf("activate signing key", err)
	}
	return got, nil
}

func scanSigningKey(row scannable) (*SigningKeyRecord, error) {
	var rec SigningKeyRecord
	var publicRaw []byte
	err := row.Scan(
		&rec.ID,
		&rec.Kid,
		&rec.Alg,
		&rec.KeyVersion,
		&publicRaw,
		&rec.SealedPrivateKey,
		&rec.Status,
		&rec.NotBefore,
		&rec.NotAfter,
		&rec.CreatedAt,
		&rec.ActivatedAt,
		&rec.RetiredAt,
		&rec.DestroyedAt,
	)
	if err != nil {
		return nil, err
	}
	jwk, err := domain.ParsePublicJWK(publicRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: public jwk", domain.ErrCorruptSigner)
	}
	rec.PublicJWK = jwk
	if err := validateLoadedSigningKey(rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func validateLoadedSigningKey(rec SigningKeyRecord) error {
	if rec.ID == uuid.Nil {
		return fmt.Errorf("%w: missing signing key id", domain.ErrCorruptSigner)
	}
	if err := domain.ValidateSigningKid(rec.Kid); err != nil {
		return fmt.Errorf("%w: kid", domain.ErrCorruptSigner)
	}
	if err := domain.ValidateSigningKeyStatus(rec.Status); err != nil {
		return fmt.Errorf("%w: status", domain.ErrCorruptSigner)
	}
	if rec.Alg != domain.SigningAlgES256 {
		return fmt.Errorf("%w: alg must be ES256", domain.ErrCorruptSigner)
	}
	if rec.KeyVersion <= 0 {
		return fmt.Errorf("%w: key_version must be positive", domain.ErrCorruptSigner)
	}
	if rec.PublicJWK.Kid != rec.Kid {
		return fmt.Errorf("%w: public kid mismatch", domain.ErrCorruptSigner)
	}
	if rec.PublicJWK.Alg != rec.Alg {
		return fmt.Errorf("%w: public alg mismatch", domain.ErrCorruptSigner)
	}
	switch rec.Status {
	case domain.SigningKeyStatusDestroyed:
		if rec.SealedPrivateKey != nil {
			return fmt.Errorf("%w: destroyed key must not retain sealed private material", domain.ErrCorruptSigner)
		}
	default:
		if len(rec.SealedPrivateKey) == 0 || len(rec.SealedPrivateKey) > domain.MaxSealedPrivate {
			return fmt.Errorf("%w: sealed private length", domain.ErrCorruptSigner)
		}
	}
	if rec.CreatedAt.IsZero() || rec.NotBefore.IsZero() {
		return fmt.Errorf("%w: timestamps must be set", domain.ErrCorruptSigner)
	}
	if rec.NotAfter != nil && !rec.NotAfter.After(rec.NotBefore) {
		return fmt.Errorf("%w: not_after precedes not_before", domain.ErrCorruptSigner)
	}
	return nil
}
