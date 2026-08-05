package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxBeginner can start a database transaction. *pgxpool.Pool implements it;
// tests that only have a Querier skip multi-statement transactions and run
// statements directly (they already wrap work in an outer test transaction).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx runs fn inside a transaction when q is a pool; otherwise it runs fn
// with q directly. Nested Begin is not supported on an existing pgx.Tx.
func WithTx(ctx context.Context, q Querier, fn func(Querier) error) error {
	if beginner, ok := q.(TxBeginner); ok {
		tx, err := beginner.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if err := fn(tx); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	}
	return fn(q)
}

// Ensure pool satisfies TxBeginner.
var _ TxBeginner = (*pgxpool.Pool)(nil)
