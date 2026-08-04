package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.DatabaseURL)
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 8080, cfg.Port)
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
