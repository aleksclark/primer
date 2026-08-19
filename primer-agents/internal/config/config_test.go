package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/config"
)

func TestLoadDefaultsInDevelopment(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8091, cfg.Port, "agents default port must not clash with LMS/TV/Identity/Studio")
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.False(t, cfg.WorkerEnabled)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 10*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, 30*time.Second, cfg.HTTPReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.HTTPWriteTimeout)
	assert.Equal(t, 60*time.Second, cfg.HTTPIdleTimeout)
	assert.Equal(t, int64(1<<20), cfg.HTTPMaxBodyBytes)
	assert.Equal(t, "0.0.0.0:8091", cfg.Addr())
}

func TestDisabledLiveLLMDoesNotInspectAmbientProviderKey(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM", "0")
	t.Setenv("OPENAI_API_KEY", "ambient-marker-must-not-be-read")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_API_KEY", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.False(t, cfg.LiveLLMEnabled)
	assert.Empty(t, cfg.LiveLLMAPIKey())
}

func TestLiveLLMRequiresExplicitBoundedConfiguration(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM", "1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_API_KEY", "named-live-key")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_MODEL", "gpt-4o-mini")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_MAX_CALLS", "1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_TIMEOUT", "20s")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o-mini", cfg.LiveLLMModel)
	assert.Equal(t, 1, cfg.LiveLLMMaxCalls)
	assert.Equal(t, "named-live-key", cfg.LiveLLMAPIKey())
}

func TestLiveLLMAcceptsAllowlistedCloudflareGatewayURL(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM", "1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_API_KEY", "named-live-key")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_BASE_URL", "https://gateway.ai.cloudflare.com/v1/a9d106d880527eaecdaf7835b792849d/curri-gateway/openai")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_MODEL", "gpt-4o-mini")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_MAX_CALLS", "1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_TIMEOUT", "20s")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, config.ApprovedLiveLLMBaseURL(cfg.LiveLLMBaseURL))
}

func TestLiveLLMRejectsUnsafeOrExpensiveConfiguration(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM", "1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_API_KEY", "named-live-key")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_BASE_URL", "http://127.0.0.1:1234/v1")
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_MODEL", "expensive-model")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "model")
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_HOST", "127.0.0.1")
	t.Setenv("PRIMER_AGENTS_PORT", "9099")
	t.Setenv("PRIMER_AGENTS_ENV", "test")
	t.Setenv("PRIMER_AGENTS_LOG_LEVEL", "debug")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@db:5432/primer_agents?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("PRIMER_AGENTS_HTTP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("PRIMER_AGENTS_HTTP_READ_TIMEOUT", "4s")
	t.Setenv("PRIMER_AGENTS_HTTP_WRITE_TIMEOUT", "5s")
	t.Setenv("PRIMER_AGENTS_HTTP_IDLE_TIMEOUT", "7s")
	t.Setenv("PRIMER_AGENTS_HTTP_MAX_BODY_BYTES", "4096")
	t.Setenv("PRIMER_AGENTS_WORKER_ENABLED", "true")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9099", cfg.Addr())
	assert.Equal(t, "test", cfg.Env)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.WorkerEnabled)
	assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 2*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, 4*time.Second, cfg.HTTPReadTimeout)
	assert.Equal(t, 5*time.Second, cfg.HTTPWriteTimeout)
	assert.Equal(t, 7*time.Second, cfg.HTTPIdleTimeout)
	assert.Equal(t, int64(4096), cfg.HTTPMaxBodyBytes)
}

func TestLoadFailFastMissingDatabaseURL(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "production")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", " ")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadIgnoresBareDatabaseURL(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://lms:x@localhost:5432/primer?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", " ")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadIgnoresForeignAmbientDSNs(t *testing.T) {
	for _, k := range []string{
		"DATABASE_URL",
		"STUDIO_DATABASE_URL",
		"IDENTITY_DATABASE_URL",
		"TV_DATABASE_URL",
	} {
		t.Setenv(k, "postgres://other:x@localhost:5432/primer?sslmode=disable")
	}
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", " ")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadRejectsForeignDatabaseNames(t *testing.T) {
	cases := []string{
		"postgres://u:x@localhost:5432/primer?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_tv?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_identity?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_identity_test?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_test?sslmode=disable",
		"postgres://u:x@localhost:5432/curriculum_studio?sslmode=disable",
		"postgres://u:x@localhost:5432/curriculum_studio_test?sslmode=disable",
		"postgres://u:x@localhost:5432/tv?sslmode=disable",
		"host=localhost user=u password=p dbname=primer sslmode=disable",
		"host=localhost dbname=primer_tv",
		"user=u password=x dbname=primer_identity host=db",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("PRIMER_AGENTS_ENV", "development")
			t.Setenv("PRIMER_AGENTS_DATABASE_URL", dsn)

			_, err := config.Load()
			require.Error(t, err)
			msg := strings.ToLower(err.Error())
			assert.True(t,
				strings.Contains(msg, "forbidden") || strings.Contains(msg, "database"),
				"expected rejection, got %v", err)
		})
	}
}

func TestLoadAcceptsAgentsDatabaseDSNForms(t *testing.T) {
	cases := []string{
		"postgres://agents:x@localhost:5432/primer_agents?sslmode=disable",
		"postgresql://agents:x@db:5432/primer_agents",
		"host=localhost user=agents password=p dbname=primer_agents sslmode=disable",
		"postgres://agents:x@localhost:5432/primer_agents_test?sslmode=disable",
		"host=localhost dbname=PRIMER_AGENTS",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("PRIMER_AGENTS_ENV", "development")
			t.Setenv("PRIMER_AGENTS_DATABASE_URL", dsn)

			cfg, err := config.Load()
			require.NoError(t, err)
			assert.Equal(t, dsn, cfg.DatabaseURL)
		})
	}
}

func TestValidatePortZeroAllowed(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:           "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable",
		Host:                  "127.0.0.1",
		Port:                  0,
		Env:                   "test",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPReadTimeout:       time.Second,
		HTTPWriteTimeout:      time.Second,
		HTTPIdleTimeout:       time.Second,
		HTTPMaxBodyBytes:      1,
	}
	require.NoError(t, cfg.Validate())
}

func TestValidateRejectsNegativePort(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:           "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable",
		Port:                  -1,
		Env:                   "test",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPReadTimeout:       time.Second,
		HTTPWriteTimeout:      time.Second,
		HTTPIdleTimeout:       time.Second,
		HTTPMaxBodyBytes:      1,
	}
	require.Error(t, cfg.Validate())
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "staging")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "env")
}

func TestLoadRejectsNonPositiveTimeouts(t *testing.T) {
	base := func() {
		t.Setenv("PRIMER_AGENTS_ENV", "development")
		t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")
		t.Setenv("PRIMER_AGENTS_HTTP_READ_HEADER_TIMEOUT", "1s")
		t.Setenv("PRIMER_AGENTS_HTTP_READ_TIMEOUT", "1s")
		t.Setenv("PRIMER_AGENTS_HTTP_WRITE_TIMEOUT", "1s")
		t.Setenv("PRIMER_AGENTS_HTTP_IDLE_TIMEOUT", "1s")
		t.Setenv("PRIMER_AGENTS_HTTP_MAX_BODY_BYTES", "1024")
		t.Setenv("PRIMER_AGENTS_SHUTDOWN_TIMEOUT", "1s")
	}

	base()
	t.Setenv("PRIMER_AGENTS_SHUTDOWN_TIMEOUT", "0s")
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "shutdown")

	base()
	t.Setenv("PRIMER_AGENTS_HTTP_READ_HEADER_TIMEOUT", "0s")
	_, err = config.Load()
	require.Error(t, err)

	base()
	t.Setenv("PRIMER_AGENTS_HTTP_MAX_BODY_BYTES", "0")
	_, err = config.Load()
	require.Error(t, err)
}

func TestLoadRejectsMalformedPort(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENV", "development")
	t.Setenv("PRIMER_AGENTS_PORT", "not-a-number")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@localhost:5432/primer_agents?sslmode=disable")

	_, err := config.Load()
	assert.Error(t, err)
}

func TestValidateConfigIsForbiddenProductionHostCoverage(t *testing.T) {
	// Exercises the isForbiddenProductionHost helper via production config validation.
	t.Setenv("PRIMER_AGENTS_ENV", "production")
	t.Setenv("PRIMER_AGENTS_DATABASE_URL", "postgres://agents:x@db:5432/primer_agents?sslmode=disable")
	// Use HTTPS to pass the scheme check, but localhost triggers the forbidden host check.
	t.Setenv("PRIMER_AGENTS_IDENTITY_JWKS_URL", "https://localhost:8090/.well-known/jwks.json")
	t.Setenv("PRIMER_AGENTS_IDENTITY_ISSUER", "https://identity.example")
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "loopback")
}
