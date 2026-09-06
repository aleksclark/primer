package artifacts_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/artifacts"
	"github.com/stretchr/testify/require"
)

func TestStoreConfigurationAndKeyValidation(t *testing.T) {
	_, err := artifacts.NewFsStore("")
	require.Error(t, err)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	_, err = artifacts.NewFsStore(file)
	require.Error(t, err)
	for _, opts := range []artifacts.S3Options{
		{}, {Endpoint: "file:///tmp"}, {Endpoint: "http://host/bucket", Bucket: "bucket"},
		{Endpoint: "http://host"}, {Endpoint: "http://host", Bucket: "bucket", AccessKey: "only-one"},
	} {
		_, err := artifacts.NewS3Store(opts)
		require.Error(t, err)
	}
	// These inputs are rejected locally before any network access.
	s, err := artifacts.NewS3Store(artifacts.S3Options{Endpoint: "http://127.0.0.1:1", Bucket: "bucket", AccessKey: "test", SecretKey: "test"})
	require.NoError(t, err)
	require.Error(t, s.Put(t.Context(), "../bad", nil, "text/plain"))
	_, err = s.Get(t.Context(), "../bad")
	require.Error(t, err)
	require.Error(t, s.Delete(t.Context(), "../bad"))
	_, err = s.SignURL(t.Context(), "../bad", time.Minute)
	require.Error(t, err)
	for _, ttl := range []time.Duration{0, 8 * 24 * time.Hour} {
		_, err = s.SignURL(t.Context(), "exports/file", ttl)
		require.Error(t, err)
	}
}

func TestFSClosedStoreAndObstruction(t *testing.T) {
	dir := t.TempDir()
	s, err := artifacts.NewFsStore(dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exports"), nil, 0600))
	require.Error(t, s.Put(t.Context(), "exports/plan", []byte("content"), "text/plain"))
	require.NoError(t, s.Close())
	require.Error(t, s.Put(t.Context(), "file", nil, "text/plain"))
	_, err = s.Get(t.Context(), "file")
	require.Error(t, err)
	require.Error(t, s.Delete(t.Context(), "file"))
}
