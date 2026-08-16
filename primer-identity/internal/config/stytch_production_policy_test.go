package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
)

const productionStytchSecret = "secret-value-must-not-leak"

func assertGenericConfigError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, productionStytchSecret)
	assert.NotContains(t, strings.ToLower(msg), "password=")
}

func TestValidateProductionRequiresStytchEnabledEvenWhenLivePairIsPresent(t *testing.T) {
	cfg := validConfig()
	cfg.Stytch.Enabled = false
	cfg.Stytch.Env = "live"
	cfg.Stytch.ProjectID = "project-live-example"
	cfg.Stytch.Secret = productionStytchSecret
	cfg.Stytch.BaseURI = ""

	err := cfg.Validate()
	assertGenericConfigError(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "enabled")
}

func TestValidateDevelopmentAndTestAllowExplicitlyDisabledStytch(t *testing.T) {
	for _, serviceEnv := range []string{"development", "test"} {
		t.Run(serviceEnv, func(t *testing.T) {
			cfg := validConfig()
			cfg.Env = serviceEnv
			cfg.Stytch.Enabled = false
			cfg.Stytch.ProjectID = ""
			cfg.Stytch.Secret = ""
			cfg.Stytch.Env = "test"
			cfg.Stytch.BaseURI = ""
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestValidateProductionStytchPolicyMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*config.Config)
		want    string
		wantErr bool
	}{
		{
			name: "prod disabled",
			mutate: func(cfg *config.Config) {
				cfg.Stytch.Enabled = false
			},
			want:    "enabled",
			wantErr: true,
		},
		{
			name: "prod test env",
			mutate: func(cfg *config.Config) {
				cfg.Stytch.Env = "test"
				cfg.Stytch.ProjectID = "project-test-example"
			},
			want:    "live",
			wantErr: true,
		},
		{
			name: "prod missing secret",
			mutate: func(cfg *config.Config) {
				cfg.Stytch.Secret = ""
			},
			want:    "credential",
			wantErr: true,
		},
		{
			name: "prod missing project",
			mutate: func(cfg *config.Config) {
				cfg.Stytch.ProjectID = ""
			},
			want:    "credential",
			wantErr: true,
		},
		{
			name: "prod base uri override",
			mutate: func(cfg *config.Config) {
				cfg.Stytch.BaseURI = "https://stytch.internal.example"
			},
			want:    "override",
			wantErr: true,
		},
		{
			name: "prod enabled live valid",
			mutate: func(cfg *config.Config) {
				cfg.Stytch.BaseURI = ""
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Stytch.Secret = productionStytchSecret
			tc.mutate(cfg)
			err := cfg.Validate()
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			assertGenericConfigError(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tc.want)
		})
	}
}

func clearStytchEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"IDENTITY_STYTCH_ENABLED",
		"IDENTITY_STYTCH_PROJECT_ID",
		"IDENTITY_STYTCH_SECRET",
		"IDENTITY_STYTCH_ENV",
		"IDENTITY_STYTCH_BASE_URI",
	} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func TestLoadProductionStytchPolicyMatrix(t *testing.T) {
	const identityDSN = "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable"

	for _, tc := range []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{
			name: "prod disabled with live pair set",
			env: map[string]string{
				"IDENTITY_ENV":               "production",
				"IDENTITY_DATABASE_URL":      identityDSN,
				"IDENTITY_ISSUER":            "https://id.example",
				"IDENTITY_STYTCH_ENABLED":    "false",
				"IDENTITY_STYTCH_ENV":        "live",
				"IDENTITY_STYTCH_PROJECT_ID": "project-live-example",
				"IDENTITY_STYTCH_SECRET":     productionStytchSecret,
				"IDENTITY_STYTCH_BASE_URI":   "",
			},
			want:    "enabled",
			wantErr: true,
		},
		{
			name: "prod default disabled is not an incidental live-env failure",
			env: map[string]string{
				"IDENTITY_ENV":               "production",
				"IDENTITY_DATABASE_URL":      identityDSN,
				"IDENTITY_ISSUER":            "https://id.example",
				"IDENTITY_STYTCH_ENV":        "live",
				"IDENTITY_STYTCH_PROJECT_ID": "project-live-example",
				"IDENTITY_STYTCH_SECRET":     productionStytchSecret,
			},
			want:    "enabled",
			wantErr: true,
		},
		{
			name: "prod test env",
			env: map[string]string{
				"IDENTITY_ENV":               "production",
				"IDENTITY_DATABASE_URL":      identityDSN,
				"IDENTITY_ISSUER":            "https://id.example",
				"IDENTITY_STYTCH_ENABLED":    "true",
				"IDENTITY_STYTCH_ENV":        "test",
				"IDENTITY_STYTCH_PROJECT_ID": "project-test-example",
				"IDENTITY_STYTCH_SECRET":     productionStytchSecret,
			},
			want:    "live",
			wantErr: true,
		},
		{
			name: "prod missing secret pair",
			env: map[string]string{
				"IDENTITY_ENV":               "production",
				"IDENTITY_DATABASE_URL":      identityDSN,
				"IDENTITY_ISSUER":            "https://id.example",
				"IDENTITY_STYTCH_ENABLED":    "true",
				"IDENTITY_STYTCH_ENV":        "live",
				"IDENTITY_STYTCH_PROJECT_ID": "project-live-example",
			},
			want:    "credential",
			wantErr: true,
		},
		{
			name: "prod missing project pair",
			env: map[string]string{
				"IDENTITY_ENV":            "production",
				"IDENTITY_DATABASE_URL":   identityDSN,
				"IDENTITY_ISSUER":         "https://id.example",
				"IDENTITY_STYTCH_ENABLED": "true",
				"IDENTITY_STYTCH_ENV":     "live",
				"IDENTITY_STYTCH_SECRET":  productionStytchSecret,
			},
			want:    "credential",
			wantErr: true,
		},
		{
			name: "prod base uri override",
			env: map[string]string{
				"IDENTITY_ENV":               "production",
				"IDENTITY_DATABASE_URL":      identityDSN,
				"IDENTITY_ISSUER":            "https://id.example",
				"IDENTITY_STYTCH_ENABLED":    "true",
				"IDENTITY_STYTCH_ENV":        "live",
				"IDENTITY_STYTCH_PROJECT_ID": "project-live-example",
				"IDENTITY_STYTCH_SECRET":     productionStytchSecret,
				"IDENTITY_STYTCH_BASE_URI":   "https://stytch.internal.example",
			},
			want:    "override",
			wantErr: true,
		},
		{
			name: "prod enabled live valid",
			env: map[string]string{
				"IDENTITY_ENV":               "production",
				"IDENTITY_DATABASE_URL":      identityDSN,
				"IDENTITY_ISSUER":            "https://id.example",
				"IDENTITY_STYTCH_ENABLED":    "true",
				"IDENTITY_STYTCH_ENV":        "live",
				"IDENTITY_STYTCH_PROJECT_ID": "project-live-example",
				"IDENTITY_STYTCH_SECRET":     productionStytchSecret,
			},
		},
		{
			name: "development disabled",
			env: map[string]string{
				"IDENTITY_ENV":            "development",
				"IDENTITY_DATABASE_URL":   identityDSN,
				"IDENTITY_ISSUER":         "http://localhost:8090",
				"IDENTITY_STYTCH_ENABLED": "false",
			},
		},
		{
			name: "test disabled",
			env: map[string]string{
				"IDENTITY_ENV":            "test",
				"IDENTITY_DATABASE_URL":   identityDSN,
				"IDENTITY_ISSUER":         "https://id.example.test",
				"IDENTITY_STYTCH_ENABLED": "false",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearIdentityEnv(t)
			clearStytchEnv(t)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			for key, value := range map[string]string{
				"IDENTITY_STATE_SEAL_KEYS": "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "IDENTITY_STATE_SEAL_ACTIVE_VERSION": "1",
				"IDENTITY_STATE_HASH_PEPPERS": "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "IDENTITY_STATE_HASH_ACTIVE_VERSION": "1",
				"IDENTITY_BROKER_COOKIE_PEPPERS": "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "IDENTITY_BROKER_COOKIE_ACTIVE_VERSION": "1",
				"IDENTITY_AUTHORIZATION_CODE_PEPPERS": "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION": "1",
			} {
				t.Setenv(key, value)
			}
			cfg, err := config.Load()
			if !tc.wantErr {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				return
			}
			assertGenericConfigError(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tc.want)
			assert.Nil(t, cfg)
		})
	}
}
