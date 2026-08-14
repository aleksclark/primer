package repo

import (
	"context"
	"fmt"
)

// Factory constructs domain repositories bound to a Querier.
// Domain methods are filled in D3+; this phase lands the type and health probe.
type Factory struct {
	Q Querier
}

// NewFactory binds repositories to q. q must be non-nil.
func NewFactory(q Querier) *Factory {
	if q == nil {
		panic("repo.NewFactory: nil Querier")
	}
	return &Factory{Q: q}
}

// Ping verifies the underlying Querier can execute a trivial query.
func (f *Factory) Ping(ctx context.Context) error {
	if f == nil || f.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	var one int
	if err := f.Q.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return MapError(err)
	}
	if one != 1 {
		return fmt.Errorf("unexpected ping result %d", one)
	}
	return nil
}

// Health is an optional thin health repository exposed via the factory.
func (f *Factory) Health() *HealthRepo {
	return &HealthRepo{Q: f.Q}
}

// HealthRepo exposes readiness probes used by later service wiring.
type HealthRepo struct {
	Q Querier
}

// Ready returns nil when the database answers SELECT 1.
func (h *HealthRepo) Ready(ctx context.Context) error {
	if h == nil || h.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	var one int
	if err := h.Q.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return MapError(err)
	}
	return nil
}
