package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/arcgistest"
)

var permitsServiceReal = permitsService

func TestMain(m *testing.M) {
	// Tests that need a real process re-run this test binary as the program itself
	if os.Getenv("STREETWORKS_RUN_MAIN") == "1" {
		os.Args = append([]string{"streetworks"}, strings.Split(os.Getenv("STREETWORKS_ARGS"), "\x1f")...)
		main()
		os.Exit(0)
	}
	permitsService = "http://127.0.0.1:1/none" // never contact the real city from tests
	newClient = func() *arcgis.Client {
		c := arcgis.New()
		c.Retries, c.Backoff = 1, time.Millisecond
		return c
	}
	os.Exit(m.Run())
}

func realRows(t *testing.T) []arcgis.Feature {
	t.Helper()
	raw, err := os.ReadFile("../../internal/permits/testdata/real_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	rows = append(rows, map[string]any{"OBJECTID": 99.0, "PERMIT": "T99EX00001", "PERMIT_TYPE": "EXCAVATION", "PERMIT_STATUS": "CLOSED",
		"DATE_ISSUED": 2514240000000.0, "DATE_ENTERED": 1158883200000.0, "STREET_NO": "500", "STREET_DIR": "N", "STREET_NAME": "STONE AV"})
	var out []arcgis.Feature
	for i, r := range rows {
		out = append(out, arcgistest.Feat(r, fmt.Sprintf(`{"type":"Point","coordinates":[%f,32.2]}`, -110.9-float64(i)/100)))
	}
	return out
}

func fakeCity(t *testing.T) *arcgistest.Server {
	t.Helper()
	srv := arcgistest.New(
		arcgistest.Layer{ID: 75, Name: "TDOT_PERMITS_RW_INACTIVE", Geometry: "esriGeometryPolygon", MaxRecordCount: 4,
			Fields:   []arcgis.Field{arcgistest.OIDField(), {Name: "DATE_ISSUED", Type: "esriFieldTypeDate"}, {Name: "STREET_NAME", Type: "esriFieldTypeString"}},
			Features: realRows(t)},
		arcgistest.Layer{ID: 13, Name: "Prop409", Geometry: "esriGeometryPolyline",
			Fields: []arcgis.Field{arcgistest.OIDField()},
			Features: []arcgis.Feature{
				arcgistest.Feat(map[string]any{"OBJECTID": 1.0, "NAME": "Speedway"}, `{"type":"LineString","coordinates":[[-110.9,32.2],[-110.8,32.2]]}`),
				arcgistest.Feat(map[string]any{"OBJECTID": 2.0, "NAME": "Grant"}, `{"type":"LineString","coordinates":[[-110.9,32.3],[-110.8,32.3]]}`)}},
	)
	t.Cleanup(srv.Close)
	return srv
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

func TestDefaultLayersAreWellFormed(t *testing.T) {
	permitsLayers := 0
	seen := map[string]bool{}
	for _, l := range defaultLayers {
		if !nameRE.MatchString(l.Name) || seen[l.Name] {
			t.Errorf("bad or duplicate layer name %q", l.Name)
		}
		seen[l.Name] = true
		u, err := url.Parse(l.URL)
		if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Host, "tucsonaz.gov") || !strings.Contains(u.Path, "/MapServer/") {
			t.Errorf("layer %s has a suspicious URL %q", l.Name, l.URL)
		}
		if strings.HasPrefix(l.Name, "permits") {
			permitsLayers++
		}
	}
	if permitsLayers < 1 {
		t.Error("no layer will be parsed as permits")
	}
}

func TestLayersFrom(t *testing.T) {
	names := func(ls []layer) string {
		var n []string
		for _, l := range ls {
			n = append(n, l.Name)
		}
		return strings.Join(n, ",")
	}
	const allDefaults = "permits_inactive,prop409_major,prop409_residential,permits_row_active,permits_row_utility,permits_row_general,permits_row_cip_joc,permits_row_development,permits_row_homedev,permits_row_residential"
	ls, err := layersFrom(nil, "")
	if err != nil || names(ls) != allDefaults {
		t.Fatalf("defaults: %s %v", names(ls), err)
	}
	ls, err = layersFrom(multi{"permits_active=https://h/MapServer/7"}, "")
	if err != nil || names(ls) != allDefaults+",permits_active" || ls[len(ls)-1].URL != "https://h/MapServer/7" {
		t.Errorf("extra added: %s %v", names(ls), err)
	}
	ls, err = layersFrom(multi{"prop409_major=http://localhost:9/x"}, "")
	if err != nil || len(ls) != len(defaultLayers) || ls[1].URL != "http://localhost:9/x" {
		t.Errorf("extra with an existing name must replace its URL, not duplicate: %+v %v", ls, err)
	}
	ls, err = layersFrom(nil, "prop409_major, permits_inactive")
	if err != nil || names(ls) != "prop409_major,permits_inactive" {
		t.Errorf("only (order and spaces): %s %v", names(ls), err)
	}
	ls, err = layersFrom(multi{"t=http://h/1"}, "t")
	if err != nil || len(ls) != 1 || ls[0].Name != "t" {
		t.Errorf("only+extra: %+v %v", ls, err)
	}
	for _, c := range []struct {
		extra multi
		only  string
		want  string
	}{
		{multi{"novalue"}, "", "name=url"},
		{multi{"=http://h"}, "", "name=url"},
		{multi{"name="}, "", "name=url"},
		{multi{"Bad-Name=http://h"}, "", "lowercase"},
		{multi{"has space=http://h"}, "", "lowercase"},
		{multi{"../etc=http://h"}, "", "lowercase"},
		{multi{"ok=ftp://h"}, "", "http"},
		{multi{"ok=javascript:alert(1)"}, "", "http"},
		{nil, "nope", `unknown layer "nope"`},
		{nil, "permits_inactive,nope", `unknown layer "nope"`},
		{nil, ",", "unknown layer"},
	} {
		if _, err := layersFrom(c.extra, c.only); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("extra=%v only=%q: err = %v, want %q", c.extra, c.only, err, c.want)
		}
	}
}

func TestPullWritesLayerAndMetadata(t *testing.T) {
	srv, dir := fakeCity(t), t.TempDir()
	l := layer{"overlay_t", srv.LayerURL(13)}
	n, err := pull(context.Background(), newClient(), l, dir, 2000)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got := countLines(t, filepath.Join(dir, "overlay_t.jsonl")); got != 2 {
		t.Errorf("%d lines", got)
	}
	var m arcgis.SourceMeta
	b, err := os.ReadFile(filepath.Join(dir, "overlay_t.meta.json"))
	if err != nil || json.Unmarshal(b, &m) != nil || m.Name != "overlay_t" || m.Count != 2 || m.URL != l.URL || time.Since(m.FetchedAt) > time.Minute {
		t.Errorf("meta = %+v err=%v", m, err)
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmp) != 0 {
		t.Errorf("temp files left behind: %v", tmp)
	}
}
