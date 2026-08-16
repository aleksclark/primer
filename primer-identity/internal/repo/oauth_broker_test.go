package repo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
	"github.com/stretchr/testify/require"
)

func TestIB1RepositoryRegistrationTransactionAndCAS(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	client, err := repo.CreateOAuthClient(ctx, tx, domain.OAuthClient{ClientID: "client-a", Name: "A", ClientType: "public", TokenEndpointAuthMethod: "none", AllowedGrants: []string{"authorization_code"}, Enabled: true})
	require.NoError(t, err)
	redirect, err := repo.CreateOAuthClientRedirect(ctx, tx, domain.OAuthClientRedirect{OAuthClientID: client.ID, RedirectURI: "https://bff.example/callback", ResourceURI: "https://resource.example", Audience: "aud", AllowedScopes: []string{"openid"}, Enabled: true})
	require.NoError(t, err)
	got, err := repo.ResolveRegistration(ctx, tx, "client-a", "https://bff.example/callback", "https://resource.example", "aud")
	require.NoError(t, err)
	require.Equal(t, redirect.ID, got.ID)
	_, err = repo.ResolveRegistration(ctx, tx, "client-a", "https://bff.example/other", "https://resource.example", "aud")
	require.ErrorIs(t, err, domain.ErrNotFound)
	now := time.Now().UTC()
	in := domain.CreateBrokerTransactionInput{OAuthClientID: client.ID, RedirectID: redirect.ID, StateHash: []byte(strings.Repeat("s", 32)), StatePepperVersion: 1, StateSealed: []byte(strings.Repeat("x", 30)), StateKeyVersion: 1, StateLength: 1, PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256", RequestedScopes: []string{"openid"}, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, BrokerCookieHash: []byte(strings.Repeat("c", 32)), BrokerCookiePepperVersion: 1, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	broker, err := repo.CreateBrokerTransaction(ctx, tx, in)
	require.NoError(t, err)
	transitioned, err := repo.TransitionBrokerTransaction(ctx, tx, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.NoError(t, err)
	require.Equal(t, domain.BrokerStatusProviderStarted, transitioned.Status)
	_, err = repo.TransitionBrokerTransaction(ctx, tx, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.ErrorIs(t, err, repo.ErrStaleCAS)
	_ = factory.Account
	_ = errors.Is
}
