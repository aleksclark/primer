package domain

import "fmt"

// InvalidError is a typed validation failure at the store/domain boundary.
type InvalidError struct {
	Field   string
	Message string
}

func (e InvalidError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%v: %s", ErrInvalid, e.Message)
	}
	return fmt.Sprintf("%v: %s: %s", ErrInvalid, e.Field, e.Message)
}

// Unwrap allows errors.Is(err, ErrInvalid).
func (e InvalidError) Unwrap() error { return ErrInvalid }

func invalidf(field, format string, args ...any) error {
	return InvalidError{Field: field, Message: fmt.Sprintf(format, args...)}
}
