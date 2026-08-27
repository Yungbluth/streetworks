package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

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

var listenRE = regexp.MustCompile(`listening on (http://[0-9.:\[\]a-f]+)`)

func writeTestData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	line := `{"layer":"permits_t","properties":{"PERMIT":"T1","PERMIT_TYPE":"EXCAVATION","PERMIT_STATUS":"CLOSED","STREET_NO":"100","STREET_DIR":"E","STREET_NAME":"MAIN ST","DATE_ISSUED":1158883200000},"geometry":{"type":"Point","coordinates":[-110.9,32.2]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "permits_t.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
