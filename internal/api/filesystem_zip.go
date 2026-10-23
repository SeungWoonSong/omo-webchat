package api

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxZipEntries = 10000
const maxZipBytes int64 = 256 << 20

var errZipLimit = errors.New("directory ZIP exceeds limit (10000 entries / 256 MiB); download a smaller folder")

type zipEntry struct {
	path string
	info fs.FileInfo
}

// Build names from validated components, without cleaning away unsafe input.
func zipEntryName(rel string) (string, bool) {
	if strings.ContainsAny(rel, "\\\x00") || strings.HasPrefix(rel, "/") {
		return "", false
	}
	components := strings.Split(rel, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." ||
			(len(component) >= 2 && ((component[0] >= 'A' && component[0] <= 'Z') || (component[0] >= 'a' && component[0] <= 'z')) && component[1] == ':') {
			return "", false
		}
	}
	return strings.Join(components, "/"), true
}

// Preflight a bounded manifest before writing headers. ReadDir batches avoid
// WalkDir's whole-directory allocation; cancellation and limits stop traversal.
func zipManifest(ctx context.Context, root fs.FS) ([]zipEntry, int, error) {
	entries := make([]zipEntry, 0)
	var total int64
	skipped := 0
	var walk func(string) error
	walk = func(dir string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := root.Open(dir)
		if err != nil {
			return err
		}
		defer f.Close()
		reader, ok := f.(fs.ReadDirFile)
		if !ok {
			return fmt.Errorf("directory cannot be read: %s", dir)
		}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			children, readErr := reader.ReadDir(64)
			for _, d := range children {
				if err := ctx.Err(); err != nil {
					return err
				}
				rel := d.Name()
				if dir != "." {
					rel = dir + "/" + rel
				}
				name, safe := zipEntryName(rel)
				if !safe {
					skipped++
					continue
				}
				if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == ".venv") {
					continue
				}
				if d.Type()&os.ModeSymlink != 0 {
					continue
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				if !info.IsDir() && !info.Mode().IsRegular() {
					continue
				}
				if len(entries) >= maxZipEntries {
					return errZipLimit
				}
				if info.Mode().IsRegular() {
					if info.Size() > maxZipBytes-total {
						return errZipLimit
					}
					total += info.Size()
				}
				entries = append(entries, zipEntry{name, info})
				if info.IsDir() {
					if err := walk(name); err != nil {
						return err
					}
				}
			}
			if readErr == io.EOF {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	err := walk(".")
	return entries, skipped, err
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
	entries, skipped, err := zipManifest(r.Context(), root.FS())
	if errors.Is(err, errZipLimit) {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	w.Header().Set("X-Zip-Skipped", strconv.Itoa(skipped))
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
	name, safe := zipEntryName(rel)
	if !safe {
		return fmt.Errorf("unsafe ZIP entry: %q", rel)
	}
	header.Name = name
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
