// Package artifactstore provides the deliberately small object-storage boundary
// used by Tasks media uploads. Callers never construct object-store URLs or keys.
package artifactstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotFound = errors.New("object not found")

type Object struct {
	Key         string
	ContentType string
	Size        int64
	ETag        string
}

type Store interface {
	Put(context.Context, string, string, io.Reader, int64) (Object, error)
	Open(context.Context, string) (io.ReadCloser, Object, error)
	Delete(context.Context, string) error
	Stat(context.Context, string) (Object, error)
}

// Optional is implemented by stores which can issue a least-authority URL.
// The API may fall back to its bounded streaming endpoint when unavailable.
type Optional interface {
	PresignPut(context.Context, string, string, int64, time.Duration) (string, error)
}

// Composer joins independently uploaded parts into the server-owned final key.
// It is optional so a simple test adapter can remain deliberately small.
type Composer interface {
	Compose(context.Context, string, string, []string, int64) (Object, error)
}

// FSStore is intentionally used by deterministic unit tests and local
// development only. It rejects traversal and writes via a same-directory
// temporary file before rename, so readers never observe a partial object.
type FSStore struct{ Root string }

func NewFS(root string) (*FSStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("artifact store root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &FSStore{Root: root}, nil
}
func (s *FSStore) path(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) || strings.Contains(key, "\\") {
		return "", errors.New("invalid object key")
	}
	clean := filepath.Clean(key)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid object key")
	}
	return filepath.Join(s.Root, clean), nil
}
func (s *FSStore) Put(ctx context.Context, key, contentType string, src io.Reader, size int64) (Object, error) {
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	if size < 0 {
		return Object{}, errors.New("negative object size")
	}
	path, err := s.path(key)
	if err != nil {
		return Object{}, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Object{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return Object{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	n, copyErr := io.CopyN(tmp, src, size)
	if copyErr != nil && !(copyErr == io.EOF && n == size) {
		tmp.Close()
		return Object{}, copyErr
	}
	if n != size {
		tmp.Close()
		return Object{}, fmt.Errorf("object size mismatch: got %d want %d", n, size)
	}
	var one [1]byte
	if nread, _ := src.Read(one[:]); nread != 0 {
		tmp.Close()
		return Object{}, errors.New("object has trailing bytes")
	}
	if err = tmp.Sync(); err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err != nil {
		return Object{}, err
	}
	if err = os.Chmod(tmpName, 0o600); err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		return Object{}, err
	}
	return Object{Key: key, ContentType: contentType, Size: size}, nil
}
func (s *FSStore) Open(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, Object{}, err
	}
	path, err := s.path(key)
	if err != nil {
		return nil, Object{}, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, Object{}, ErrNotFound
	}
	if err != nil {
		return nil, Object{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Object{}, err
	}
	return f, Object{Key: key, Size: st.Size()}, nil
}
func (s *FSStore) Stat(ctx context.Context, key string) (Object, error) {
	f, obj, err := s.Open(ctx, key)
	if f != nil {
		f.Close()
	}
	return obj, err
}
func (s *FSStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err = os.Remove(p); os.IsNotExist(err) {
		return nil
	}
	return err
}

// No network URL is generated for filesystem storage. This keeps tests honest:
// the API must authenticate every download instead of leaking a local path.
func (s *FSStore) PresignPut(context.Context, string, string, int64, time.Duration) (string, error) {
	return "", errors.New("filesystem store does not issue presigned URLs")
}
func (s *FSStore) Compose(ctx context.Context, key, contentType string, parts []string, size int64) (Object, error) {
	readers := make([]io.Reader, 0, len(parts))
	closers := make([]io.Closer, 0, len(parts))
	var total int64
	for _, part := range parts {
		f, obj, err := s.Open(ctx, part)
		if err != nil {
			for _, c := range closers {
				_ = c.Close()
			}
			return Object{}, err
		}
		readers = append(readers, f)
		closers = append(closers, f)
		total += obj.Size
	}
	obj, err := s.Put(ctx, key, contentType, io.MultiReader(readers...), total)
	for _, c := range closers {
		_ = c.Close()
	}
	if err != nil {
		return Object{}, err
	}
	if total != size {
		return Object{}, fmt.Errorf("part size mismatch: got %d want %d", total, size)
	}
	return obj, nil
}

func ValidateKey(key string) error { _, err := (&FSStore{Root: "."}).path(key); return err }
func ParseEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("object store endpoint must be an absolute HTTP(S) URL")
	}
	return u, nil
}
