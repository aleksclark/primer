package oauth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/oauth"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestClientCredentialsIssuesServiceTokenWithoutRefreshOrProvider(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	secrets := testSecrets()
	clientID := "service-client-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	plain := "service-secret"
	hash, err := secrethash.Hash(secrethash.Peppers(secrets.ClientSecretPeppers), 1, "primer.oauth.client-secret", []byte(plain))
	require.NoError(t, err)
	var client, principal uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO oauth_clients(client_id,name,client_type,token_endpoint_auth_method,client_secret_hash,client_secret_pepper_version,allowed_grants) VALUES($1,'svc','confidential','client_secret_basic',$2,1,ARRAY['client_credentials']) RETURNING id`, clientID, hash).Scan(&client))
	principal = uuid.New()
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO oauth_service_principals(id,subject_ref,display_name) VALUES($1,$2,'service') RETURNING id`, principal, "identity:svc:"+principal.String()).Scan(&principal))
	_, err = pool.Exec(ctx, `INSERT INTO oauth_service_credentials(service_principal_id,oauth_client_id,secret_hash,pepper_version,resource_uri,audience,allowed_scopes) VALUES($1,$2,$3,1,$4,$5,$6)`, principal, client, hash, "https://resource.example", "curriculum-studio", []string{"studio.read", "studio.draft"})
	require.NoError(t, err)

	svc := newTestService(t, secrets, frozenClock{now: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)})
	resp, err := svc.Exchange(ctx, oauth.ExchangeRequest{GrantType: oauth.GrantClientCredentials, Resource: "https://resource.example", Scope: "studio.read"}, oauth.ClientAuth{Method: oauth.AuthBasic, ClientID: clientID, Secret: plain})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.Empty(t, resp.RefreshToken)
	var payload map[string]any
	parts := strings.Split(resp.AccessToken, ".")
	require.Len(t, parts, 3)
	require.NoError(t, decodeJSONSegment(parts[1], &payload))
	require.Equal(t, "identity:svc:"+principal.String(), payload["sub"])
	require.Equal(t, "curriculum-studio", payload["aud"])
	require.Equal(t, "studio.read", payload["scope"])
	var grants, refreshFamilies int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_grants WHERE service_principal_id=$1`, principal).Scan(&grants))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_families f JOIN oauth_grants g ON g.id=f.grant_id WHERE g.service_principal_id=$1`, principal).Scan(&refreshFamilies))
	require.Equal(t, 1, grants)
	require.Zero(t, refreshFamilies)
}

func decodeJSONSegment(segment string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, target)
}
