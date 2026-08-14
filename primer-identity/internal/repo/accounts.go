package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const accountColumns = `id, status, display_name, primary_email, primary_email_verified_at, created_at, updated_at`

// CreateAccount inserts a new account with a server-generated UUID sub.
// Status defaults to active. Email is optional and never treated as unique.
func CreateAccount(ctx context.Context, q Querier, in domain.CreateAccountInput) (*domain.Account, error) {
	if err := domain.ValidateCreateAccount(in); err != nil {
		return nil, wrapf("create account", err)
	}
	status := in.Status
	if status == "" {
		status = domain.AccountStatusActive
	}
	const sqlStr = `
INSERT INTO accounts (status, display_name, primary_email)
VALUES ($1, $2, $3)
RETURNING ` + accountColumns

	row := q.QueryRow(ctx, sqlStr, status, in.DisplayName, in.PrimaryEmail)
	acct, err := scanAccount(row)
	if err != nil {
		return nil, wrapf("create account", err)
	}
	return acct, nil
}

// GetAccount loads an account by UUID sub.
func GetAccount(ctx context.Context, q Querier, id uuid.UUID) (*domain.Account, error) {
	if id == uuid.Nil {
		return nil, wrapf("get account", fmt.Errorf("%w: nil id", domain.ErrInvalid))
	}
	const sqlStr = `SELECT ` + accountColumns + ` FROM accounts WHERE id = $1`
	acct, err := scanAccount(q.QueryRow(ctx, sqlStr, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("get account", domain.ErrNotFound)
		}
		return nil, wrapf("get account", err)
	}
	return acct, nil
}

// LockAccount sets status to locked. Later token-mint phases refuse locked accounts.
func LockAccount(ctx context.Context, q Querier, id uuid.UUID) (*domain.Account, error) {
	if id == uuid.Nil {
		return nil, wrapf("lock account", fmt.Errorf("%w: nil id", domain.ErrInvalid))
	}
	const sqlStr = `
UPDATE accounts
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING ` + accountColumns
	acct, err := scanAccount(q.QueryRow(ctx, sqlStr, id, domain.AccountStatusLocked))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("lock account", domain.ErrNotFound)
		}
		return nil, wrapf("lock account", err)
	}
	return acct, nil
}

// ListAccountsByEmail returns all accounts whose primary_email matches (case-insensitive).
// This is non-authoritative recovery/search only — multiple rows are expected and
// there is no single canonical login-by-email. Never use this to auto-link providers.
func ListAccountsByEmail(ctx context.Context, q Querier, email string) ([]domain.Account, error) {
	if err := domain.ValidateEmail(email); err != nil {
		return nil, wrapf("list accounts by email", err)
	}
	const sqlStr = `
SELECT ` + accountColumns + `
FROM accounts
WHERE primary_email IS NOT NULL AND lower(primary_email) = lower($1)
ORDER BY created_at ASC, id ASC`
	rows, err := q.Query(ctx, sqlStr, email)
	if err != nil {
		return nil, wrapf("list accounts by email", err)
	}
	defer rows.Close()

	var out []domain.Account
	for rows.Next() {
		acct, err := scanAccount(rows)
		if err != nil {
			return nil, wrapf("list accounts by email", err)
		}
		out = append(out, *acct)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapf("list accounts by email", err)
	}
	if out == nil {
		out = []domain.Account{}
	}
	return out, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAccount(row scannable) (*domain.Account, error) {
	var a domain.Account
	var email *string
	var verified *time.Time
	err := row.Scan(
		&a.ID,
		&a.Status,
		&a.DisplayName,
		&email,
		&verified,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.PrimaryEmail = email
	a.PrimaryEmailVerifiedAt = verified
	return &a, nil
}
