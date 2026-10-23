package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DevNewbie1826/omo-webchat/internal/cursorstore"
	"github.com/DevNewbie1826/omo-webchat/internal/procexec"
)

const maxPreviewInput int64 = 50 << 20
const maxPreviewCache int64 = 500 << 20
const pdfCSP = "script-src 'none'; connect-src 'none'; base-uri 'none'; form-action 'none'"

// One process-wide conversion lane also serializes cache publication/eviction.
var previewLane = make(chan struct{}, 1)
var previewWaiting = make(chan struct{}, 8)
var previewFlights = struct {
	sync.Mutex
	jobs map[string]*previewJob
}{jobs: make(map[string]*previewJob)}

type previewJob struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	status  int
	result  string
	tempDir string
}

func officeExtension(ext string) bool {
	switch ext {
	case ".docx", ".doc", ".odt", ".rtf", ".xlsx", ".xls", ".ods", ".pptx", ".ppt", ".odp", ".hwp":
		return true
	}
	return false
}

func previewBinary(env, fallback string) (string, error) {
	name := os.Getenv(env)
	if name == "" {
		name = fallback
	}
	return exec.LookPath(name)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Context().Err() != nil {
		return
	}
	target, err := s.resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, 400, "invalid path")
		return
	}
	ext := strings.ToLower(filepath.Ext(target))
	if !officeExtension(ext) {
		writeError(w, 501, "unsupported_format")
		return
	}
	rel, err := filepath.Rel(s.cfg.Root, target)
	if err != nil {
		writeError(w, 400, "invalid path")
		return
	}
	// Metadata does not retain an input descriptor while waiting for the lane.
	info, err := os.Stat(target)
	if err != nil {
		s.writeFsError(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeError(w, 400, "not a regular file")
		return
	}
	if info.Size() > maxPreviewInput {
		writeError(w, 413, "file_too_large")
		return
	}
	stateDir := s.cfg.StateDir
	if stateDir == "" && s.cursors != nil {
		stateDir = s.cursors.StateDir()
	}
	if stateDir == "" {
		stateDir, err = cursorstore.StateDir()
		if err != nil {
			s.writeFsError(w, err)
			return
		}
	}
	cacheDir := filepath.Join(stateDir, "preview-cache")
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", target, info.Size(), info.ModTime().UnixNano()))))
	cached := filepath.Join(cacheDir, key+".pdf")
	cacheOK := preparePreviewCache(cacheDir)
	if !cacheOK || !servePreviewPDF(w, r, cached, target) {
		soffice, err := previewBinary("CHAT_SOFFICE_BINARY", "soffice")
		if err != nil {
			writeError(w, 501, "preview_tool_missing")
			return
		}
		hwp := ""
		if ext == ".hwp" {
			hwp, err = previewBinary("CHAT_HWP5ODT_BINARY", "/opt/ai-portal/tools/pyhwp/bin/hwp5odt")
			if err != nil {
				writeError(w, 501, "preview_tool_missing")
				return
			}
		}
		previewFlights.Lock()
		job := previewFlights.jobs[cached]
		if job == nil {
			// Followers share the existing queue entry; admit only unique jobs.
			laneHeld := false
			select {
			case previewLane <- struct{}{}:
				laneHeld = true
			default:
				select {
				case previewWaiting <- struct{}{}:
				default:
					previewFlights.Unlock()
					w.Header().Set("Retry-After", "2")
					writeError(w, http.StatusServiceUnavailable, "busy")
					return
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			job = &previewJob{done: make(chan struct{}), cancel: cancel}
			previewFlights.jobs[cached] = job
			go func() {
				status, result, tempDir := s.convertPreview(ctx, rel, info, ext, soffice, hwp, cacheDir, cached, laneHeld, cacheOK)
				previewFlights.Lock()
				job.status, job.result, job.tempDir = status, result, tempDir
				if job.waiters == 0 && tempDir != "" {
					os.RemoveAll(tempDir)
				}
				if previewFlights.jobs[cached] == job {
					delete(previewFlights.jobs, cached)
				}
				close(job.done)
				previewFlights.Unlock()
				cancel()
			}()
		}
		job.waiters++
		previewFlights.Unlock()
		defer func() {
			previewFlights.Lock()
			job.waiters--
			if job.waiters == 0 {
				if previewFlights.jobs[cached] == job {
					delete(previewFlights.jobs, cached)
				}
				job.cancel()
				if job.tempDir != "" {
					os.RemoveAll(job.tempDir)
				}
			}
			previewFlights.Unlock()
		}()
		select {
		case <-r.Context().Done():
			return
		case <-job.done:
		}
		if job.status != 0 {
			reason := "conversion_failed"
			if job.status == 504 {
				reason = "conversion_timeout"
			}
			if job.status == 413 {
				reason = "file_too_large"
			}
			writeError(w, job.status, reason)
			return
		}
		if !servePreviewPDF(w, r, job.result, target) {
			writeError(w, 422, "conversion_failed")
		}
	}
}

func servePreviewPDF(w http.ResponseWriter, r *http.Request, cached, target string) bool {
	f, err := openPreviewPDF(cached)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 5)
	n, _ := f.ReadAt(head, 0)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || n != 5 || !bytes.Equal(head, []byte("%PDF-")) {
		return false
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", pdfCSP)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": strings.TrimSuffix(filepath.Base(target), filepath.Ext(target)) + ".pdf"}))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return true
}

func (s *Server) convertPreview(ctx context.Context, rel string, expected os.FileInfo, ext, soffice, hwp, cacheDir, cached string, laneHeld, cacheOK bool) (status int, result, tempDir string) {
	if !laneHeld {
		select {
		case previewLane <- struct{}{}:
			laneHeld = true
		case <-ctx.Done():
		}
		<-previewWaiting
	}
	if laneHeld {
		defer func() { <-previewLane }()
	}
	if ctx.Err() != nil {
		return 504, "", ""
	}
	// A previous job may have published after this request's initial cache miss.
	if cacheOK && preparePreviewCache(cacheDir) {
		if f, err := openPreviewPDF(cached); err == nil {
			var head [5]byte
			n, _ := f.Read(head[:])
			f.Close()
			if n == 5 && string(head[:]) == "%PDF-" {
				return 0, cached, ""
			}
		}
	}
	// Open only after admission to the conversion lane, always root-confined.
	source, err := os.OpenInRoot(s.cfg.Root, rel)
	if err != nil {
		return 422, "", ""
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(expected, info) || info.Size() != expected.Size() || !info.ModTime().Equal(expected.ModTime()) {
		return 422, "", ""
	}
	timeout := s.previewTimeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tmp, err := os.MkdirTemp("", "webchat-preview-")
	if err != nil {
		return 422, "", ""
	}
	defer func() {
		if tempDir == "" {
			os.RemoveAll(tmp)
		}
	}()
	input := filepath.Join(tmp, "input"+ext)
	dst, err := os.OpenFile(input, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 422, "", ""
	}
	n, copyErr := io.Copy(dst, io.LimitReader(source, maxPreviewInput+1))
	closeErr := dst.Close()
	if n > maxPreviewInput {
		return 413, "", ""
	}
	if copyErr != nil || closeErr != nil || n != expected.Size() {
		return 422, "", ""
	}
	if hwp != "" {
		odt := filepath.Join(tmp, "input.odt")
		if err = s.runPreviewCommand(ctx, tmp, hwp, "--output", odt, input); err == nil {
			input = odt
		}
	}
	if err == nil {
		profile := (&url.URL{Scheme: "file", Path: filepath.Join(tmp, "profile")}).String()
		err = s.runPreviewCommand(ctx, tmp, soffice, "-env:UserInstallation="+profile, "--headless", "--norestore", "--convert-to", "pdf", "--outdir", tmp, input)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return 504, "", ""
	}
	if err != nil || ctx.Err() != nil {
		return 422, "", ""
	}
	pdfPath := filepath.Join(tmp, "input.pdf")
	pdf, err := openPreviewPDF(pdfPath)
	if err != nil {
		return 422, "", ""
	}
	defer pdf.Close()
	var head [5]byte
	nread, _ := pdf.ReadAt(head[:], 0)
	if nread != 5 || string(head[:]) != "%PDF-" {
		return 422, "", ""
	}
	// Cache failures must not prevent streaming a successfully converted PDF.
	if !cacheOK || !preparePreviewCache(cacheDir) {
		return 0, pdfPath, tmp
	}
	dest, err := os.CreateTemp(cacheDir, ".preview-*")
	if err != nil {
		return 0, pdfPath, tmp
	}
	defer os.Remove(dest.Name())
	_, err = io.Copy(dest, pdf)
	closeErr = dest.Close()
	if err != nil || closeErr != nil {
		return 0, pdfPath, tmp
	}
	if err = os.Rename(dest.Name(), cached); err != nil {
		return 0, pdfPath, tmp
	}
	evictPreviewCache(cacheDir)
	return 0, cached, ""
}

// Keep only a bounded stderr tail; converters may emit document text or paths,
// so it is logged on the server only, never included in an HTTP error.
type previewTail []byte

func (b *previewTail) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= 4096 {
		*b = append((*b)[:0], p[len(p)-4096:]...)
	} else {
		*b = append(*b, p...)
		if len(*b) > 4096 {
			*b = (*b)[len(*b)-4096:]
		}
	}
	return n, nil
}
func (s *Server) runPreviewCommand(ctx context.Context, home, binary string, args ...string) error {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	procexec.SetupCommand(cmd)
	var stderr previewTail
	cmd.Stderr = &stderr
	tracked, err := procexec.StartTracked(cmd)
	if err != nil {
		return err
	}
	defer tracked.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = tracked.TerminateTree()
		err = <-done
	}
	// Remove descendants even if a converter exits before its children.
	_ = tracked.TerminateTree()
	if err != nil {
		s.logger.Warn("document preview conversion failed", "stderr_tail", string(stderr))
	}
	return err
}
func evictPreviewCache(dir string) {
	entries, _ := os.ReadDir(dir)
	type entry struct {
		path     string
		size     int64
		modified time.Time
	}
	var files []entry
	var total int64
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".pdf") {
			if info, err := e.Info(); err == nil {
				files = append(files, entry{filepath.Join(dir, e.Name()), info.Size(), info.ModTime()})
				total += info.Size()
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified.Before(files[j].modified) })
	for _, f := range files {
		if total <= maxPreviewCache {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
