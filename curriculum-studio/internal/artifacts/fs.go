package artifacts

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
)

// FsStore uses os.Root to confine operations even in the presence of symlinks.
// The configured root must be on persistent storage in a deployed process.
type FsStore struct{ root *os.Root }

func NewFsStore(dir string) (*FsStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("artifact root is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &FsStore{root: root}, nil
}
func (s *FsStore) Close() error { return s.root.Close() }
func (s *FsStore) Put(ctx context.Context, key string, data []byte, _ string) error {
	if err := validKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.root.MkdirAll(path.Dir(key), 0700); err != nil {
		return err
	}
	tmp := key + "." + uuid.NewString() + ".partial"
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.root.Rename(tmp, key); err != nil {
		return err
	}
	// Sync the directory entry as well as the file before reporting success.
	dir, err := s.root.Open(path.Dir(key))
	if err != nil {
		return err
	}
	err = dir.Sync()
	closeErr = dir.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (s *FsStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.root.Open(key)
}
func (s *FsStore) Delete(ctx context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := s.root.Remove(key)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *FsStore) SignURL(ctx context.Context, key string, _ time.Duration) (string, error) {
	if err := validKey(key); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "", ErrSigningUnsupported
}
