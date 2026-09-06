package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Config struct {
	Env, DatabaseURL, AuthMode, IssuerURL, ClientID, RedirectURL, PublicOrigin, SessionSecret, TestPrincipal, ModelProvider string
	ClerkIssuer, ClerkJWKSURL, ClerkAudience, BasePath, WebDir                                                              string
	ClerkAuthorizedParties                                                                                                  []string
	ReleasePublisherToken, ReleaseSigningKey, ReleaseArtifactDir                                                            string
}

func Load() (Config, error) {
	c := Config{Env: value("TASKS_ENV", "development"), DatabaseURL: os.Getenv("TASKS_DATABASE_URL"), AuthMode: value("TASKS_AUTH_MODE", "test"), IssuerURL: strings.TrimRight(value("TASKS_ISSUER_URL", "http://test-issuer:8091"), "/"), ClientID: value("TASKS_OIDC_CLIENT_ID", "primer-tasks-web"), PublicOrigin: value("TASKS_PUBLIC_ORIGIN", "http://127.0.0.1:8080"), SessionSecret: os.Getenv("TASKS_SESSION_SECRET"), TestPrincipal: value("TASKS_TEST_PRINCIPAL", "parent-a"), ModelProvider: value("TASKS_MODEL_PROVIDER", "disabled")}
	c.ClerkIssuer = os.Getenv("TASKS_CLERK_ISSUER")
	c.ClerkJWKSURL = os.Getenv("TASKS_CLERK_JWKS_URL")
	c.ClerkAudience = os.Getenv("TASKS_CLERK_AUDIENCE")
	c.ClerkAuthorizedParties = parseAuthorizedParties(os.Getenv("TASKS_CLERK_AUTHORIZED_PARTIES"))
	c.BasePath = strings.TrimRight(os.Getenv("TASKS_BASE_PATH"), "/")
	c.WebDir = os.Getenv("TASKS_WEB_DIR")
	c.ReleasePublisherToken = os.Getenv("TASKS_RELEASE_PUBLISHER_TOKEN")
	c.ReleaseSigningKey = os.Getenv("TASKS_RELEASE_SIGNING_KEY")
	c.ReleaseArtifactDir = os.Getenv("TASKS_RELEASE_ARTIFACT_DIR")
	c.RedirectURL = value("TASKS_OIDC_REDIRECT_URL", c.PublicOrigin+"/auth/callback")
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Env != "development" && c.Env != "test" && c.Env != "production" {
		return fmt.Errorf("TASKS_ENV must be development, test, or production")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("TASKS_DATABASE_URL is required")
	}
	if c.AuthMode != "test" && c.AuthMode != "oidc" && c.AuthMode != "clerk" {
		return fmt.Errorf("TASKS_AUTH_MODE must be test, oidc, or clerk")
	}
	if c.Env == "production" && c.AuthMode != "clerk" {
		return fmt.Errorf("production requires TASKS_AUTH_MODE=clerk")
	}
	if c.BasePath != "" && c.BasePath != "/tasks" {
		return fmt.Errorf("TASKS_BASE_PATH must be empty or /tasks")
	}
	if c.ModelProvider != "" && c.ModelProvider != "disabled" {
		return fmt.Errorf("TASKS_MODEL_PROVIDER must be disabled in Phase 2")
	}
	if c.AuthMode == "clerk" {
		for name, raw := range map[string]string{"TASKS_CLERK_ISSUER": c.ClerkIssuer, "TASKS_CLERK_JWKS_URL": c.ClerkJWKSURL, "TASKS_PUBLIC_ORIGIN": c.PublicOrigin} {
			u, err := url.Parse(raw)
			if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && (c.Env == "production" || u.Scheme != "http")) {
				return fmt.Errorf("%s must be an explicit absolute HTTPS URL in production", name)
			}
			if name == "TASKS_PUBLIC_ORIGIN" && u.Path != "" {
				return fmt.Errorf("TASKS_PUBLIC_ORIGIN must be an exact origin without path or trailing slash")
			}
			if c.Env == "production" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || strings.Contains(u.Hostname(), "test-issuer")) {
				return fmt.Errorf("%s cannot use a development issuer or origin", name)
			}
		}
		for _, party := range c.ClerkAuthorizedParties {
			if party == c.PublicOrigin {
				continue
			}
			u, err := url.Parse(party)
			switch {
			case err == nil && u.Scheme != "" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "":
				if c.Env == "production" && u.Scheme != "https" {
					return fmt.Errorf("TASKS_CLERK_AUTHORIZED_PARTIES extra origins must be HTTPS")
				}
			case packageAZP.MatchString(party):
				// Native Clerk sessions may present the Android application ID as azp.
			default:
				return fmt.Errorf("TASKS_CLERK_AUTHORIZED_PARTIES values must be origins or Android application IDs")
			}
		}
		return nil // Clerk does not use Identity, a client secret, or a BFF parent session key.
	}
	// Legacy authorization-code settings are development/test compatibility only.
	if c.IssuerURL == "" {
		return fmt.Errorf("TASKS_ISSUER_URL is required")
	}
	u, e := url.Parse(c.IssuerURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("TASKS_ISSUER_URL must be an absolute HTTP(S) URL")
	}
	if c.ClientID == "" || c.RedirectURL == "" {
		return fmt.Errorf("OIDC client id and redirect URL are required")
	}
	return nil
}
func value(k, f string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return f
}

var packageAZP = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

func parseAuthorizedParties(raw string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		party := strings.TrimSpace(part)
		if party == "" {
			continue
		}
		if _, ok := seen[party]; ok {
			continue
		}
		seen[party] = struct{}{}
		out = append(out, party)
	}
	return out
}

// AuthorizedParties always includes the web public origin. Extra native parties
// are additive and never replace that check.
func (c Config) AuthorizedParties() []string {
	seen := map[string]struct{}{c.PublicOrigin: {}}
	out := []string{c.PublicOrigin}
	for _, party := range c.ClerkAuthorizedParties {
		if _, ok := seen[party]; ok {
			continue
		}
		seen[party] = struct{}{}
		out = append(out, party)
	}
	return out
}
