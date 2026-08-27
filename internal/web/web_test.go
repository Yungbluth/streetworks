package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/permits"
)

func newTestServer(t *testing.T, files map[string]string, recs []permits.Record) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer((&Server{Index: permits.Build(recs), Dir: dir}).Handler())
	t.Cleanup(srv.Close)
	return srv, dir
}

func rec(permit, cat, status, street string, block, year int) permits.Record {
	w := time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC)
	return permits.Record{Permit: permit, Category: cat, Status: status, Street: street, Block: block,
		HasGeo: true, Lat: 32.2, Lng: -110.9, When: w, Date: w.Format("2006-01-02")}
}

func fetch(t *testing.T, srv *httptest.Server, method, path string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestParseFilter(t *testing.T) {
	cases := []struct {
		query string
		want  permits.Filter
		cats  []string
	}{
		{"", permits.Filter{}, nil},
		{"from=2019&to=2021", permits.Filter{From: 2019, To: 2021}, nil},
		{"from=abc&to=", permits.Filter{}, nil},
		{"from=2019.5", permits.Filter{}, nil},
		{"nowork=1", permits.Filter{NoWork: true}, nil},
		{"nowork=0", permits.Filter{}, nil},
		{"nowork=true", permits.Filter{}, nil},
		{"void=1", permits.Filter{}, nil},       // the old parameter name is ignored
		{"cats=", permits.Filter{}, []string{}}, // present but empty: no categories
		{"cats=Excavation,Combo", permits.Filter{}, []string{"Combo", "Excavation"}},
		{"cats=Civic%20event", permits.Filter{}, []string{"Civic event"}},
		{"from=2020&to=2022&cats=Barricade&nowork=1", permits.Filter{From: 2020, To: 2022, NoWork: true}, []string{"Barricade"}},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/api/blocks?"+c.query, nil)
		got := parseFilter(r)
		var cats []string
		for k := range got.Cats {
			cats = append(cats, k)
		}
		if len(cats) == 2 && cats[0] > cats[1] {
			cats[0], cats[1] = cats[1], cats[0]
		}
		if (got.Cats == nil) != (c.cats == nil) {
			t.Errorf("%q: nil-ness of Cats = %v, want %v (nil means every category)", c.query, got.Cats == nil, c.cats == nil)
		}
		if got.From != c.want.From || got.To != c.want.To || got.NoWork != c.want.NoWork || strings.Join(cats, ",") != strings.Join(c.cats, ",") {
			t.Errorf("%q: got %+v cats=%v; want %+v cats=%v", c.query, got, cats, c.want, c.cats)
		}
	}
}

func TestBlocksEndpoint(t *testing.T) {
	var recs []permits.Record
	for i := 0; i < 12; i++ { // 12 blocks with 1..12 permits each
		for j := 0; j <= i; j++ {
			recs = append(recs, rec(fmt.Sprintf("P%d-%d", i, j), "Excavation", "CLOSED", "E A ST", (i+1)*100, 2020))
		}
	}
	srv, _ := newTestServer(t, nil, recs)

	resp, body := fetch(t, srv, "GET", "/api/blocks")
	var all []permits.BlockSummary
	if err := json.Unmarshal([]byte(body), &all); err != nil || len(all) != 12 {
		t.Fatalf("err=%v n=%d", err, len(all))
	}
	if resp.Header.Get("Content-Type") != "application/json" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Total-Blocks") != "12" {
		t.Errorf("headers: %v", resp.Header)
	}
	if all[0].Count != 12 || all[11].Count != 1 {
		t.Errorf("not sorted busiest first: %d..%d", all[0].Count, all[11].Count)
	}

	for _, c := range []struct {
		q    string
		want int
	}{{"limit=5", 5}, {"limit=12", 12}, {"limit=100", 12}, {"limit=0", 12}, {"limit=-3", 12}, {"limit=abc", 12}} {
		resp, body := fetch(t, srv, "GET", "/api/blocks?"+c.q)
		var got []permits.BlockSummary
		_ = json.Unmarshal([]byte(body), &got)
		if len(got) != c.want || resp.Header.Get("X-Total-Blocks") != "12" {
			t.Errorf("%s: got %d (total header %q), want %d of 12", c.q, len(got), resp.Header.Get("X-Total-Blocks"), c.want)
		}
	}
	// The limit keeps the busiest blocks, not an arbitrary subset
	_, body = fetch(t, srv, "GET", "/api/blocks?limit=3")
	var top3 []permits.BlockSummary
	_ = json.Unmarshal([]byte(body), &top3)
	if top3[0].Count != 12 || top3[2].Count != 10 {
		t.Errorf("top3 = %+v", top3)
	}
}

func TestEmptyResultsAreArraysNotNull(t *testing.T) {
	srv, _ := newTestServer(t, nil, []permits.Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020)})
	_, body := fetch(t, srv, "GET", "/api/blocks?from=2100")
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("empty blocks = %q", body)
	}
	_, body = fetch(t, srv, "GET", "/api/block?key="+"E+A+ST%7C100"+"&from=2100")
	if !strings.Contains(body, `"records":[]`) {
		t.Errorf("empty records = %q", body)
	}
}

func TestMetaNeverContainsNullCollections(t *testing.T) {
	// No meta files, no overlays, no data: the UI calls .map on these
	srv, _ := newTestServer(t, nil, nil)
	_, body := fetch(t, srv, "GET", "/api/meta")
	if strings.Contains(body, "null") {
		t.Errorf("meta has null: %s", body)
	}
	var m struct {
		Sources  []any    `json:"sources"`
		Overlays []string `json:"overlays"`
		Attrib   string   `json:"attribution"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil || m.Sources == nil || m.Overlays == nil || !strings.Contains(m.Attrib, "Tucson") {
		t.Errorf("%v %+v", err, m)
	}
}

func TestMetaListsSourcesAndOverlaysAndSkipsJunk(t *testing.T) {
	srv, _ := newTestServer(t, map[string]string{
		"permits_inactive.jsonl":     "",
		"permits_inactive.meta.json": `{"name":"permits_inactive","url":"https://x/75","fetched_at":"2026-08-21T10:00:00Z","count":92323}`,
		"prop409_major.jsonl":        "",
		"prop409_major.meta.json":    `{"name":"prop409_major","url":"https://x/13","fetched_at":"2026-08-21T10:01:00Z","count":300}`,
		"broken.meta.json":           `{not json`,
		"Bad-Name.jsonl":             "",
		"notes.txt":                  "x",
	}, nil)
	_, body := fetch(t, srv, "GET", "/api/meta")
	var m struct {
		Sources []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"sources"`
		Overlays []string `json:"overlays"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 2 {
		t.Errorf("sources = %+v (broken meta must be skipped)", m.Sources)
	}
	if len(m.Overlays) != 1 || m.Overlays[0] != "prop409_major" {
		t.Errorf("overlays = %v (permit layers and badly named files must be excluded)", m.Overlays)
	}
}
