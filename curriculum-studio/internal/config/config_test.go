package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/config"
)

func TestLoadDefaultsInDevelopment(t *testing.T) {
	t.Setenv("STUDIO_ENV", "development")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8088, cfg.Port, "studio default port must not clash with LMS/TV/Identity")
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "jwks", cfg.AuthMode)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 10*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, 30*time.Second, cfg.HTTPReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.HTTPWriteTimeout)
	assert.Equal(t, 60*time.Second, cfg.HTTPIdleTimeout)
	assert.Equal(t, int64(1<<20), cfg.HTTPMaxBodyBytes)
	assert.Contains(t, cfg.DatabaseURL, "curriculum_studio")
	assert.Equal(t, "0.0.0.0:8088", cfg.Addr())
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("STUDIO_HOST", "127.0.0.1")
	t.Setenv("STUDIO_PORT", "9091")
	t.Setenv("STUDIO_ENV", "test")
	t.Setenv("STUDIO_LOG_LEVEL", "debug")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@db:5432/curriculum_studio?sslmode=disable")
	t.Setenv("STUDIO_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("STUDIO_HTTP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("STUDIO_HTTP_READ_TIMEOUT", "4s")
	t.Setenv("STUDIO_HTTP_WRITE_TIMEOUT", "5s")
	t.Setenv("STUDIO_HTTP_IDLE_TIMEOUT", "7s")
	t.Setenv("STUDIO_HTTP_MAX_BODY_BYTES", "4096")
	t.Setenv("STUDIO_AUTH_MODE", "test")
	t.Setenv("STUDIO_ARTIFACT_STORE_DIR", "/tmp/studio-artifacts")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9091", cfg.Addr())
	assert.Equal(t, "test", cfg.Env)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "test", cfg.AuthMode)
	assert.Equal(t, "/tmp/studio-artifacts", cfg.ArtifactStoreDir)
	assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 2*time.Second, cfg.HTTPReadHeaderTimeout)
	assert.Equal(t, 4*time.Second, cfg.HTTPReadTimeout)
	assert.Equal(t, 5*time.Second, cfg.HTTPWriteTimeout)
	assert.Equal(t, 7*time.Second, cfg.HTTPIdleTimeout)
	assert.Equal(t, int64(4096), cfg.HTTPMaxBodyBytes)
}

func TestLoadFailFastProductionMissingDatabaseURL(t *testing.T) {
	t.Setenv("STUDIO_ENV", "production")
	t.Setenv("STUDIO_DATABASE_URL", " ")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadIgnoresBareDatabaseURL(t *testing.T) {
	t.Setenv("STUDIO_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://lms:x@localhost:5432/primer?sslmode=disable")
	t.Setenv("STUDIO_DATABASE_URL", " ")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadFailFastRejectsForeignDatabaseNames(t *testing.T) {
	cases := []string{
		"postgres://u:x@localhost:5432/primer?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_tv?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_identity?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_identity_test?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_test?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_tv_test?sslmode=disable",
		"postgres://u:x@localhost:5432/tv_test?sslmode=disable",
		"host=localhost user=u password=p dbname=primer sslmode=disable",
		"host=localhost dbname=primer_tv",
		"user=studio password=x dbname=primer_identity host=db",
		"postgres://u:x@localhost:5432//primer",
		"postgres://u:x@localhost:5432/primer/",
		"postgres://u:x@localhost/PRIMER",
		"host=localhost dbname= tv ",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("STUDIO_ENV", "production")
			t.Setenv("STUDIO_DATABASE_URL", dsn)

			_, err := config.Load()
			require.Error(t, err)
			msg := strings.ToLower(err.Error())
			assert.True(t,
				strings.Contains(msg, "forbidden") ||
					strings.Contains(msg, "database") ||
					strings.Contains(msg, "reserved"),
				"expected forbidden/database rejection, got %v", err,
			)
		})
	}
}

func TestLoadAcceptsStudioDatabaseDSNForms(t *testing.T) {
	cases := []string{
		"postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable",
		"postgresql://studio:x@db:5432/curriculum_studio",
		"host=localhost user=studio password=p dbname=curriculum_studio sslmode=disable",
		"user=studio password=p dbname=curriculum_studio host=127.0.0.1 port=5432",
		"postgres://studio:x@localhost:5432/curriculum_studio_test?sslmode=disable",
		"host=localhost dbname=CURRICULUM_STUDIO",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("STUDIO_ENV", "development")
			t.Setenv("STUDIO_DATABASE_URL", dsn)

			cfg, err := config.Load()
			require.NoError(t, err)
			assert.Equal(t, dsn, cfg.DatabaseURL)
		})
	}
}

func TestLoadRejectsKeywordDSNMissingDatabaseName(t *testing.T) {
	t.Setenv("STUDIO_ENV", "production")
	t.Setenv("STUDIO_DATABASE_URL", "host=localhost user=studio password=p sslmode=disable")
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "database")
}

func TestLoadRejectsMalformedPort(t *testing.T) {
	t.Setenv("STUDIO_ENV", "development")
	t.Setenv("STUDIO_PORT", "not-a-number")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable")

	_, err := config.Load()
	assert.Error(t, err)
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("STUDIO_ENV", "staging")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "env")
}

func TestLoadRejectsNonPositiveTimeouts(t *testing.T) {
	t.Setenv("STUDIO_ENV", "development")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable")
	t.Setenv("STUDIO_SHUTDOWN_TIMEOUT", "0s")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "shutdown")
}

func TestLoadRejectsNonPositiveHTTPTimeoutAndBody(t *testing.T) {
	base := func() {
		t.Setenv("STUDIO_ENV", "development")
		t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable")
		t.Setenv("STUDIO_HTTP_READ_HEADER_TIMEOUT", "1s")
		t.Setenv("STUDIO_HTTP_READ_TIMEOUT", "1s")
		t.Setenv("STUDIO_HTTP_WRITE_TIMEOUT", "1s")
		t.Setenv("STUDIO_HTTP_IDLE_TIMEOUT", "1s")
		t.Setenv("STUDIO_HTTP_MAX_BODY_BYTES", "1024")
		t.Setenv("STUDIO_SHUTDOWN_TIMEOUT", "1s")
	}

	base()
	t.Setenv("STUDIO_HTTP_READ_HEADER_TIMEOUT", "0s")
	_, err := config.Load()
	require.Error(t, err)

	base()
	t.Setenv("STUDIO_HTTP_READ_TIMEOUT", "0s")
	_, err = config.Load()
	require.Error(t, err)

	base()
	t.Setenv("STUDIO_HTTP_WRITE_TIMEOUT", "0s")
	_, err = config.Load()
	require.Error(t, err)

	base()
	t.Setenv("STUDIO_HTTP_IDLE_TIMEOUT", "0s")
	_, err = config.Load()
	require.Error(t, err)

	base()
	t.Setenv("STUDIO_HTTP_MAX_BODY_BYTES", "0")
	_, err = config.Load()
	require.Error(t, err)
}

func TestLoadRejectsDBURLWithoutName(t *testing.T) {
	t.Setenv("STUDIO_ENV", "production")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost:5432/")
	_, err := config.Load()
	require.Error(t, err)
}

func TestValidatePortZeroAllowed(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:           "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable",
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
		DatabaseURL:           "postgres://studio:x@localhost:5432/curriculum_studio?sslmode=disable",
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
