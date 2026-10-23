//go:build darwin || linux

package api

import (
	"archive/zip"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func previewTestServer(t *testing.T, script string) (*Server, string) {
	t.Helper()
	s, root := newWriteTestServer(t)
	s.cfg.StateDir = t.TempDir()
	binary := filepath.Join(t.TempDir(), "converter")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHAT_SOFFICE_BINARY", binary)
	if err := os.WriteFile(filepath.Join(root, "file.docx"), []byte("office"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, root
}

const fakePDFConverter = `for arg do
 if [ "$prev" = "--outdir" ]; then out="$arg"; fi
 prev="$arg"
done
printf '%%PDF-1.4\nconverted' > "$out/input.pdf"
`

func previewRequest(s *Server, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.handlePreview(w, httptest.NewRequest("GET", "/api/fs/preview?path="+path, nil))
	return w
}

func TestPreviewSuccessCacheDedupeHeaders(t *testing.T) {
	count := filepath.Join(t.TempDir(), "count")
	s, _ := previewTestServer(t, "echo call >> '"+count+"'\n"+fakePDFConverter)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			w := previewRequest(s, "file.docx")
			if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "%PDF-") {
				t.Errorf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	wg.Wait()
	w := previewRequest(s, "file.docx")
	if w.Header().Get("Content-Type") != "application/pdf" || w.Header().Get("Content-Security-Policy") != pdfCSP || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") {
		t.Fatal(w.Header())
	}
	calls, _ := os.ReadFile(count)
	if string(calls) != "call\n" {
		t.Fatalf("converter calls: %q", calls)
	}
	entries, err := os.ReadDir(filepath.Join(s.cfg.StateDir, "preview-cache"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries %v %v", entries, err)
	}
	info, _ := entries[0].Info()
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	dir, _ := os.Stat(filepath.Join(s.cfg.StateDir, "preview-cache"))
	if dir.Mode().Perm() != 0700 {
		t.Fatal(dir.Mode())
	}
}
func TestPreviewErrors(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		status       int
	}{
		{"non PDF", strings.ReplaceAll(fakePDFConverter, "%%PDF-1.4", "invalid"), 422},
		{"failure", "echo private-content >&2; exit 1", 422},
		{"timeout", "sleep 60 & wait", 504},
		{"missing", "", 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := previewTestServer(t, tc.script)
			if tc.name == "timeout" {
				s.previewTimeout = 200 * time.Millisecond
			}
			if tc.name == "missing" {
				t.Setenv("CHAT_SOFFICE_BINARY", filepath.Join(t.TempDir(), "absent"))
			}
			start := time.Now()
			w := previewRequest(s, "file.docx")
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private-content") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if time.Since(start) > 5*time.Second {
				t.Fatal("process tree did not terminate promptly")
			}
		})
	}
}
func TestPreviewLimitsEscapesAndHWP(t *testing.T) {
	s, root := previewTestServer(t, fakePDFConverter)
	large, err := os.Create(filepath.Join(root, "large.docx"))
	if err != nil {
		t.Fatal(err)
	}
	if err = large.Truncate(maxPreviewInput + 1); err != nil {
		t.Fatal(err)
	}
	large.Close()
	if w := previewRequest(s, "large.docx"); w.Code != 413 {
		t.Fatal(w.Code)
	}
	outside := filepath.Join(t.TempDir(), "secret.docx")
	os.WriteFile(outside, []byte("secret"), 0600)
	os.Symlink(outside, filepath.Join(root, "escape.docx"))
	for _, p := range []string{"../secret.docx", "escape.docx"} {
		if w := previewRequest(s, p); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	os.WriteFile(filepath.Join(root, "file.hwp"), []byte("hwp"), 0600)
	t.Setenv("CHAT_HWP5ODT_BINARY", filepath.Join(t.TempDir(), "missing"))
	if w := previewRequest(s, "file.hwp"); w.Code != 501 {
		t.Fatal(w.Code)
	}
	binary := filepath.Join(t.TempDir(), "hwp5odt")
	os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'odt' > \"$2\"\n"), 0700)
	t.Setenv("CHAT_HWP5ODT_BINARY", binary)
	if w := previewRequest(s, "file.hwp"); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
func TestPreviewQueueCancellation(t *testing.T) {
	s, _ := previewTestServer(t, fakePDFConverter)
	previewLane <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handlePreview(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/fs/preview?path=file.docx", nil).WithContext(ctx))
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("cancelled waiter blocked")
	}
	<-previewLane
}
func TestPreviewCacheEviction(t *testing.T) {
	dir := t.TempDir()
	for i := range 3 {
		p := filepath.Join(dir, fmt.Sprintf("%d.pdf", i))
		f, _ := os.Create(p)
		f.Truncate(200 << 20)
		f.Close()
		stamp := time.Unix(int64(i), 0)
		os.Chtimes(p, stamp, stamp)
	}
	evictPreviewCache(dir)
	if _, err := os.Stat(filepath.Join(dir, "0.pdf")); !os.IsNotExist(err) {
		t.Fatal("oldest entry not evicted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatal(entries)
	}
}
func TestPreviewRealSofficeDOCX(t *testing.T) {
	binary, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("soffice unavailable")
	}
	s, root := previewTestServer(t, "")
	t.Setenv("CHAT_SOFFICE_BINARY", binary)
	f, err := os.Create(filepath.Join(root, "file.docx"))
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>안녕하세요 문서 미리보기</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	w := previewRequest(s, "file.docx")
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "%PDF-") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPreviewRouteRequiresAuthentication(t *testing.T) {
	s, _, _ := newChatCreateTestServer(t)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/fs/preview?path=file.docx", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestPreviewGlobalConcurrency(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "converter-lock")
	s, root := previewTestServer(t, "mkdir '"+lock+"' || exit 1\ntrap \"rmdir '"+lock+"'\" EXIT\n"+fakePDFConverter)
	var wg sync.WaitGroup
	for i := range 10 {
		name := fmt.Sprintf("file-%d.docx", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		wg.Go(func() {
			w := previewRequest(s, name)
			if w.Code != 200 {
				t.Errorf("concurrent conversion %s: %d", name, w.Code)
			}
		})
	}
	wg.Wait()
}
