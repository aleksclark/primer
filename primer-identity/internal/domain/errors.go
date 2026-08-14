package domain

import "errors"

// Typed store/domain errors. Repos wrap these so callers can errors.Is.
var (
	// ErrNotFound is returned when a requested row does not exist.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned on unique/constraint violations (e.g. provider+subject).
	ErrConflict = errors.New("conflict")

	// ErrInvalid is returned when a field fails store-boundary validation.
	ErrInvalid = errors.New("invalid")

	// ErrPasswordDisabled is returned when a password credential is disabled.
	ErrPasswordDisabled = errors.New("password disabled")
)
