//go:build !linux

package main

import (
	"fmt"
	"os"
)

func renameNoReplace(oldpath, newpath string) error {
	return fmt.Errorf("atomic no-replace rename (renameat2 RENAME_NOREPLACE) is required; unsupported on this platform")
}

func openRegularFile(path string) (*os.File, os.FileInfo, error) {
	return nil, nil, fmt.Errorf("nonblocking regular-file snapshot is required; unsupported on this platform")
}
