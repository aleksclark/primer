// Package artifacts owns export bytes. PostgreSQL stores only object references.
package artifacts

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"time"
)

var ErrSigningUnsupported = errors.New("artifact store does not support signed URLs; use the authorized download endpoint")

// Store keys are relative object names, not URLs or filesystem paths. Signing
// is optional; callers must authorize access before requesting a signed URL.
type Store interface {
	Put(context.Context, string, []byte, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
	SignURL(context.Context, string, time.Duration) (string, error)
}

func validKey(key string) error {
	if key == "" || key == "." || strings.HasPrefix(key, "/") || path.Clean(key) != key || strings.ContainsAny(key, "\\\x00:") || key == ".." || strings.HasPrefix(key, "../") {
		return errors.New("invalid artifact key")
	}
	return nil
}
