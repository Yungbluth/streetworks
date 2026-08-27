package web

import (
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
