package config_test

import (
	"github.com/aleksclark/primer/curriculum-studio/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestArtifactStoreConfig(t *testing.T) {
	t.Setenv("STUDIO_ENV", "test")
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:x@localhost/curriculum_studio_test")
	t.Setenv("STUDIO_ARTIFACT_STORE", "fs")
	t.Setenv("STUDIO_ARTIFACT_STORE_DIR", t.TempDir())
	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "fs", cfg.ArtifactStore)
	t.Setenv("STUDIO_ARTIFACT_STORE", "unknown")
	_, err = config.Load()
	require.ErrorContains(t, err, "fs|s3")
	t.Setenv("STUDIO_ARTIFACT_STORE", "s3")
	t.Setenv("STUDIO_ARTIFACT_S3_BUCKET", "")
	_, err = config.Load()
	require.ErrorContains(t, err, "bucket")
	t.Setenv("STUDIO_ARTIFACT_S3_BUCKET", "studio-private")
	for _, endpoint := range []string{"", "file:///tmp/data", "http://user:pass@host", "https://host/bucket", "https://host?x=y"} {
		t.Setenv("STUDIO_ARTIFACT_S3_ENDPOINT", endpoint)
		_, err = config.Load()
		require.Error(t, err)
	}
	t.Setenv("STUDIO_ARTIFACT_S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("STUDIO_ARTIFACT_S3_ACCESS_KEY", "key")
	t.Setenv("STUDIO_ARTIFACT_S3_SECRET_KEY", "")
	_, err = config.Load()
	require.ErrorContains(t, err, "both")
	t.Setenv("STUDIO_ARTIFACT_S3_SECRET_KEY", "secret")
	cfg, err = config.Load()
	require.NoError(t, err)
	require.Equal(t, "studio-private", cfg.ArtifactS3Bucket)
	t.Setenv("STUDIO_ENV", "production")
	t.Setenv("STUDIO_AUTH_MODE", "jwks")
	t.Setenv("STUDIO_JWKS_URL", "https://identity.example.org/jwks")
	t.Setenv("STUDIO_ISSUER", "https://identity.example.org")
	t.Setenv("STUDIO_MCP_ENABLED", "false")
	_, err = config.Load()
	require.ErrorContains(t, err, "https")
}
