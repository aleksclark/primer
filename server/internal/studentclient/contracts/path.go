package contracts

import (
	"fmt"
	"path"
	"strings"
)

// ErrUnsafePath is returned when a curriculum path escapes the workspace.
type ErrUnsafePath struct {
	Path   string
	Reason string
}

func (e *ErrUnsafePath) Error() string {
	return fmt.Sprintf("unsafe path %q: %s", e.Path, e.Reason)
}

// SafeRelPath validates a workspace-relative path from curriculum or checks.
// It rejects empty paths, absolute paths, Windows drive paths, tilde homes,
// backslashes, and any ".." segment (before or after cleaning).
func SafeRelPath(p string) (string, error) {
	if p == "" {
		return "", &ErrUnsafePath{Path: p, Reason: "path is empty"}
	}
	if strings.Contains(p, "\x00") {
		return "", &ErrUnsafePath{Path: p, Reason: "path contains NUL"}
	}
	if strings.ContainsAny(p, `:\`) {
		return "", &ErrUnsafePath{Path: p, Reason: "path contains illegal ':' or '\\'"}
	}
	if strings.HasPrefix(p, "~") {
		return "", &ErrUnsafePath{Path: p, Reason: "path must not start with '~'"}
	}
	if strings.HasPrefix(p, "/") {
		return "", &ErrUnsafePath{Path: p, Reason: "path must be relative (no leading '/')"}
	}
	// Reject ".." before Clean; Clean("/"+ "../x") would quietly become "/x".
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", &ErrUnsafePath{Path: p, Reason: "path escapes workspace via '..'"}
		}
	}
	cleaned := path.Clean(p)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", &ErrUnsafePath{Path: p, Reason: "path resolves outside workspace"}
	}
	if path.IsAbs(cleaned) {
		return "", &ErrUnsafePath{Path: p, Reason: "path must be relative"}
	}
	if cleaned == "" {
		return "", &ErrUnsafePath{Path: p, Reason: "path is empty after clean"}
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." || seg == "" {
			return "", &ErrUnsafePath{Path: p, Reason: "path escapes workspace via '..'"}
		}
	}
	return cleaned, nil
}

// JoinUnder joins a validated relative path onto root and ensures the result
// stays within root. root must be an absolute path.
func JoinUnder(root, rel string) (string, error) {
	safe, err := SafeRelPath(rel)
	if err != nil {
		return "", err
	}
	if root == "" || !strings.HasPrefix(root, "/") {
		return "", fmt.Errorf("workspace root must be an absolute path")
	}
	rootClean := path.Clean(root)
	full := path.Join(rootClean, safe)
	if full != rootClean && !strings.HasPrefix(full, rootClean+"/") {
		return "", &ErrUnsafePath{Path: rel, Reason: "resolved path escapes workspace root"}
	}
	return full, nil
}
