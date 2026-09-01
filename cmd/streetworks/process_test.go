package main

import (
	"bytes"
	"context"

	"io"

	"os"
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
