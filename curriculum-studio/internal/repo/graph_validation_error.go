package repo

import "github.com/aleksclark/primer/curriculum-studio/internal/domain"

// GraphValidationError is distinct from an immutable revision or invalid
// lifecycle transition. Approval never waives deterministic graph validation.
// Findings come from the graph read under the publication transaction's locks.
type GraphValidationError struct{ Findings []domain.ValidationFinding }

func (e *GraphValidationError) Error() string { return "revision graph validation failed" }
