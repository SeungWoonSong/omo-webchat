package api

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

const maxZipEntries = 10000
const maxZipBytes int64 = 256 << 20

var errZipLimit = errors.New("directory ZIP exceeds limit (10000 entries / 256 MiB); download a smaller folder")

type zipEntry struct {
	path string
	info fs.FileInfo
}

// Preflight the complete bounded manifest before sending headers, so limits
// produce a clear HTTP error rather than a silently truncated successful ZIP.
func zipManifest(root fs.FS) ([]zipEntry, error) {
	entries := make([]zipEntry, 0)
	var total int64
	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == ".venv") {
			return fs.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		if len(entries) >= maxZipEntries || total > maxZipBytes {
			return errZipLimit
		}
		entries = append(entries, zipEntry{path, info})
		return nil
	})
	return entries, err
}

func (s *Server) handleZip(w http.ResponseWriter, r *http.Request, target string) {
	allowed, err := os.OpenRoot(s.cfg.Root)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	defer allowed.Close()
	rel, err := filepath.Rel(s.cfg.Root, target)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	root, err := allowed.OpenRoot(rel)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	defer root.Close()
	entries, err := zipManifest(root.FS())
	if errors.Is(err, errZipLimit) {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(target) + ".zip"}))
	zw := zip.NewWriter(w)
	for _, entry := range entries {
		if err := r.Context().Err(); err != nil {
			panic(http.ErrAbortHandler)
		}
		if err := writeZipEntry(zw, root, entry.path, entry.info); err != nil {
			s.logger.Warn("ZIP stream aborted", "err", err)
			panic(http.ErrAbortHandler)
		}
	}
	if err := zw.Close(); err != nil {
		s.logger.Warn("ZIP close failed", "err", err)
	}
}

func writeZipEntry(zw *zip.Writer, root *os.Root, rel string, info fs.FileInfo) error {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(rel)
	if info.IsDir() {
		header.Name += "/"
		_, err = zw.CreateHeader(header)
		return err
	}
	f, err := root.Open(rel)
	if err != nil {
		return err
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || current.Size() != info.Size() {
		return fmt.Errorf("file changed during ZIP download: %s", rel)
	}
	header.Method = zip.Deflate
	dst, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(f, info.Size()+1))
	if err != nil {
		return err
	}
	if n != info.Size() {
		return fmt.Errorf("file changed during ZIP download: %s", rel)
	}
	return nil
}
