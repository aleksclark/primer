package broker

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultTokenFileName is the broker-owned device token basename.
const DefaultTokenFileName = "device.token"

// WriteTokenFile writes token to path with mode 0600, replacing any existing file.
func WriteTokenFile(path, token string) error {
	if path == "" {
		return fmt.Errorf("token file path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("token dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token), 0o600); err != nil {
		return fmt.Errorf("write token tmp: %w", err)
	}
	// Re-assert mode in case umask interfered.
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod token tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename token: %w", err)
	}
	// Final mode check / fix.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod token: %w", err)
	}
	return nil
}

// ReadTokenFile reads the token file. Empty path or missing file returns ("", nil).
func ReadTokenFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// TokenFileModeOK reports whether path exists with permission bits 0600
// (no group/other access). Returns (false, nil) if missing.
func TokenFileModeOK(path string) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	perm := st.Mode().Perm()
	return perm == 0o600, nil
}
