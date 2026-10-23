package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/DevNewbie1826/omo-webchat/internal/cursorstore"
)

const (
	maxUploadBytes = 100 << 20 // 100 MiB
	// The write limit must cover any file the editor can read.
	maxWriteBytes = maxReadBytes
)

func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	target, err := s.resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The client sends content as JSON; escaping can inflate a max-size
	// payload to 6x (control chars become \u00XX). The body cap must absorb
	// that or valid editor-readable files fail to save; the post-decode
	// length check below is the real limit.
	r.Body = http.MaxBytesReader(w, r.Body, 6*maxWriteBytes+4096)
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "content too large to save")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Content) > maxWriteBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "content too large to save")
		return
	}
	if err := writeFileAtomic(target, []byte(req.Content)); err != nil {
		s.writeFsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".th-edit-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	// Best-effort directory sync persists the rename.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ws, err := s.cursors.GetWorkspace(r.PathValue("wsId"))
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	chatID := r.PathValue("chatId")
	chat, err := s.cursors.GetChat(chatID)
	if err != nil || chat.WorkspaceID != ws.ID {
		s.writeStoreError(w, cursorstore.ErrNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds 100 MiB per request")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	defer r.MultipartForm.RemoveAll()
	// os.Root constrains creation and symlink traversal to this workspace, even
	// when directories are swapped concurrently with an upload.
	root, err := os.OpenRoot(ws.Path)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	defer root.Close()
	dir := r.FormValue("dir")
	if dir == "" {
		dir = "."
	}
	if filepath.IsAbs(dir) || !filepath.IsLocal(dir) || strings.ContainsAny(dir, "\\\r\n") {
		writeError(w, http.StatusBadRequest, "invalid upload directory")
		return
	}
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		if part == ".." {
			writeError(w, http.StatusBadRequest, "invalid upload directory")
			return
		}
	}
	if err := root.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload directory")
		return
	}
	dest, err := root.OpenRoot(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload directory")
		return
	}
	defer dest.Close()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, `no "files" field in multipart form`)
		return
	}
	uploaded := make([]string, 0, len(files))
	for _, fh := range files {
		name, err := saveUpload(fh, dest)
		if err != nil {
			s.logger.Error("saving upload", "file", fh.Filename, "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save "+fh.Filename)
			return
		}
		uploaded = append(uploaded, filepath.ToSlash(filepath.Join(dir, name)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"uploaded": uploaded})
}

func saveUpload(fh *multipart.FileHeader, dest *os.Root) (string, error) {
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	name := strings.TrimFunc(fh.Filename, func(r rune) bool { return r == '.' || unicode.IsSpace(r) })
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	if name == "" {
		return "", errors.New("invalid filename")
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 0; ; n++ {
		candidate := name
		if n > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		dst, err := dest.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			_ = dest.Remove(candidate)
			return "", err
		}
		return candidate, nil
	}
}
