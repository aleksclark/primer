// Package parentauth constructs the published Authstack Clerk verifier at the
// application boundary. It loads public keys, never implements JWT verification.
package parentauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.clark.team/aleksclark/authstack/auth"
	"git.clark.team/aleksclark/authstack/clerk"
	"github.com/go-jose/go-jose/v4"
)

// Clerk reloads configured public JWKS on a bounded interval and after failed
// verification (including unknown signing keys). Failed refresh never replaces
// a good key set. An outage can use cached keys for at most one hour.
type Clerk struct {
	mu                sync.Mutex
	issuer, jwksURL   string
	client            *http.Client
	verifier          auth.Authenticator
	loaded, attempted time.Time
}

func New(ctx context.Context, issuer, jwksURL string) (*Clerk, error) {
	c := &Clerk{issuer: issuer, jwksURL: jwksURL, client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Clerk) refresh(ctx context.Context) error {
	c.attempted = time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURL, nil)
	if err != nil {
		return errors.New("invalid Clerk JWKS URL")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.New("Clerk JWKS unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("Clerk JWKS unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return errors.New("invalid Clerk JWKS")
	}
	var keys jose.JSONWebKeySet
	if json.Unmarshal(body, &keys) != nil || len(keys.Keys) == 0 {
		return errors.New("invalid Clerk JWKS")
	}
	for _, key := range keys.Keys {
		if !key.Valid() || !key.IsPublic() || key.KeyID == "" {
			return errors.New("invalid Clerk public signing key")
		}
	}
	verifier, err := clerk.NewAuthenticator(clerk.Config{Issuer: c.issuer, JWKS: &keys})
	if err != nil {
		return errors.New("invalid Clerk verification configuration")
	}
	c.verifier, c.loaded = verifier, time.Now()
	return nil
}

func (c *Clerk) Authenticate(ctx context.Context, credential auth.Credential, policy auth.AuthenticationPolicy) (auth.Principal, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.loaded) > 5*time.Minute && time.Since(c.attempted) > 5*time.Second {
		_ = c.refresh(ctx)
	}
	if c.verifier == nil || time.Since(c.loaded) > time.Hour {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	principal, err := c.authenticate(ctx, credential, policy)
	if err != nil && time.Since(c.attempted) > 5*time.Second {
		if c.refresh(ctx) == nil {
			return c.authenticate(ctx, credential, policy)
		}
	}
	return principal, err
}

func (c *Clerk) authenticate(ctx context.Context, credential auth.Credential, policy auth.AuthenticationPolicy) (auth.Principal, error) {
	return c.verifier.Authenticate(ctx, credential, sessionPartyPolicy(policy, credential.Token))
}

// sessionPartyPolicy keeps AuthorizedParties when Clerk included azp. Clerk's
// session-token docs say azp is the Frontend API Origin header and may be
// omitted when Origin is empty or null (native SDK token fetch). Omitting the
// party allowlist in that case does not invent azp, skip issuer/signature/
// expiry/session checks, or accept a present mismatched azp.
func sessionPartyPolicy(policy auth.AuthenticationPolicy, raw string) auth.AuthenticationPolicy {
	if _, present := clerkAuthorizedParty(raw); present {
		return policy
	}
	next := policy
	next.AuthorizedParties = nil
	return next
}

func clerkAuthorizedParty(raw string) (string, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return "", false
	}
	value, ok := claims["azp"]
	if !ok || value == nil {
		return "", false
	}
	party, _ := value.(string)
	party = strings.TrimSpace(party)
	if party == "" {
		return "", false
	}
	return party, true
}
