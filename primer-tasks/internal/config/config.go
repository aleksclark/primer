package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct{ Env, DatabaseURL, AuthMode, IssuerURL, ClientID, RedirectURL, PublicOrigin, SessionSecret, TestPrincipal string }

func Load() (Config, error) {
	c := Config{Env: value("TASKS_ENV", "development"), DatabaseURL: os.Getenv("TASKS_DATABASE_URL"), AuthMode: value("TASKS_AUTH_MODE", "test"), IssuerURL: strings.TrimRight(value("TASKS_ISSUER_URL", "http://test-issuer:8091"), "/"), ClientID: value("TASKS_OIDC_CLIENT_ID", "primer-tasks-web"), PublicOrigin: value("TASKS_PUBLIC_ORIGIN", "http://127.0.0.1:8080"), SessionSecret: os.Getenv("TASKS_SESSION_SECRET"), TestPrincipal: value("TASKS_TEST_PRINCIPAL", "parent-a")}
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
	if c.AuthMode != "test" && c.AuthMode != "oidc" {
		return fmt.Errorf("TASKS_AUTH_MODE must be test or oidc")
	}
	if c.Env == "production" && c.AuthMode == "test" {
		return fmt.Errorf("test authentication is forbidden in production")
	}
	if c.Env == "production" && (strings.Contains(strings.ToLower(c.IssuerURL), "test-issuer") || strings.Contains(strings.ToLower(c.IssuerURL), "localhost") || strings.Contains(strings.ToLower(c.IssuerURL), "127.0.0.1")) {
		return fmt.Errorf("production cannot use a development or test issuer")
	}
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
	if c.Env == "production" && len(c.SessionSecret) < 32 {
		return fmt.Errorf("TASKS_SESSION_SECRET must contain at least 32 bytes in production")
	}
	if c.AuthMode == "oidc" && (u.Scheme != "https" && c.Env == "production") {
		return fmt.Errorf("production OIDC issuer must use HTTPS")
	}
	return nil
}
func value(k, f string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return f
}
