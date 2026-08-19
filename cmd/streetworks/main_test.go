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

func TestDefaultLayersAreOnTheHostThatWorks(t *testing.T) {
	for _, l := range defaultLayers {
		if hostOf(l.URL) != "gis.tucsonaz.gov" {
			t.Errorf("%s: primary host is %s", l.Name, hostOf(l.URL))
		}
	}
	for name, alts := range fallbacks {
		if len(alts) == 0 || hostOf(alts[0]) != "publicgis.tucsonaz.gov" {
			t.Errorf("%s: expected the OpenData host as fallback, got %v", name, alts)
		}
	}
}

func TestAltsForAndHostOf(t *testing.T) {
	major := defaultLayers[1]
	if got := altsFor(major); len(got) != 1 || !strings.Contains(got[0], "/MapServer/13") {
		t.Errorf("default layer should get its fallback: %v", got)
	}
	if got := altsFor(layer{major.Name, "http://localhost:1/other"}); got != nil {
		t.Errorf("a URL the user chose must not be silently swapped: %v", got)
	}
	if got := altsFor(defaultLayers[0]); got != nil {
		t.Errorf("layer with no fallback: %v", got)
	}
	for in, want := range map[string]string{"https://gis.tucsonaz.gov/arcgis/x": "gis.tucsonaz.gov", "http://127.0.0.1:8080/a": "127.0.0.1:8080", "not a url": "not a url", "": ""} {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPullLayerFallsBackWhenHostIsUnreachable(t *testing.T) {
	good, dir := fakeCity(t), t.TempDir()
	var out bytes.Buffer
	n, used, err := pullLayer(context.Background(), &out, newClient(), layer{"overlay_t", deadURL(t)}, []string{good.LayerURL(13)}, dir, 2000)
	if err != nil || n != 2 || used != good.LayerURL(13) {
		t.Fatalf("n=%d used=%s err=%v", n, used, err)
	}
	if !strings.Contains(out.String(), "could not reach") {
		t.Errorf("the switch should be announced:\n%s", out.String())
	}
	// the metadata must record the address that actually supplied the data
	var m arcgis.SourceMeta
	b, _ := os.ReadFile(filepath.Join(dir, "overlay_t.meta.json"))
	if json.Unmarshal(b, &m) != nil || m.URL != good.LayerURL(13) {
		t.Errorf("meta URL = %q", m.URL)
	}
}

func TestPullLayerDoesNotFallBackOnServerErrors(t *testing.T) {
	broken := arcgistest.New(arcgistest.Layer{ID: 1, Name: "L", Fields: []arcgis.Field{arcgistest.OIDField()}, QueryStatus: 500})
	defer broken.Close()
	alt := fakeCity(t)
	_, used, err := pullLayer(context.Background(), &bytes.Buffer{}, newClient(), layer{"overlay_t", broken.LayerURL(1)}, []string{alt.LayerURL(13)}, t.TempDir(), 2000)
	if err == nil || used != broken.LayerURL(1) {
		t.Fatalf("a server that answers with an error must be reported, not hidden: used=%s err=%v", used, err)
	}
	if arcgis.IsUnreachable(err) {
		t.Error("a 500 is not an unreachable host")
	}
	if len(alt.QueryLog()) != 0 {
		t.Error("the fallback should not have been contacted")
	}
}

func TestPullLayerLeavesFallbackAloneWhenPrimaryWorks(t *testing.T) {
	good, alt := fakeCity(t), fakeCity(t)
	n, used, err := pullLayer(context.Background(), &bytes.Buffer{}, newClient(), layer{"overlay_t", good.LayerURL(13)}, []string{alt.LayerURL(13)}, t.TempDir(), 2000)
	if err != nil || n != 2 || used != good.LayerURL(13) || len(alt.QueryLog()) != 0 {
		t.Errorf("n=%d used=%s err=%v altQueries=%d", n, used, err, len(alt.QueryLog()))
	}
}

func TestPullLayerWhenEveryAddressIsUnreachable(t *testing.T) {
	dir := t.TempDir()
	_, used, err := pullLayer(context.Background(), &bytes.Buffer{}, newClient(), layer{"overlay_t", deadURL(t)}, []string{deadURL(t)}, dir, 2000)
	if !arcgis.IsUnreachable(err) {
		t.Fatalf("err = %v", err)
	}
	if used == "" {
		t.Error("should report which address was tried last")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("nothing should be written: %v", entries)
	}
}

func TestIngestExplainsAnUnreachableHost(t *testing.T) {
	good, dir := fakeCity(t), t.TempDir()
	dead := deadURL(t)
	var out bytes.Buffer
	err := ingest(context.Background(), &out, []string{"-out", dir, "-only", "overlay_t,overlay_dead",
		"-extra", "overlay_t=" + good.LayerURL(13), "-extra", "overlay_dead=" + dead})
	if err == nil || !strings.Contains(err.Error(), "1 layer(s) failed") {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"network problem, not a data problem", hostOf(dead), "VPN"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should say %q:\n%s", want, out.String())
		}
	}
	if countLines(t, filepath.Join(dir, "overlay_t.jsonl")) != 2 {
		t.Error("the reachable layer must still be saved")
	}
}

func TestSuggestName(t *testing.T) {
	for in, want := range map[string]string{
		"TDOT_PERMITS_RW_ACTIVE": "permits_rw_active",
		"DTM_PERMITS_ROW_ACTIVE": "permits_row_active",
		"TDOT_PERMITS_RW":        "permits_rw",
		"Permits RW (Active)":    "permits_rw_active",
		"RW_ACTIVE":              "permits_rw_active",
		"tdot_row-permits":       "permits_row_permits",
		"Inactive General ROW Permits (2020 - Present)": "permits_inactive_general_row_permits_2020_presen",
	} {
		got := suggestName(in)
		if got != want || !nameRE.MatchString(got) || !strings.HasPrefix(got, "permits") || len(got) > 48 {
			t.Errorf("suggestName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsRowPermitLayer(t *testing.T) {
	for name, want := range map[string]bool{
		"TDOT_PERMITS_RW_ACTIVE": true, "DTM_PERMITS_ROW_ACTIVE": true, "Active General ROW Permits": true, "TDOT_PERMITS_CIVIC_EVENTS": true,
		"PDSD_BUILDING_PERMITS": false, "Prop409": false, "BUSLIC": false, "litecode": false, "": false,
	} {
		if got := isRowPermitLayer(name); got != want {
			t.Errorf("isRowPermitLayer(%q) = %v, want %v", name, got, want)
		}
	}
}

func ms(y int, m time.Month, d int) float64 {
	return float64(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).UnixMilli())
}

// surveyCity is a permit service shaped like the city's
func surveyCity(t *testing.T) *arcgistest.Server { return surveyCityWith(t, false) }

func surveyCityWith(t *testing.T, rejectWhere bool) *arcgistest.Server {
	t.Helper()
	full := []arcgis.Field{arcgistest.OIDField(), {Name: "PERMIT", Type: "esriFieldTypeString"}, {Name: "PERMIT_TYPE", Type: "esriFieldTypeString"}, {Name: "PERMIT_STATUS", Type: "esriFieldTypeString"},
		{Name: "STREET_NO", Type: "esriFieldTypeString"}, {Name: "STREET_NAME", Type: "esriFieldTypeString"}, {Name: "DATE_ISSUED", Type: "esriFieldTypeDate"}}
	with := func(dates ...float64) []arcgis.Feature {
		var fs []arcgis.Feature
		for i, d := range dates {
			fs = append(fs, arcgistest.Feat(map[string]any{"OBJECTID": float64(i + 1), "DATE_ISSUED": d}, ""))
		}
		return fs
	}
	srv := arcgistest.New(
		arcgistest.Layer{ID: 75, Name: "TDOT_PERMITS_RW_INACTIVE", Fields: full, Features: with(ms(2010, 1, 1), ms(2025, 2, 12), ms(2049, 9, 3)), RejectWhere: rejectWhere}, // 2049 is a typo date
		arcgistest.Layer{ID: 32, Name: "TDOT_PERMITS_RW_ACTIVE", Fields: full, Features: with(ms(2026, 6, 1), ms(2026, 7, 1)), RejectWhere: rejectWhere},
		arcgistest.Layer{ID: 133, Name: "DTM_PERMITS_ROW_ACTIVE", Fields: full, Features: with(ms(2026, 7, 20))},
		arcgistest.Layer{ID: 33, Name: "TDOT_PERMITS_PIA_ACTIVE", Fields: []arcgis.Field{arcgistest.OIDField(), {Name: "PERMIT", Type: "esriFieldTypeString"}, {Name: "DATE_ISSUED", Type: "esriFieldTypeDate"}}, Features: with(ms(2026, 7, 25))},
		arcgistest.Layer{ID: 62, Name: "DTM_PERMITS_ROW_CIP_JOC", Fields: full},                            // empty
		arcgistest.Layer{ID: 90, Name: "TDOT_PERMITS_OLDER", Fields: full, Features: with(ms(2019, 5, 5))}, // older than what we have
		arcgistest.Layer{ID: 91, Name: "TDOT_PERMITS_BROKEN", Fields: full, Features: with(ms(2026, 7, 1)), QueryStatus: 500},
		arcgistest.Layer{ID: 12, Name: "PDSD_BUILDING_PERMITS", Fields: full, Features: with(ms(2026, 7, 25))}, // not right-of-way
		arcgistest.Layer{ID: 13, Name: "Prop409", Fields: full},
	)
	t.Cleanup(srv.Close)
	return srv
}

func TestSurveyTablesAndRecommendsOnlyLayersThatFitAndAreNewer(t *testing.T) {
	srv := surveyCity(t)
	var out bytes.Buffer
	if err := survey(context.Background(), &out, []string{"-service", srv.ServiceURL(), "-extra", "permits_inactive=" + srv.LayerURL(75)}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	s := out.String()
	line := func(id string) string {
		for _, l := range strings.Split(s, "\n") {
			if f := strings.Fields(l); len(f) > 0 && f[0] == id {
				return l
			}
		}
		return ""
	}
	if l := line("75"); !strings.Contains(l, "2025-02-12") || !strings.Contains(l, "already downloaded") || !strings.Contains(l, "6/6") {
		t.Errorf("inactive layer (typo year must be ignored): %q", l)
	}
	if l := line("32"); !strings.Contains(l, "2026-07-01") || !strings.Contains(l, "6/6") {
		t.Errorf("active layer: %q", l)
	}
	if l := line("33"); !strings.Contains(l, "2/6") {
		t.Errorf("poorly fitting layer: %q", l)
	}
	if l := line("62"); !strings.Contains(l, "none") {
		t.Errorf("empty layer: %q", l)
	}
	if l := line("91"); !strings.Contains(l, "could not read") {
		t.Errorf("broken layer should be reported, not abort the survey: %q", l)
	}
	for _, id := range []string{"12", "13"} {
		if line(id) != "" {
			t.Errorf("layer %s is not a right-of-way permit layer and should not be listed", id)
		}
	}
	i := strings.Index(s, "Layers that fit")
	if i < 0 {
		t.Fatalf("no recommendation block:\n%s", s)
	}
	rec := s[i:]
	for _, want := range []string{"-extra permits_rw_active=" + srv.LayerURL(32), "-extra permits_row_active=" + srv.LayerURL(133)} {
		if !strings.Contains(rec, want) {
			t.Errorf("should recommend %q:\n%s", want, rec)
		}
	}
	for _, unwanted := range []string{srv.LayerURL(75), srv.LayerURL(33), srv.LayerURL(62), srv.LayerURL(90), srv.LayerURL(91)} {
		if strings.Contains(rec, unwanted+"\n") || strings.HasSuffix(strings.TrimSpace(rec), unwanted) {
			t.Errorf("should not recommend %s:\n%s", unwanted, rec)
		}
	}
	if strings.Index(s, "  133 ") > strings.Index(s, "  75 ") && strings.Index(s, "  133 ") != -1 {
		t.Errorf("table should list newest layers first:\n%s", s)
	}
}
