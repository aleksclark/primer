package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

func TestLoadDefaultsInDevelopment(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:s3cret@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8090, cfg.Port, "identity default port must not clash with LMS/TV")
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 10*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, int64(1<<20), cfg.HTTPMaxBodyBytes)
	assert.Equal(t, "http://localhost:8090", cfg.Issuer)
	assert.Contains(t, cfg.DatabaseURL, "primer_identity")
	assert.Equal(t, "0.0.0.0:8090", cfg.Addr())
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("IDENTITY_HOST", "127.0.0.1")
	t.Setenv("IDENTITY_PORT", "9090")
	t.Setenv("IDENTITY_ENV", "test")
	t.Setenv("IDENTITY_LOG_LEVEL", "debug")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@db:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example.test")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("IDENTITY_HTTP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("IDENTITY_HTTP_MAX_BODY_BYTES", "4096")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9090", cfg.Addr())
	assert.Equal(t, "test", cfg.Env)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "https://id.example.test", cfg.Issuer)
	assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 2*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, int64(4096), cfg.HTTPMaxBodyBytes)
}

func TestLoadFailFastProductionMissingDatabaseURL(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", "")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	// Clear any ambient default by forcing empty after Process defaults.
	t.Setenv("IDENTITY_DATABASE_URL", " ")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadFailFastProductionMissingIssuer(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@db:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "issuer")
}

func TestLoadFailFastRejectsForeignDatabaseNames(t *testing.T) {
	cases := []string{
		"postgres://u:p@localhost:5432/primer?sslmode=disable",
		"postgres://u:p@localhost:5432/primer_tv?sslmode=disable",
		"postgres://u:p@localhost:5432/curriculum_studio?sslmode=disable",
		"postgres://u:p@localhost:5432/primer_test?sslmode=disable",
		// Keyword/libpq form previously bypassed url.Parse.Path checks.
		"host=localhost user=u password=p dbname=primer sslmode=disable",
		"host=localhost dbname=primer_tv",
		"user=identity password=x dbname=curriculum_studio host=db",
		// Double-slash / trailing-slash URI forms normalize via pgx.
		"postgres://u:p@localhost:5432//primer",
		"postgres://u:p@localhost:5432/primer/",
		"postgres://u:p@localhost/PRIMER",
		"postgresql://u:p@localhost:5432/studio?sslmode=disable",
		"host=localhost dbname= tv ",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("IDENTITY_ENV", "production")
			t.Setenv("IDENTITY_DATABASE_URL", dsn)
			t.Setenv("IDENTITY_ISSUER", "https://id.example")

			_, err := config.Load()
			require.Error(t, err)
			msg := strings.ToLower(err.Error())
			assert.True(t,
				strings.Contains(msg, "database") || strings.Contains(msg, "reserved"),
				"expected reserved/database rejection, got %v", err,
			)
		})
	}
}

func TestLoadAcceptsIdentityDatabaseDSNForms(t *testing.T) {
	cases := []string{
		"postgres://identity:p@localhost:5432/primer_identity?sslmode=disable",
		"postgresql://identity:p@db:5432/primer_identity",
		"host=localhost user=identity password=p dbname=primer_identity sslmode=disable",
		"user=identity password=p dbname=primer_identity host=127.0.0.1 port=5432",
		"postgres://identity:p@localhost:5432/primer_identity_test?sslmode=disable",
		"host=localhost dbname=PRIMER_IDENTITY",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("IDENTITY_ENV", "development")
			t.Setenv("IDENTITY_DATABASE_URL", dsn)
			t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

			cfg, err := config.Load()
			require.NoError(t, err)
			assert.Equal(t, dsn, cfg.DatabaseURL)
		})
	}
}

func TestLoadRejectsKeywordDSNMissingDatabaseName(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", "host=localhost user=identity password=p sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadRejectsMalformedPort(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_PORT", "not-a-number")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

	_, err := config.Load()
	assert.Error(t, err)
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "staging")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "env")
}

func TestLoadRejectsNonPositiveTimeouts(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "0s")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "shutdown")
}

func TestLoadRejectsNonPositiveHTTPTimeoutAndBody(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	t.Setenv("IDENTITY_HTTP_READ_HEADER_TIMEOUT", "0s")
	_, err := config.Load()
	require.Error(t, err)

	t.Setenv("IDENTITY_HTTP_READ_HEADER_TIMEOUT", "1s")
	t.Setenv("IDENTITY_HTTP_MAX_BODY_BYTES", "0")
	_, err = config.Load()
	require.Error(t, err)
}

func TestLoadRejectsDBURLWithoutName(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:pw@localhost:5432/")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	_, err := config.Load()
	require.Error(t, err)
}

func TestValidatePortZeroAllowed(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:           "postgres://identity:pw@localhost:5432/primer_identity?sslmode=disable",
		Host:                  "127.0.0.1",
		Port:                  0,
		Env:                   "test",
		Issuer:                "http://id",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1,
	}
	require.NoError(t, cfg.Validate())
}

func TestValidateRejectsNegativePort(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:           "postgres://identity:pw@localhost:5432/primer_identity?sslmode=disable",
		Port:                  -1,
		Env:                   "test",
		Issuer:                "http://id",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1,
	}
	require.Error(t, cfg.Validate())
}
