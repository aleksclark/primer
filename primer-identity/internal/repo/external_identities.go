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

const externalIdentityColumns = `id, account_id, provider, provider_subject, created_at, updated_at`

// AttachExternalIdentity inserts a new (provider, provider_subject) link.
// Duplicate provider+subject yields domain.ErrConflict — never email-based.
func AttachExternalIdentity(ctx context.Context, q Querier, accountID uuid.UUID, provider, subject string) (*domain.ExternalIdentity, error) {
	if accountID == uuid.Nil {
		return nil, wrapf("attach external identity", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	if err := domain.ValidateProvider(provider); err != nil {
		return nil, wrapf("attach external identity", err)
	}
	if err := domain.ValidateProviderSubject(subject); err != nil {
		return nil, wrapf("attach external identity", err)
	}

	const sqlStr = `
INSERT INTO external_identities (account_id, provider, provider_subject)
VALUES ($1, $2, $3)
RETURNING ` + externalIdentityColumns

	ident, err := scanExternalIdentity(q.QueryRow(ctx, sqlStr, accountID, provider, subject))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, wrapf("attach external identity", fmt.Errorf("%w: %s", domain.ErrConflict, constraintName(err)))
		}
		return nil, wrapf("attach external identity", err)
	}
	return ident, nil
}

// UpsertExternalIdentity attaches provider+subject to accountID.
// If the pair already exists for the same account, the existing row is returned.
// If it exists for a different account, domain.ErrConflict is returned.
// Lookup and write key solely on (provider, provider_subject) — never email.
func UpsertExternalIdentity(ctx context.Context, q Querier, accountID uuid.UUID, provider, subject string) (*domain.ExternalIdentity, error) {
	if accountID == uuid.Nil {
		return nil, wrapf("upsert external identity", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	if err := domain.ValidateProvider(provider); err != nil {
		return nil, wrapf("upsert external identity", err)
	}
	if err := domain.ValidateProviderSubject(subject); err != nil {
		return nil, wrapf("upsert external identity", err)
	}

	existing, err := FindByProviderSubject(ctx, q, provider, subject)
	if err == nil {
		if existing.AccountID != accountID {
			return nil, wrapf("upsert external identity", fmt.Errorf("%w: provider subject owned by another account", domain.ErrConflict))
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, wrapf("upsert external identity", err)
	}

	ident, err := AttachExternalIdentity(ctx, q, accountID, provider, subject)
	if err != nil {
		// Race: another writer attached the same pair concurrently.
		if errors.Is(err, domain.ErrConflict) {
			again, ferr := FindByProviderSubject(ctx, q, provider, subject)
			if ferr == nil {
				if again.AccountID != accountID {
					return nil, wrapf("upsert external identity", fmt.Errorf("%w: provider subject owned by another account", domain.ErrConflict))
				}
				return again, nil
			}
		}
		return nil, err
	}
	return ident, nil
}

// FindByProviderSubject loads the external identity for an exact provider+subject pair.
func FindByProviderSubject(ctx context.Context, q Querier, provider, subject string) (*domain.ExternalIdentity, error) {
	if err := domain.ValidateProvider(provider); err != nil {
		return nil, wrapf("find by provider subject", err)
	}
	if err := domain.ValidateProviderSubject(subject); err != nil {
		return nil, wrapf("find by provider subject", err)
	}
	const sqlStr = `
SELECT ` + externalIdentityColumns + `
FROM external_identities
WHERE provider = $1 AND provider_subject = $2`
	ident, err := scanExternalIdentity(q.QueryRow(ctx, sqlStr, provider, subject))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("find by provider subject", domain.ErrNotFound)
		}
		return nil, wrapf("find by provider subject", err)
	}
	return ident, nil
}

// ListExternalIdentitiesByAccount returns all external identities for an account.
func ListExternalIdentitiesByAccount(ctx context.Context, q Querier, accountID uuid.UUID) ([]domain.ExternalIdentity, error) {
	if accountID == uuid.Nil {
		return nil, wrapf("list external identities", fmt.Errorf("%w: nil account id", domain.ErrInvalid))
	}
	const sqlStr = `
SELECT ` + externalIdentityColumns + `
FROM external_identities
WHERE account_id = $1
ORDER BY created_at ASC, id ASC`
	rows, err := q.Query(ctx, sqlStr, accountID)
	if err != nil {
		return nil, wrapf("list external identities", err)
	}
	defer rows.Close()

	var out []domain.ExternalIdentity
	for rows.Next() {
		ident, err := scanExternalIdentity(rows)
		if err != nil {
			return nil, wrapf("list external identities", err)
		}
		out = append(out, *ident)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapf("list external identities", err)
	}
	if out == nil {
		out = []domain.ExternalIdentity{}
	}
	return out, nil
}

func scanExternalIdentity(row scannable) (*domain.ExternalIdentity, error) {
	var e domain.ExternalIdentity
	var created, updated time.Time
	err := row.Scan(
		&e.ID,
		&e.AccountID,
		&e.Provider,
		&e.ProviderSubject,
		&created,
		&updated,
	)
	if err != nil {
		return nil, err
	}
	e.CreatedAt = created
	e.UpdatedAt = updated
	return &e, nil
}
