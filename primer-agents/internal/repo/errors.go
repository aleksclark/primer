package repo

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel errors returned by all repository operations.
var (
	// ErrNotFound is returned when a row matching the given ID and namespace does not exist.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned by idempotent creates when the same key is reused
	// with materially different inputs (different hash).
	ErrConflict = errors.New("conflict")

	// ErrInvalidTransition is returned when a requested status change is not
	// permitted by the frozen lifecycle state machine.
	ErrInvalidTransition = errors.New("invalid transition")

	// ErrStaleVersion is returned when a compare-and-swap state update finds a
	// different state_version than the caller supplied.
	ErrStaleVersion = errors.New("stale version")

	// ErrForbidden is returned when an operation is attempted by an owner
	// namespace that does not match the row's owner.
	ErrForbidden = errors.New("forbidden")
)

// isUniqueViolation reports whether err is a PostgreSQL unique_violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
