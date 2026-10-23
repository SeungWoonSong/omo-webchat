//go:build darwin || linux

package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Poll observable process/job state, never infer completion from a fixed sleep.
func awaitPreview(t *testing.T, description string, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if ready() {
			return
		}
		select {
		case <-tick.C:
		case <-timer.C:
			t.Fatalf("timed out: %s", description)
		}
	}
}

func startPreviewRequest(t *testing.T, s *Server, name string) (context.CancelFunc, <-chan *httptest.ResponseRecorder) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		s.handlePreview(w, httptest.NewRequest("GET", "/api/fs/preview?path="+name, nil).WithContext(ctx))
		done <- w
	}()
	return cancel, done
}

func finishPreviewRequest(t *testing.T, done <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case w := <-done:
		return w
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
		return nil
	}
}

func previewWaiterCount() int {
	previewFlights.Lock()
	defer previewFlights.Unlock()
	n := 0
	for _, job := range previewFlights.jobs {
		n += job.waiters
	}
	return n
}

func awaitPreviewIdle(t *testing.T) {
	t.Helper()
	awaitPreview(t, "conversion slots released", func() bool {
		return previewWaiterCount() == 0 && len(previewLane) == 0 && len(previewWaiting) == 0
	})
}

func previewFDCount(t *testing.T) int {
	t.Helper()
	files, err := os.ReadDir("/proc/self/fd")
	if os.IsNotExist(err) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func TestPreviewCancellationProcessGroupAndSharedWaiters(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	// exec makes the recorded Python PID the converter's process-group leader.
	script := fmt.Sprintf(`exec python3 - "$@" <<'PY'
import os, signal, sys
from pathlib import Path
out = sys.argv[sys.argv.index('--outdir') + 1]
def complete(*_):
    Path(out, 'input.pdf').write_bytes(b'%%PDF-1.4\nconverted')
    sys.exit(0)
signal.signal(signal.SIGUSR1, complete)
Path(%q).write_text(str(os.getpid()))
while True:
    signal.pause()
PY
`, pidfile)
	s, root := previewTestServer(t, script)
	// Warm HTTP/runtime lazy initialization before the descriptor baseline.
	if w := previewRequest(s, "../invalid.docx"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	baseline := previewFDCount(t)
	readPID := func() int {
		t.Helper()
		pid := 0
		awaitPreview(t, "converter PID recorded", func() bool {
			data, _ := os.ReadFile(pidfile)
			pid, _ = strconv.Atoi(string(data))
			return pid > 0
		})
		t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
		return pid
	}
	gone := func(pid int) {
		t.Helper()
		awaitPreview(t, "PID and process group gone", func() bool {
			return syscall.Kill(pid, 0) == syscall.ESRCH && syscall.Kill(-pid, 0) == syscall.ESRCH
		})
	}
	// Only waiter cancellation must kill the already-running converter.
	cancel, done := startPreviewRequest(t, s, "file.docx")
	pid := readPID()
	cancel()
	finishPreviewRequest(t, done)
	gone(pid)
	awaitPreviewIdle(t)
	os.Remove(pidfile)

	// Two waiters share a single process; the survivor receives a valid PDF.
	cancel1, done1 := startPreviewRequest(t, s, "file.docx")
	pid = readPID()
	_, done2 := startPreviewRequest(t, s, "file.docx")
	awaitPreview(t, "two shared waiters", func() bool { return previewWaiterCount() == 2 })
	cancel1()
	finishPreviewRequest(t, done1)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("survivor lost converter: %v", err)
	}
	if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	if w := finishPreviewRequest(t, done2); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "%PDF-") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	gone(pid)
	awaitPreviewIdle(t)
	os.Remove(pidfile)

	// On a fresh key, cancellation of the last of two waiters kills the group.
	os.WriteFile(filepath.Join(root, "next.docx"), []byte("next"), 0600)
	cancel1, done1 = startPreviewRequest(t, s, "next.docx")
	pid = readPID()
	cancel2, done2 := startPreviewRequest(t, s, "next.docx")
	awaitPreview(t, "two final waiters", func() bool { return previewWaiterCount() == 2 })
	cancel1()
	finishPreviewRequest(t, done1)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatal(err)
	}
	cancel2()
	finishPreviewRequest(t, done2)
	gone(pid)
	awaitPreviewIdle(t)
	os.Remove(pidfile)

	// A subsequent request really starts a converter, proving lane reuse.
	_, done = startPreviewRequest(t, s, "next.docx")
	pid = readPID()
	if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	if w := finishPreviewRequest(t, done); w.Code != 200 {
		t.Fatal(w.Code)
	}
	gone(pid)
	awaitPreviewIdle(t)
	if baseline >= 0 {
		awaitPreview(t, "no descriptor leak", func() bool { return previewFDCount(t) <= baseline })
	}
}

func TestPreviewBoundedQueueCancellationAndFollowers(t *testing.T) {
	s, root := previewTestServer(t, fakePDFConverter)
	previewLane <- struct{}{} // Hold the running lane deterministically.
	held := true
	defer func() {
		if held {
			<-previewLane
		}
	}()
	var cancels []context.CancelFunc
	var requests []<-chan *httptest.ResponseRecorder
	baseline := previewFDCount(t)
	for i := range 8 {
		name := fmt.Sprintf("queued-%d.docx", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		cancel, done := startPreviewRequest(t, s, name)
		cancels = append(cancels, cancel)
		requests = append(requests, done)
		awaitPreview(t, "queue admission", func() bool { return len(previewWaiting) == i+1 })
	}
	if baseline >= 0 && previewFDCount(t) != baseline {
		t.Fatal("queued requests retained input FDs")
	}
	w := previewRequest(s, "file.docx")
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"error":"busy"`) || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %s %v", w.Code, w.Body.String(), w.Header())
	}
	// A follower is accepted even with every queue entry occupied.
	followerCancel, follower := startPreviewRequest(t, s, "queued-0.docx")
	awaitPreview(t, "follower joins full queue", func() bool { return previewWaiterCount() == 9 })
	cancels[0]()
	finishPreviewRequest(t, requests[0])
	if len(previewWaiting) != 8 {
		t.Fatal("follower did not retain shared queue entry")
	}
	followerCancel()
	finishPreviewRequest(t, follower)
	awaitPreview(t, "cancelled entry released", func() bool { return len(previewWaiting) == 7 })
	for i := 1; i < 8; i++ {
		cancels[i]()
		finishPreviewRequest(t, requests[i])
	}
	awaitPreview(t, "queue drained", func() bool { return len(previewWaiting) == 0 })
	<-previewLane
	held = false
	awaitPreviewIdle(t)
	if w := previewRequest(s, "file.docx"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if baseline >= 0 && previewFDCount(t) > baseline {
		t.Fatal("queue descriptor leak")
	}
}

func TestPreviewQueueDeadline(t *testing.T) {
	s, root := previewTestServer(t, fakePDFConverter)
	info, err := os.Stat(filepath.Join(root, "file.docx"))
	if err != nil {
		t.Fatal(err)
	}
	previewLane <- struct{}{}
	defer func() { <-previewLane }()
	previewWaiting <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	status, _, _ := s.convertPreview(ctx, "file.docx", info, ".docx", "unused", "", "", "", false, false)
	if status != 504 || len(previewWaiting) != 0 {
		t.Fatalf("deadline status=%d queue=%d", status, len(previewWaiting))
	}
}
