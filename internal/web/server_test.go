package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/permits"
)

func TestAPI(t *testing.T) {
	dir := t.TempDir()
	line := `{"layer":"x","properties":{"A":1},"geometry":{"type":"Point","coordinates":[-110.9,32.2]}}` + "\n"
	for _, n := range []string{"overlay_x.jsonl", "permits_x.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	ix := permits.Build([]permits.Record{
		{Permit: "A1", Category: "Excavation", Status: "Final", Street: "E X ST", Block: 100, HasGeo: true, Lat: 32.2, Lng: -110.9, When: when, Date: "2023-06-01"},
		{Permit: "A2", Category: "Barricade", Status: "Void", Street: "E X ST", Block: 100, HasGeo: true, Lat: 32.2, Lng: -110.9, When: when, Date: "2023-06-01"},
	})
	srv := httptest.NewServer((&Server{Index: ix, Dir: dir}).Handler())
	defer srv.Close()

	get := func(path string, v any) int {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if v != nil && resp.StatusCode == 200 {
			if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		}
		return resp.StatusCode
	}

	var bs []permits.BlockSummary
	get("/api/blocks", &bs)
	if len(bs) != 1 || bs[0].Count != 1 { // void hidden by default
		t.Fatalf("blocks = %+v", bs)
	}
	get("/api/blocks?nowork=1", &bs)
	if bs[0].Count != 2 {
		t.Errorf("nowork=1 blocks = %+v", bs)
	}
	var d permits.BlockDetail
	if c := get("/api/block?key="+url.QueryEscape("E X ST|100")+"&nowork=1", &d); c != 200 || d.Count != 2 {
		t.Errorf("detail status=%d %+v", c, d)
	}
	if c := get("/api/block?key=nope", nil); c != 404 {
		t.Errorf("unknown block status = %d", c)
	}
	var m struct {
		UniquePermits int      `json:"unique_permits"`
		Overlays      []string `json:"overlays"`
	}
	get("/api/meta", &m)
	if m.UniquePermits != 2 || len(m.Overlays) != 1 || m.Overlays[0] != "overlay_x" {
		t.Errorf("meta = %+v", m)
	}
	var fc struct {
		Features []json.RawMessage `json:"features"`
	}
	if c := get("/api/overlay/overlay_x", &fc); c != 200 || len(fc.Features) != 1 {
		t.Errorf("overlay status=%d features=%d", c, len(fc.Features))
	}
	for _, p := range []string{"/api/overlay/permits_x", "/api/overlay/..%2F..%2Fetc%2Fpasswd", "/api/overlay/UPPER"} {
		if c := get(p, nil); c != 404 {
			t.Errorf("%s status = %d, want 404", p, c)
		}
	}
}
