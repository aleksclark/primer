package securityreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	minioUser   = "security-review"
	minioPass   = "security-review-password"
	minioBucket = "review-fixtures"
)

// TestMinIOObjectBoundary uses an actual S3-compatible server and real media
// bytes. It is skipped for ordinary local unit runs when Docker is unavailable,
// but the coverage gate makes that omission fail closed.
func TestMinIOObjectBoundary(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("PRIMER_TASKS_COVERAGE_GATE") == "1" {
			t.Fatalf("Docker is required for MinIO security coverage: %v", err)
		}
		t.Skipf("Docker is unavailable; skipping MinIO integration test: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "minio/minio:RELEASE.2024-06-13T22-53-53Z",
			Env:          map[string]string{"MINIO_ROOT_USER": minioUser, "MINIO_ROOT_PASSWORD": minioPass},
			Cmd:          []string{"server", "/data"},
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor:   wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp").WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + host + ":" + port.Port()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(minioUser, minioPass, "")))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	if _, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(minioBucket)}); err != nil {
		t.Fatal(err)
	}

	original := JPEGWithEXIF()
	key := "tenant/tenant-a/0123456789abcdef0123456789abcdef"
	if err := TenantObjectKey(key, "tenant-a"); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(original)
	_, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(minioBucket), Key: aws.String(key), Body: bytes.NewReader(original), ContentType: aws.String("image/jpeg"), Metadata: map[string]string{"sha256": hex.EncodeToString(digest[:])}})
	if err != nil {
		t.Fatal(err)
	}
	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(minioBucket), Key: aws.String(key)})
	if err != nil || head.ContentLength == nil || *head.ContentLength != int64(len(original)) {
		t.Fatalf("head object = (%v, %#v)", err, head)
	}
	got, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(minioBucket), Key: aws.String(key)})
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if readErr != nil || string(body) != string(original) {
		t.Fatalf("downloaded object differs: %v", readErr)
	}

	presigner := s3.NewPresignClient(client)
	signed, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(minioBucket), Key: aws.String(key)}, func(options *s3.PresignOptions) { options.Expires = 60 * time.Second })
	if err != nil {
		t.Fatal(err)
	}
	if err := ShortLivedURL(signed.URL, 300); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(signed.URL)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("presigned download = (%v, %d)", err, response.StatusCode)
	}
	_ = response.Body.Close()

	// Changing the signed expiry to zero is an actual expired-URL request, not
	// a string-only assertion. MinIO must reject it without revealing bytes.
	expired := strings.Replace(signed.URL, "X-Amz-Expires=60", "X-Amz-Expires=0", 1)
	response, err = http.Get(expired)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode == http.StatusOK {
		t.Fatal("expired presigned URL returned object bytes")
	}

	if DigestMatches(body, hex.EncodeToString(digest[:])) == false {
		t.Fatal("stored object digest does not match uploaded bytes")
	}
	wrong := sha256.Sum256([]byte("different valid upload"))
	if DigestMatches(body, hex.EncodeToString(wrong[:])) {
		t.Fatal("digest mismatch was accepted")
	}

	// Cleanup is verified against the object server, including a second delete
	// (replay) which must not resurrect or substitute an object.
	if _, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(minioBucket), Key: aws.String(key)}); err != nil {
		t.Fatal(err)
	}
	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(minioBucket), Key: aws.String(key)})
	var apiErr interface{ ErrorCode() string }
	if err == nil || !errors.As(err, &apiErr) {
		t.Fatalf("deleted object head = %v, want S3 not-found", err)
	}
}
