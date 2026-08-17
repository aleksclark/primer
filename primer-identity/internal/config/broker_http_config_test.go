package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

const (
	brokerAllowedOrigin = "https://id.example"
	brokerDiscoveryURL  = "https://id.example/broker/stytch/callback"
	brokerLoginURL      = "https://id.example/broker/stytch/callback"
	brokerSignupURL     = "https://id.example/broker/stytch/callback"
	brokerPublicToken   = "public-token-live-example"
	hostileBareOrigin   = "https://hostile-bare.example"
	hostileBareToken    = "hostile-bare-public-token"
)

func setBrokerHTTPEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_BROKER_ALLOWED_ORIGIN", brokerAllowedOrigin)
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", brokerDiscoveryURL)
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", brokerLoginURL)
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", brokerSignupURL)
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", brokerPublicToken)
}

func TestLoadBrokerHTTPFieldsFromPrefixedEnv(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	baseEnv(t)
	setBrokerHTTPEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, brokerAllowedOrigin, cfg.BrokerAllowedOrigin)
	assert.Equal(t, brokerDiscoveryURL, cfg.BrokerDiscoveryRedirectURL)
	assert.Equal(t, brokerLoginURL, cfg.BrokerLoginRedirectURL)
	assert.Equal(t, brokerSignupURL, cfg.BrokerSignupRedirectURL)
	assert.Equal(t, brokerPublicToken, cfg.StytchPublicToken)
	assert.False(t, cfg.InsecureBrokerCookie)
}

func TestLoadIgnoresHostileBareBrokerHTTPNames(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	baseEnv(t)
	t.Setenv("ALLOWED_ORIGIN", hostileBareOrigin)
	t.Setenv("DISCOVERY_REDIRECT_URL", hostileBareOrigin+"/callback")
	t.Setenv("LOGIN_REDIRECT_URL", hostileBareOrigin+"/login")
	t.Setenv("SIGNUP_REDIRECT_URL", hostileBareOrigin+"/signup")
	t.Setenv("PUBLIC_TOKEN", hostileBareToken)
	t.Setenv("STYTCH_PUBLIC_TOKEN", hostileBareToken)
	t.Setenv("INSECURE_BROKER_COOKIE", "true")
	t.Setenv("BROKER_ALLOWED_ORIGIN", hostileBareOrigin)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.BrokerAllowedOrigin)
	assert.Empty(t, cfg.BrokerDiscoveryRedirectURL)
	assert.Empty(t, cfg.BrokerLoginRedirectURL)
	assert.Empty(t, cfg.BrokerSignupRedirectURL)
	assert.Empty(t, cfg.StytchPublicToken)
	assert.False(t, cfg.InsecureBrokerCookie)
}

func TestValidateProductionRequiresExactHTTPSBrokerHTTPAndPublicToken(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.Config)
		want   string
	}{
		{"missing origin", func(c *config.Config) { c.BrokerAllowedOrigin = "" }, "origin"},
		{"http origin", func(c *config.Config) { c.BrokerAllowedOrigin = "http://id.example" }, "origin"},
		{"wildcard origin", func(c *config.Config) { c.BrokerAllowedOrigin = "https://id.example,https://other.example" }, "origin"},
		{"missing discovery", func(c *config.Config) { c.BrokerDiscoveryRedirectURL = "" }, "redirect"},
		{"http login", func(c *config.Config) { c.BrokerLoginRedirectURL = "http://id.example/callback" }, "redirect"},
		{"query signup", func(c *config.Config) { c.BrokerSignupRedirectURL = brokerSignupURL + "?x=1" }, "redirect"},
		{"missing public token", func(c *config.Config) { c.StytchPublicToken = "" }, "public token"},
		{"insecure cookie", func(c *config.Config) { c.InsecureBrokerCookie = true }, "insecure"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBrokerProductionConfig()
			tc.mutate(cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tc.want)
			assert.NotContains(t, err.Error(), productionStytchSecret)
			assert.NotContains(t, err.Error(), brokerPublicToken)
		})
	}
}

func TestValidateDevelopmentAllowsDisabledBrokerWithoutHTTPFields(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "development"
	cfg.Stytch.Enabled = false
	cfg.Stytch.ProjectID = ""
	cfg.Stytch.Secret = ""
	cfg.Stytch.Env = "test"
	cfg.BrokerAllowedOrigin = ""
	cfg.BrokerDiscoveryRedirectURL = ""
	cfg.BrokerLoginRedirectURL = ""
	cfg.BrokerSignupRedirectURL = ""
	cfg.StytchPublicToken = ""
	require.NoError(t, cfg.Validate())
	assert.False(t, cfg.BrokerEnabled())
}

func TestValidateDevelopmentEnabledStytchRequiresBrokerHTTPFields(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "development"
	cfg.Stytch.Enabled = true
	cfg.Stytch.Env = "test"
	cfg.Stytch.ProjectID = "project-test-example"
	cfg.Stytch.Secret = "secret"
	cfg.BrokerAllowedOrigin = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "origin")
}

func TestValidateDevelopmentAllowsHTTPOriginAndRedirects(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "development"
	cfg.Stytch.Enabled = true
	cfg.Stytch.Env = "test"
	cfg.Stytch.ProjectID = "project-test-example"
	cfg.Stytch.Secret = "secret"
	cfg.BrokerAllowedOrigin = "http://localhost:8090"
	cfg.BrokerDiscoveryRedirectURL = "http://localhost:8090/broker/stytch/callback"
	cfg.BrokerLoginRedirectURL = "http://localhost:8090/broker/stytch/callback"
	cfg.BrokerSignupRedirectURL = "http://localhost:8090/broker/stytch/callback"
	cfg.StytchPublicToken = "public-token-dev"
	cfg.InsecureBrokerCookie = true
	require.NoError(t, cfg.Validate())
	assert.True(t, cfg.BrokerEnabled())
}

func TestValidateRejectsMalformedBrokerOriginAndRedirectsWithoutEchoingThem(t *testing.T) {
	const hostile = "https://user:pass@evil.example/steal#frag"
	cfg := validBrokerProductionConfig()
	cfg.BrokerAllowedOrigin = hostile
	err := cfg.Validate()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "user:pass")
	assert.NotContains(t, err.Error(), hostile)
}

func TestLoadProductionFailsWhenPrefixedBrokerOriginMissingDespiteBareOrigin(t *testing.T) {
	clearPrefixedIdentityEnv(t)
	plantHostileBareEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", identityDSN)
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_ENV", "live")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-live-example")
	t.Setenv("IDENTITY_STYTCH_SECRET", productionStytchSecret)
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", brokerPublicToken)
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", brokerDiscoveryURL)
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", brokerLoginURL)
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", brokerSignupURL)
	t.Setenv("IDENTITY_STATE_SEAL_KEYS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_STATE_SEAL_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_STATE_HASH_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_STATE_HASH_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_BROKER_COOKIE_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_BROKER_COOKIE_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION", "1")
	t.Setenv("ALLOWED_ORIGIN", hostileBareOrigin)
	require.NoError(t, os.Unsetenv("IDENTITY_BROKER_ALLOWED_ORIGIN"))

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, strings.ToLower(err.Error()), "origin")
	assert.NotContains(t, err.Error(), productionStytchSecret)
	assert.NotContains(t, err.Error(), hostileBareOrigin)
}

func validBrokerProductionConfig() *config.Config {
	cfg := validConfig()
	cfg.BrokerAllowedOrigin = brokerAllowedOrigin
	cfg.BrokerDiscoveryRedirectURL = brokerDiscoveryURL
	cfg.BrokerLoginRedirectURL = brokerLoginURL
	cfg.BrokerSignupRedirectURL = brokerSignupURL
	cfg.StytchPublicToken = brokerPublicToken
	cfg.InsecureBrokerCookie = false
	return cfg
}
