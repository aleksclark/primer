package testutil

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aleksclark/primer/server/internal/repo"
)

// SavepointQuerier wraps a transaction so that every statement runs inside a
// savepoint. Statements that fail (e.g. expected constraint violations in
// tests) roll back to the savepoint instead of aborting the outer
// transaction, letting a single rollback-wrapped tx serve an entire test even
// when the test deliberately triggers database errors.
type SavepointQuerier struct {
	tx  pgx.Tx
	seq int
}

var _ repo.Querier = (*SavepointQuerier)(nil)

// NewSavepointQuerier wraps tx with per-statement savepoints.
func NewSavepointQuerier(tx pgx.Tx) *SavepointQuerier {
	return &SavepointQuerier{tx: tx}
}

func (s *SavepointQuerier) begin(ctx context.Context) (string, error) {
	s.seq++
	name := fmt.Sprintf("sp_%d", s.seq)
	if _, err := s.tx.Exec(ctx, "SAVEPOINT "+name); err != nil {
		return "", fmt.Errorf("savepoint: %w", err)
	}
	return name, nil
}

func (s *SavepointQuerier) finish(ctx context.Context, name string, failed bool) {
	if failed {
		_, _ = s.tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name)
		return
	}
	_, _ = s.tx.Exec(ctx, "RELEASE SAVEPOINT "+name)
}

// Exec runs a statement inside a savepoint.
func (s *SavepointQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	name, err := s.begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	tag, err := s.tx.Exec(ctx, sql, args...)
	s.finish(ctx, name, err != nil)
	return tag, err
}

// Query runs a query inside a savepoint. The savepoint is resolved when the
// returned rows are closed (pgx surfaces most errors during iteration).
func (s *SavepointQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	name, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.tx.Query(ctx, sql, args...)
	if err != nil {
		s.finish(ctx, name, true)
		return nil, err
	}
	return &spRows{Rows: rows, done: func(failed bool) { s.finish(ctx, name, failed) }}, nil
}

// QueryRow runs a single-row query inside a savepoint. The savepoint is
// resolved when Scan is called.
func (s *SavepointQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	name, err := s.begin(ctx)
	if err != nil {
		return errRow{err: err}
	}
	row := s.tx.QueryRow(ctx, sql, args...)
	return spRow{row: row, done: func(failed bool) { s.finish(ctx, name, failed) }}
}

type spRows struct {
	pgx.Rows
	done   func(failed bool)
	closed bool
}

func (r *spRows) Close() {
	r.Rows.Close()
	if !r.closed {
		r.closed = true
		r.done(r.Rows.Err() != nil)
	}
}

type spRow struct {
	row  pgx.Row
	done func(failed bool)
}

func (r spRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	r.done(err != nil)
	return err
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }
