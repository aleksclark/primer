package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

func TestLoadStytchDefaultsAndEnvironment(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "test")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example.test")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-test-example")
	t.Setenv("IDENTITY_STYTCH_SECRET", "secret-value")
	t.Setenv("IDENTITY_STYTCH_ENV", "test")
	t.Setenv("IDENTITY_BROKER_ALLOWED_ORIGIN", "https://id.example.test")
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", "https://id.example.test/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", "https://id.example.test/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", "https://id.example.test/broker/stytch/callback")
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", "public-token-test")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.Stytch.Enabled)
	assert.Equal(t, "project-test-example", cfg.Stytch.ProjectID)
	assert.Equal(t, "test", cfg.Stytch.Env)
	assert.Equal(t, 3*time.Second, cfg.Stytch.RequestTimeout)
	assert.Equal(t, 15*time.Second, cfg.Stytch.PositiveCacheTTL)
	assert.Equal(t, 5*time.Second, cfg.Stytch.NegativeCacheTTL)
	assert.Equal(t, 10000, cfg.Stytch.PositiveCacheCapacity)
	assert.Equal(t, 2000, cfg.Stytch.NegativeCacheCapacity)
}

func validConfig() *config.Config {
	cfg := &config.Config{
		DatabaseURL:           "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable",
		Issuer:                "https://id.example",
		Env:                   "production",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1,
		StateSealKeys:         "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", StateSealActiveVersion: 1,
		StateHashPeppers: "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", StateHashActiveVersion: 1,
		BrokerCookiePeppers: "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", BrokerCookieActiveVersion: 1,
		AuthorizationCodePeppers: "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", AuthorizationCodeActiveVersion: 1,
		BrokerAllowedOrigin:        "https://id.example",
		BrokerDiscoveryRedirectURL: "https://id.example/broker/stytch/callback",
		BrokerLoginRedirectURL:     "https://id.example/broker/stytch/callback",
		BrokerSignupRedirectURL:    "https://id.example/broker/stytch/callback",
		StytchPublicToken:          "public-token-live-example",
		Stytch: config.StytchConfig{
			Enabled:               true,
			ProjectID:             "project-live-example",
			Secret:                "secret",
			Env:                   "live",
			RequestTimeout:        3 * time.Second,
			PositiveCacheTTL:      15 * time.Second,
			NegativeCacheTTL:      5 * time.Second,
			PositiveCacheCapacity: 10000,
			NegativeCacheCapacity: 2000,
		},
	}
	cfg.Key.SetSealSecretForTest("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	return cfg
}

func TestValidateStytchRejectsPartialCredentials(t *testing.T) {
	for _, tc := range []struct {
		name    string
		project string
		secret  string
	}{
		{"missing secret", "project-live-example", ""},
		{"missing project", "", "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Stytch.ProjectID = tc.project
			cfg.Stytch.Secret = tc.secret
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "credential")
		})
	}
}

func TestValidateStytchRejectsInvalidEnvironmentTTLAndCapacity(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.StytchConfig)
		want   string
	}{
		{"environment", func(c *config.StytchConfig) { c.Env = "staging" }, "env"},
		{"positive ttl", func(c *config.StytchConfig) { c.PositiveCacheTTL = 0 }, "ttl"},
		{"negative ttl", func(c *config.StytchConfig) { c.NegativeCacheTTL = -time.Second }, "ttl"},
		{"positive capacity", func(c *config.StytchConfig) { c.PositiveCacheCapacity = 0 }, "capacity"},
		{"negative capacity", func(c *config.StytchConfig) { c.NegativeCacheCapacity = -1 }, "capacity"},
		{"request timeout", func(c *config.StytchConfig) { c.RequestTimeout = 4 * time.Second }, "timeout"},
		{"positive ttl over limit", func(c *config.StytchConfig) { c.PositiveCacheTTL = 16 * time.Second }, "ttl"},
		{"negative ttl over limit", func(c *config.StytchConfig) { c.NegativeCacheTTL = 6 * time.Second }, "ttl"},
		{"positive capacity over limit", func(c *config.StytchConfig) { c.PositiveCacheCapacity = 10001 }, "capacity"},
		{"negative capacity over limit", func(c *config.StytchConfig) { c.NegativeCacheCapacity = 2001 }, "capacity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(&cfg.Stytch)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tc.want)
		})
	}
}

func TestValidateStytchRejectsProductionHTTPBaseURI(t *testing.T) {
	cfg := validConfig()
	cfg.Stytch.BaseURI = "http://localhost:9999"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "base uri")
}

func TestValidateProductionRequiresLiveStytchAndRejectsEveryBaseURI(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     string
		baseURI string
	}{
		{name: "test environment", env: "test"},
		{name: "empty environment", env: ""},
		{name: "https override", env: "live", baseURI: "https://stytch.internal.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Stytch.Env = tc.env
			cfg.Stytch.BaseURI = tc.baseURI
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "production")
		})
	}
}

func TestValidateDevelopmentAndTestAllowOnlySafeAbsoluteCustomBaseURIs(t *testing.T) {
	for _, serviceEnv := range []string{"development", "test"} {
		t.Run(serviceEnv, func(t *testing.T) {
			for _, baseURI := range []string{
				"http://localhost:9999",
				"https://stytch.test.example/path",
			} {
				cfg := validConfig()
				cfg.Env = serviceEnv
				cfg.Stytch.Env = "test"
				cfg.Stytch.ProjectID = "project-test-example"
				cfg.Stytch.BaseURI = baseURI
				cfg.BrokerAllowedOrigin = "http://localhost:8090"
				cfg.BrokerDiscoveryRedirectURL = "http://localhost:8090/broker/stytch/callback"
				cfg.BrokerLoginRedirectURL = "http://localhost:8090/broker/stytch/callback"
				cfg.BrokerSignupRedirectURL = "http://localhost:8090/broker/stytch/callback"
				cfg.StytchPublicToken = "public-token-dev"
				require.NoError(t, cfg.Validate(), baseURI)
			}

			for _, baseURI := range []string{
				"/relative",
				"ftp://stytch.example",
				"https://user:password@stytch.example",
				"https://stytch.example/path?raw=query",
				"https://stytch.example/path%3Fdecoded=query",
				"https://stytch.example/path#fragment",
				"https://stytch.example/path%23decoded-fragment",
			} {
				cfg := validConfig()
				cfg.Env = serviceEnv
				cfg.Stytch.Env = "test"
				cfg.Stytch.ProjectID = "project-test-example"
				cfg.Stytch.BaseURI = baseURI
				err := cfg.Validate()
				require.Error(t, err, baseURI)
				assert.Contains(t, strings.ToLower(err.Error()), "base uri", baseURI)
			}
		})
	}
}

func TestValidateRejectsLiveStytchOverrideOutsideProduction(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "development"
	cfg.Stytch.Env = "live"
	cfg.Stytch.ProjectID = "project-live-example"
	cfg.Stytch.BaseURI = "https://stytch.internal.example"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "base uri")
}

func TestValidateRejectsProductionLiveStytchWithTestProject(t *testing.T) {
	cfg := validConfig()
	cfg.Stytch.ProjectID = "project-test-example"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "project")
}

func TestValidateRejectsTestStytchWithLiveProject(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "test"
	cfg.Stytch.Env = "test"
	cfg.Stytch.ProjectID = "project-live-example"
	cfg.Stytch.BaseURI = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "project")
}

func TestValidateAllowsTestProjectWithTestOverride(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "development"
	cfg.Stytch.Env = "test"
	cfg.Stytch.ProjectID = "project-test-example"
	cfg.Stytch.BaseURI = "https://stytch.test.example/path"
	cfg.BrokerAllowedOrigin = "http://localhost:8090"
	cfg.BrokerDiscoveryRedirectURL = "http://localhost:8090/broker/stytch/callback"
	cfg.BrokerLoginRedirectURL = "http://localhost:8090/broker/stytch/callback"
	cfg.BrokerSignupRedirectURL = "http://localhost:8090/broker/stytch/callback"
	cfg.StytchPublicToken = "public-token-dev"

	require.NoError(t, cfg.Validate())
}

func TestValidateAllowsLiveProjectWithoutOverride(t *testing.T) {
	cfg := validConfig()
	cfg.Stytch.BaseURI = ""

	require.NoError(t, cfg.Validate())
}

func TestValidateStytchProductionRequiresCredentialsWhenEnabled(t *testing.T) {
	cfg := validConfig()
	cfg.Stytch.ProjectID = ""
	cfg.Stytch.Secret = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "credential")
}

func TestValidateStytchDisabledDoesNotRequireCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "development"
	cfg.Stytch.Enabled = false
	cfg.Stytch.ProjectID = ""
	cfg.Stytch.Secret = ""
	cfg.Stytch.Env = "test"
	require.NoError(t, cfg.Validate())
}

func TestValidateProductionRequiresVersionedSecretsAndRejectsInvalidIB1Bounds(t *testing.T) {
	cfg := validConfig()
	cfg.StateSealKeys = ""
	require.Error(t, cfg.Validate())
	cfg = validConfig()
	cfg.StateSealActiveVersion = 9
	require.Error(t, cfg.Validate())
	cfg = validConfig()
	cfg.ProviderProofCacheTTL = 16 * time.Second
	require.Error(t, cfg.Validate())
	cfg = validConfig()
	cfg.ProviderProofCacheCapacity = 2048
	require.Error(t, cfg.Validate())
	cfg = validConfig()
	cfg.ProviderRevalidationDeadline = 3 * time.Second
	require.Error(t, cfg.Validate())
}

func TestLoadProductionFailsWithoutPrefixedSealSecretsDespiteBareKeys(t *testing.T) {
	clearIdentityEnv(t)
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_ENV", "live")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-live-example")
	t.Setenv("IDENTITY_STYTCH_SECRET", "secret-value-must-not-leak")
	t.Setenv("STATE_SEAL_KEYS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("STATE_SEAL_ACTIVE_VERSION", "1")
	cfg, err := config.Load()
	require.Error(t, err)
	require.Nil(t, cfg)
	require.NotContains(t, err.Error(), "secret-value-must-not-leak")
}
