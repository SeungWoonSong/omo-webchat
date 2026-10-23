package api

import (
	"archive/zip"
	"bytes"
	"github.com/DevNewbie1826/omo-webchat/internal/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRawHeadersAndRange(t *testing.T) {
	s, root := newWriteTestServer(t)
	for _, tc := range []struct{ name, body, mime string }{
		{"page.html", "<script>alert(1)</script>", "text/html"},
		{"page.htm", "<html/>", "text/html"},
		{"page.xhtml", "<html/>", "application/xhtml+xml"},
		{"image.svg", "<svg/>", "image/svg+xml"},
		{"doc.pdf", "%PDF-1.4\nhello", "application/pdf"},
		{"spoof.pdf", "<html>not PDF</html>", "application/octet-stream"},
		{"unknown", "plain text", "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, tc.name), []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/fs/raw?path="+tc.name, nil)
			s.handleRaw(w, r)
			if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), tc.mime) {
				t.Fatalf("status/type: %d %v", w.Code, w.Header())
			}
			if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal(w.Header())
			}
			csp := w.Header().Get("Content-Security-Policy")
			if tc.name == "doc.pdf" {
				if csp != "script-src 'none'; connect-src 'none'; base-uri 'none'; form-action 'none'" {
					t.Fatal(csp)
				}
			} else if !strings.Contains(csp, "sandbox allow-scripts allow-popups allow-forms;") || strings.Contains(csp, "allow-same-origin") || !strings.Contains(csp, "connect-src 'none'") {
				t.Fatal(csp)
			}
			w = httptest.NewRecorder()
			r.Header.Set("Range", "bytes=0-3")
			s.handleRaw(w, r)
			if w.Code != http.StatusPartialContent || w.Body.String() != tc.body[:4] {
				t.Fatalf("range: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleRawRejectsEscapesAndDirectory(t *testing.T) {
	s, root := newWriteTestServer(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "secret")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, "../" + filepath.Base(outside) + "/secret", "escape", root} {
		w := httptest.NewRecorder()
		s.handleRaw(w, httptest.NewRequest("GET", "/api/fs/raw?path="+url.QueryEscape(path), nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestHandleDirectoryZip(t *testing.T) {
	s, root := newWriteTestServer(t)
	for _, dir := range []string{"nested", ".git", "node_modules", ".venv"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "file.txt"), []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handleDownload(w, httptest.NewRequest("GET", "/api/fs/download", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("entries=%d", len(zr.File))
	}
	f, err := zr.Open("nested/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || string(data) != "hello" {
		t.Fatalf("%s %v", data, err)
	}
	// Ordinary downloads retain their attachment/octet-stream behavior.
	w = httptest.NewRecorder()
	s.handleDownload(w, httptest.NewRequest("GET", "/api/fs/download?path=nested/file.txt", nil))
	if w.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal(w.Header())
	}
}

func TestHandleDirectoryZipSizeLimit(t *testing.T) {
	s, root := newWriteTestServer(t)
	f, err := os.Create(filepath.Join(root, "large"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(maxZipBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	w := httptest.NewRecorder()
	s.handleDownload(w, httptest.NewRequest("GET", "/api/fs/download", nil))
	if w.Code != 413 || !strings.Contains(w.Body.String(), "256 MiB") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestZipManifestEntryLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= maxZipEntries; i++ {
		if _, err := os.MkdirTemp(root, "entry-"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := zipManifest(os.DirFS(root)); err != errZipLimit {
		t.Fatalf("got %v", err)
	}
}

func TestRawAndZipRoutesRequireAuthentication(t *testing.T) {
	s, _, ws := newChatCreateTestServer(t)
	s.cfg.Root = ws.Path
	if err := os.WriteFile(filepath.Join(ws.Path, "sample.html"), []byte("<html>test</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	token, err := s.sessions.Create(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/api/fs/raw?path=sample.html", "/api/fs/download"} {
		handler := s.Handler()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", endpoint, nil))
		if w.Code != 401 {
			t.Fatalf("unauthenticated %s: %d", endpoint, w.Code)
		}
		r := httptest.NewRequest("GET", endpoint, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("authenticated %s: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
}

func TestDirectoryZipRejectsOutsideSymlink(t *testing.T) {
	s, root := newWriteTestServer(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape-dir")); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handleDownload(w, httptest.NewRequest("GET", "/api/fs/download?path=escape-dir", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}
