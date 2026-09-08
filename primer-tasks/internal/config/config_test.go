package config

import "testing"

func validConfig() Config {
	return Config{Env: "production", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "clerk", ClerkIssuer: "https://clerk.example", ClerkJWKSURL: "https://clerk.example/.well-known/jwks.json", PublicOrigin: "https://api.primerlms.com", BasePath: "/tasks", ModelProvider: "disabled"}
}
func TestValidateRejectsInvalidProductionConfiguration(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Config)
	}{
		{"environment", func(c *Config) { c.Env = "staging" }},
		{"database", func(c *Config) { c.DatabaseURL = "" }},
		{"auth mode", func(c *Config) { c.AuthMode = "magic" }},
		{"test auth", func(c *Config) { c.AuthMode = "test" }},
		{"legacy production oidc", func(c *Config) { c.AuthMode = "oidc" }},
		{"missing issuer", func(c *Config) { c.ClerkIssuer = "" }},
		{"insecure issuer", func(c *Config) { c.ClerkIssuer = "http://clerk.example" }},
		{"missing jwks", func(c *Config) { c.ClerkJWKSURL = "" }},
		{"insecure jwks", func(c *Config) { c.ClerkJWKSURL = "http://clerk.example/jwks" }},
		{"relative jwks", func(c *Config) { c.ClerkJWKSURL = "/jwks" }},
		{"origin missing", func(c *Config) { c.PublicOrigin = "" }},
		{"origin path", func(c *Config) { c.PublicOrigin = "https://api.primerlms.com/tasks/" }},
		{"origin credentials", func(c *Config) { c.PublicOrigin = "https://user@api.primerlms.com" }},
		{"origin query", func(c *Config) { c.PublicOrigin = "https://api.primerlms.com?x=y" }},
		{"development issuer", func(c *Config) { c.ClerkIssuer = "https://test-issuer" }},
		{"bad base", func(c *Config) { c.BasePath = "//tasks" }},
		{"unknown model", func(c *Config) { c.ModelProvider = "unknown" }},
		{"scripted production", func(c *Config) { c.ModelProvider = "scripted" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.edit(&c)
			if c.Validate() == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
func TestClerkNeedsNoIdentityOrParentSecret(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestAuthorizedPartiesStayAdditive(t *testing.T) {
	c := validConfig()
	c.ClerkAuthorizedParties = parseAuthorizedParties("com.aleksclark.primer.control,, " + c.PublicOrigin + ", com.aleksclark.primer.control, https://control.example")
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	got := c.AuthorizedParties()
	if got[0] != c.PublicOrigin {
		t.Fatalf("web origin must remain first, got %q", got)
	}
	if len(got) != 3 || got[1] != "com.aleksclark.primer.control" || got[2] != "https://control.example" {
		t.Fatalf("authorized parties = %v", got)
	}
	c.ClerkAuthorizedParties = []string{"not a party"}
	if c.Validate() == nil {
		t.Fatal("invalid extra azp must fail")
	}
	c = validConfig()
	c.ClerkAuthorizedParties = []string{"http://insecure.example"}
	if c.Validate() == nil {
		t.Fatal("insecure extra origin accepted in production")
	}
}
func TestLoadDefaultsAndClerk(t *testing.T) {
	for _, key := range []string{"TASKS_ENV", "TASKS_DATABASE_URL", "TASKS_AUTH_MODE", "TASKS_ISSUER_URL", "TASKS_PUBLIC_ORIGIN", "TASKS_CLERK_ISSUER", "TASKS_CLERK_JWKS_URL", "TASKS_CLERK_AUDIENCE", "TASKS_BASE_PATH", "TASKS_SESSION_SECRET"} {
		t.Setenv(key, "")
	}
	t.Setenv("TASKS_DATABASE_URL", "postgres://tasks@db/primer_tasks")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthMode != "test" || c.RedirectURL != "http://127.0.0.1:8080/auth/callback" {
		t.Fatal("development defaults changed")
	}
	t.Setenv("TASKS_ENV", "production")
	t.Setenv("TASKS_AUTH_MODE", "clerk")
	t.Setenv("TASKS_CLERK_ISSUER", "https://clerk.example")
	t.Setenv("TASKS_CLERK_JWKS_URL", "https://clerk.example/jwks")
	t.Setenv("TASKS_PUBLIC_ORIGIN", "https://api.primerlms.com")
	t.Setenv("TASKS_BASE_PATH", "/tasks/")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.BasePath != "/tasks" || c.SessionSecret != "" {
		t.Fatal("Clerk release config mismatch")
	}
}
func TestParentAgentProviderConfiguration(t *testing.T) {
	for _, provider := range []string{"disabled", "bedrock", "openrouter"} {
		c := validConfig()
		c.ModelProvider = provider
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	c := validConfig()
	c.Env, c.ModelProvider = "test", "scripted"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	c = validConfig()
	if c.Validate() == nil {
		t.Fatal("scripted override bypassed production guard")
	}
}

func TestDevelopmentOIDCCompatibility(t *testing.T) {
	c := Config{Env: "development", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "oidc", IssuerURL: "http://issuer", ClientID: "tasks", RedirectURL: "http://tasks/auth/callback"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.ClientID = ""
	if c.Validate() == nil {
		t.Fatal("missing legacy client")
	}
	c = Config{Env: "development", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "oidc", ClientID: "tasks", RedirectURL: "http://tasks/auth/callback"}
	if c.Validate() == nil {
		t.Fatal("missing issuer accepted")
	}
	c = Config{Env: "development", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "oidc", IssuerURL: "://bad", ClientID: "tasks", RedirectURL: "http://tasks/auth/callback"}
	if c.Validate() == nil {
		t.Fatal("invalid issuer accepted")
	}
}
