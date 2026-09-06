//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldpath, newpath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if err == unix.ENOSYS || err == unix.EINVAL {
		return fmt.Errorf("atomic no-replace rename is required (renameat2 RENAME_NOREPLACE): %w", err)
	}
	if err == unix.EEXIST || err == syscall.EEXIST {
		return fmt.Errorf("-out already exists; refuse to clobber %s", newpath)
	}
	return err
}

func openRegularFile(path string) (*os.File, os.FileInfo, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if err == unix.ELOOP {
			return nil, nil, fmt.Errorf("apk is not a regular file")
		}
		if err == unix.ENOENT {
			return nil, nil, fmt.Errorf("apk missing")
		}
		return nil, nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("apk is not a regular file")
	}
	if err := unix.SetNonblock(int(f.Fd()), false); err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, info, nil
}
