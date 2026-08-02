package repo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrBadRequest wraps validation errors from list/filter parameters.
type ErrBadRequest struct{ Msg string }

func (e ErrBadRequest) Error() string { return e.Msg }

// Resource is a generic repository for a single table. T must be a struct
// with db tags matching the configured columns.
type Resource[T any] struct {
	cfg ListConfig
}

// columnList joins column names for a RETURNING clause.
func columnList(cols []string) string {
	return strings.Join(cols, ", ")
}

// columnsFromType returns db tag names from exported fields of T.
// Nested anonymous structs are flattened. Fields tagged `db:"-"` or without
// a db tag are skipped.
func columnsFromType[T any]() []string {
	var walk func(t reflect.Type) []string
	walk = func(t reflect.Type) []string {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return nil
		}
		var cols []string
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if f.Anonymous {
				cols = append(cols, walk(f.Type)...)
				continue
			}
			tag := f.Tag.Get("db")
			if tag == "" || tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if name != "" && name != "-" {
				cols = append(cols, name)
			}
		}
		return cols
	}
	return walk(reflect.TypeFor[T]())
}

// NewResource builds a repository for the given list configuration.
// When cfg.Columns is empty, column names are derived from T's db tags so the
// domain struct remains the single source of truth for selected columns.
func NewResource[T any](cfg ListConfig) *Resource[T] {
	if len(cfg.Columns) == 0 {
		cfg.Columns = columnsFromType[T]()
	}
	return &Resource[T]{cfg: cfg}
}

// Config exposes the resource's list configuration.
func (r *Resource[T]) Config() ListConfig { return r.cfg }

// Create inserts a row from the given column->value map and returns the
// created entity.
func (r *Resource[T]) Create(ctx context.Context, q Querier, values map[string]any) (*T, error) {
	ib := psql.Insert(r.cfg.Table).Suffix("RETURNING " + columnList(r.cfg.Columns))
	cols := make([]string, 0, len(values))
	vals := make([]any, 0, len(values))
	for col, val := range values {
		cols = append(cols, col)
		vals = append(vals, val)
	}
	ib = ib.Columns(cols...).Values(vals...)
	sqlStr, args, err := ib.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build insert: %w", err)
	}
	return r.one(ctx, q, sqlStr, args)
}

// Get fetches a row by primary key.
func (r *Resource[T]) Get(ctx context.Context, q Querier, id string) (*T, error) {
	sqlStr, args, err := psql.Select(r.cfg.Columns...).From(r.cfg.Table).Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select: %w", err)
	}
	return r.one(ctx, q, sqlStr, args)
}

// Update applies the given column->value map to the row with the given id
// and returns the updated entity.
func (r *Resource[T]) Update(ctx context.Context, q Querier, id string, values map[string]any) (*T, error) {
	if len(values) == 0 {
		return r.Get(ctx, q, id)
	}
	ub := psql.Update(r.cfg.Table).Where(sq.Eq{"id": id}).
		Set("updated_at", sq.Expr("now()")).
		Suffix("RETURNING " + columnList(r.cfg.Columns))
	for col, val := range values {
		ub = ub.Set(col, val)
	}
	sqlStr, args, err := ub.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build update: %w", err)
	}
	return r.one(ctx, q, sqlStr, args)
}

// Delete removes a row by primary key.
func (r *Resource[T]) Delete(ctx context.Context, q Querier, id string) error {
	sqlStr, args, err := psql.Delete(r.cfg.Table).Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return fmt.Errorf("build delete: %w", err)
	}
	tag, err := q.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("exec delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns a page of rows matching the given parameters, plus the total
// count of matching rows (ignoring pagination).
func (r *Resource[T]) List(ctx context.Context, q Querier, p ListParams) (*Page[T], error) {
	p.Normalize("created_at", 200)
	data, count, err := buildSelect(r.cfg, p)
	if err != nil {
		return nil, ErrBadRequest{Msg: err.Error()}
	}

	countSQL, countArgs, err := count.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build count: %w", err)
	}
	var total int
	if err := q.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("exec count: %w", err)
	}

	dataSQL, dataArgs, err := data.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list: %w", err)
	}
	rows, err := q.Query(ctx, dataSQL, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec list: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
	if err != nil {
		return nil, fmt.Errorf("scan list: %w", err)
	}
	return &Page[T]{Items: items, TotalCount: total, Limit: p.Limit, Offset: p.Offset}, nil
}

func (r *Resource[T]) one(ctx context.Context, q Querier, sqlStr string, args []any) (*T, error) {
	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("exec query: %w", err)
	}
	item, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[T])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan row: %w", err)
	}
	return &item, nil
}
