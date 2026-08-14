package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// Sentinel-mapped error classes for persistence callers.
var (
	ErrNotFound       = errors.New("studio repo: not found")
	ErrConflict       = errors.New("studio repo: conflict")
	ErrForeignKey     = errors.New("studio repo: foreign key violation")
	ErrCheckViolation = errors.New("studio repo: check violation")
)

// MapError converts pgx/pgconn errors into stable package sentinels when possible.
// SQLSTATE 22021 (character_not_in_repertoire / invalid byte sequence) maps to
// domain.ErrInvalidIntegrationIdentity as defense-in-depth for unsanitized text.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w", ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", ErrForeignKey, pgErr.ConstraintName)
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", ErrCheckViolation, pgErr.ConstraintName)
		case "22021": // character_not_in_repertoire
			return fmt.Errorf("%w: invalid byte sequence", domain.ErrInvalidIntegrationIdentity)
		case "57014": // query_canceled
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
		}
	}
	// Closed pool / use-after-close often surfaces as connection errors.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "closed pool") || strings.Contains(msg, "conn closed") {
		return fmt.Errorf("%w: %v", ErrClosed, err)
	}
	return err
}
