package artifactstore

import (
	"context"
	"strings"
	"testing"
)

func TestS3PresignUsesPublicEndpoint(t *testing.T) {
	store, err := NewS3(context.Background(), S3Config{Endpoint: "http://minio:9000", PublicEndpoint: "http://object-store.example:9000", Bucket: "tasks", AccessKey: "key", SecretKey: "secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	url, err := store.PresignPut(context.Background(), "tenants/a/artifacts/b/original", "image/png", 4, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://object-store.example:9000/") {
		t.Fatalf("presign host = %s", url)
	}
	if strings.Contains(url, "minio:9000") {
		t.Fatal("presigned browser URL leaked internal endpoint")
	}
}
