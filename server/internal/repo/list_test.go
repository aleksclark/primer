package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListParamsNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   ListParams
		want ListParams
	}{
		{
			name: "defaults applied",
			in:   ListParams{},
			want: ListParams{Limit: 25, Offset: 0, Sort: "created_at", Dir: SortAsc},
		},
		{
			name: "limit clamped to max",
			in:   ListParams{Limit: 10_000},
			want: ListParams{Limit: 200, Offset: 0, Sort: "created_at", Dir: SortAsc},
		},
		{
			name: "negative offset reset",
			in:   ListParams{Offset: -5},
			want: ListParams{Limit: 25, Offset: 0, Sort: "created_at", Dir: SortAsc},
		},
		{
			name: "explicit values preserved",
			in:   ListParams{Limit: 10, Offset: 30, Sort: "name", Dir: SortDesc},
			want: ListParams{Limit: 10, Offset: 30, Sort: "name", Dir: SortDesc},
		},
		{
			name: "invalid dir coerced to asc",
			in:   ListParams{Dir: "sideways"},
			want: ListParams{Limit: 25, Offset: 0, Sort: "created_at", Dir: SortAsc},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Normalize("created_at", 200)
			assert.Equal(t, tt.want.Limit, tt.in.Limit)
			assert.Equal(t, tt.want.Offset, tt.in.Offset)
			assert.Equal(t, tt.want.Sort, tt.in.Sort)
			assert.Equal(t, tt.want.Dir, tt.in.Dir)
		})
	}
}

func TestBuildSelectValidation(t *testing.T) {
	t.Parallel()

	cfg := ListConfig{
		Table:             "things",
		Columns:           []string{"id", "name"},
		SearchColumns:     []string{"name"},
		SortableColumns:   []string{"name"},
		FilterableColumns: []string{"name"},
	}

	// Disallowed sort column
	_, _, err := buildSelect(cfg, ListParams{Sort: "id; DROP TABLE things", Limit: 10})
	assert.Error(t, err)

	// Disallowed filter column
	_, _, err = buildSelect(cfg, ListParams{Filters: map[string]any{"evil": 1}, Limit: 10})
	assert.Error(t, err)

	// Valid params produce SQL with search, filter, sort, and pagination
	data, count, err := buildSelect(cfg, ListParams{
		Search: "abc", Sort: "name", Dir: SortDesc,
		Filters: map[string]any{"name": "x"}, Limit: 10, Offset: 5,
	})
	assert.NoError(t, err)

	dataSQL, args, err := data.ToSql()
	assert.NoError(t, err)
	assert.Contains(t, dataSQL, "ILIKE")
	assert.Contains(t, dataSQL, "ORDER BY name DESC")
	assert.Contains(t, dataSQL, "LIMIT 10")
	assert.Contains(t, dataSQL, "OFFSET 5")
	assert.NotEmpty(t, args)

	countSQL, _, err := count.ToSql()
	assert.NoError(t, err)
	assert.Contains(t, countSQL, "COUNT(*)")
	assert.NotContains(t, countSQL, "LIMIT")
}

func TestErrBadRequest(t *testing.T) {
	t.Parallel()
	err := ErrBadRequest{Msg: "bad column"}
	assert.Equal(t, "bad column", err.Error())
}
