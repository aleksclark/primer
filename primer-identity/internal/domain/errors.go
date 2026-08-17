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

	// ErrSignerUnavailable is returned when no usable active signer exists.
	ErrSignerUnavailable = errors.New("signer unavailable")

	// ErrCorruptSigner is returned when stored key material cannot be used.
	ErrCorruptSigner = errors.New("corrupt signer")

	// ErrRetryableSerialization is a cause-free signal that a serializable
	// transaction should be retried. It never wraps SQL or persist text.
	ErrRetryableSerialization = errors.New("retryable serialization")
)
