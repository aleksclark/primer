package repo

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestCoverageUniqueViolation(t *testing.T) {
	require.True(t, isUniqueViolation(&pgconn.PgError{Code: "23505"}))
	require.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}))
	require.False(t, isUniqueViolation(context.Canceled))
}

func TestCoverageTurnHashesAndWakeups(t *testing.T) {
	h := computeTurnInputHash("session", "key", "preview")
	require.Len(t, h, 64)
	require.Equal(t, h, computeTurnInputHash("session", "key", "preview"))
	require.NotEqual(t, h, computeTurnInputHash("session", "key", "other"))
	require.Len(t, computeTurnIdempHash("owner", "session", "key", h), 64)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	require.True(t, WaitForNewEvents(ctx, ch, time.Hour))
	cancel()
	require.False(t, WaitForNewEvents(ctx, nil, time.Millisecond))
	require.True(t, WaitForNewEvents(context.Background(), nil, time.Millisecond))
}
