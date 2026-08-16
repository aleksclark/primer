package factory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
)

func OAuthClient(t *testing.T, q repo.Querier) *domain.OAuthClient {
	t.Helper()
	n := seq.Add(1)
	v, err := repo.CreateOAuthClient(context.Background(), q, domain.OAuthClient{ClientID: fmt.Sprintf("client-%d", n), Name: "Test client", ClientType: "public", TokenEndpointAuthMethod: "none", AllowedGrants: []string{"authorization_code"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func OAuthClientRedirect(t *testing.T, q repo.Querier, c *domain.OAuthClient) *domain.OAuthClientRedirect {
	t.Helper()
	v, err := repo.CreateOAuthClientRedirect(context.Background(), q, domain.OAuthClientRedirect{OAuthClientID: c.ID, RedirectURI: "https://example.test/callback", ResourceURI: "https://resource.example.test", Audience: "test", AllowedScopes: []string{"openid"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func BrokerTransaction(t *testing.T, q repo.Querier, c *domain.OAuthClient, r *domain.OAuthClientRedirect) *domain.BrokerTransaction {
	t.Helper()
	now := time.Now().UTC()
	n := seq.Add(1)
	state := []byte(fmt.Sprintf("s%031d", n))
	cookie := []byte(fmt.Sprintf("c%031d", n))
	v, err := repo.CreateBrokerTransaction(context.Background(), q, domain.CreateBrokerTransactionInput{OAuthClientID: c.ID, RedirectID: r.ID, StateHash: state, StatePepperVersion: 1, StateSealed: []byte(strings.Repeat("x", 30)), StateKeyVersion: 1, StateLength: 1, PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256", RequestedScopes: []string{"openid"}, ResourceURI: r.ResourceURI, Audience: r.Audience, BrokerCookieHash: cookie, BrokerCookiePepperVersion: 1, CreatedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
