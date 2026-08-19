package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.DatabaseURL)
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 8080, cfg.Port)
	assert.False(t, cfg.AgentRuntimeEnabled)
	assert.Equal(t, "2m0s", cfg.AgentRuntimeRunBudget.String())
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("CORS_ORIGINS", "https://a.example,https://b.example")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9999", cfg.Addr())
	assert.Equal(t, []string{"https://a.example", "https://b.example"}, cfg.CORSOrigins)
}

func TestLoadInvalid(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	_, err := Load()
	assert.Error(t, err)
}

func TestServiceTokenDefaultsToOpen(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.ServiceToken, "no token by default; deployments must set one")

	t.Setenv("SERVICE_TOKEN", "s3cret")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, "s3cret", cfg.ServiceToken)
}

func TestTutorConfigDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "fake", cfg.TutorProvider)
	assert.True(t, cfg.TutorEnabled)

	t.Setenv("TUTOR_PROVIDER", "bedrock")
	t.Setenv("TUTOR_ENABLED", "false")
	t.Setenv("TUTOR_BEDROCK_URL", "https://example.invalid/invoke")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, "bedrock", cfg.TutorProvider)
	assert.False(t, cfg.TutorEnabled)
	assert.Equal(t, "https://example.invalid/invoke", cfg.TutorBedrockURL)
}

func TestPrimerAgentsDefaultsDisabled(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.PrimerAgentsEnabled,
		"PRIMER_AGENTS_ENABLED must default to false; all legacy paths must remain unchanged")
	assert.Empty(t, cfg.PrimerAgentsBaseURL)
	assert.Empty(t, cfg.PrimerAgentsTokenEnvVar)
	assert.Equal(t, 30*time.Second, cfg.PrimerAgentsTimeout)
}

func TestPrimerAgentsEnabledRequiresBaseURL(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_ENABLED", "true")
	// No base URL — must fail.
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PRIMER_AGENTS_BASE_URL")
}

func TestPrimerAgentsProductionRequiresHTTPS(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("PRIMER_AGENTS_ENABLED", "true")
	t.Setenv("PRIMER_AGENTS_BASE_URL", "http://insecure.example.com/agents")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPS")
}

func TestPrimerAgentsProductionAcceptsHTTPS(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://primer:primer@localhost:5432/primer?sslmode=disable")
	t.Setenv("PRIMER_AGENTS_ENABLED", "true")
	t.Setenv("PRIMER_AGENTS_BASE_URL", "https://agents.example.com")
	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.PrimerAgentsEnabled)
}

func TestPrimerAgentsDevelopmentAllowsHTTP(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("PRIMER_AGENTS_ENABLED", "true")
	t.Setenv("PRIMER_AGENTS_BASE_URL", "http://localhost:8091")
	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.PrimerAgentsEnabled)
	assert.Equal(t, "http://localhost:8091", cfg.PrimerAgentsBaseURL)
}

func TestIdentityConfigDefaultsToInactive(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.IdentityIssuer, "identity issuer must default to empty (JWT path disabled)")
	assert.Empty(t, cfg.IdentityAudience)
	assert.Empty(t, cfg.IdentityJWKSURL)

	iaCfg := cfg.IdentityAuthConfig()
	assert.Empty(t, iaCfg.Issuer)
	assert.Empty(t, iaCfg.Audience)
	assert.Empty(t, iaCfg.JWKSURL)
}

func TestIdentityConfigFromEnv(t *testing.T) {
	t.Setenv("IDENTITY_ISSUER", "https://identity.primer.dev")
	t.Setenv("IDENTITY_AUDIENCE", "primer-lms")
	t.Setenv("IDENTITY_JWKS_URL", "https://identity.primer.dev/.well-known/jwks.json")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "https://identity.primer.dev", cfg.IdentityIssuer)
	assert.Equal(t, "primer-lms", cfg.IdentityAudience)
	assert.Equal(t, "https://identity.primer.dev/.well-known/jwks.json", cfg.IdentityJWKSURL)

	iaCfg := cfg.IdentityAuthConfig()
	assert.Equal(t, "https://identity.primer.dev", iaCfg.Issuer)
	assert.Equal(t, "primer-lms", iaCfg.Audience)
	assert.Equal(t, "https://identity.primer.dev/.well-known/jwks.json", iaCfg.JWKSURL)
}

func TestIdentityPartialConfigStillReturnsConfig(t *testing.T) {
	// Only issuer set — the verifier will be nil (fail-closed).
	t.Setenv("IDENTITY_ISSUER", "https://identity.primer.dev")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "https://identity.primer.dev", cfg.IdentityIssuer)
	assert.Empty(t, cfg.IdentityAudience)
	assert.Empty(t, cfg.IdentityJWKSURL)
}
