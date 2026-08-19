package config

import "testing"

func TestValidateRejectsProductionTestAuth(t *testing.T) {
	c := Config{Env: "production", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "test", IssuerURL: "https://identity.test", ClientID: "tasks", RedirectURL: "https://tasks.test/auth/callback", SessionSecret: "01234567890123456789012345678901"}
	if c.Validate() == nil {
		t.Fatal("expected rejection")
	}
}
func TestValidateRequiresAbsoluteIssuer(t *testing.T) {
	c := Config{Env: "development", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "oidc", ClientID: "tasks", RedirectURL: "http://localhost/auth/callback"}
	if c.Validate() == nil {
		t.Fatal("expected issuer requirement")
	}
	c.IssuerURL = "identity.test"
	if c.Validate() == nil {
		t.Fatal("expected absolute URL validation")
	}
}
func TestValidateRejectsTestIssuerInProduction(t *testing.T) {
	c := Config{Env: "production", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "oidc", IssuerURL: "http://test-issuer:8091", ClientID: "tasks", RedirectURL: "https://tasks.test/auth/callback", SessionSecret: "01234567890123456789012345678901"}
	if c.Validate() == nil {
		t.Fatal("expected test issuer rejection")
	}
}
func TestValidateAcceptsOIDC(t *testing.T) {
	c := Config{Env: "test", DatabaseURL: "postgres://tasks@db/primer_tasks", AuthMode: "oidc", IssuerURL: "http://issuer:8091", ClientID: "tasks", RedirectURL: "http://tasks/auth/callback"}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
}
