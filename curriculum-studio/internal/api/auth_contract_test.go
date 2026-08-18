package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func TestMigrationServiceTokenAliasIsOptInAndJWTOnly(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	validator, err := authn.NewValidator(authn.Options{
		Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	token := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))

	_, enabled := api.NewWithPinger(nil, api.Options{Validator: validator, AcceptServiceTokenAlias: true})
	req := httptest.NewRequest(http.MethodGet, "/studio/v1/machine/probes/materialize", nil)
	req.Header.Set("X-Service-Token", token)
	rec := httptest.NewRecorder()
	enabled.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	_, disabled := api.NewWithPinger(nil, api.Options{Validator: validator})
	req = httptest.NewRequest(http.MethodGet, "/studio/v1/machine/probes/materialize", nil)
	req.Header.Set("X-Service-Token", token)
	rec = httptest.NewRecorder()
	disabled.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
