package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func flaky(t *testing.T) string {
	t.Helper()
	var queries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/query") {
			fmt.Fprint(w, `{"name":"x","maxRecordCount":2,"fields":[{"name":"OBJECTID","type":"esriFieldTypeOID"}]}`)
			return
		}
		if r.URL.Query().Get("returnCountOnly") == "true" {
			fmt.Fprint(w, `{"count":10}`)
			return
		}
		if queries.Add(1) > 1 {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, `{"features":[{"properties":{"OBJECTID":1},"geometry":null},{"properties":{"OBJECTID":2},"geometry":null}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/L/0"
}

func TestFailedPullLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	n, err := pull(context.Background(), newClient(), layer{"overlay_t", flaky(t)}, dir, 2)
	if err == nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("a failed download left files behind: %v", entries)
	}
}

func TestFailedPullDoesNotDestroyPreviousGoodData(t *testing.T) {
	srv, dir := fakeCity(t), t.TempDir()
	good := layer{"overlay_t", srv.LayerURL(13)}
	if _, err := pull(context.Background(), newClient(), good, dir, 2000); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "overlay_t.jsonl"))
	metaBefore, _ := os.ReadFile(filepath.Join(dir, "overlay_t.meta.json"))
	if _, err := pull(context.Background(), newClient(), layer{"overlay_t", flaky(t)}, dir, 2); err == nil {
		t.Fatal("expected failure")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "overlay_t.jsonl"))
	metaAfter, _ := os.ReadFile(filepath.Join(dir, "overlay_t.meta.json"))
	if string(before) != string(after) || string(metaBefore) != string(metaAfter) {
		t.Error("previous data or its metadata was modified by a failed refresh")
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmp) != 0 {
		t.Errorf("temp files left: %v", tmp)
	}
}

func TestPullIntoMissingDirectoryFails(t *testing.T) {
	srv := fakeCity(t)
	if _, err := pull(context.Background(), newClient(), layer{"overlay_t", srv.LayerURL(13)}, filepath.Join(t.TempDir(), "nope"), 10); err == nil {
		t.Error("expected an error when the output directory does not exist")
	}
}

func TestIngestEndToEnd(t *testing.T) {
	srv, dir := fakeCity(t), t.TempDir()
	var out bytes.Buffer
	args := []string{"-out", dir, "-only", "permits_t,overlay_t", "-page", "3",
		"-extra", "permits_t=" + srv.LayerURL(75), "-extra", "overlay_t=" + srv.LayerURL(13)}
	if err := ingest(context.Background(), &out, args); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got := countLines(t, filepath.Join(dir, "permits_t.jsonl")); got != 11 {
		t.Errorf("permits lines = %d, want 11 (paged 3 at a time over a server capped at 4)", got)
	}
	report := out.String()
	for _, want := range []string{"permits_t ...", "11 rows saved", "2 rows saved", "Data quality for permit layers",
		"Rows read:            11", "Unique permits:       11",
		"Implausible dates:   1 (9.1%)", // the 2049 typo is detected
		"No street/block:      2 (18.2%)", "Street from text:     1 (9.1%)",
		"Date range:           2005-07-07 to 2006-10-02", // 2049 did not become the max
		"CLOSED", "EXCAVATION"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n%s", want, report)
		}
	}
	ix, err := loadIndex(dir)
	if err != nil || ix.Meta.UniquePermits != 11 || ix.Meta.Blocks != 9 {
		t.Fatalf("index: %+v err=%v", ix.Meta, err)
	}
	// Re-running replaces data rather than appending to it
	if err := ingest(context.Background(), &bytes.Buffer{}, args); err != nil {
		t.Fatal(err)
	}
	if got := countLines(t, filepath.Join(dir, "permits_t.jsonl")); got != 11 {
		t.Errorf("after second ingest: %d lines", got)
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmp) != 0 {
		t.Errorf("temp files left: %v", tmp)
	}
}

func TestIngestSavesGoodLayersWhenOneFails(t *testing.T) {
	srv, dir := fakeCity(t), t.TempDir()
	var out bytes.Buffer
	err := ingest(context.Background(), &out, []string{"-out", dir, "-only", "permits_t,overlay_t,gone",
		"-extra", "permits_t=" + srv.LayerURL(75), "-extra", "overlay_t=" + srv.LayerURL(13), "-extra", "gone=" + srv.LayerURL(404)})
	if err == nil || !strings.Contains(err.Error(), "1 layer(s) failed") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "FAILED") {
		t.Errorf("failure not reported:\n%s", out.String())
	}
	if countLines(t, filepath.Join(dir, "permits_t.jsonl")) != 11 || countLines(t, filepath.Join(dir, "overlay_t.jsonl")) != 2 {
		t.Error("good layers were not saved")
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.jsonl")); err == nil {
		t.Error("failed layer must not leave a file")
	}
}

func TestIngestArgumentErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"zero page", []string{"-page", "0", "-only", "permits_inactive"}, "-page must be positive"},
		{"negative page", []string{"-page", "-5"}, "-page must be positive"},
		{"unknown flag does not exit the process", []string{"-nope"}, "flag provided but not defined"},
		{"help", []string{"-h"}, "help requested"},
		{"unknown layer", []string{"-only", "nope"}, "unknown layer"},
		{"bad extra", []string{"-extra", "oops"}, "name=url"},
		{"output path is a file", []string{"-out", file, "-only", "permits_inactive"}, ""},
	} {
		err := ingest(context.Background(), &bytes.Buffer{}, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want containing %q", c.name, err, c.want)
		}
	}
}

func TestIngestRespectsCancellation(t *testing.T) {
	srv := fakeCity(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := ingest(ctx, &bytes.Buffer{}, []string{"-out", t.TempDir(), "-only", "overlay_t", "-extra", "overlay_t=" + srv.LayerURL(13)})
	if err == nil || time.Since(start) > 5*time.Second {
		t.Errorf("err=%v after %v", err, time.Since(start))
	}
}

func TestDiscoverReportsWhatDecidesViability(t *testing.T) {
	srv := fakeCity(t)
	var out bytes.Buffer
	err := discover(context.Background(), &out, []string{"-service", srv.ServiceURL(), "-only", "permits_t,overlay_t",
		"-extra", "permits_t=" + srv.LayerURL(75), "-extra", "overlay_t=" + srv.LayerURL(13)})
	if err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"75  TDOT_PERMITS_RW_INACTIVE   <-- permits-related", // flags the layer to add with -extra
		"13  Prop409",
		"11 rows", "max 4 per request", "esriGeometryPolygon",
		"DATE_ISSUED:Date", "STREET_NAME:String",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("discover output missing %q:\n%s", want, s)
		}
	}
	if !strings.Contains(s, "2005-07-07 to 2049-09-03") {
		t.Errorf("date range not shown or wrong:\n%s", s)
	}
}

func TestDiscoverSurvivesBrokenServices(t *testing.T) {
	srv := fakeCity(t)
	var out bytes.Buffer
	// Unknown service path and an unknown layer: must report, not crash or hang
	err := discover(context.Background(), &out, []string{"-service", srv.URL + "/nowhere", "-only", "x", "-extra", "x=" + srv.LayerURL(404)})
	if err != nil {
		t.Fatalf("discover should report problems, not fail: %v", err)
	}
	if !strings.Contains(out.String(), "could not list") || !strings.Contains(out.String(), "FAILED") {
		t.Errorf("problems not reported:\n%s", out.String())
	}
	if err := discover(context.Background(), &out, []string{"-only", "nope"}); err == nil {
		t.Error("unknown -only layer should be an error")
	}
}

func TestLoadIndexErrors(t *testing.T) {
	empty := t.TempDir()
	if _, err := loadIndex(empty); err == nil || !strings.Contains(err.Error(), "run ingest first") {
		t.Errorf("empty dir: %v", err)
	}
	if _, err := loadIndex(filepath.Join(empty, "missing")); err == nil {
		t.Error("missing dir")
	}
	_ = os.WriteFile(filepath.Join(empty, "prop409_major.jsonl"), []byte(`{"layer":"p","properties":{},"geometry":null}`+"\n"), 0o644)
	if _, err := loadIndex(empty); err == nil {
		t.Error("overlay-only dir should not count as permit data")
	}
	bad := t.TempDir()
	_ = os.WriteFile(filepath.Join(bad, "permits_x.jsonl"), []byte("garbage"), 0o644)
	if _, err := loadIndex(bad); err == nil || !strings.Contains(err.Error(), "permits_x.jsonl") {
		t.Errorf("corrupt file should be named in the error: %v", err)
	}
}

func TestIngestWarnsWhenRowCountDoesNotMatchServer(t *testing.T) {
	srv := arcgistest.New(arcgistest.Layer{ID: 1, Name: "L", Geometry: "esriGeometryPoint",
		Fields: []arcgis.Field{arcgistest.OIDField()}, ReportedCount: 50,
		Features: []arcgis.Feature{arcgistest.Feat(map[string]any{"OBJECTID": 1.0}, ""), arcgistest.Feat(map[string]any{"OBJECTID": 2.0}, "")}})
	defer srv.Close()
	var out bytes.Buffer
	if err := ingest(context.Background(), &out, []string{"-out", t.TempDir(), "-only", "overlay_t", "-extra", "overlay_t=" + srv.LayerURL(1)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "WARNING: saved 2 rows but the server reports 50") {
		t.Errorf("no mismatch warning:\n%s", out.String())
	}
	good := fakeCity(t)
	out.Reset()
	if err := ingest(context.Background(), &out, []string{"-out", t.TempDir(), "-only", "overlay_t", "-extra", "overlay_t=" + good.LayerURL(13)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "WARNING") {
		t.Errorf("unexpected warning:\n%s", out.String())
	}
}

func TestIngestFlagsAnEmptyLayer(t *testing.T) {
	srv := arcgistest.New(arcgistest.Layer{ID: 1, Name: "Empty", Geometry: "esriGeometryPolyline", Fields: []arcgis.Field{arcgistest.OIDField()}})
	defer srv.Close()
	var out bytes.Buffer
	if err := ingest(context.Background(), &out, []string{"-out", t.TempDir(), "-only", "overlay_t", "-extra", "overlay_t=" + srv.LayerURL(1)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "returned no rows") || !strings.Contains(out.String(), "discover -only overlay_t") {
		t.Errorf("empty layer not called out:\n%s", out.String())
	}
}

func deadURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	return u + "/arcgis/rest/services/Test/MapServer/1"
}
