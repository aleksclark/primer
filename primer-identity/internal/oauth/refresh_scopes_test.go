package oauth

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"
)

func TestExchangeRefreshRejectsMalformedRequest(t *testing.T) {
	var svc Service
	_, err := svc.exchangeRefreshToken(nil, ExchangeRequest{}, ClientAuth{})
	require.Equal(t, ErrorInvalidRequest, ErrorCodeOf(err))
	_, err = svc.exchangeRefreshToken(nil, ExchangeRequest{Resource: "https://resource", RefreshToken: "bad"}, ClientAuth{Method: AuthNone})
	require.Equal(t, ErrorInvalidGrant, ErrorCodeOf(err))
	_, err = svc.exchangeRefreshToken(nil, ExchangeRequest{Resource: "https://resource", RefreshToken: "bad"}, ClientAuth{})
	require.Equal(t, ErrorInvalidRequest, ErrorCodeOf(err))
}

func TestRefreshLifecycleGuards(t *testing.T) {
	var svc Service
	_, err := svc.PurgeExpiredAssertionReplays(nil, time.Time{})
	require.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(err))
	err = svc.RevokeInitialFamily(nil, uuid.Nil, "")
	require.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(err))
}

func TestRequestedRefreshScopes(t *testing.T) {
	got, err := requestedRefreshScopes("studio.read openid")
	require.NoError(t, err)
	require.Equal(t, []string{"openid", "studio.read"}, got)
	got, err = requestedRefreshScopes("")
	require.NoError(t, err)
	require.Nil(t, got)
	_, err = requestedRefreshScopes("studio.read studio.read")
	require.Error(t, err)
	require.Equal(t, "invalid_scope", ErrorCodeOf(err))
}
