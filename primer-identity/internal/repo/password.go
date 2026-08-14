package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/password"
)

// SetPassword hashes plaintext with Argon2id and upserts credentials_password.
// Plaintext is never persisted. Re-enables a previously disabled credential.
// Empty and oversize plaintext map to domain.ErrInvalid before KDF.
func SetPassword(ctx context.Context, q Querier, accountID uuid.UUID, plaintext string) error {
	if accountID == uuid.Nil {
		return wrapf("set password", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	hash, err := password.Hash(plaintext)
	if err != nil {
		if errors.Is(err, password.ErrInvalidPassword) {
			return wrapf("set password", fmt.Errorf("%w: %v", domain.ErrInvalid, err))
		}
		return wrapf("set password", err)
	}
	const sqlStr = `
INSERT INTO credentials_password (account_id, password_hash, algorithm, rotated_at, disabled_at)
VALUES ($1, $2, $3, now(), NULL)
ON CONFLICT (account_id) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    algorithm = EXCLUDED.algorithm,
    rotated_at = now(),
    disabled_at = NULL`
	_, err = q.Exec(ctx, sqlStr, accountID, hash, password.AlgorithmArgon2id)
	if err != nil {
		return wrapf("set password", err)
	}
	return nil
}

// DisablePassword marks the password credential disabled (fail-closed on check).
func DisablePassword(ctx context.Context, q Querier, accountID uuid.UUID) error {
	if accountID == uuid.Nil {
		return wrapf("disable password", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	tag, err := q.Exec(ctx,
		`UPDATE credentials_password SET disabled_at = now() WHERE account_id = $1 AND disabled_at IS NULL`,
		accountID,
	)
	if err != nil {
		return wrapf("disable password", err)
	}
	if tag.RowsAffected() == 0 {
		// Either missing or already disabled — treat missing as not found.
		var n int
		_ = q.QueryRow(ctx, `SELECT count(*) FROM credentials_password WHERE account_id = $1`, accountID).Scan(&n)
		if n == 0 {
			return wrapf("disable password", domain.ErrNotFound)
		}
	}
	return nil
}

// CheckPassword verifies plaintext against the stored hash.
// Missing credentials run DummyVerify then return false (non-enumerating).
// Disabled credentials fail closed (false). Malformed hashes fail closed (false).
// Empty/oversize plaintext fails closed (false) without Argon2.
// Does not distinguish wrong password vs missing via error type for auth paths;
// returns (false, nil) on normal negative outcomes and error only on infra failure.
func CheckPassword(ctx context.Context, q Querier, accountID uuid.UUID, plaintext string) (bool, error) {
	// Bound inputs before any DB/KDF work so giant plaintext cannot DoS.
	if plaintext == "" || len(plaintext) > password.MaxPasswordBytes {
		return false, nil
	}
	if accountID == uuid.Nil {
		password.DummyVerify()
		return false, nil
	}
	var hash string
	var disabledAt *time.Time
	err := q.QueryRow(ctx,
		`SELECT password_hash, disabled_at FROM credentials_password WHERE account_id = $1`,
		accountID,
	).Scan(&hash, &disabledAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			password.DummyVerify()
			return false, nil
		}
		return false, wrapf("check password", err)
	}
	if disabledAt != nil {
		// Still burn verify work so disabled vs wrong is not trivially timed out-of-band.
		_, _ = password.Verify(hash, plaintext)
		return false, nil
	}
	ok, verr := password.Verify(hash, plaintext)
	if verr != nil {
		// Malformed stored hash or invalid password: fail closed, do not accept.
		return false, nil
	}
	return ok, nil
}

// GetPasswordCredential loads the credential row (hash included for test/raw checks).
// Not for logging. Callers must never log PasswordHash.
func GetPasswordCredential(ctx context.Context, q Querier, accountID uuid.UUID) (*domain.PasswordCredential, error) {
	if accountID == uuid.Nil {
		return nil, wrapf("get password credential", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	const sqlStr = `
SELECT account_id, password_hash, algorithm, rotated_at, disabled_at
FROM credentials_password WHERE account_id = $1`
	var c domain.PasswordCredential
	err := q.QueryRow(ctx, sqlStr, accountID).Scan(
		&c.AccountID,
		&c.PasswordHash,
		&c.Algorithm,
		&c.RotatedAt,
		&c.DisabledAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("get password credential", domain.ErrNotFound)
		}
		return nil, wrapf("get password credential", err)
	}
	return &c, nil
}
