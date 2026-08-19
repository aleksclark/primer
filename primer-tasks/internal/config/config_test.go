package config

import "testing"

func validConfig() Config {
	return Config{
		Env:           "production",
		DatabaseURL:   "postgres://tasks@db/primer_tasks",
		AuthMode:      "oidc",
		IssuerURL:     "https://identity.example",
		ClientID:      "tasks",
		RedirectURL:   "https://tasks.example/auth/callback",
		SessionSecret: "01234567890123456789012345678901",
	}
}

func TestValidateRejectsInvalidProductionConfiguration(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Config)
	}{
		{"environment", func(c *Config) { c.Env = "staging" }},
		{"database", func(c *Config) { c.DatabaseURL = "" }},
		{"auth mode", func(c *Config) { c.AuthMode = "magic" }},
		{"production test auth", func(c *Config) { c.AuthMode = "test" }},
		{"test issuer", func(c *Config) { c.IssuerURL = "http://test-issuer:8091" }},
		{"localhost issuer", func(c *Config) { c.IssuerURL = "http://localhost:8091" }},
		{"missing issuer", func(c *Config) { c.IssuerURL = "" }},
		{"relative issuer", func(c *Config) { c.IssuerURL = "identity.example" }},
		{"unsupported issuer scheme", func(c *Config) { c.IssuerURL = "ftp://identity.example" }},
		{"missing client", func(c *Config) { c.ClientID = "" }},
		{"missing redirect", func(c *Config) { c.RedirectURL = "" }},
		{"short secret", func(c *Config) { c.SessionSecret = "short" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.edit(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateAcceptsDevelopmentAndOIDC(t *testing.T) {
	c := validConfig()
	c.Env = "development"
	c.SessionSecret = ""
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Env = "test"
	c.AuthMode = "test"
	c.IssuerURL = "http://issuer:8091"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadUsesSafeDefaultsAndDerivedRedirect(t *testing.T) {
	for _, key := range []string{"TASKS_ENV", "TASKS_DATABASE_URL", "TASKS_AUTH_MODE", "TASKS_ISSUER_URL", "TASKS_OIDC_CLIENT_ID", "TASKS_OIDC_REDIRECT_URL", "TASKS_PUBLIC_ORIGIN", "TASKS_SESSION_SECRET", "TASKS_TEST_PRINCIPAL"} {
		t.Setenv(key, "")
	}
	t.Setenv("TASKS_DATABASE_URL", "postgres://tasks@localhost/primer_tasks")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "development" || c.AuthMode != "test" || c.TestPrincipal != "parent-a" {
		t.Fatalf("defaults = %#v", c)
	}
	if c.RedirectURL != "http://127.0.0.1:8080/auth/callback" {
		t.Fatalf("derived redirect = %q", c.RedirectURL)
	}

	t.Setenv("TASKS_ENV", "production")
	t.Setenv("TASKS_AUTH_MODE", "oidc")
	t.Setenv("TASKS_ISSUER_URL", "https://identity.example/")
	t.Setenv("TASKS_OIDC_CLIENT_ID", "web")
	t.Setenv("TASKS_PUBLIC_ORIGIN", "https://tasks.example")
	t.Setenv("TASKS_SESSION_SECRET", "01234567890123456789012345678901")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.IssuerURL != "https://identity.example" || c.RedirectURL != "https://tasks.example/auth/callback" {
		t.Fatalf("production load normalization = %#v", c)
	}
}

func TestValidateRejectsProductionTestAuthAndRequiresAbsoluteIssuer(t *testing.T) {
	c := validConfig()
	c.AuthMode = "test"
	if c.Validate() == nil {
		t.Fatal("expected test authentication rejection")
	}
	c = validConfig()
	c.IssuerURL = "identity.test"
	if c.Validate() == nil {
		t.Fatal("expected absolute URL validation")
	}
}
