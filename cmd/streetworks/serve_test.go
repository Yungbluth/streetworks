package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/store"
)

// testDB writes a database of three permits and returns its path
func testDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "streetworks.db")
	ps := []store.Permit{
		{ID: 0, Place: 0, Year: 2023, Detail: store.Detail{Permit: "T23SY041246", Type: "Street Cut", Status: "Issued", Description: "Curb ramp", Filed: "100 Main St"}},
		{ID: 1, Place: 0, Year: 2006, Detail: store.Detail{Permit: "T06EX000123", Type: "Excavation", Status: "Final", Description: "Water tap", Filed: "200 Oak Ave"}},
		{ID: 2, Place: 1, Detail: store.Detail{Permit: "T19CO000777", Type: "Conduit", Status: "Issued", Description: "Fiber bore", Filed: "300 Pine Rd"}},
	}
	if err := store.Write(path, store.Meta{Built: "2026-10-23T13:17:00Z", Version: 2, Permits: len(ps)}, ps); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServeAnswers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &lockedBuf{}
	done := make(chan error, 1)
	go func() { done <- serveCmd(ctx, out, []string{"-db", testDB(t), "-addr", "127.0.0.1:0"}) }()
	waitFor(t, "the API to start", 5*time.Second, func() bool { return listenRE.MatchString(out.String()) })
	base := listenRE.FindStringSubmatch(out.String())[1]

	if code, body := get(t, base+"/health"); code != 200 || body != "ok" {
		t.Errorf("/health: %d %q", code, body)
	}
	if code, body := get(t, base+"/v1/meta"); code != 200 || !strings.Contains(body, `"permits":3`) {
		t.Errorf("/v1/meta: %d %q", code, body)
	}
	// Request logs go to stdout with Cloud Run's field names
	if log := out.String(); !strings.Contains(log, `"severity":"INFO"`) || !strings.Contains(log, `"message":"request"`) {
		t.Errorf("no Cloud Run log line in the output:\n%s", log)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stopping on request is not an error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("serve kept running after its context was cancelled")
	}
}

func TestServeNeedsADatabase(t *testing.T) {
	err := serveCmd(context.Background(), &lockedBuf{}, []string{"-db", filepath.Join(t.TempDir(), "nope.db"), "-addr", "127.0.0.1:0"})
	if err == nil || !strings.Contains(err.Error(), "run streetworks build first") {
		t.Errorf("err = %v", err)
	}
}
