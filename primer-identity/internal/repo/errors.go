package repo

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aleksclark/primer/identity/internal/domain"
)

func mapPGError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", domain.ErrConflict, pgErr.ConstraintName)
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", domain.ErrInvalid, pgErr.ConstraintName)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", domain.ErrNotFound, pgErr.ConstraintName)
		}
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func constraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func wrapf(op string, err error) error {
	if err == nil {
		return nil
	}
	if mapped := mapPGError(err); mapped != err {
		return fmt.Errorf("%s: %w", op, mapped)
	}
	// Preserve sentinel errors from domain packages.
	if errors.Is(err, domain.ErrNotFound) ||
		errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrInvalid) ||
		errors.Is(err, domain.ErrPasswordDisabled) ||
		errors.Is(err, domain.ErrSignerUnavailable) ||
		errors.Is(err, domain.ErrCorruptSigner) {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
