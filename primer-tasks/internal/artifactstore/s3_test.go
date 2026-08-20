package artifactstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestS3RejectsUnsafeKeysAndInvalidConfiguration(t *testing.T) {
	if _, err := NewS3(context.Background(), S3Config{}); err == nil {
		t.Fatal("empty bucket accepted")
	}
	store, err := NewS3(context.Background(), S3Config{Endpoint: "http://127.0.0.1:1", Bucket: "tasks", AccessKey: "key", SecretKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../escape", "/absolute", "tenant\\escape"} {
		if _, err = store.Put(context.Background(), key, "image/png", bytes.NewReader(nil), 0); err == nil {
			t.Fatalf("unsafe Put key accepted: %q", key)
		}
		if _, _, err = store.Open(context.Background(), key); err == nil {
			t.Fatalf("unsafe Open key accepted: %q", key)
		}
		if _, err = store.Stat(context.Background(), key); err == nil {
			t.Fatalf("unsafe Stat key accepted: %q", key)
		}
		if err = store.Delete(context.Background(), key); err == nil {
			t.Fatalf("unsafe Delete key accepted: %q", key)
		}
		if _, err = store.PresignPut(context.Background(), key, "image/png", 1, time.Minute); err == nil {
			t.Fatalf("unsafe presign key accepted: %q", key)
		}
	}
	if _, err = store.Put(context.Background(), "tenant/object", "image/png", bytes.NewReader([]byte("x")), -1); err == nil {
		t.Fatal("negative S3 size accepted")
	}
	if _, err = store.Put(context.Background(), "tenant/object", "image/png", bytes.NewReader([]byte("short")), 10); err == nil {
		t.Fatal("short S3 object accepted")
	}
}

func TestS3PresignAndOperationsHonorCanceledContext(t *testing.T) {
	store, err := NewS3(context.Background(), S3Config{Endpoint: "http://127.0.0.1:1", Bucket: "tasks", AccessKey: "key", SecretKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PresignPut(ctx, "tenant/object", "image/png", 1, time.Minute); err == nil {
		t.Fatal("canceled presign succeeded")
	}
	if _, err := store.Put(ctx, "tenant/object", "image/png", bytes.NewReader([]byte("x")), 1); err == nil {
		t.Fatal("canceled put succeeded")
	}
	if _, _, err := store.Open(ctx, "tenant/object"); err == nil {
		t.Fatal("canceled open succeeded")
	}
	if _, err := store.Stat(ctx, "tenant/object"); err == nil {
		t.Fatal("canceled stat succeeded")
	}
	if err := store.Delete(ctx, "tenant/object"); err == nil {
		t.Fatal("canceled delete succeeded")
	}
}

func TestS3ServerErrorsRemainClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	store, err := NewS3(context.Background(), S3Config{Endpoint: server.URL, Bucket: "tasks", AccessKey: "key", SecretKey: "secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.EnsureBucket(context.Background()); err == nil {
		t.Fatal("bucket server error ignored")
	}
	if _, err = store.Put(context.Background(), "tenant/object", "image/png", bytes.NewReader([]byte("data")), 4); err == nil {
		t.Fatal("put server error ignored")
	}
	if _, _, err = store.Open(context.Background(), "tenant/object"); err == nil {
		t.Fatal("open server error ignored")
	}
	if _, err = store.Stat(context.Background(), "tenant/object"); err == nil {
		t.Fatal("stat server error ignored")
	}
	if err = store.Delete(context.Background(), "tenant/object"); err == nil {
		t.Fatal("delete server error ignored")
	}
	if _, err = store.Compose(context.Background(), "tenant/composed", "image/png", []string{"tenant/missing"}, 4); err == nil {
		t.Fatal("compose source error ignored")
	}
}

func TestS3StoreHandlesOptionalMetadataAndIdempotentBucketResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			_, _ = w.Write([]byte("data"))
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
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
		t.Fatalf("idempotent bucket response: %v", err)
	}
	opened, object, err := store.Open(context.Background(), "tenant/object")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(opened)
	_ = opened.Close()
	if err != nil || string(body) != "data" || object.Size != 4 || object.ContentType == "" {
		t.Fatalf("object metadata body=%q object=%+v err=%v", body, object, err)
	}
	stat, err := store.Stat(context.Background(), "tenant/object")
	if err != nil || stat.Size != 0 || stat.ETag != "" {
		t.Fatalf("optional stat=%+v err=%v", stat, err)
	}
	if _, err := store.Compose(context.Background(), "tenant/composed", "image/png", []string{"tenant/object"}, 5); err == nil {
		t.Fatal("compose accepted a declared size that differed from source")
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
