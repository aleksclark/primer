package artifactstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestFSStoreAtomicRoundTripAndKeyIsolation(t *testing.T) {
	s, e := NewFS(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	payload := []byte("deterministic object bytes")
	o, e := s.Put(ctx, "tenants/a/artifacts/b/original", "application/octet-stream", bytes.NewReader(payload), int64(len(payload)))
	if e != nil {
		t.Fatal(e)
	}
	if o.Size != int64(len(payload)) {
		t.Fatalf("size=%d", o.Size)
	}
	f, _, e := s.Open(ctx, o.Key)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	body := make([]byte, len(payload))
	if _, e = f.Read(body); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("round trip=%q", body)
	}
	for _, key := range []string{"../escape", "/absolute", "tenants\\escape"} {
		if _, e = s.Stat(ctx, key); e == nil {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
}
func TestFSStoreRejectsInvalidInputsAndCanceledOperations(t *testing.T) {
	if _, err := NewFS(""); err == nil {
		t.Fatal("empty root accepted")
	}
	for _, raw := range []string{"", "/tmp/store", "ftp://store", "store.example"} {
		if _, err := ParseEndpoint(raw); err == nil {
			t.Fatalf("invalid endpoint accepted: %q", raw)
		}
	}
	if endpoint, err := ParseEndpoint("https://store.example:9000/path"); err != nil || endpoint.Host != "store.example:9000" {
		t.Fatalf("valid endpoint=%v err=%v", endpoint, err)
	}
	s, err := NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Put(canceled, "a", "", bytes.NewReader(nil), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled put=%v", err)
	}
	if _, _, err = s.Open(canceled, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open=%v", err)
	}
	if err = s.Delete(canceled, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delete=%v", err)
	}
	if _, err = s.Put(context.Background(), "a", "", bytes.NewReader(nil), -1); err == nil {
		t.Fatal("negative size accepted")
	}
	if _, err = s.Put(context.Background(), "a", "", bytes.NewReader([]byte("short")), 10); err == nil {
		t.Fatal("short object accepted")
	}
	if _, err = s.Put(context.Background(), "a", "", bytes.NewReader([]byte("trailing")), 4); err == nil {
		t.Fatal("trailing object bytes accepted")
	}
	if _, _, err = s.Open(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object=%v", err)
	}
	if _, err = s.PresignPut(context.Background(), "a", "", 1, time.Minute); err == nil {
		t.Fatal("filesystem presign unexpectedly available")
	}
}

func TestFSStoreComposeJoinsPartsAndChecksDeclaredSize(t *testing.T) {
	s, err := NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = s.Put(ctx, "parts/one", "text/plain", bytes.NewReader([]byte("one")), 3); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Put(ctx, "parts/two", "text/plain", bytes.NewReader([]byte("two")), 3); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compose(ctx, "joined", "text/plain", []string{"parts/one", "parts/two"}, 6); err != nil {
		t.Fatal(err)
	}
	joined, _, err := s.Open(ctx, "joined")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(joined)
	_ = joined.Close()
	if err != nil || string(body) != "onetwo" {
		t.Fatalf("joined=%q err=%v", body, err)
	}
	if _, err = s.Compose(ctx, "wrong-size", "text/plain", []string{"parts/one", "parts/two"}, 5); err == nil {
		t.Fatal("compose accepted wrong declared size")
	}
	if _, err = s.Compose(ctx, "missing-part", "text/plain", []string{"parts/missing"}, 1); err == nil {
		t.Fatal("compose accepted missing part")
	}
}

func TestFSStoreRejectsSizeMismatchAndDeletesIdempotently(t *testing.T) {
	s, _ := NewFS(t.TempDir())
	if _, e := s.Put(context.Background(), "a", "", bytes.NewReader([]byte("long")), 2); e == nil {
		t.Fatal("accepted short declared size")
	}
	if e := s.Delete(context.Background(), "missing"); e != nil {
		t.Fatal(e)
	}
}
