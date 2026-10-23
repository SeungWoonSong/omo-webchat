//go:build darwin || linux

package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// The descendant inherits the converter's process group but not its stderr pipe:
// a parent-only kill must return promptly and fail the explicit PID assertion.
func previewProcessTreeFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	pidfile := filepath.Join(t.TempDir(), "pids")
	script := fmt.Sprintf(`exec python3 - "$@" <<'PYTHON'
import os, signal, subprocess, sys
from pathlib import Path
out = sys.argv[sys.argv.index('--outdir') + 1]
child = subprocess.Popen(['sleep', '300'], stdin=subprocess.DEVNULL,
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
def complete(*_):
    child.terminate()
    child.wait()
    Path(out, 'input.pdf').write_bytes(b'%%PDF-1.4\nconverted')
    sys.exit(0)
signal.signal(signal.SIGUSR1, complete)
Path(%q).write_text(f'{os.getpid()} {child.pid}')
while True:
    signal.pause()
PYTHON
`, pidfile)
	s, root := previewTestServer(t, script)
	return s, root, pidfile
}

func readPreviewProcessTree(t *testing.T, pidfile string) (int, int) {
	t.Helper()
	var parent, child int
	awaitPreview(t, "converter and descendant PIDs recorded", func() bool {
		data, _ := os.ReadFile(pidfile)
		fields := strings.Fields(string(data))
		if len(fields) != 2 {
			return false
		}
		parent, _ = strconv.Atoi(fields[0])
		child, _ = strconv.Atoi(fields[1])
		return parent > 0 && child > 0
	})
	t.Cleanup(func() {
		_ = syscall.Kill(-parent, syscall.SIGKILL)
		_ = syscall.Kill(child, syscall.SIGKILL)
	})
	for _, pid := range []int{parent, child} {
		if err := syscall.Kill(pid, 0); err != nil {
			t.Fatalf("fixture PID %d not alive before teardown: %v", pid, err)
		}
	}
	return parent, child
}

func awaitPreviewProcessTreeGone(t *testing.T, parent, child int) {
	t.Helper()
	awaitPreview(t, fmt.Sprintf("converter %d and descendant %d PIDs gone", parent, child), func() bool {
		for _, pid := range []int{parent, child} {
			if runtime.GOOS == "linux" {
				// Require disappearance, not just a zombie or a successful signal.
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); !os.IsNotExist(err) {
					return false
				}
			} else if syscall.Kill(pid, 0) != syscall.ESRCH {
				return false
			}
		}
		return syscall.Kill(-parent, 0) == syscall.ESRCH
	})
}

func TestPreviewCancellationProcessGroupAndSharedWaiters(t *testing.T) {
	s, root, pidfile := previewProcessTreeFixture(t)
	// Warm HTTP/runtime lazy initialization before the descriptor baseline.
	if w := previewRequest(s, "../invalid.docx"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	baseline := previewFDCount(t)
	readPID := func() (int, int) { return readPreviewProcessTree(t, pidfile) }
	gone := func(parent, child int) { awaitPreviewProcessTreeGone(t, parent, child) }
	// Only waiter cancellation must kill the already-running converter.
	cancel, done := startPreviewRequest(t, s, "file.docx")
	pid, child := readPID()
	cancel()
	finishPreviewRequest(t, done)
	gone(pid, child)
	awaitPreviewIdle(t)
	os.Remove(pidfile)

	// Two waiters share a single process; the survivor receives a valid PDF.
	cancel1, done1 := startPreviewRequest(t, s, "file.docx")
	pid, child = readPID()
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
	gone(pid, child)
	awaitPreviewIdle(t)
	os.Remove(pidfile)

	// On a fresh key, cancellation of the last of two waiters kills the group.
	os.WriteFile(filepath.Join(root, "next.docx"), []byte("next"), 0600)
	cancel1, done1 = startPreviewRequest(t, s, "next.docx")
	pid, child = readPID()
	cancel2, done2 := startPreviewRequest(t, s, "next.docx")
	awaitPreview(t, "two final waiters", func() bool { return previewWaiterCount() == 2 })
	cancel1()
	finishPreviewRequest(t, done1)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatal(err)
	}
	cancel2()
	finishPreviewRequest(t, done2)
	gone(pid, child)
	awaitPreviewIdle(t)
	os.Remove(pidfile)

	// A subsequent request really starts a converter, proving lane reuse.
	_, done = startPreviewRequest(t, s, "next.docx")
	pid, child = readPID()
	if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	if w := finishPreviewRequest(t, done); w.Code != 200 {
		t.Fatal(w.Code)
	}
	gone(pid, child)
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

func TestPreviewTimeoutProcessGroupAndDescendant(t *testing.T) {
	s, _, pidfile := previewProcessTreeFixture(t)
	s.previewTimeout = time.Second
	_, done := startPreviewRequest(t, s, "file.docx")
	parent, child := readPreviewProcessTree(t, pidfile)
	w := finishPreviewRequest(t, done)
	if w.Code != 504 || !strings.Contains(w.Body.String(), `"error":"conversion_timeout"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	awaitPreviewProcessTreeGone(t, parent, child)
	awaitPreviewIdle(t)
}

func TestPreviewQueuedInputReplacedWithFIFO(t *testing.T) {
	s, root := previewTestServer(t, fakePDFConverter)
	if w := previewRequest(s, "../invalid.docx"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	baseline := previewFDCount(t)
	baselineGoroutines := runtime.NumGoroutine()
	previewLane <- struct{}{}
	held := true
	defer func() {
		if held {
			<-previewLane
		}
	}()
	_, done := startPreviewRequest(t, s, "file.docx")
	awaitPreview(t, "input enqueued", func() bool { return len(previewWaiting) == 1 })
	input := filepath.Join(root, "file.docx")
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(input, 0600); err != nil {
		t.Fatal(err)
	}
	// Rescue a blocking-open regression on failure without leaving the lane or
	// a goroutine stuck in the test process. No writer connects on success.
	t.Cleanup(func() {
		fd, err := syscall.Open(input, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = syscall.Close(fd)
		}
		awaitPreviewIdle(t)
	})
	<-previewLane
	held = false
	w := finishPreviewRequest(t, done)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"not a regular file"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	awaitPreviewIdle(t)
	// Existing FIFOs are rejected before queue admission too.
	if w := previewRequest(s, "file.docx"); w.Code != 400 || !strings.Contains(w.Body.String(), "not a regular file") {
		t.Fatalf("initial FIFO: %d %s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(filepath.Join(root, "next.docx"), []byte("next"), 0600); err != nil {
		t.Fatal(err)
	}
	_, next := startPreviewRequest(t, s, "next.docx")
	if w := finishPreviewRequest(t, next); w.Code != 200 {
		t.Fatalf("next conversion: %d %s", w.Code, w.Body.String())
	}
	awaitPreviewIdle(t)
	awaitPreview(t, "no conversion goroutine leak", func() bool { return runtime.NumGoroutine() <= baselineGoroutines })
	if baseline >= 0 {
		awaitPreview(t, "no FIFO descriptor leak", func() bool { return previewFDCount(t) <= baseline })
	}
}
