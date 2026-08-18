package gorest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/clients/go-rest"
	"github.com/aleksclark/primer/curriculum-studio/clients/go-rest/generated"
	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func TestGeneratedClientCallsProtectedMachineProbeWithBearerJWT(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwksBody, err := json.Marshal(jwttest.JWKSDocument(key))
	require.NoError(t, err)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwksBody) }))
	t.Cleanup(jwks.Close)
	validator, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL, Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, handler := api.NewWithPinger(nil, api.Options{Validator: validator})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	token := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client, err := gorest.NewClient(srv.URL, token, srv.Client())
	require.NoError(t, err)
	result, err := client.MachineMaterializeProbe(context.Background())
	require.NoError(t, err)
	probe, ok := result.(*generated.MachineOutBody)
	require.True(t, ok, "generated probe response type = %T", result)
	require.True(t, probe.Ok)
}

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
