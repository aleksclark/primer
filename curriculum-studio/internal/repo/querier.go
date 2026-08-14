// Package repo provides Querier/UoW abstractions and the repository factory
// skeleton for Curriculum Studio persistence.
package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of pgx methods shared by *pgxpool.Pool and pgx.Tx.
// Repositories accept a Querier so the same code path works in production
// (against a pool) and in tests (against a transaction that is rolled back).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)

// ErrClosed is returned when a closed pool/handle is used.
var ErrClosed = errors.New("studio db: closed")

// ErrNotImplemented marks factory methods that land in later phases.
var ErrNotImplemented = errors.New("studio repo: not implemented")
