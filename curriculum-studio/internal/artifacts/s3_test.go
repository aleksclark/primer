package artifacts_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aleksclark/primer/curriculum-studio/internal/artifacts"
)

// No build tag or Docker-unavailable skip: S3 persistence requires real MinIO.
func TestS3MinIORoundtrip(t *testing.T) {
	ctx := t.Context()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
		Image: "minio/minio:RELEASE.2025-04-22T22-12-26Z", ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{"MINIO_ROOT_USER": "studio-test", "MINIO_ROOT_PASSWORD": "studio-test-password"},
		Cmd: []string{"server", "/data"}, WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}, Started: true})
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, c)
	endpoint, err := c.Endpoint(ctx, "http")
	require.NoError(t, err)
	client, err := minio.New(endpoint[len("http://"):], &minio.Options{Creds: credentials.NewStaticV4("studio-test", "studio-test-password", "")})
	require.NoError(t, err)
	require.NoError(t, client.MakeBucket(ctx, "studio-exports", minio.MakeBucketOptions{}))
	opts := artifacts.S3Options{Endpoint: endpoint, Bucket: "studio-exports", AccessKey: "studio-test", SecretKey: "studio-test-password"}
	s, err := artifacts.NewS3Store(opts)
	require.NoError(t, err)
	data := []byte("%PDF-1.4\nreal object bytes\n")
	require.NoError(t, s.Put(ctx, "exports/plan.pdf", data, "application/pdf"))
	other, err := artifacts.NewS3Store(opts)
	require.NoError(t, err)
	r, err := other.Get(ctx, "exports/plan.pdf")
	require.NoError(t, err)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.Equal(t, data, got)
	signed, err := s.SignURL(ctx, "exports/plan.pdf", time.Minute)
	require.NoError(t, err)
	resp, err := http.Get(signed)
	require.NoError(t, err)
	got, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
	require.Equal(t, data, got)
	// An unsigned URL is not a public download.
	resp, err = http.Get(endpoint + "/studio-exports/exports/plan.pdf")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 403, resp.StatusCode)
	require.NoError(t, s.Delete(ctx, "exports/plan.pdf"))
	_, err = s.Get(ctx, "exports/plan.pdf")
	require.Error(t, err)
	opts.Bucket = "missing-bucket"
	broken, err := artifacts.NewS3Store(opts)
	require.NoError(t, err)
	require.Error(t, broken.Put(ctx, "exports/file", data, "text/plain"))
}
