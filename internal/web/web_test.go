package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

func TestMethodsAndUnknownRoutes(t *testing.T) {
	srv, _ := newTestServer(t, nil, nil)
	for _, p := range []string{"/api/meta", "/api/blocks", "/api/block?key=x", "/api/overlay/x"} {
		if resp, _ := fetch(t, srv, "POST", p); resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", p, resp.StatusCode)
		}
	}
	for _, p := range []string{"/api/nope", "/nope.html", "/../etc/passwd", "/static/../../go.mod"} {
		if resp, _ := fetch(t, srv, "GET", p); resp.StatusCode == http.StatusOK {
			t.Errorf("GET %s = 200", p)
		}
	}
}

func TestOverlayStreamsValidGeoJSON(t *testing.T) {
	var lines []string
	for i := 0; i < 3000; i++ {
		geom := `{"type":"LineString","coordinates":[[-110.9,32.2],[-110.8,32.3]]}`
		if i%100 == 0 {
			geom = `null`
		}
		lines = append(lines, fmt.Sprintf(`{"layer":"o","properties":{"OBJECTID":%d,"NAME":"S%d"},"geometry":%s}`, i, i, geom))
	}
	srv, _ := newTestServer(t, map[string]string{"overlay_x.jsonl": strings.Join(lines, "\n") + "\n"}, nil)
	resp, body := fetch(t, srv, "GET", "/api/overlay/overlay_x")
	var fc struct {
		Type     string `json:"type"`
		Features []struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal([]byte(body), &fc); err != nil || fc.Type != "FeatureCollection" || len(fc.Features) != 3000 || fc.Features[0].Type != "Feature" {
		t.Fatalf("err=%v n=%d", err, len(fc.Features))
	}
	if resp.Header.Get("Content-Type") != "application/geo+json" {
		t.Errorf("content type %q", resp.Header.Get("Content-Type"))
	}
}

func TestOverlayEdgeCases(t *testing.T) {
	srv, _ := newTestServer(t, map[string]string{
		"overlay_empty.jsonl":   "",
		"overlay_corrupt.jsonl": `{"layer":"o","properties":{"A":1},"geometry":null}` + "\n" + `{"layer":"o","prop`,
		"permits_x.jsonl":       `{"layer":"x","properties":{},"geometry":null}` + "\n",
	}, nil)
	if _, body := fetch(t, srv, "GET", "/api/overlay/overlay_empty"); !json.Valid([]byte(body)) || !strings.Contains(body, `"features":[]`) {
		t.Errorf("empty overlay: %s", body)
	}
	if _, body := fetch(t, srv, "GET", "/api/overlay/overlay_corrupt"); !json.Valid([]byte(body)) || !strings.Contains(body, `"A":1`) {
		t.Errorf("corrupt overlay: %s", body)
	}
	for _, name := range []string{"missing", "permits_x", "..%2F..%2Fetc%2Fpasswd", "UPPER", "a.b", "with space", "overlay_empty.jsonl"} {
		if resp, _ := fetch(t, srv, "GET", "/api/overlay/"+name); resp.StatusCode != 404 {
			t.Errorf("overlay %q = %d, want 404", name, resp.StatusCode)
		}
	}
}

func TestBlockEndpointValidation(t *testing.T) {
	srv, _ := newTestServer(t, nil, []permits.Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020)})
	for _, c := range []struct {
		q    string
		code int
	}{{"key=E+A+ST%7C100", 200}, {"key=E%20A%20ST%7C100", 200}, {"key=", 404}, {"", 404}, {"key=E+A+ST%7C999", 404}, {"key=%00", 404}, {"key=" + strings.Repeat("A", 100000), 404}} {
		if resp, _ := fetch(t, srv, "GET", "/api/block?"+c.q); resp.StatusCode != c.code {
			t.Errorf("%.40s = %d, want %d", c.q, resp.StatusCode, c.code)
		}
	}
}

func TestConcurrentRequests(t *testing.T) { // run with -race
	srv, _ := newTestServer(t, nil, []permits.Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020), rec("2", "Barricade", "CLOSED", "E A ST", 100, 2021)})
	done := make(chan error, 40)
	for i := 0; i < 40; i++ {
		go func(i int) {
			_, body := fetch(t, srv, "GET", fmt.Sprintf("/api/blocks?from=%d", 2019+i%3))
			if !json.Valid([]byte(body)) {
				done <- fmt.Errorf("invalid json: %s", body)
				return
			}
			done <- nil
		}(i)
	}
	for i := 0; i < 40; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// --- UI checks: there is no browser in `go test`

func readStatic(t *testing.T, name string) string {
	t.Helper()
	b, err := static.ReadFile("static/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIndexIsServed(t *testing.T) {
	srv, _ := newTestServer(t, nil, nil)
	resp, body := fetch(t, srv, "GET", "/")
	if resp.StatusCode != 200 || !strings.Contains(body, "Streetworks") || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestEveryLocalAssetIsServed(t *testing.T) {
	srv, _ := newTestServer(t, nil, nil)
	html := readStatic(t, "index.html")
	refs := regexp.MustCompile(`(?:src|href)="([^"#]+)"`).FindAllStringSubmatch(html, -1)
	if len(refs) < 3 {
		t.Fatalf("found only %d asset references", len(refs))
	}
	for _, m := range refs {
		if strings.HasPrefix(m[1], "http") {
			continue
		}
		resp, body := fetch(t, srv, "GET", "/"+m[1])
		if resp.StatusCode != 200 || len(body) < 100 {
			t.Errorf("/%s: status %d, %d bytes", m[1], resp.StatusCode, len(body))
		}
	}
	if !strings.Contains(readStatic(t, "vendor/leaflet/leaflet.js"), "Leaflet 1.9") {
		t.Error("vendored Leaflet missing or unexpected version")
	}
	if len(readStatic(t, "vendor/leaflet/LICENSE")) < 100 {
		t.Error("Leaflet's license must ship with it")
	}
}

func TestUIReferencesOnlyElementsThatExist(t *testing.T) {
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(readStatic(t, "index.html"), -1) {
		ids[m[1]] = true
	}
	refs := regexp.MustCompile(`\$\("#([A-Za-z0-9_-]+)"\)`).FindAllStringSubmatch(readStatic(t, "app.js"), -1)
	if len(refs) < 10 {
		t.Fatalf("only found %d id references; pattern broken?", len(refs))
	}
	for _, m := range refs {
		if !ids[m[1]] {
			t.Errorf(`app.js uses $("#%s") but index.html has no id="%s"`, m[1], m[1])
		}
	}
}

func TestUIUsesOnlyRoutesAndFieldsTheAPIProvides(t *testing.T) {
	js := readStatic(t, "app.js")
	srv, _ := newTestServer(t, map[string]string{"overlay_x.jsonl": ""}, []permits.Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020)})
	for _, m := range regexp.MustCompile(`["'`+"`"+`](/api/[a-z]+)`).FindAllStringSubmatch(js, -1) {
		path := m[1]
		switch path {
		case "/api/overlay":
			path += "/overlay_x"
		case "/api/block":
			path += "?key=E+A+ST%7C100"
		case "/api/nearby":
			path += "?lat=32.2&lng=-110.9&r=100"
		case "/api/street":
			path += "?name=E+A+ST"
		}
		if resp, _ := fetch(t, srv, "GET", path); resp.StatusCode != 200 {
			t.Errorf("app.js calls %s but the server answers %d", path, resp.StatusCode)
		}
	}
	// Fields read from /api/meta, /api/blocks and /api/block responses.
	_, meta := fetch(t, srv, "GET", "/api/meta")
	for _, f := range []string{"unique_permits", "date_min", "date_max", "categories", "overlays", "sources", "no_block", "no_date", "bad_dates", "no_work", "attribution", "bounds"} {
		if !strings.Contains(js, "."+f) {
			t.Errorf("app.js no longer reads meta.%s; update this list if intentional", f)
		}
		if !strings.Contains(meta, `"`+f+`"`) {
			t.Errorf("app.js reads meta.%s but the API does not send it", f)
		}
	}
	_, blocks := fetch(t, srv, "GET", "/api/blocks")
	_, block := fetch(t, srv, "GET", "/api/block?key=E+A+ST%7C100")
	for _, f := range []string{"key", "street", "block", "count", "lat", "lng"} {
		if !strings.Contains(blocks, `"`+f+`"`) {
			t.Errorf("/api/blocks does not send %q", f)
		}
	}
	for _, f := range []string{"records", "date", "category", "status", "permit", "raw_street", "street"} {
		if !strings.Contains(block, `"`+f+`"`) {
			t.Errorf("/api/block does not send %q", f)
		}
	}
	for _, f := range []string{"description", "sub_type", "pro_url", "date_basis", "date_flag", "street_source", "no_work"} {
		if !strings.Contains(js, "r."+f) {
			t.Errorf("app.js no longer shows record field %q", f)
		}
	}
}
