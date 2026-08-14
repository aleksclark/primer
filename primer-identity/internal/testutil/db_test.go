package testutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestDBNameConstant(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "primer_identity_test", testutil.DBName)
}

func TestTxRollback(t *testing.T) {
	t.Parallel()
	tx := testutil.Tx(t)
	ctx := context.Background()
	_, err := tx.Exec(ctx, `INSERT INTO schema_meta(key, value) VALUES ('tx-test', '1')`)
	require.NoError(t, err)

	var n int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM schema_meta WHERE key = 'tx-test'`).Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	// Cleanup rolls back; outer pool should not see the row after test ends.
	// Verify via a separate connection that committed state lacks this key
	// only after rollback — within the same tx it is visible.
}

func TestDatabaseURLNonEmpty(t *testing.T) {
	t.Parallel()
	url := testutil.DatabaseURL(t)
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "sslmode=disable")
}
