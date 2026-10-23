//go:build darwin || linux

package api

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func previewOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

func preparePreviewCache(dir string) bool {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !previewOwned(info) {
		return false
	}
	f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || !previewOwned(current) {
		return false
	}
	return current.Mode().Perm() == 0700 || f.Chmod(0700) == nil
}

func openPreviewPDF(path string) (*os.File, error) {
	// Validate the containing directory too: O_NOFOLLOW only covers the leaf.
	info, err := os.Lstat(filepath.Dir(path))
	if err != nil || !info.IsDir() || !previewOwned(info) || info.Mode().Perm() != 0700 {
		return nil, os.ErrPermission
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || !previewOwned(info) {
		f.Close()
		return nil, errors.New("unsafe preview file")
	}
	return f, nil
}
