package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
)

// IngestInstructionLog records instructional time reported by another service.
//
// It is idempotent on (source, source_ref): a producer that retries — because
// its own bookkeeping write failed, or because it never saw our response —
// gets the log it already created rather than a second helping of hours. The
// caller learns which happened from the returned flag.
func IngestInstructionLog(ctx context.Context, q Querier, values map[string]any) (*domain.InstructionLog, bool, error) {
	cols := InstructionLogs.Config().Columns
	ib := psql.Insert(InstructionLogs.Config().Table).
		Suffix("ON CONFLICT (source, source_ref) WHERE source_ref <> '' DO NOTHING").
		Suffix("RETURNING " + strings.Join(cols, ", "))

	insertCols := make([]string, 0, len(values))
	insertVals := make([]any, 0, len(values))
	for col, val := range values {
		insertCols = append(insertCols, col)
		insertVals = append(insertVals, val)
	}
	sqlStr, args, err := ib.Columns(insertCols...).Values(insertVals...).ToSql()
	if err != nil {
		return nil, false, fmt.Errorf("build instruction log insert: %w", err)
	}

	log, err := oneInstructionLog(ctx, q, sqlStr, args...)
	if err == nil {
		return log, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	source, _ := values["source"].(string)
	ref, _ := values["source_ref"].(string)
	existing, err := InstructionLogBySourceRef(ctx, q, source, ref)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

// InstructionLogBySourceRef finds the log a producer already recorded under
// the given reference.
func InstructionLogBySourceRef(ctx context.Context, q Querier, source, ref string) (*domain.InstructionLog, error) {
	sqlStr, args, err := psql.Select(InstructionLogs.Config().Columns...).
		From(InstructionLogs.Config().Table).
		Where(sq.Eq{"source": source, "source_ref": ref}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build instruction log select: %w", err)
	}
	return oneInstructionLog(ctx, q, sqlStr, args...)
}

func oneInstructionLog(ctx context.Context, q Querier, sqlStr string, args ...any) (*domain.InstructionLog, error) {
	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query instruction log: %w", err)
	}
	log, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.InstructionLog])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan instruction log: %w", err)
	}
	return &log, nil
}
