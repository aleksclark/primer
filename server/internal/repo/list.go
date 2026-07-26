package repo

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
)

// psql is the squirrel builder configured for PostgreSQL placeholders ($1, $2...).
var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// SortDir is a sort direction.
type SortDir string

const (
	SortAsc  SortDir = "asc"
	SortDesc SortDir = "desc"
)

// ListParams captures the pagination, search, sorting, and filtering options
// that every list endpoint supports out of the box.
type ListParams struct {
	// Limit is the maximum number of rows to return (page size).
	Limit int
	// Offset is the number of rows to skip.
	Offset int
	// Search is a free-text query applied across the resource's searchable columns.
	Search string
	// Sort is the column to order by. Must be in the resource's allowed set.
	Sort string
	// Dir is the sort direction.
	Dir SortDir
	// Filters is a map of exact-match column filters (column -> value).
	Filters map[string]any
}

// Normalize applies sane defaults and clamps values to safe bounds.
func (p *ListParams) Normalize(defaultSort string, maxLimit int) {
	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Sort == "" {
		p.Sort = defaultSort
	}
	if p.Dir != SortAsc && p.Dir != SortDesc {
		p.Dir = SortAsc
	}
}

// Page is a generic paginated result wrapper.
type Page[T any] struct {
	Items      []T
	TotalCount int
	Limit      int
	Offset     int
}

// ListConfig describes how a resource participates in generic list queries.
type ListConfig struct {
	// Table is the table (or view) name to select from.
	Table string
	// Columns are the columns to select, in order.
	Columns []string
	// SearchColumns are text columns matched (ILIKE) by the Search param.
	SearchColumns []string
	// SortableColumns is the whitelist of columns allowed in ORDER BY.
	SortableColumns []string
	// FilterableColumns is the whitelist of columns allowed as exact filters.
	FilterableColumns []string
}

// sortable reports whether col is an allowed sort column.
func (c ListConfig) sortable(col string) bool {
	for _, s := range c.SortableColumns {
		if s == col {
			return true
		}
	}
	return false
}

// filterable reports whether col is an allowed filter column.
func (c ListConfig) filterable(col string) bool {
	for _, s := range c.FilterableColumns {
		if s == col {
			return true
		}
	}
	return false
}

// buildSelect constructs the data + count queries for a list request.
// It returns the paginated select and a matching count select.
func buildSelect(cfg ListConfig, p ListParams) (sq.SelectBuilder, sq.SelectBuilder, error) {
	data := psql.Select(cfg.Columns...).From(cfg.Table)
	count := psql.Select("COUNT(*)").From(cfg.Table)

	// Free-text search across configured columns.
	if p.Search != "" && len(cfg.SearchColumns) > 0 {
		or := sq.Or{}
		like := "%" + p.Search + "%"
		for _, col := range cfg.SearchColumns {
			or = append(or, sq.ILike{col: like})
		}
		data = data.Where(or)
		count = count.Where(or)
	}

	// Exact-match filters (whitelisted).
	for col, val := range p.Filters {
		if !cfg.filterable(col) {
			return data, count, fmt.Errorf("filter column %q is not allowed", col)
		}
		data = data.Where(sq.Eq{col: val})
		count = count.Where(sq.Eq{col: val})
	}

	// Sorting (whitelisted).
	if p.Sort != "" {
		if !cfg.sortable(p.Sort) {
			return data, count, fmt.Errorf("sort column %q is not allowed", p.Sort)
		}
		dir := "ASC"
		if p.Dir == SortDesc {
			dir = "DESC"
		}
		data = data.OrderBy(fmt.Sprintf("%s %s", p.Sort, dir))
	}

	data = data.Limit(uint64(p.Limit)).Offset(uint64(p.Offset))
	return data, count, nil
}
