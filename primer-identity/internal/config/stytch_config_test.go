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
	return &config.Config{
		DatabaseURL:           "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable",
		Issuer:                "https://id.example",
		Env:                   "production",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1,
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
	cfg.Stytch.Enabled = false
	cfg.Stytch.ProjectID = ""
	cfg.Stytch.Secret = ""
	require.NoError(t, cfg.Validate())
}
