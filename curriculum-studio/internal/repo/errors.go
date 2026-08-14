package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel-mapped error classes for persistence callers.
var (
	ErrNotFound       = errors.New("studio repo: not found")
	ErrConflict       = errors.New("studio repo: conflict")
	ErrForeignKey     = errors.New("studio repo: foreign key violation")
	ErrCheckViolation = errors.New("studio repo: check violation")
)

// MapError converts pgx/pgconn errors into stable package sentinels when possible.
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
