//go:build darwin || linux

package api

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// Open nonblocking under the root so a queued input swapped for a FIFO cannot
// pin the conversion lane. O_NOFOLLOW rejects a swapped leaf symlink too.
func openPreviewSource(root, rel string) (*os.File, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, errPreviewNotRegular
	}
	// Regular files ignore O_NONBLOCK; retain it through the bounded copy.
	return f, nil
}

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
