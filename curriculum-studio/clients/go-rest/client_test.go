package gorest_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/clients/go-rest"
	"github.com/aleksclark/primer/curriculum-studio/clients/go-rest/generated"
	"github.com/aleksclark/primer/curriculum-studio/internal/api"
)

func TestGeneratedClientCallsRealHumaHealthHandler(t *testing.T) {
	t.Parallel()
	_, handler := api.NewWithPinger(nil, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := gorest.NewClient(srv.URL, "", srv.Client())
	require.NoError(t, err)
	result, err := client.Health(context.Background())
	require.NoError(t, err)

	health, ok := result.(*generated.HealthOutBody)
	require.True(t, ok, "generated health response type = %T", result)
	require.Equal(t, "ok", health.Status)
}
