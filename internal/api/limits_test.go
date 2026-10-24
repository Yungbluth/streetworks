package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/store"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func limited(f *fakeStore, c *clock) http.Handler {
	return New(f, built, Options{Now: c.now})
}

func TestBodyCap(t *testing.T) {
	body := `{"ids":[` + strings.Repeat("1,", 35<<10) + `1]}`
	rec := do(t, newHandler(&fakeStore{}), "POST", "/v1/permits/lookup", body)
	if rec.Code != 400 {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if got := decode(t, rec)["message"]; got != "That request is too large." {
		t.Errorf("message = %q", got)
	}
}

type bigStore struct{ fakeStore }

func (bigStore) Lookup(context.Context, []int) (map[int]store.Detail, error) {
	return map[int]store.Detail{1: {Permit: strings.Repeat("x", 3<<20)}}, nil
}

func TestMaxResponse(t *testing.T) {
	h := New(&bigStore{}, built, Options{})
	rec := do(t, h, "POST", "/v1/permits/lookup", `{"ids":[1]}`)
	if rec.Code != 400 {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	want := "That's too many permits for one request; ask for fewer."
	if got := decode(t, rec)["message"]; got != want {
		t.Errorf("message = %q", got)
	}
}

func TestGzip(t *testing.T) {
	h := newHandler(&fakeStore{})

	rec := do(t, h, "GET", "/v1/meta", "", "Accept-Encoding", "br, gzip")
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("response is not gzipped")
	}
	if got := strings.Join(rec.Header().Values("Vary"), ", "); got != "Origin, Accept-Encoding" {
		t.Errorf("Vary = %q", got)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.NewDecoder(zr).Decode(&m); err != nil {
		t.Fatalf("gzipped body isn't JSON: %v", err)
	}

	plain := do(t, h, "GET", "/v1/meta", "")
	if plain.Header().Get("Content-Encoding") != "" {
		t.Error("response was gzipped without being asked")
	}
	if decode(t, plain)["version"] != m["version"] {
		t.Error("plain and gzipped bodies differ")
	}
}

func TestLogLine(t *testing.T) {
	var buf bytes.Buffer
	h := New(&fakeStore{}, built, Options{Log: slog.New(slog.NewJSONHandler(&buf, nil))})
	do(t, h, "GET", "/v1/search?q=secretword", "")
	do(t, h, "POST", "/v1/permits/lookup", `{"ids":[1]}`)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d log lines, want 2:\n%s", len(lines), buf.String())
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e["route"] != "/v1/search" || e["status"] != float64(200) || e["bytes"] == nil || e["ms"] == nil {
		t.Errorf("log entry = %v", e)
	}
	if strings.Contains(buf.String(), "secretword") {
		t.Error("the query string was logged")
	}
}
