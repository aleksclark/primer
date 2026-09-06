package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const stytchMappingColumns = `id, account_id, project_id, organization_id, member_id, created_at, updated_at`

// FindAccountIDByStytchTuple resolves only the exact normalized provider tuple.
// It never consults email, display metadata, or a concatenated subject.
func FindAccountIDByStytchTuple(ctx context.Context, q Querier, principal domain.StytchPrincipal) (uuid.UUID, error) {
	if err := domain.ValidateStytchPrincipal(principal); err != nil {
		return uuid.Nil, wrapf("find account by stytch tuple", err)
	}
	var accountID uuid.UUID
	err := q.QueryRow(ctx, `
SELECT account_id
FROM stytch_mappings
WHERE project_id = $1 AND organization_id = $2 AND member_id = $3`,
		principal.ProjectID, principal.OrganizationID, principal.MemberID,
	).Scan(&accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, wrapf("find account by stytch tuple", domain.ErrNotFound)
		}
		return uuid.Nil, wrapf("find account by stytch tuple", err)
	}
	if accountID == uuid.Nil {
		return uuid.Nil, wrapf("find account by stytch tuple", fmt.Errorf("%w: mapping has nil account", domain.ErrConflict))
	}
	return accountID, nil
}

// CreateStytchMapping inserts a normalized tuple-to-account link. Any tuple or
// account ownership conflict is returned to the caller; it is never resolved
// by email or by silently changing an existing mapping.
func CreateStytchMapping(ctx context.Context, q Querier, accountID uuid.UUID, principal domain.StytchPrincipal) (*domain.StytchMapping, error) {
	if accountID == uuid.Nil {
		return nil, wrapf("create stytch mapping", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	if err := domain.ValidateStytchPrincipal(principal); err != nil {
		return nil, wrapf("create stytch mapping", err)
	}
	mapping, err := scanStytchMapping(q.QueryRow(ctx, `
INSERT INTO stytch_mappings (account_id, project_id, organization_id, member_id)
VALUES ($1, $2, $3, $4)
RETURNING `+stytchMappingColumns,
		accountID, principal.ProjectID, principal.OrganizationID, principal.MemberID))
	if err != nil {
		return nil, wrapf("create stytch mapping", err)
	}
	return mapping, nil
}

// ResolveOrCreateStytchMapping returns the account for an exact tuple. It owns
// a serializable transaction for each bounded attempt. The loser of a
// concurrent first-login race deletes its unused account before re-reading the
// winner, so the tuple converges without orphans.
func ResolveOrCreateStytchMapping(ctx context.Context, pool *pgxpool.Pool, in domain.StytchMappingInput) (*domain.Account, error) {
	if err := domain.ValidateStytchMappingInput(in); err != nil {
		return nil, wrapf("resolve or create stytch mapping", err)
	}
	if pool == nil {
		return nil, wrapf("resolve or create stytch mapping", fmt.Errorf("%w: nil pool", domain.ErrInvalid))
	}

	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, withRetryFailure(wrapf("resolve or create stytch mapping", err), attempt-1, maxAttempts, false)
		}

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, withRetryFailure(wrapf("resolve or create stytch mapping", err), attempt, maxAttempts, false)
		}

		account, bodyErr := resolveOrCreateStytchMappingTx(ctx, tx, in)
		if bodyErr != nil {
			_ = tx.Rollback(context.Background())
			retryable := isRetryableStytchMappingError(bodyErr)
			if retryable && attempt < maxAttempts {
				continue
			}
			return nil, withRetryFailure(bodyErr, attempt, maxAttempts, retryable && attempt == maxAttempts)
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(context.Background())
			retryable := isRetryableStytchMappingError(err)
			if retryable && attempt < maxAttempts {
				continue
			}
			return nil, withRetryFailure(wrapf("resolve or create stytch mapping commit", err), attempt, maxAttempts, retryable && attempt == maxAttempts)
		}
		return account, nil
	}

	return nil, wrapf("resolve or create stytch mapping", fmt.Errorf("%w: retry attempts exhausted", domain.ErrConflict))
}

func isRetryableStytchMappingError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

func resolveOrCreateStytchMappingTx(ctx context.Context, tx pgx.Tx, in domain.StytchMappingInput) (*domain.Account, error) {
	accountID, err := FindAccountIDByStytchTuple(ctx, tx, in.StytchPrincipal)
	if err == nil {
		return GetAccount(ctx, tx, accountID)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, wrapf("resolve or create stytch mapping", err)
	}

	account, err := CreateAccount(ctx, tx, domain.CreateAccountInput{
		DisplayName: in.DisplayName, PrimaryEmail: in.PrimaryEmail,
	})
	if err != nil {
		return nil, wrapf("resolve or create stytch mapping", err)
	}

	mapping, err := scanStytchMapping(tx.QueryRow(ctx, `
INSERT INTO stytch_mappings (account_id, project_id, organization_id, member_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (project_id, organization_id, member_id) DO NOTHING
RETURNING `+stytchMappingColumns,
		account.ID, in.ProjectID, in.OrganizationID, in.MemberID))
	if err == nil {
		if mapping.AccountID != account.ID {
			return nil, cleanupCreatedAccount(ctx, tx, account.ID, fmt.Errorf("%w: mapping account changed during insert", domain.ErrConflict))
		}
		return account, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// The unique tuple was won by a concurrent transaction. Delete the
		// account created by this transaction before returning the winner.
		if cleanupErr := deleteCreatedAccount(ctx, tx, account.ID); cleanupErr != nil {
			return nil, cleanupErr
		}
		winnerID, findErr := FindAccountIDByStytchTuple(ctx, tx, in.StytchPrincipal)
		if findErr != nil {
			return nil, wrapf("resolve or create stytch mapping", fmt.Errorf("%w: concurrent tuple conflict could not be resolved: %v", domain.ErrConflict, findErr))
		}
		winner, getErr := GetAccount(ctx, tx, winnerID)
		if getErr != nil {
			return nil, wrapf("resolve or create stytch mapping", fmt.Errorf("%w: concurrent tuple winner unavailable: %v", domain.ErrConflict, getErr))
		}
		return winner, nil
	}

	return nil, cleanupCreatedAccount(ctx, tx, account.ID, wrapf("resolve or create stytch mapping", err))
}

func deleteCreatedAccount(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID); err != nil {
		return wrapf("resolve or create stytch mapping cleanup", err)
	}
	return nil
}

func cleanupCreatedAccount(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, cause error) error {
	if err := deleteCreatedAccount(ctx, tx, accountID); err != nil {
		return fmt.Errorf("%w; cleanup: %v", cause, err)
	}
	return cause
}

func scanStytchMapping(row scannable) (*domain.StytchMapping, error) {
	var mapping domain.StytchMapping
	err := row.Scan(
		&mapping.ID, &mapping.AccountID, &mapping.ProjectID,
		&mapping.OrganizationID, &mapping.MemberID,
		&mapping.CreatedAt, &mapping.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &mapping, nil
}

// GetStytchMapping loads a mapping by primary key.
func GetStytchMapping(ctx context.Context, q Querier, id uuid.UUID) (*domain.StytchMapping, error) {
	if id == uuid.Nil {
		return nil, wrapf("get stytch mapping", fmt.Errorf("%w: nil id", domain.ErrInvalid))
	}
	mapping, err := scanStytchMapping(q.QueryRow(ctx, `SELECT `+stytchMappingColumns+` FROM stytch_mappings WHERE id=$1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("get stytch mapping", domain.ErrNotFound)
		}
		return nil, wrapf("get stytch mapping", err)
	}
	return mapping, nil
}
