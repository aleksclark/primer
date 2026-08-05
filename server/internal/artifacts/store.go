// Package artifacts provides bounded, path-safe byte storage for session
// evidence and parent-approved fixture bundles (Phase 6).
package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Default quotas applied when activity policy omits bounds.
const (
	DefaultMaxBytesEach  int64 = 1 << 20 // 1 MiB
	DefaultMaxBytesTotal int64 = 5 << 20 // 5 MiB
	DefaultMaxFiles            = 20
	DefaultMaxStudentBytes int64 = 50 << 20 // 50 MiB soft global cap
)

// Store is a filesystem-backed content-addressed artifact store.
// Layout under Root:
//
//	objects/sha256/<aa>/<digest>          # immutable blob
//	sessions/<sessionID>/<artifactID>     # hardlink or copy of object
//	bundles/<studentID>/<digest>/...      # materialized approved bundles
type Store struct {
	Root string
}

// NewStore validates root and ensures base directories exist.
func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("artifact store root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("artifact root: %w", err)
	}
	for _, sub := range []string{"objects/sha256", "sessions", "bundles", "tmp"} {
		if err := os.MkdirAll(filepath.Join(abs, sub), 0o750); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}
	return &Store{Root: abs}, nil
}

// ObjectPath returns the content-addressed path for a hex sha256 digest.
func (s *Store) ObjectPath(digest string) (string, error) {
	d, err := normalizeDigest(digest)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Root, "objects", "sha256", d[:2], d), nil
}

// SessionArtifactPath is the per-session pointer path for an uploaded artifact.
func (s *Store) SessionArtifactPath(sessionID, artifactID string) (string, error) {
	if err := requireSafeID(sessionID, "sessionID"); err != nil {
		return "", err
	}
	if err := requireSafeID(artifactID, "artifactID"); err != nil {
		return "", err
	}
	return filepath.Join(s.Root, "sessions", sessionID, artifactID), nil
}

// BundleRoot is the directory holding a materialized approved fixture bundle.
func (s *Store) BundleRoot(studentID, digest string) (string, error) {
	if err := requireSafeID(studentID, "studentID"); err != nil {
		return "", err
	}
	d, err := normalizeDigest(digest)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Root, "bundles", studentID, d), nil
}

// PutObject writes bytes under the content-addressed path after verifying size
// and digest. Idempotent: existing matching object is reused.
func (s *Store) PutObject(digest string, r io.Reader, expectedSize int64) (relPath string, n int64, err error) {
	objAbs, err := s.ObjectPath(digest)
	if err != nil {
		return "", 0, err
	}
	if st, err := os.Stat(objAbs); err == nil {
		// Existing object is authoritative. Callers that pass wrong bytes with the
		// same digest are rejected after hashing below only when the object is new.
		got, err := hashFile(objAbs)
		if err != nil {
			return "", 0, err
		}
		if !strings.EqualFold(got, digest) {
			return "", 0, fmt.Errorf("corrupt object at %s", objAbs)
		}
		if expectedSize >= 0 && st.Size() != expectedSize {
			return "", 0, fmt.Errorf("object %s exists with size %d, expected %d", digest, st.Size(), expectedSize)
		}
		rel, _ := filepath.Rel(s.Root, objAbs)
		return rel, st.Size(), nil
	} else if !os.IsNotExist(err) {
		return "", 0, err
	}

	if err := os.MkdirAll(filepath.Dir(objAbs), 0o750); err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(filepath.Join(s.Root, "tmp"), "upload-*")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	h := sha256.New()
	limited := io.LimitReader(r, expectedSize+1)
	mw := io.MultiWriter(tmp, h)
	written, err := io.Copy(mw, limited)
	if err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	if expectedSize >= 0 && written != expectedSize {
		return "", 0, fmt.Errorf("size mismatch: got %d want %d", written, expectedSize)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, digest) {
		return "", 0, fmt.Errorf("digest mismatch: got %s want %s", got, digest)
	}
	if err := os.Rename(tmpName, objAbs); err != nil {
		// Another writer may have won the race.
		if st, e2 := os.Stat(objAbs); e2 == nil {
			rel, _ := filepath.Rel(s.Root, objAbs)
			return rel, st.Size(), nil
		}
		return "", 0, err
	}
	_ = os.Chmod(objAbs, 0o640)
	rel, _ := filepath.Rel(s.Root, objAbs)
	return rel, written, nil
}

// LinkSessionArtifact creates sessions/<sid>/<aid> pointing at the object.
func (s *Store) LinkSessionArtifact(sessionID, artifactID, digest string) (relPath string, err error) {
	objAbs, err := s.ObjectPath(digest)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(objAbs); err != nil {
		return "", fmt.Errorf("object missing: %w", err)
	}
	linkAbs, err := s.SessionArtifactPath(sessionID, artifactID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(linkAbs), 0o750); err != nil {
		return "", err
	}
	if _, err := os.Lstat(linkAbs); err == nil {
		rel, _ := filepath.Rel(s.Root, linkAbs)
		return rel, nil
	}
	// Prefer hardlink; fall back to copy.
	if err := os.Link(objAbs, linkAbs); err != nil {
		data, rerr := os.ReadFile(objAbs)
		if rerr != nil {
			return "", rerr
		}
		if werr := os.WriteFile(linkAbs, data, 0o640); werr != nil {
			return "", werr
		}
	}
	rel, _ := filepath.Rel(s.Root, linkAbs)
	return rel, nil
}

// OpenObject opens a content-addressed object for reading.
func (s *Store) OpenObject(digest string) (*os.File, error) {
	p, err := s.ObjectPath(digest)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// WriteBundleEntry writes one safe relative path under a bundle root.
// Rejects traversal, absolute paths, and non-regular destinations.
func (s *Store) WriteBundleEntry(bundleRoot, rel string, data []byte, mode os.FileMode) error {
	full, err := safeJoin(bundleRoot, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	// Disallow setuid/setgid/sticky and device-like bits.
	mode &^= os.ModeSetuid | os.ModeSetgid | os.ModeSticky | os.ModeType
	if err := os.WriteFile(full, data, mode); err != nil {
		return err
	}
	return os.Chmod(full, mode)
}

// MaterializeBundle copies approved bundle entries into destDir with safety checks.
// Entries must be relative file paths; symlinks/devices in the source tree are rejected.
func MaterializeBundle(bundleRoot, destDir string) error {
	if bundleRoot == "" || destDir == "" {
		return errors.New("bundle root and dest dir are required")
	}
	rootAbs, err := filepath.Abs(bundleRoot)
	if err != nil {
		return err
	}
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Path safety on every entry.
		if _, err := contracts.SafeRelPath(filepath.ToSlash(rel)); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("reject symlink in bundle: %s", rel)
		}
		if mode&os.ModeDevice != 0 || mode&os.ModeNamedPipe != 0 || mode&os.ModeSocket != 0 {
			return fmt.Errorf("reject special file in bundle: %s", rel)
		}
		target, err := safeJoin(destAbs, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !mode.IsRegular() {
			return fmt.Errorf("reject non-regular file in bundle: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		perm := mode.Perm()
		if perm == 0 {
			perm = 0o644
		}
		perm &^= 0o222 // drop write for group/other from source bits; keep owner write
		if err := os.WriteFile(target, data, perm); err != nil {
			return err
		}
		return os.Chmod(target, perm)
	})
}

// MaterializeFixturesSafe is a thin wrapper that reuses terminal materialize
// semantics after validating fixture paths (symlinks never created).
func MaterializeFixturesSafe(destDir string, fixtures []contracts.FixtureEntry) error {
	for _, f := range fixtures {
		if _, err := contracts.SafeRelPath(f.Path); err != nil {
			return err
		}
		switch f.Type {
		case contracts.FixtureFile, contracts.FixtureDirectory, "":
			// ok
		default:
			return fmt.Errorf("unsupported fixture type %q for %s", f.Type, f.Path)
		}
		if f.Mode != "" {
			if _, err := contracts.ParseMode(f.Mode); err != nil {
				return fmt.Errorf("fixture %q: %w", f.Path, err)
			}
		}
	}
	// Import terminal package lazily via local reimplementation to avoid cycles:
	// write files/dirs only — never symlinks/devices.
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, f := range fixtures {
		full, err := safeJoin(destDir, f.Path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		isDir := f.Type == contracts.FixtureDirectory
		if isDir {
			mode = 0o755
		}
		if f.Mode != "" {
			m, err := contracts.ParseMode(f.Mode)
			if err != nil {
				return err
			}
			mode = os.FileMode(m)
			mode &^= os.ModeSetuid | os.ModeSetgid | os.ModeSticky
		}
		if isDir {
			if err := os.MkdirAll(full, mode); err != nil {
				return err
			}
			if err := os.Chmod(full, mode); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(f.Content), mode); err != nil {
			return err
		}
		if err := os.Chmod(full, mode); err != nil {
			return err
		}
	}
	return nil
}

// PolicyCheck validates upload meta against an activity artifact policy.
func PolicyCheck(pol *contracts.ArtifactPolicy, mediaType string, byteSize int64, sessionFileCount int, sessionByteTotal int64) error {
	if pol == nil || !pol.Enabled {
		return errors.New("artifacts are not enabled for this activity revision")
	}
	maxEach := pol.MaxBytesEach
	if maxEach <= 0 {
		maxEach = DefaultMaxBytesEach
	}
	maxTotal := pol.MaxBytesTotal
	if maxTotal <= 0 {
		maxTotal = DefaultMaxBytesTotal
	}
	maxFiles := pol.MaxFiles
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}
	if byteSize < 0 {
		return errors.New("byteSize must be non-negative")
	}
	if byteSize > maxEach {
		return fmt.Errorf("file exceeds maxBytesEach (%d > %d)", byteSize, maxEach)
	}
	if int64(sessionFileCount) >= int64(maxFiles) {
		return fmt.Errorf("session file quota exceeded (max %d)", maxFiles)
	}
	if sessionByteTotal+byteSize > maxTotal {
		return fmt.Errorf("session byte quota exceeded (max %d)", maxTotal)
	}
	if len(pol.AllowedTypes) > 0 {
		ok := false
		for _, t := range pol.AllowedTypes {
			if strings.EqualFold(t, mediaType) || t == "*/*" {
				ok = true
				break
			}
			// prefix match e.g. text/*
			if strings.HasSuffix(t, "/*") {
				prefix := strings.TrimSuffix(t, "/*")
				if strings.HasPrefix(strings.ToLower(mediaType), strings.ToLower(prefix)+"/") {
					ok = true
					break
				}
			}
		}
		if !ok {
			return fmt.Errorf("media type %q is not allowed", mediaType)
		}
	}
	return nil
}

// SafeFilename strips path components from a client-supplied filename.
func SafeFilename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("filename is required")
	}
	base := filepath.Base(name)
	base = strings.ReplaceAll(base, "\\", "_")
	if base == "." || base == ".." || base == "" {
		return "", errors.New("invalid filename")
	}
	if strings.Contains(base, "/") || strings.Contains(base, "\x00") {
		return "", errors.New("invalid filename")
	}
	return base, nil
}

func normalizeDigest(d string) (string, error) {
	d = strings.ToLower(strings.TrimSpace(d))
	if len(d) != 64 {
		return "", fmt.Errorf("sha256 digest must be 64 hex chars")
	}
	for _, c := range d {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("sha256 digest must be hex")
		}
	}
	return d, nil
}

func requireSafeID(id, label string) error {
	if id == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") || strings.Contains(id, "\x00") {
		return fmt.Errorf("%s contains illegal characters", label)
	}
	return nil
}

func safeJoin(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// Prefer slash-style relative paths from curriculum.
	relSlash := filepath.ToSlash(rel)
	safe, err := contracts.SafeRelPath(relSlash)
	if err != nil {
		return "", err
	}
	full := filepath.Join(rootAbs, filepath.FromSlash(safe))
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	sep := string(os.PathSeparator)
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+sep) {
		return "", &contracts.ErrUnsafePath{Path: rel, Reason: "resolved path escapes root"}
	}
	return fullAbs, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
