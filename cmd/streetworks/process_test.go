package main

import (
	"bytes"
	"context"

	"io"

	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func waitFor(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var listenRE = regexp.MustCompile(`serving .* at (http://[0-9.:\[\]a-f]+)`)

// testSite is a tiny built site: a page and its data.
func testSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>streetworks</title>"), 0o644)
	os.WriteFile(filepath.Join(dir, "data", "index.json"), []byte(`{"version":1}`), 0o644)
	return dir
}

func TestPreviewServesFilesAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuf{}
	done := make(chan error, 1)
	go func() {
		done <- preview(ctx, strings.NewReader(""), out, []string{"-dir", testSite(t), "-addr", "127.0.0.1:0"})
	}()
	waitFor(t, "the preview to start", 5*time.Second, func() bool { return listenRE.MatchString(out.String()) })
	base := listenRE.FindStringSubmatch(out.String())[1]

	if code, body := get(t, base+"/"); code != 200 || !strings.Contains(body, "streetworks") {
		t.Errorf("/: %d %q", code, body)
	}
	if code, body := get(t, base+"/data/index.json"); code != 200 || body != `{"version":1}` {
		t.Errorf("index.json: %d %q", code, body)
	}
	if code, _ := get(t, base+"/data/nope.json"); code != 404 {
		t.Errorf("missing file: %d", code)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stopping on request is not an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preview kept running after its context was cancelled")
	}
}

func TestPreviewStopsWhenQIsTyped(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- preview(context.Background(), strings.NewReader("hello\nq\n"), io.Discard, []string{"-dir", testSite(t), "-addr", "127.0.0.1:0"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("typing q didn't stop it")
	}
}

func TestPreviewErrors(t *testing.T) {
	ln := listener(t)
	defer ln.Close()
	err := preview(context.Background(), strings.NewReader(""), io.Discard, []string{"-dir", testSite(t), "-addr", ln.Addr().String()})
	if err == nil || !strings.Contains(err.Error(), "another copy of streetworks") || !strings.Contains(err.Error(), "-addr") {
		t.Errorf("busy port: %v", err)
	}
	err = preview(context.Background(), strings.NewReader(""), io.Discard, []string{"-dir", filepath.Join(t.TempDir(), "nope")})
	if err == nil || !strings.Contains(err.Error(), "streetworks build") {
		t.Errorf("missing site: %v", err)
	}
	if err := preview(context.Background(), strings.NewReader(""), io.Discard, []string{"-nope"}); err == nil {
		t.Error("unknown flag accepted")
	}
}

func TestPreviewNoBuild(t *testing.T) {
	dir := testSite(t)
	os.Remove(filepath.Join(dir, "index.html"))
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuf{}
	done := make(chan error, 1)
	go func() {
		done <- preview(ctx, strings.NewReader(""), out, []string{"-dir", dir, "-addr", "127.0.0.1:0"})
	}()
	waitFor(t, "the preview to start", 5*time.Second, func() bool { return listenRE.MatchString(out.String()) })
	cancel()
	<-done
	if !strings.Contains(out.String(), "npm run build") {
		t.Errorf("output:\n%s", out.String())
	}
}

// --- the real program, as a separate process ---------------------------------

type proc struct {
	cmd  *exec.Cmd
	out  *lockedBuf
	done chan error
}

// startProgram runs this test binary as the program itself (see TestMain).
// With pipe set, the returned writer is its stdin.
func startProgram(t *testing.T, pipe bool, args ...string) (*proc, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "STREETWORKS_RUN_MAIN=1", "STREETWORKS_ARGS="+strings.Join(args, "\x1f"))
	out := &lockedBuf{}
	cmd.Stdout, cmd.Stderr = out, out
	var w io.WriteCloser
	if pipe {
		var err error
		if w, err = cmd.StdinPipe(); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &proc{cmd: cmd, out: out, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return p, w
}

func (p *proc) baseURL(t *testing.T) string {
	t.Helper()
	waitFor(t, "the program to start listening", 15*time.Second, func() bool { return listenRE.MatchString(p.out.String()) })
	return listenRE.FindStringSubmatch(p.out.String())[1]
}

func (p *proc) mustExitWithin(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatalf("exited with %v\n%s", err, p.out.String())
		}
	case <-time.After(d):
		t.Fatalf("the program was still running %v after being asked to stop\n%s", d, p.out.String())
	}
}
