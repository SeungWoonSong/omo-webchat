package api

import (
	"bytes"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Active and unknown raw documents are sandboxed. Verified PDFs use the
// browser-native PDF viewer, which CSP sandbox would disable.
// No allow-same-origin: scripts must never acquire the application's origin.
const rawCSP = "sandbox allow-scripts allow-popups allow-forms; default-src 'self' data: blob: 'unsafe-inline'; connect-src 'none'; form-action 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'"

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", rawCSP)
	target, err := s.resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rel, err := filepath.Rel(s.cfg.Root, target)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	f, err := os.OpenInRoot(s.cfg.Root, rel)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeError(w, http.StatusBadRequest, "path is not a regular file")
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(target))
	if contentType == "" {
		head := make([]byte, 512)
		n, _ := f.Read(head)
		contentType = http.DetectContentType(head[:n])
		if _, err := f.Seek(0, 0); err != nil {
			s.writeFsError(w, err)
			return
		}
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(target)}))
	csp := rawCSP
	if strings.EqualFold(filepath.Ext(target), ".pdf") {
		signature := make([]byte, 5)
		n, _ := f.ReadAt(signature, 0)
		contentType = "application/octet-stream"
		if n == 5 && bytes.Equal(signature, []byte("%PDF-")) {
			contentType = "application/pdf"
			csp = "script-src 'none'; connect-src 'none'; base-uri 'none'; form-action 'none'"
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
