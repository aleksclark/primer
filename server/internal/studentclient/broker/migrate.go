package broker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aleksclark/primer/server/internal/studentclient/cache"
)

// MigrateLegacyState moves a student-owned state.db into the broker state dir
// when the broker DB path does not yet exist. The original file is renamed to
// *.pre-broker.bak for one-release rollback recovery.
//
// If legacyPath == destDB or dest already exists, this is a no-op.
func MigrateLegacyState(legacyPath, destDB, tokenFile string) error {
	if legacyPath == "" || destDB == "" {
		return nil
	}
	legacyPath, err := filepath.Abs(legacyPath)
	if err != nil {
		return err
	}
	destDB, err = filepath.Abs(destDB)
	if err != nil {
		return err
	}
	if legacyPath == destDB {
		return nil
	}
	if _, err := os.Stat(destDB); err == nil {
		return nil // already migrated / broker-owned
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destDB), 0o750); err != nil {
		return err
	}

	// Copy then rename original to bak so a crash mid-move is recoverable.
	if err := copyFile(legacyPath, destDB); err != nil {
		return fmt.Errorf("migrate state.db: %w", err)
	}
	if err := os.Chmod(destDB, 0o600); err != nil {
		return err
	}
	bak := legacyPath + ".pre-broker.bak"
	if err := os.Rename(legacyPath, bak); err != nil {
		// Non-fatal: dest is already usable.
		_ = err
	}

	// Extract token from migrated DB into token file if missing.
	if tokenFile != "" {
		if tok, _ := ReadTokenFile(tokenFile); tok == "" {
			store, err := cache.Open(destDB)
			if err == nil {
				defer store.Close()
				if t, err := store.DeviceToken(context.Background()); err == nil && t != "" {
					_ = WriteTokenFile(tokenFile, t)
				}
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
