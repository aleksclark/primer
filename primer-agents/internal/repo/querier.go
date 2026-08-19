package repo

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of pgx methods shared by *pgxpool.Pool and pgx.Tx.
// All repository functions accept a Querier so the same code path works in
// production (pool) and in rollback-based integration tests (tx).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)

// TxBeginner can begin a database transaction.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ TxBeginner = (*pgxpool.Pool)(nil)

// WithTx runs fn inside a transaction if q is a TxBeginner (pool); otherwise
// it runs fn directly against q (already-open tx in tests).
func WithTx(ctx context.Context, q Querier, fn func(Querier) error) error {
	beginner, ok := q.(TxBeginner)
	if !ok {
		return fn(q)
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
