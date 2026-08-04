package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/aleksclark/primer/server/internal/authutil"
	"github.com/aleksclark/primer/server/internal/domain"
)

// ParentSessionTTL is the absolute lifetime of a parent login session.
const ParentSessionTTL = 12 * time.Hour

// HashPassword returns a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// SetEducatorPassword stores a bcrypt password hash on the educator.
func SetEducatorPassword(ctx context.Context, q Querier, educatorID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx,
		`UPDATE educators SET password_hash = $2, updated_at = now() WHERE id = $1`,
		educatorID, hash)
	if err != nil {
		return fmt.Errorf("set educator password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EducatorByEmail loads an educator including the password hash.
func EducatorByEmail(ctx context.Context, q Querier, email string) (*domain.Educator, error) {
	const sqlStr = `
SELECT id, email, name, role, password_hash, created_at, updated_at
FROM educators WHERE lower(email) = lower($1)`
	rows, err := q.Query(ctx, sqlStr, email)
	if err != nil {
		return nil, fmt.Errorf("query educator by email: %w", err)
	}
	ed, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Educator])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan educator: %w", err)
	}
	return &ed, nil
}

// CreateParentSession issues a new session token for the educator.
// Returns the plaintext token once.
func CreateParentSession(ctx context.Context, q Querier, educatorID string, now time.Time) (token string, session *domain.ParentSession, err error) {
	token, hash, err := authutil.NewToken()
	if err != nil {
		return "", nil, err
	}
	expires := now.Add(ParentSessionTTL)
	const sqlStr = `
INSERT INTO parent_sessions (educator_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, educator_id, token_hash, expires_at, created_at`
	rows, err := q.Query(ctx, sqlStr, educatorID, hash, expires)
	if err != nil {
		return "", nil, fmt.Errorf("insert parent session: %w", err)
	}
	sess, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ParentSession])
	if err != nil {
		return "", nil, fmt.Errorf("scan parent session: %w", err)
	}
	return token, &sess, nil
}

// ParentSessionByToken loads a non-expired session and its educator.
func ParentSessionByToken(ctx context.Context, q Querier, token string, now time.Time) (*domain.ParentSession, *domain.Educator, error) {
	hash := authutil.HashToken(token)
	const sqlStr = `
SELECT id, educator_id, token_hash, expires_at, created_at
FROM parent_sessions
WHERE token_hash = $1 AND expires_at > $2`
	rows, err := q.Query(ctx, sqlStr, hash, now)
	if err != nil {
		return nil, nil, fmt.Errorf("query parent session: %w", err)
	}
	sess, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ParentSession])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("scan parent session: %w", err)
	}
	ed, err := Educators.Get(ctx, q, sess.EducatorID)
	if err != nil {
		return nil, nil, err
	}
	return &sess, ed, nil
}

// DeleteParentSession revokes a session by plaintext token.
func DeleteParentSession(ctx context.Context, q Querier, token string) error {
	hash := authutil.HashToken(token)
	_, err := q.Exec(ctx, `DELETE FROM parent_sessions WHERE token_hash = $1`, hash)
	if err != nil {
		return fmt.Errorf("delete parent session: %w", err)
	}
	return nil
}
