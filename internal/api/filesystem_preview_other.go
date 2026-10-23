//go:build !darwin && !linux

package api

import "os"

func openPreviewSource(root, rel string) (*os.File, error) {
	return os.OpenInRoot(root, rel)
}

// Persistent caching requires Unix ownership and no-follow checks.
func preparePreviewCache(string) bool { return false }

func openPreviewPDF(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	return os.Open(path)
}
