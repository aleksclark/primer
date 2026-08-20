package artifactstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestS3StoreOperationsUseObjectBoundary(t *testing.T) {
	t.Setenv("AWS_REQUEST_CHECKSUM_CALCULATION", "when_required")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Length", "4")
			_, _ = w.Write([]byte("data"))
		case http.MethodHead:
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Length", "4")
			w.Header().Set("ETag", "etag")
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("ETag", "etag")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store, err := NewS3(context.Background(), S3Config{Endpoint: server.URL, Bucket: "tasks", AccessKey: "key", SecretKey: "secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "tenants/a/object", "image/png", bytes.NewReader([]byte("data")), 4); err != nil {
		t.Fatal(err)
	}
	opened, object, err := store.Open(context.Background(), "tenants/a/object")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(opened)
	_ = opened.Close()
	if err != nil || string(body) != "data" || object.Size != 4 {
		t.Fatalf("open body=%q object=%+v err=%v", body, object, err)
	}
	stat, err := store.Stat(context.Background(), "tenants/a/object")
	if err != nil || stat.ETag != "etag" {
		t.Fatalf("stat=%+v err=%v", stat, err)
	}
	if err := store.Delete(context.Background(), "tenants/a/object"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Compose(context.Background(), "tenants/a/composed", "image/png", []string{"tenants/a/object"}, 4); err != nil {
		t.Fatal(err)
	}
}
