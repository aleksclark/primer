package artifactstore

import (
	"bytes"
	"context"
	"testing"
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
func TestFSStoreRejectsSizeMismatchAndDeletesIdempotently(t *testing.T) {
	s, _ := NewFS(t.TempDir())
	if _, e := s.Put(context.Background(), "a", "", bytes.NewReader([]byte("long")), 2); e == nil {
		t.Fatal("accepted short declared size")
	}
	if e := s.Delete(context.Background(), "missing"); e != nil {
		t.Fatal(e)
	}
}
