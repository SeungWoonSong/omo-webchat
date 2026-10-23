package api

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestZipSkipsUnsafeNames(t *testing.T) {
	s, root := newWriteTestServer(t)
	for _, name := range []string{`..\outside.txt`, `C:\outside.txt`, `C:outside.txt`, "normal/nested.txt", `bad\dir/child.txt`} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	s.handleDownload(w, httptest.NewRequest("GET", "/api/fs/download", nil))
	if w.Code != 200 || w.Result().Header.Get("X-Zip-Skipped") != "4" {
		t.Fatalf("status/headers: %d %v", w.Code, w.Result().Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != "normal/" && f.Name != "normal/nested.txt" {
			t.Errorf("unsafe/unexpected ZIP name %q", f.Name)
		}
	}
	if len(zr.File) != 2 {
		t.Fatalf("entries=%d", len(zr.File))
	}
}

func TestRawErrorSecurityHeaders(t *testing.T) {
	s, _ := newWriteTestServer(t)
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"missing.txt", ".", "../outside"} {
			t.Run(method+"/"+path, func(t *testing.T) {
				w := httptest.NewRecorder()
				s.handleRaw(w, httptest.NewRequest(method, "/api/fs/raw?path="+path, nil))
				if w.Code != 400 {
					t.Fatalf("status=%d", w.Code)
				}
				if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") != rawCSP {
					t.Fatalf("headers=%v", w.Header())
				}
			})
		}
	}
}

// Count entries actually read, including WalkDir's unbounded ReadDir(-1).
type countingZipFS struct {
	fs.FS
	read      int
	afterRead func()
}
type countingZipDir struct {
	fs.ReadDirFile
	owner *countingZipFS
}

func (f *countingZipFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if dir, ok := file.(fs.ReadDirFile); ok {
		return &countingZipDir{dir, f}, nil
	}
	return file, nil
}
func (d *countingZipDir) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := d.ReadDirFile.ReadDir(n)
	d.owner.read += len(entries)
	if d.owner.afterRead != nil {
		d.owner.afterRead()
	}
	return entries, err
}
func TestZipManifestStopsReadingAtEntryCap(t *testing.T) {
	files := fstest.MapFS{}
	for i := 0; i < maxZipEntries+2000; i++ {
		files[fmt.Sprintf("file-%05d", i)] = &fstest.MapFile{Data: []byte("x")}
	}
	root := &countingZipFS{FS: files}
	if _, _, err := zipManifest(t.Context(), root); err != errZipLimit {
		t.Fatalf("err=%v", err)
	}
	if root.read > maxZipEntries+64 {
		t.Fatalf("read remainder: %d entries", root.read)
	}
}
func TestZipCancelledRequestStopsBeforeResponse(t *testing.T) {
	s, root := newWriteTestServer(t)
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	s.handleDownload(w, httptest.NewRequest("GET", "/api/fs/download", nil).WithContext(ctx))
	if w.Header().Get("Content-Type") == "application/zip" || bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		t.Fatal("cancelled request produced ZIP")
	}
}

func TestZipManifestCancellationDuringWalk(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	files := fstest.MapFS{}
	for i := 0; i < 200; i++ {
		files[fmt.Sprintf("file-%03d", i)] = &fstest.MapFile{Data: []byte("x")}
	}
	root := &countingZipFS{FS: files, afterRead: cancel}
	entries, _, err := zipManifest(ctx, root)
	if !errors.Is(err, context.Canceled) || len(entries) != 0 || root.read > 64 {
		t.Fatalf("entries=%d read=%d err=%v", len(entries), root.read, err)
	}
}

func TestZipEntryNameValidation(t *testing.T) {
	for _, path := range []string{`..\outside.txt`, `C:\outside.txt`, "C:outside.txt", "/outside", "a//b", "a/./b", "a/../b", "a/", "a/\x00b", "a/D:outside.txt", ""} {
		if _, ok := zipEntryName(path); ok {
			t.Errorf("accepted %q", path)
		}
	}
	if name, ok := zipEntryName("normal/nested.txt"); !ok || name != "normal/nested.txt" {
		t.Fatalf("name=%q ok=%v", name, ok)
	}
}
