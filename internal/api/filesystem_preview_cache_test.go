//go:build darwin || linux

package api

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPreviewCacheDirectorySafety(t *testing.T) {
	for _, mode := range []os.FileMode{0755, 0777, 0700} {
		t.Run(mode.String(), func(t *testing.T) {
			s, _ := previewTestServer(t, fakePDFConverter)
			dir := filepath.Join(s.cfg.StateDir, "preview-cache")
			if err := os.Mkdir(dir, mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			if w := previewRequest(s, "file.docx"); w.Code != 200 {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			info, err := os.Lstat(dir)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatalf("permissions: %v %v", info, err)
			}
		})
	}
	t.Run("symlink bypasses cache and streams private PDF", func(t *testing.T) {
		s, _ := previewTestServer(t, fakePDFConverter)
		outside := t.TempDir()
		dir := filepath.Join(s.cfg.StateDir, "preview-cache")
		if err := os.Symlink(outside, dir); err != nil {
			t.Fatal(err)
		}
		if preparePreviewCache(dir) {
			t.Fatal("symlink accepted")
		}
		for range 2 {
			w := previewRequest(s, "file.docx")
			if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "%PDF-") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 0 {
			t.Fatalf("unsafe cache was written: %v %v", entries, err)
		}
	})
	t.Run("non directory bypasses cache", func(t *testing.T) {
		s, _ := previewTestServer(t, fakePDFConverter)
		dir := filepath.Join(s.cfg.StateDir, "preview-cache")
		if err := os.WriteFile(dir, []byte("sentinel"), 0600); err != nil {
			t.Fatal(err)
		}
		if w := previewRequest(s, "file.docx"); w.Code != 200 {
			t.Fatal(w.Code)
		}
		data, _ := os.ReadFile(dir)
		if string(data) != "sentinel" {
			t.Fatal("cache obstacle modified")
		}
	})
}

func TestPreviewCacheFileNoFollowAndRegular(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.pdf")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if f, err := openPreviewPDF(link); err == nil {
		f.Close()
		t.Fatal("cache symlink opened")
	}
	fifo := filepath.Join(dir, "fifo.pdf")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := openPreviewPDF(fifo); err == nil {
		f.Close()
		t.Fatal("nonregular cache opened")
	}
	nested := filepath.Join(dir, "directory.pdf")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if f, err := openPreviewPDF(nested); err == nil {
		f.Close()
		t.Fatal("directory served as PDF")
	}
}

type previewForeignOwnerInfo struct{ os.FileInfo }

func (info previewForeignOwnerInfo) Sys() any { return &syscall.Stat_t{Uid: uint32(os.Getuid() + 1)} }

func TestPreviewCacheRejectsForeignOwner(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if previewOwned(previewForeignOwnerInfo{info}) {
		t.Fatal("foreign UID accepted")
	}
	if !previewOwned(info) {
		t.Fatal("current UID rejected")
	}
	// Real foreign ownership, when the test runner has chown permission.
	if os.Getuid() != 0 {
		return
	}
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	if err := os.Mkdir(cache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(cache, 1, -1); err != nil {
		t.Fatal(err)
	}
	if preparePreviewCache(cache) {
		t.Fatal("foreign cache directory accepted")
	}
	file := filepath.Join(dir, "foreign.pdf")
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(file, 1, -1); err != nil {
		t.Fatal(err)
	}
	if f, err := openPreviewPDF(file); err == nil {
		f.Close()
		t.Fatal("foreign cache file accepted")
	}
}
