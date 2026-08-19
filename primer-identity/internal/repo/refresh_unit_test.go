package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRefreshHelpersRejectInvalidAndScopeWidening(t *testing.T) {
	ctx := context.Background()
	_, err := RedeemRefreshToken(ctx, nil, nil, uuid.Nil, "", nil, nil, 0, time.Time{})
	require.Error(t, err)
	require.NoError(t, validateRefreshScope([]string{"openid", "studio.read"}, nil))
	require.NoError(t, validateRefreshScope([]string{"openid"}, []string{"openid"}))
	require.ErrorIs(t, validateRefreshScope([]string{"openid"}, []string{"studio.read"}), ErrRefreshScopeWidening)
}
