package testutil_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestSavepointQueryAndQueryRow(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := testutil.NewSavepointQuerier(tx)
	slug := "spq-" + uuid.NewString()[:8]
	_, err := q.Exec(ctx, `
		INSERT INTO curriculum_studio.tenants (slug, name) VALUES ($1,$2)`, slug, "Q")
	require.NoError(t, err)

	rows, err := q.Query(ctx, `SELECT slug FROM curriculum_studio.tenants WHERE slug=$1`, slug)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var got string
	require.NoError(t, rows.Scan(&got))
	require.Equal(t, slug, got)
	rows.Close()

	var name string
	require.NoError(t, q.QueryRow(ctx, `SELECT name FROM curriculum_studio.tenants WHERE slug=$1`, slug).Scan(&name))
	require.Equal(t, "Q", name)

	// failing query row should not poison outer tx
	err = q.QueryRow(ctx, `SELECT name FROM curriculum_studio.tenants WHERE slug=$1`, "missing").Scan(&name)
	require.Error(t, err)
	require.NoError(t, q.QueryRow(ctx, `SELECT name FROM curriculum_studio.tenants WHERE slug=$1`, slug).Scan(&name))
}

func TestSavepointQueryError(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := testutil.NewSavepointQuerier(tx)
	_, err := q.Query(ctx, `SELECT * FROM curriculum_studio.no_such_table`)
	require.Error(t, err)
	// still usable
	var one int
	require.NoError(t, q.QueryRow(ctx, `SELECT 1`).Scan(&one))
	require.Equal(t, 1, one)
}
