package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/arcgistest"
)

func testClient() *arcgis.Client {
	c := arcgis.New()
	c.Retries, c.Backoff = 1, time.Millisecond
	return c
}

func overlayServer(t *testing.T) *arcgistest.Server {
	t.Helper()
	srv := arcgistest.New(arcgistest.Layer{ID: 13, Name: "Prop409", Geometry: "esriGeometryPolyline",
		Fields: []arcgis.Field{arcgistest.OIDField()},
		Features: []arcgis.Feature{
			arcgistest.Feat(map[string]any{"OBJECTID": 1.0, "NAME": "Speedway"}, `{"type":"LineString","coordinates":[[-110.9,32.2],[-110.8,32.2]]}`),
			arcgistest.Feat(map[string]any{"OBJECTID": 2.0, "NAME": "Grant"}, `{"type":"LineString","coordinates":[[-110.9,32.3],[-110.8,32.3]]}`),
		}})
	t.Cleanup(srv.Close)
	return srv
}

func flaky(t *testing.T) string {
	t.Helper()
	var pages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !strings.HasSuffix(r.URL.Path, "/query"):
			fmt.Fprint(w, `{"name":"x","maxRecordCount":2,"fields":[{"name":"OBJECTID","type":"esriFieldTypeOID"}]}`)
		case r.URL.Query().Get("returnCountOnly") == "true":
			fmt.Fprint(w, `{"count":10}`)
		case pages.Add(1) > 1:
			w.WriteHeader(500)
		default:
			fmt.Fprint(w, `{"features":[{"properties":{"OBJECTID":1},"geometry":null},{"properties":{"OBJECTID":2},"geometry":null}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/L/0"
}

func deadURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	return u + "/arcgis/rest/services/Test/MapServer/1"
}

func lines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

func names(ls []Layer) string {
	var n []string
	for _, l := range ls {
		n = append(n, l.Name)
	}
	return strings.Join(n, ",")
}

func TestLayerOrder(t *testing.T) {
	for _, l := range Defaults() {
		switch {
		case l.Name == "permits_inactive" && l.Priority != 1,
			strings.HasPrefix(l.Name, "permits_row_") && l.Priority != 2,
			!l.IsPermits() && l.Priority != 0:
			t.Errorf("%s has priority %d", l.Name, l.Priority)
		}
	}
}

func TestSelect(t *testing.T) {
	all := names(Defaults())
	ls, err := Select(nil, "")
	if err != nil || names(ls) != all {
		t.Fatalf("defaults: %s %v", names(ls), err)
	}

	ls, err = Select([]string{"permits_new=https://h/MapServer/7"}, "")
	last := ls[len(ls)-1]
	if err != nil || names(ls) != all+",permits_new" || last.URL != "https://h/MapServer/7" || last.Priority != 2 {
		t.Errorf("new permit layer: %+v %v", last, err)
	}

	ls, _ = Select([]string{"prop409_major=http://localhost:9/x"}, "prop409_major")
	if len(ls) != 1 || ls[0].URL != "http://localhost:9/x" || ls[0].Fallbacks != nil {
		t.Errorf("an override replaces the URL and drops the fallbacks: %+v", ls)
	}

	ls, err = Select(nil, "prop409_major, permits_inactive")
	if err != nil || names(ls) != "prop409_major,permits_inactive" {
		t.Errorf("only keeps its own order: %s %v", names(ls), err)
	}
}

func TestSelectBad(t *testing.T) {
	for _, c := range []struct {
		extra []string
		only  string
		want  string
	}{
		{[]string{"novalue"}, "", "name=url"},
		{[]string{"=http://h"}, "", "name=url"},
		{[]string{"name="}, "", "name=url"},
		{[]string{"Bad-Name=http://h"}, "", "lowercase"},
		{[]string{"../etc=http://h"}, "", "lowercase"},
		{[]string{"ok=ftp://h"}, "", "http"},
		{[]string{"ok=javascript:alert(1)"}, "", "http"},
		{nil, "nope", `unknown layer "nope"`},
		{nil, "permits_inactive,nope", `unknown layer "nope"`},
		{nil, ",", "unknown layer"},
	} {
		if _, err := Select(c.extra, c.only); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("extra=%v only=%q: err = %v, want %q", c.extra, c.only, err, c.want)
		}
	}
}

func TestSave(t *testing.T) {
	srv, dir := overlayServer(t), t.TempDir()
	n, err := save(context.Background(), testClient(), "overlay_t", srv.LayerURL(13), dir, 2000)
	if err != nil || n != 2 || lines(t, filepath.Join(dir, "overlay_t.jsonl")) != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var m arcgis.SourceMeta
	b, _ := os.ReadFile(filepath.Join(dir, "overlay_t.meta.json"))
	if json.Unmarshal(b, &m) != nil || m.Name != "overlay_t" || m.Count != 2 || m.URL != srv.LayerURL(13) || time.Since(m.FetchedAt) > time.Minute {
		t.Errorf("meta = %+v", m)
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmp) != 0 {
		t.Errorf("left behind: %v", tmp)
	}
}

func TestDownloadFallback(t *testing.T) {
	good, dir := overlayServer(t), t.TempDir()
	var out bytes.Buffer
	l := Layer{Name: "overlay_t", URL: deadURL(t), Fallbacks: []string{good.LayerURL(13)}}
	n, used, err := Download(context.Background(), &out, testClient(), l, dir, 2000)
	if err != nil || n != 2 || used != good.LayerURL(13) || !strings.Contains(out.String(), "could not reach") {
		t.Fatalf("n=%d used=%s err=%v\n%s", n, used, err, out.String())
	}
	var m arcgis.SourceMeta
	b, _ := os.ReadFile(filepath.Join(dir, "overlay_t.meta.json"))
	if json.Unmarshal(b, &m) != nil || m.URL != good.LayerURL(13) {
		t.Errorf("metadata should name the URL that supplied the data, got %q", m.URL)
	}
}

func TestDownloadUnreachable(t *testing.T) {
	dir := t.TempDir()
	l := Layer{Name: "overlay_t", URL: deadURL(t), Fallbacks: []string{deadURL(t)}}
	_, used, err := Download(context.Background(), &bytes.Buffer{}, testClient(), l, dir, 2000)
	if !arcgis.IsUnreachable(err) || used == "" {
		t.Fatalf("used=%q err=%v", used, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("nothing should be written: %v", entries)
	}
}

func TestRunFails(t *testing.T) {
	good, dir := overlayServer(t), t.TempDir()
	dead := deadURL(t)
	var out bytes.Buffer
	failed := Run(context.Background(), &out, testClient(), []Layer{
		{Name: "overlay_t", URL: good.LayerURL(13)},
		{Name: "overlay_dead", URL: dead},
		{Name: "overlay_404", URL: good.LayerURL(404)},
	}, dir, 2000)
	if failed != 2 || lines(t, filepath.Join(dir, "overlay_t.jsonl")) != 2 {
		t.Fatalf("failed=%d\n%s", failed, out.String())
	}
	for _, want := range []string{"2 rows saved", "FAILED", "network problem, not a data problem", hostOf(dead), "VPN"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should mention %q:\n%s", want, out.String())
		}
	}
	for _, name := range []string{"overlay_dead", "overlay_404"} {
		if _, err := os.Stat(filepath.Join(dir, name+".jsonl")); err == nil {
			t.Errorf("%s left a file", name)
		}
	}
}

func TestRunWarns(t *testing.T) {
	short := arcgistest.New(arcgistest.Layer{ID: 1, Name: "L", Fields: []arcgis.Field{arcgistest.OIDField()}, ReportedCount: 50,
		Features: []arcgis.Feature{arcgistest.Feat(map[string]any{"OBJECTID": 1.0}, ""), arcgistest.Feat(map[string]any{"OBJECTID": 2.0}, "")}})
	empty := arcgistest.New(arcgistest.Layer{ID: 1, Name: "E", Fields: []arcgis.Field{arcgistest.OIDField()}})
	defer short.Close()
	defer empty.Close()
	var out bytes.Buffer
	Run(context.Background(), &out, testClient(), []Layer{{Name: "short", URL: short.LayerURL(1)}, {Name: "empty", URL: empty.LayerURL(1)}}, t.TempDir(), 2000)
	for _, want := range []string{"saved 2 rows but the server reports 50", "returned no rows"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	Run(context.Background(), &out, testClient(), []Layer{{Name: "fine", URL: overlayServer(t).LayerURL(13)}}, t.TempDir(), 2000)
	if strings.Contains(out.String(), "WARNING") {
		t.Errorf("unexpected warning:\n%s", out.String())
	}
}
